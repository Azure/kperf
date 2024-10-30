// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"io"
	"math"
	"sync"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/metrics"

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
					}
					latency := time.Since(start).Seconds()

					respMetric.ObserveReceivedBytes(bytes)
					if err != nil {
						respMetric.ObserveFailure(err)
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
