// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/request"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// Warning thresholds for validateAndWarnConfig.
// These are soft limits used to emit warnings, not enforced maximums.
const (
	warnConnsPerRunner    = 50   // Warn when connections per runner exceed this
	warnTotalConnections  = 1000 // Warn when total connections across all runners exceed this
	qpsPerWorkerEstimate  = 10   // Estimated QPS each worker can handle
	qpsPerConnEstimate    = 100  // Estimated QPS each connection can handle
)

// ScheduleResult contains the aggregated result from all runners.
type ScheduleResult struct {
	// Per-runner results
	RunnerResults []*RunnerResult
	// Total duration of the entire replay
	Duration time.Duration
	// Aggregated stats
	Aggregated types.ResponseStats
	// Total requests across all runners
	TotalRequests int
	// Total requests run
	TotalRun int
	// Total requests failed
	TotalFailed int
}

// Schedule orchestrates replay execution across multiple local runners.
func Schedule(ctx context.Context, kubeconfigPath string, profile *types.ReplayProfile) (*ScheduleResult, error) {
	// Build REST config
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	// Create REST clients using the same method as existing kperf
	// This handles auth, TLS, content negotiation properly
	restClis, err := request.NewClients(kubeconfigPath,
		profile.Spec.ConnsPerRunner,
		request.WithClientContentTypeOpt(profile.Spec.ContentType),
		request.WithClientDisableHTTP2Opt(profile.Spec.DisableHTTP2),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST clients: %w", err)
	}

	// Determine worker count per runner
	// Default to ConnsPerRunner if ClientsPerRunner not specified (legacy behavior)
	workersPerRunner := profile.Spec.ClientsPerRunner
	if workersPerRunner <= 0 {
		workersPerRunner = profile.Spec.ConnsPerRunner
	}

	runnerCount := profile.Spec.RunnerCount

	// Partition requests across runners
	runnerRequests := make([][]types.ReplayRequest, runnerCount)
	for i := 0; i < runnerCount; i++ {
		runnerRequests[i] = PartitionRequests(profile.Requests, runnerCount, i)
		klog.V(3).InfoS("Partitioned requests for runner",
			"runner", i,
			"requests", len(runnerRequests[i]),
		)
	}

	// Validate configuration and provide warnings
	validateAndWarnConfig(profile, runnerRequests)

	// Log distribution analysis
	if klog.V(2).Enabled() {
		dist := AnalyzeDistribution(profile.Requests, runnerCount)
		klog.V(2).InfoS("Request distribution analysis",
			"imbalance%", dist["imbalance"],
			"min", dist["min"],
			"max", dist["max"],
			"avg", dist["average"])
	}

	klog.V(2).InfoS("Starting local replay",
		"runnerCount", runnerCount,
		"connsPerRunner", len(restClis),
		"workersPerRunner", workersPerRunner,
		"totalRequests", len(profile.Requests),
		"duration", fmt.Sprintf("%dms", profile.Duration()),
	)

	// Client options for creating overflow WATCH connections
	clientOpts := []request.ClientCfgOpt{
		request.WithClientContentTypeOpt(profile.Spec.ContentType),
		request.WithClientDisableHTTP2Opt(profile.Spec.DisableHTTP2),
	}

	// Create runners (each runner gets ALL connections for round-robin)
	runners := make([]*Runner, runnerCount)
	for i := 0; i < runnerCount; i++ {
		runners[i] = NewRunner(
			i,
			runnerRequests[i],
			restClis,
			restConfig.Host,
			workersPerRunner,
			kubeconfigPath,
			clientOpts,
		)
	}

	// Synchronize start time across all runners
	startTime := time.Now()

	// Run all runners concurrently
	var wg sync.WaitGroup
	results := make([]*RunnerResult, runnerCount)
	runnerErrors := make([]error, runnerCount)

	for i := 0; i < runnerCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := runners[idx].Run(ctx, startTime)
			results[idx] = result
			runnerErrors[idx] = err
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(startTime)

	// Check for errors
	for i, err := range runnerErrors {
		if err != nil {
			klog.V(2).ErrorS(err, "Runner failed", "runner", i)
		}
	}

	// Aggregate results
	return aggregateResults(results, totalDuration), nil
}

// ScheduleSingleRunner runs a single runner for distributed mode.
// In distributed mode, each Kubernetes pod (created by runner group as an indexed Job)
// invokes this with its own runnerIndex (from JOB_COMPLETION_INDEX).
// The runner partitions the full request list and only executes its assigned slice.
func ScheduleSingleRunner(ctx context.Context, kubeconfigPath string, profile *types.ReplayProfile, runnerIndex int) (*RunnerResult, error) {
	// Build REST config
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	// Create REST clients using the same method as existing kperf
	restClis, err := request.NewClients(kubeconfigPath,
		profile.Spec.ConnsPerRunner,
		request.WithClientContentTypeOpt(profile.Spec.ContentType),
		request.WithClientDisableHTTP2Opt(profile.Spec.DisableHTTP2),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST clients: %w", err)
	}

	// Determine worker count
	workersPerRunner := profile.Spec.ClientsPerRunner
	if workersPerRunner <= 0 {
		workersPerRunner = profile.Spec.ConnsPerRunner
	}

	// Partition requests for this runner
	requests := PartitionRequests(profile.Requests, profile.Spec.RunnerCount, runnerIndex)

	klog.V(2).InfoS("Starting single runner",
		"runnerIndex", runnerIndex,
		"runnerCount", profile.Spec.RunnerCount,
		"connections", len(restClis),
		"workers", workersPerRunner,
		"requests", len(requests),
	)

	// Client options for overflow WATCH connections
	clientOpts := []request.ClientCfgOpt{
		request.WithClientContentTypeOpt(profile.Spec.ContentType),
		request.WithClientDisableHTTP2Opt(profile.Spec.DisableHTTP2),
	}

	// Create runner with all connections
	runner := NewRunner(
		runnerIndex,
		requests,
		restClis,
		restConfig.Host,
		workersPerRunner,
		kubeconfigPath,
		clientOpts,
	)

	// Use current time as start (each pod will have slightly different start times)
	replayStart := time.Now()

	return runner.Run(ctx, replayStart)
}

