// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/metrics"
	"github.com/Azure/kperf/request"

	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const defaultRequestTimeout = 60 * time.Second

// Bucket sizing QPS thresholds for adaptive timer precision.
const (
	bucketQPSLow    = 100  // Below this: 10ms buckets (good timing precision)
	bucketQPSMedium = 500  // Below this: 20ms buckets
	bucketQPSHigh   = 2000 // Below this: 50ms buckets; above: 100ms buckets
)

// timeBucket groups requests that should execute in the same time window.
// Uses indices to avoid copying request data.
type timeBucket struct {
	timestamp int64  // bucket start time in ms
	startIdx  int    // start index in requests slice
	endIdx    int    // end index (exclusive) in requests slice
}

// workerMetrics holds per-worker statistics.
// For normal workers, each goroutine has its own instance (no synchronization needed).
// For WATCH metrics (shared across goroutines), atomic operations ensure correctness.
type workerMetrics struct {
	respMetric     metrics.ResponseMetric
	requestsRun    int32
	requestsFailed int32
}

// groupIntoTimeBuckets groups requests by time buckets to reduce timer overhead.
// Returns buckets with indices to avoid copying request data.
func groupIntoTimeBuckets(requests []types.ReplayRequest, bucketMs int64) []timeBucket {
	if len(requests) == 0 {
		return nil
	}

	buckets := make([]timeBucket, 0, len(requests)/100+1)
	currentBucket := timeBucket{
		timestamp: (requests[0].Timestamp / bucketMs) * bucketMs,
		startIdx:  0,
		endIdx:    0,
	}

	for i, req := range requests {
		bucketTime := (req.Timestamp / bucketMs) * bucketMs
		if bucketTime != currentBucket.timestamp {
			currentBucket.endIdx = i
			buckets = append(buckets, currentBucket)
			currentBucket = timeBucket{
				timestamp: bucketTime,
				startIdx:  i,
				endIdx:    i,
			}
		}
	}

	// Add final bucket
	currentBucket.endIdx = len(requests)
	buckets = append(buckets, currentBucket)

	klog.V(3).InfoS("Grouped requests into time buckets",
		"totalRequests", len(requests),
		"bucketCount", len(buckets),
		"bucketSizeMs", bucketMs)

	return buckets
}

// calculateBucketSize determines optimal bucket size based on request count and QPS.
func calculateBucketSize(requests []*types.ReplayRequest) int64 {
	if len(requests) == 0 {
		return 10
	}

	duration := requests[len(requests)-1].Timestamp - requests[0].Timestamp
	if duration <= 0 {
		return 10
	}

	qps := float64(len(requests)) / (float64(duration) / 1000.0)

	// Adaptive bucket sizing:
	// - Low QPS (<100): 10ms buckets (good timing precision)
	// - Medium QPS (100-500): 20ms buckets
	// - High QPS (500-2000): 50ms buckets
	// - Very high QPS (>2000): 100ms buckets
	switch {
	case qps < bucketQPSLow:
		return 10
	case qps < bucketQPSMedium:
		return 20
	case qps < bucketQPSHigh:
		return 50
	default:
		return 100
	}
}

// Runner executes replay requests at their scheduled timestamps.
type Runner struct {
	index            int
	requests         []types.ReplayRequest
	restClis         []rest.Interface             // Connection pool for round-robin distribution
	baseURL          string
	workerCount      int                          // Number of concurrent worker goroutines
	reqChan          chan *types.ReplayRequest     // Channel for dispatching requests to workers
	kubeconfigPath   string                     // For creating overflow WATCH connections
	clientOpts       []request.ClientCfgOpt     // Client options for overflow connections
	watchConnCounter int32                      // Atomic counter for WATCH connection assignment
}

// NewRunner creates a new replay runner.
func NewRunner(
	index int,
	requests []types.ReplayRequest,
	restClis []rest.Interface,
	baseURL string,
	workerCount int,
	kubeconfigPath string,
	clientOpts []request.ClientCfgOpt,
) *Runner {
	// Default to 1 worker per connection if not specified
	if workerCount <= 0 {
		workerCount = len(restClis)
		if workerCount == 0 {
			workerCount = 1
		}
	}

	return &Runner{
		index:          index,
		requests:       requests,
		restClis:       restClis,
		baseURL:        baseURL,
		workerCount:    workerCount,
		kubeconfigPath: kubeconfigPath,
		clientOpts:     clientOpts,
	}
}

// RunnerResult contains the result of running replay requests.
type RunnerResult struct {
	Total          int
	Duration       time.Duration
	ResponseStats  types.ResponseStats
	RequestsRun    int
	RequestsFailed int
}

