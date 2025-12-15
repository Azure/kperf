// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/metrics"
	"github.com/Azure/kperf/request/executor"

	"golang.org/x/net/http2"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

const defaultTimeout = 60 * time.Second

// Result contains responseStats vlaues from Gather() and adds Duration and Total values separately
type Result struct {
	types.ResponseStats
	// Duration means the time of benchmark.
	Duration time.Duration
	// Total means the total number of requests.
	Total int
}

// Schedule executes requests to apiserver based on LoadProfileSpec using the executor pattern.
func Schedule(ctx context.Context, spec *types.LoadProfileSpec, restCli []rest.Interface) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Create executor for the specified mode
	exec, err := executor.CreateExecutor(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %v", err)
	}
	defer exec.Stop()

	// Get metadata for logging
	metadata := exec.Metadata()

	// Get execution context with mode-specific timeouts
	execCtx, execCancel := exec.GetExecutionContext(ctx)
	defer execCancel()

	// Get rate limiter (nil if mode doesn't need it)
	limiter := exec.GetRateLimiter()

	// Worker pool - start workers BEFORE executor to avoid unbuffered channel deadlock
	clients := spec.Client
	if clients == 0 {
		clients = spec.Conns
	}

	respMetric := metrics.NewResponseMetric()
	var wg sync.WaitGroup

	reqBuilderCh := exec.Chan()
	for i := 0; i < clients; i++ {
		cli := restCli[i%len(restCli)]
		wg.Add(1)
		go func(workerID int, cli rest.Interface) {
			defer wg.Done()

			klog.V(5).Infof("Worker %d started, waiting for requests", workerID)
			requestCount := 0

			for builder := range reqBuilderCh {
				// Apply rate limiting (if configured)
				if limiter != nil {
					if err := limiter.Wait(ctx); err != nil {
						klog.V(5).Infof("Worker %d: Rate limiter wait failed: %v", workerID, err)
						return
					}
				}

				requestCount++
				klog.V(8).Infof("Worker %d received request #%d", workerID, requestCount)
				req := builder.Build(cli)

				klog.V(5).Infof("Request URL: %s", req.URL())

				req.Timeout(defaultTimeout)
				func() {
					start := time.Now()

					var bytes int64
					bytes, err := req.Do(context.Background())
					// Based on HTTP2 Spec Section 8.1 [1],
					//
					// A server can send a complete response prior to the client
					// sending an entire request if the response does not depend
					// on any portion of the request that has not been sent and
					// received. When this is true, a server MAY request that the
					// client abort transmission of a request without error by
					// sending a RST_STREAM with an error code of NO_ERROR after
					// sending a complete response (i.e., a frame with the END_STREAM
					// flag). Clients MUST NOT discard responses as a result of receiving
					// such a RST_STREAM, though clients can always discard responses
					// at their discretion for other reasons.
					//
					// We should mark NO_ERROR as nil here.
					//
					// [1]: https://httpwg.org/specs/rfc7540.html#HttpSequence
					if err != nil && isHTTP2StreamNoError(err) {
						err = nil
					}

					end := time.Now()
					latency := end.Sub(start).Seconds()

					respMetric.ObserveReceivedBytes(bytes)
					if err != nil {
						respMetric.ObserveFailure(req.Method(), req.MaskedURL().String(), end, latency, err)
						klog.V(5).Infof("Request stream failed: %v", err)
						return
					}
					respMetric.ObserveLatency(req.Method(), req.MaskedURL().String(), latency)
				}()
			}

			klog.V(5).Infof("Worker %d finished: processed %d requests", workerID, requestCount)
		}(i, cli)
	}

	// Extract rate from metadata for logging (mode-specific)
	rate, _ := metadata.Custom["rate"].(float64)

	klog.V(2).InfoS("Schedule started",
		"mode", spec.Mode,
		"clients", clients,
		"connections", len(restCli),
		"rate", rate,
		"expectedTotal", metadata.ExpectedTotal,
		"expectedDuration", metadata.ExpectedDuration,
		"http2", !spec.DisableHTTP2,
		"content-type", spec.ContentType,
	)

	start := time.Now()

	// Start executor AFTER workers are ready to receive
	go func() {
		if err := exec.Run(execCtx); err != nil && err != context.Canceled {
			klog.Errorf("Executor error: %v", err)
		}
		// Signal completion (success or failure)
		cancel()
	}()

	// Wait for completion
	<-ctx.Done()

	exec.Stop()
	wg.Wait()

	totalDuration := time.Since(start)
	responseStats := respMetric.Gather()
	return &Result{
		ResponseStats: responseStats,
		Duration:      totalDuration,
		Total:         metadata.ExpectedTotal,
	}, nil
}

// isHTTP2StreamNoError returns true if it's NO_ERROR.
func isHTTP2StreamNoError(err error) bool {
	if err == nil {
		return false
	}

	if streamErr, ok := err.(http2.StreamError); ok || errors.As(err, &streamErr) {
		return streamErr.Code == http2.ErrCodeNo
	}
	return false
}