// aggregateResults combines results from all runners.
func aggregateResults(results []*RunnerResult, totalDuration time.Duration) *ScheduleResult {
	schedResult := &ScheduleResult{
		RunnerResults: results,
		Duration:      totalDuration,
		Aggregated: types.ResponseStats{
			Errors:             make([]types.ResponseError, 0),
			LatenciesByURL:     make(map[string][]float64),
			TotalReceivedBytes: 0,
		},
	}

	for _, result := range results {
		if result == nil {
			continue
		}

		schedResult.TotalRequests += result.Total
		schedResult.TotalRun += result.RequestsRun
		schedResult.TotalFailed += result.RequestsFailed

		// Aggregate errors
		schedResult.Aggregated.Errors = append(schedResult.Aggregated.Errors, result.ResponseStats.Errors...)

		// Aggregate latencies by URL
		for url, latencies := range result.ResponseStats.LatenciesByURL {
			if _, exists := schedResult.Aggregated.LatenciesByURL[url]; !exists {
				schedResult.Aggregated.LatenciesByURL[url] = make([]float64, 0)
			}
			schedResult.Aggregated.LatenciesByURL[url] = append(schedResult.Aggregated.LatenciesByURL[url], latencies...)
		}

		// Sum bytes
		schedResult.Aggregated.TotalReceivedBytes += result.ResponseStats.TotalReceivedBytes
	}

	return schedResult
}

// NewClientsForReplay creates REST clients for replay execution.
// This is a convenience wrapper around request.NewClients.
func NewClientsForReplay(kubeconfigPath string, conns int, contentType types.ContentType, disableHTTP2 bool) ([]rest.Interface, error) {
	return request.NewClients(kubeconfigPath,
		conns,
		request.WithClientContentTypeOpt(contentType),
		request.WithClientDisableHTTP2Opt(disableHTTP2),
	)
}

// validateAndWarnConfig validates configuration and warns about potential issues.
func validateAndWarnConfig(profile *types.ReplayProfile, runnerRequests [][]types.ReplayRequest) {
	for i, reqs := range runnerRequests {
		if len(reqs) == 0 {
			continue
		}

		duration := float64(profile.Duration()) / 1000.0 // in seconds
		if duration <= 0 {
			continue
		}
		qps := float64(len(reqs)) / duration

		conns := profile.Spec.ConnsPerRunner
		workers := profile.Spec.ClientsPerRunner
		if workers == 0 {
			workers = conns
		}

		// Warning: Too many connections
		if conns > warnConnsPerRunner {
			klog.Warningf("Runner %d: ConnsPerRunner (%d) exceeds recommended maximum (%d). "+
				"This may overwhelm the API server. Consider increasing runnerCount instead.",
				i, conns, warnConnsPerRunner)
		}

		// Warning: Insufficient workers for QPS
		recommendedWorkers := int(qps/qpsPerWorkerEstimate) + qpsPerWorkerEstimate
		if workers < recommendedWorkers {
			klog.Warningf("Runner %d: ClientsPerRunner (%d) may be insufficient for QPS (%.0f). "+
				"Recommend at least %d workers (3-4x connections).",
				i, workers, qps, recommendedWorkers)
		}

		// Warning: Too few connections for QPS
		recommendedConns := int(qps/qpsPerConnEstimate) + 5
		if recommendedConns > warnConnsPerRunner {
			recommendedConns = warnConnsPerRunner
		}
		if conns < recommendedConns {
			klog.Warningf("Runner %d: ConnsPerRunner (%d) may be insufficient for QPS (%.0f). "+
				"Recommend at least %d connections.",
				i, conns, qps, recommendedConns)
		}

		// Info: Configuration summary
		klog.V(2).InfoS("Runner configuration",
			"runner", i,
			"requests", len(reqs),
			"qps", fmt.Sprintf("%.1f", qps),
			"connections", conns,
			"workers", workers,
			"ratio", fmt.Sprintf("%.1fx", float64(workers)/float64(conns)))
	}

	// Check total connection count
	totalConns := profile.Spec.RunnerCount * profile.Spec.ConnsPerRunner
	if totalConns > warnTotalConnections {
		klog.Warningf("Total connections across all runners: %d. "+
			"This may overwhelm the API server (recommend < %d total). "+
			"Consider reducing connsPerRunner or using fewer runners.",
			totalConns, warnTotalConnections)
	}
}
