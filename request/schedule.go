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
		qps = math.MaxInt32
	}
	limiter := rate.NewLimiter(rate.Limit(qps), 10)

	reqBuilderCh := rndReqs.Chan()
	var wg sync.WaitGroup

	respMetric := metrics.NewResponseMetric()
	for i := 0; i < spec.Client; i++ {
		// reuse connection if clients > conns
		cli := restCli[i%len(restCli)]
		wg.Add(1)
		go func(cli rest.Interface) {
			defer wg.Done()

			for builder := range reqBuilderCh {
				_, req := builder.Build(cli)

				klog.V(9).Infof("Request URL: %s", req.URL())

				if err := limiter.Wait(ctx); err != nil {
					klog.V(9).Infof("Rate limiter wait failed: %v", err)
					cancel()
					return
				}

				req = req.Timeout(defaultTimeout)
				func() {
					start := time.Now()
					defer func() {
						respMetric.ObserveLatency(time.Since(start).Seconds())
					}()

					var bytes int64
					respBody, err := req.Stream(context.Background())
					if err == nil {
						defer respBody.Close()
						bytes, err = io.Copy(io.Discard, respBody)
						respMetric.ObserveReceivedBytes(bytes)
					}

					if err != nil {
						respMetric.ObserveFailure(err)
						klog.V(9).Infof("Request stream failed: %v", err)
					}
				}()
			}
		}(cli)
	}

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
