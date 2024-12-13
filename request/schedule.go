// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"errors"
	"io"
	"math"
	"sync"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/metrics"

	"golang.org/x/net/http2"
	"golang.org/x/time/rate"
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

// Schedule files requests to apiserver based on LoadProfileSpec.
func Schedule(ctx context.Context, spec *types.LoadProfileSpec, restCli []rest.Interface) (*Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rndReqs, err := NewWeightedRandomRequests(spec)
	if err != nil {
		return nil, err
	}

	qps := spec.Rate
	if qps == 0 {
		qps = float64(math.MaxInt32)
	}
	limiter := rate.NewLimiter(rate.Limit(qps), 1)

	clients := spec.Client
	if clients == 0 {
		clients = spec.Conns
	}

	reqBuilderCh := rndReqs.Chan()
	var wg sync.WaitGroup

	respMetric := metrics.NewResponseMetric()
	for i := 0; i < clients; i++ {
		// reuse connection if clients > conns
		cli := restCli[i%len(restCli)]
		wg.Add(1)
		go func(cli rest.Interface) {
			defer wg.Done()

			for builder := range reqBuilderCh {
				_, req := builder.Build(cli)

				if err := limiter.Wait(ctx); err != nil {
					klog.V(5).Infof("Rate limiter wait failed: %v", err)
					cancel()
					return
				}

				klog.V(5).Infof("Request URL: %s", req.URL())

				req = req.Timeout(defaultTimeout)
				func() {
					start := time.Now()

					var bytes int64
					respBody, err := req.Stream(context.Background())
					if err == nil {
						defer respBody.Close()
						bytes, err = io.Copy(io.Discard, respBody)

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
					}
					end := time.Now()
					latency := end.Sub(start).Seconds()

					respMetric.ObserveReceivedBytes(bytes)
					if err != nil {
						respMetric.ObserveFailure(end, latency, err)
						klog.V(5).Infof("Request stream failed: %v", err)
						return
					}
					respMetric.ObserveLatency(req.URL().String(), latency)
				}()
			}
		}(cli)
	}

	klog.V(2).InfoS("Setting",
		"clients", clients,
		"connections", len(restCli),
		"rate", qps,
		"total", spec.Total,
		"http2", !spec.DisableHTTP2,
		"content-type", spec.ContentType,
	)

	start := time.Now()

	rndReqs.Run(ctx, spec.Total)
	rndReqs.Stop()
	wg.Wait()

	totalDuration := time.Since(start)
	responseStats := respMetric.Gather()
	return &Result{
		ResponseStats: responseStats,
		Duration:      totalDuration,
		Total:         spec.Total,
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
