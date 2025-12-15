// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package executor

import (
	"context"
	"net/url"
	"time"

	"github.com/Azure/kperf/api/types"
	"k8s.io/client-go/rest"
)

// RESTRequestBuilder builds REST requests for the Kubernetes API.
// This interface is used by executors to produce requests that workers will execute.
type RESTRequestBuilder interface {
	Build(cli rest.Interface) Requester
}

// Requester represents a request that can be executed.
type Requester interface {
	Method() string
	URL() *url.URL
	MaskedURL() *url.URL
	Timeout(time.Duration)
	Do(context.Context) (bytes int64, err error)
}

// Executor generates requests according to a specific execution mode.
// This interface abstracts different request generation strategies,
// allowing the scheduler to be mode-agnostic.
type Executor interface {
	// Chan returns a channel that produces RESTRequestBuilders.
	// The scheduler's worker pool consumes from this channel.
	Chan() <-chan RESTRequestBuilder

	// Run starts the executor and begins producing requests.
	// The executor should respect ctx cancellation.
	// Returns error if execution fails (except context.Canceled).
	Run(ctx context.Context) error

	// Stop gracefully stops the executor and closes the channel.
	// Should be idempotent.
	Stop()

	// Metadata returns information about this executor.
	// Used for logging and metrics.
	Metadata() ExecutorMetadata

	// GetRateLimiter returns a rate limiter if this mode requires rate limiting at the worker level.
	// Returns nil if no rate limiting is needed (e.g., time-series mode handles timing internally).
	GetRateLimiter() RateLimiter

	// GetExecutionContext returns a context that includes mode-specific timeouts (e.g., duration).
	// The returned context is derived from the base context and should be used for execution.
	GetExecutionContext(baseCtx context.Context) (context.Context, context.CancelFunc)
}

// RateLimiter is an interface for rate limiting.
// This allows executors to provide custom rate limiting strategies.
type RateLimiter interface {
	// Wait blocks until the limiter permits an event to happen.
	Wait(ctx context.Context) error
}

// ExecutorMetadata contains information about an executor's expected behavior.
type ExecutorMetadata struct {
	// ExpectedTotal is the total number of requests expected (0 if unbounded).
	ExpectedTotal int

	// ExpectedDuration is the expected duration of execution (0 if unbounded).
	ExpectedDuration time.Duration

	// Custom contains mode-specific metadata.
	// This allows modes to provide additional information without changing the interface.
	// Examples:
	//   - Weighted-random: {"rate": 100, "request_types": 5}
	//   - Time-series: {"bucket_count": 1800, "interval": "1s"}
	//   - Poisson: {"lambda": 50, "distribution": "poisson"}
	Custom map[string]interface{}
}

// requestBuilderFactory is a function type for creating request builders from WeightedRequest.
type requestBuilderFactory func(*types.WeightedRequest, int) (RESTRequestBuilder, error)

// exactRequestBuilderFactory is a function type for creating request builders from ExactRequest.
type exactRequestBuilderFactory func(*types.ExactRequest, int) (RESTRequestBuilder, error)

var createRequestBuilderFunc requestBuilderFactory
var createExactRequestBuilderFunc exactRequestBuilderFactory

// SetRequestBuilderFactory sets the factory function for creating request builders from WeightedRequest.
// This is called by the request package during initialization to avoid import cycles.
func SetRequestBuilderFactory(factory requestBuilderFactory) {
	createRequestBuilderFunc = factory
}

// SetExactRequestBuilderFactory sets the factory function for creating request builders from ExactRequest.
// This is called by the request package during initialization to avoid import cycles.
func SetExactRequestBuilderFactory(factory exactRequestBuilderFactory) {
	createExactRequestBuilderFunc = factory
}
