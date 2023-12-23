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
)

const defaultTimeout = 60 * time.Second

// Schedule files requests to apiserver based on LoadProfileSpec.
func Schedule(ctx context.Context, clientNum int, spec *types.LoadProfileSpec, restCli []rest.Interface) (*types.ResponseStats, error) {
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
	for i := 0; i < clientNum; i++ {
		//reuse connection if client > conns
		cli := restCli[i%len(restCli)]
		wg.Add(1)
		go func(cli rest.Interface) {
			defer wg.Done()

			for builder := range reqBuilderCh {
				_, req := builder.Build(cli)

				if err := limiter.Wait(ctx); err != nil {
					cancel()
					return
				}

				req = req.Timeout(defaultTimeout)
				func() {
					start := time.Now()
					defer func() {
						respMetric.ObserveLatency(time.Since(start).Seconds())
					}()

					respBody, err := req.Stream(context.Background())
					if err == nil {
						defer respBody.Close()
						// NOTE: It's to reduce memory usage because
						// we don't need that unmarshal object.
						_, err = io.Copy(io.Discard, respBody)
					}
					if err != nil {
						respMetric.ObserveFailure()
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

	_, percentileLatencies, failures := respMetric.Gather()
	return &types.ResponseStats{
		Total:               spec.Total,
		Failures:            failures,
		Duration:            totalDuration,
		PercentileLatencies: percentileLatencies,
	}, nil
}