// Run executes all requests at their scheduled timestamps.
// replayStart is the time when the replay started (used for synchronization across runners).
func (r *Runner) Run(ctx context.Context, replayStart time.Time) (*RunnerResult, error) {
	if len(r.requests) == 0 {
		return &RunnerResult{}, nil
	}

	// Separate WATCH from normal requests using indices to avoid copying
	normalCount := 0
	watchCount := 0
	for i := range r.requests {
		if r.requests[i].Verb == "WATCH" {
			watchCount++
		} else {
			normalCount++
		}
	}

	// Pre-allocate with exact capacity
	normalReqs := make([]*types.ReplayRequest, 0, normalCount)
	watchReqs := make([]*types.ReplayRequest, 0, watchCount)

	for i := range r.requests {
		if r.requests[i].Verb == "WATCH" {
			watchReqs = append(watchReqs, &r.requests[i])
		} else {
			normalReqs = append(normalReqs, &r.requests[i])
		}
	}

	// Calculate optimal bucket size based on QPS
	var bucketMs int64
	if len(normalReqs) > 0 {
		bucketMs = calculateBucketSize(normalReqs)

		duration := normalReqs[len(normalReqs)-1].Timestamp - normalReqs[0].Timestamp
		if duration > 0 {
			qps := float64(len(normalReqs)) / (float64(duration) / 1000.0)
			klog.V(2).InfoS("Runner configuration",
				"runner", r.index,
				"requests", len(normalReqs),
				"watches", len(watchReqs),
				"estimatedQPS", fmt.Sprintf("%.1f", qps),
				"bucketMs", bucketMs,
				"workers", r.workerCount,
				"connections", len(r.restClis))
		}
	}

	// Create buffered channel (10x worker count for high QPS) - using pointers
	r.reqChan = make(chan *types.ReplayRequest, r.workerCount*10)

	// Initialize per-worker metrics
	workers := make([]*workerMetrics, r.workerCount)
	for i := 0; i < r.workerCount; i++ {
		workers[i] = &workerMetrics{
			respMetric: metrics.NewResponseMetric(),
		}
	}

	// Start worker pool
	wg := r.startWorkers(ctx, workers)

	startTime := time.Now()

	// Scheduler goroutine: dispatch requests by direct slice iteration
	go func() {
		defer close(r.reqChan)

		for _, req := range normalReqs {
			// Wait for scheduled time
			scheduledTime := replayStart.Add(time.Duration(req.Timestamp) * time.Millisecond)
			waitDuration := time.Until(scheduledTime)

			if waitDuration > 0 {
				timer := time.NewTimer(waitDuration)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}

			// Dispatch request pointer (no copy)
			select {
			case <-ctx.Done():
				return
			case r.reqChan <- req:
				// Successfully dispatched
			}
		}
	}()

	// Handle WATCH requests separately with dedicated metrics
	var watchWg sync.WaitGroup
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	watchMetrics := &workerMetrics{
		respMetric: metrics.NewResponseMetric(),
	}

	for _, req := range watchReqs {
		watchWg.Add(1)
		go func(req *types.ReplayRequest) {
			defer watchWg.Done()

			// Wait for scheduled time
			scheduledTime := replayStart.Add(time.Duration(req.Timestamp) * time.Millisecond)
			waitDuration := time.Until(scheduledTime)
			if waitDuration > 0 {
				timer := time.NewTimer(waitDuration)
				select {
				case <-watchCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}

			// Pool-first connection assignment: first len(restClis) WATCHes
			// use pool connections (one per connection to avoid overload).
			// WATCHes beyond pool size create new dedicated connections.
			watchIdx := int(atomic.AddInt32(&r.watchConnCounter, 1) - 1)
			var watchCli rest.Interface
			if watchIdx < len(r.restClis) {
				watchCli = r.restClis[watchIdx]
			} else {
				// Overflow: create a dedicated connection for this WATCH
				if overflow := watchIdx - len(r.restClis) + 1; overflow == 1 || overflow == len(r.restClis) {
					klog.Warningf("Runner %d: WATCH overflow - creating dedicated connection "+
						"(watch #%d, pool size %d)", r.index, watchIdx+1, len(r.restClis))
				}
				clis, err := request.NewClients(r.kubeconfigPath, 1, r.clientOpts...)
				if err != nil {
					klog.Errorf("Runner %d: failed to create overflow WATCH connection: %v", r.index, err)
					atomic.AddInt32(&watchMetrics.requestsRun, 1)
					atomic.AddInt32(&watchMetrics.requestsFailed, 1)
					return
				}
				watchCli = clis[0]
			}

			// Check if context was cancelled during connection setup
			select {
			case <-watchCtx.Done():
				return
			default:
			}

			err := r.executeRequestWithClient(watchCtx, req, watchCli, watchMetrics.respMetric)
			atomic.AddInt32(&watchMetrics.requestsRun, 1)
			if err != nil {
				atomic.AddInt32(&watchMetrics.requestsFailed, 1)
			}
		}(req)
	}

	// Wait for all normal workers to complete
	wg.Wait()

	// Cancel WATCH operations and wait for them to finish
	cancelWatch()
	watchWg.Wait()

	// Aggregate results from all workers (including WATCH metrics)
	allMetrics := append(workers, watchMetrics)

	var totalRun int32
	var totalFailed int32
	aggregatedStats := types.ResponseStats{
		Errors:             make([]types.ResponseError, 0),
		LatenciesByURL:     make(map[string][]float64),
		TotalReceivedBytes: 0,
	}

	for _, wm := range allMetrics {
		totalRun += wm.requestsRun
		totalFailed += wm.requestsFailed

		stats := wm.respMetric.Gather()
		aggregatedStats.Errors = append(aggregatedStats.Errors, stats.Errors...)

		for url, latencies := range stats.LatenciesByURL {
			if _, exists := aggregatedStats.LatenciesByURL[url]; !exists {
				aggregatedStats.LatenciesByURL[url] = make([]float64, 0, len(latencies))
			}
			aggregatedStats.LatenciesByURL[url] = append(aggregatedStats.LatenciesByURL[url], latencies...)
		}

		aggregatedStats.TotalReceivedBytes += stats.TotalReceivedBytes
	}

	return &RunnerResult{
		Total:          len(r.requests),
		Duration:       time.Since(startTime),
		ResponseStats:  aggregatedStats,
		RequestsRun:    int(totalRun),
		RequestsFailed: int(totalFailed),
	}, nil
}

// startWorkers creates the worker pool that processes requests from the channel.
func (r *Runner) startWorkers(ctx context.Context, workers []*workerMetrics) *sync.WaitGroup {
	var wg sync.WaitGroup

	for i := 0; i < r.workerCount; i++ {
		wg.Add(1)

		// Connection affinity: each worker uses same connection for cache locality
		connIndex := i % len(r.restClis)
		cli := r.restClis[connIndex]

		go func(workerID int, restCli rest.Interface, wm *workerMetrics) {
			defer wg.Done()

			for req := range r.reqChan {
				// Check context
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Execute request with worker's dedicated connection (pointer, no copy)
				err := r.executeRequestWithClient(ctx, req, restCli, wm.respMetric)

				// Track metrics (per-worker, no contention for normal workers)
				atomic.AddInt32(&wm.requestsRun, 1)
				if err != nil {
					atomic.AddInt32(&wm.requestsFailed, 1)
				}
			}
		}(i, cli, workers[i])
	}

	return &wg
}

// executeRequestWithClient executes a single replay request with a specific client.
func (r *Runner) executeRequestWithClient(ctx context.Context, req *types.ReplayRequest, restCli rest.Interface, respMetric metrics.ResponseMetric) error {
	requester, err := NewReplayRequester(*req, restCli, r.baseURL)
	if err != nil {
		klog.V(5).Infof("Failed to create requester for %s %s: %v", req.Verb, req.APIPath, err)
		return err
	}

	requester.Timeout(defaultRequestTimeout)

	klog.V(5).Infof("Executing %s %s at timestamp %d", req.Verb, req.APIPath, req.Timestamp)

	start := time.Now()
	bytes, err := requester.Do(ctx)
	end := time.Now()
	latency := end.Sub(start).Seconds()

	// For WATCH operations, use connection establishment time instead of full stream duration
	// This gives a meaningful latency metric (time to establish watch) rather than the
	// duration of the long-lived connection which can be minutes
	var reportLatency float64
	if req.Verb == "WATCH" {
		reportLatency = requester.ConnectionLatency()
	} else {
		reportLatency = latency
	}

	respMetric.ObserveReceivedBytes(bytes)

	if err != nil {
		// Check if error is due to context cancellation (expected for WATCH when replay ends)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Context cancelled - treat as successful completion for WATCH operations
			// This ensures cancelled WATCHes are counted in the total
			respMetric.ObserveLatency(requester.Method(), requester.MaskedURL().String(), reportLatency)
			klog.V(5).Infof("Request cancelled (expected): %s %s", req.Verb, req.APIPath)
			return nil
		}

		// Real error - record failure using actual URL (not masked) for diagnosability
		respMetric.ObserveFailure(requester.Method(), requester.URL().String(), end, latency, err)
		klog.V(5).Infof("Request failed: %s %s: %v", req.Verb, req.APIPath, err)
		return err
	}

	respMetric.ObserveLatency(requester.Method(), requester.MaskedURL().String(), reportLatency)
	return nil
}

// GetRunnerIndex returns the runner index from environment variable or parameter.
// In distributed mode, K8s injects JOB_COMPLETION_INDEX for indexed Jobs.
func GetRunnerIndex(paramIndex int) int {
	if idx := os.Getenv("JOB_COMPLETION_INDEX"); idx != "" {
		if i, err := strconv.Atoi(idx); err == nil {
			return i
		}
	}
	return paramIndex
}
