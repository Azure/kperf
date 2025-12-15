// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package executor

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/Azure/kperf/api/types"
	"golang.org/x/time/rate"
)

// WeightedRandomExecutor implements Executor for weighted-random mode.
// It generates requests randomly based on weighted distribution.
type WeightedRandomExecutor struct {
	config       *types.WeightedRandomConfig
	spec         *types.LoadProfileSpec
	limiter      *rate.Limiter
	reqBuilderCh chan RESTRequestBuilder
	shares       []int
	reqBuilders  []RESTRequestBuilder
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	once         sync.Once
}

// NewWeightedRandomExecutor creates a new weighted random executor from spec.
func NewWeightedRandomExecutor(spec *types.LoadProfileSpec) (Executor, error) {
	if spec.Mode != types.ModeWeightedRandom {
		return nil, fmt.Errorf("expected mode %s, got %s", types.ModeWeightedRandom, spec.Mode)
	}

	if spec.ModeConfig == nil {
		return nil, fmt.Errorf("modeConfig is required")
	}

	// Type assert to WeightedRandomConfig
	config, ok := spec.ModeConfig.(*types.WeightedRandomConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for weighted-random mode")
	}

	// Build request builders
	shares := make([]int, 0, len(config.Requests))
	reqBuilders := make([]RESTRequestBuilder, 0, len(config.Requests))
	for _, r := range config.Requests {
		shares = append(shares, r.Shares)
		if createRequestBuilderFunc == nil {
			return nil, fmt.Errorf("request builder factory not initialized")
		}
		builder, err := createRequestBuilderFunc(r, spec.MaxRetries)
		if err != nil {
			return nil, fmt.Errorf("failed to create request builder: %v", err)
		}
		reqBuilders = append(reqBuilders, builder)
	}

	// Create rate limiter
	qps := config.Rate
	if qps == 0 {
		qps = float64(math.MaxInt32)
	}
	limiter := rate.NewLimiter(rate.Limit(qps), 1)

	ctx, cancel := context.WithCancel(context.Background())
	return &WeightedRandomExecutor{
		config:       config,
		spec:         spec,
		limiter:      limiter,
		reqBuilderCh: make(chan RESTRequestBuilder),
		shares:       shares,
		reqBuilders:  reqBuilders,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Chan returns the channel that produces request builders.
func (e *WeightedRandomExecutor) Chan() <-chan RESTRequestBuilder {
	return e.reqBuilderCh
}

// Run starts the executor and begins generating requests.
func (e *WeightedRandomExecutor) Run(ctx context.Context) error {
	e.wg.Add(1)
	defer e.wg.Done()

	total := e.config.Total
	sum := 0

	for {
		if total > 0 && sum >= total {
			break
		}

		builder := e.randomPick()
		select {
		case e.reqBuilderCh <- builder:
			sum++
		case <-e.ctx.Done():
			return e.ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Stop gracefully stops the executor.
func (e *WeightedRandomExecutor) Stop() {
	e.once.Do(func() {
		e.cancel()
		e.wg.Wait()
		close(e.reqBuilderCh)
	})
}

// Metadata returns executor metadata.
func (e *WeightedRandomExecutor) Metadata() ExecutorMetadata {
	return ExecutorMetadata{
		ExpectedTotal:    e.config.Total,
		ExpectedDuration: time.Duration(e.config.Duration) * time.Second,
		Custom: map[string]interface{}{
			"mode":          string(types.ModeWeightedRandom),
			"rate":          e.config.Rate,
			"request_types": len(e.config.Requests),
		},
	}
}

// randomPick randomly selects a request builder based on weights.
func (e *WeightedRandomExecutor) randomPick() RESTRequestBuilder {
	sum := 0
	for _, s := range e.shares {
		sum += s
	}

	rndInt, err := rand.Int(rand.Reader, big.NewInt(int64(sum)))
	if err != nil {
		panic(err)
	}

	rnd := rndInt.Int64()
	for i := range e.shares {
		s := int64(e.shares[i])
		if rnd < s {
			return e.reqBuilders[i]
		}
		rnd -= s
	}
	panic("unreachable")
}

// GetRateLimiter returns the rate limiter for worker-level rate limiting.
func (e *WeightedRandomExecutor) GetRateLimiter() RateLimiter {
	return e.limiter
}

// GetExecutionContext returns a context with duration timeout if configured.
func (e *WeightedRandomExecutor) GetExecutionContext(baseCtx context.Context) (context.Context, context.CancelFunc) {
	if e.config.Duration > 0 {
		return context.WithTimeout(baseCtx, time.Duration(e.config.Duration)*time.Second)
	}
	return context.WithCancel(baseCtx)
}
