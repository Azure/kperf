// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"testing"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/stretchr/testify/assert"
)

func TestAggregateResults(t *testing.T) {
	tests := []struct {
		name           string
		results        []*RunnerResult
		wantTotal      int
		wantRun        int
		wantFailed     int
		wantErrorCount int
		wantURLCount   int
		wantTotalBytes int64
	}{
		{
			name:    "nil results",
			results: []*RunnerResult{nil, nil},
		},
		{
			name:    "empty slice",
			results: []*RunnerResult{},
		},
		{
			name: "single runner",
			results: []*RunnerResult{
				{
					Total:          100,
					RequestsRun:    95,
					RequestsFailed: 5,
					Duration:       time.Second,
					ResponseStats: types.ResponseStats{
						Errors: []types.ResponseError{
							{Method: "GET", URL: "/api/v1/pods", Type: types.ResponseErrorTypeHTTP, Code: 404},
						},
						LatenciesByURL: map[string][]float64{
							"GET /api/v1/pods/:name": {0.1, 0.2, 0.3},
						},
						TotalReceivedBytes: 5000,
					},
				},
			},
			wantTotal:      100,
			wantRun:        95,
			wantFailed:     5,
			wantErrorCount: 1,
			wantURLCount:   1,
			wantTotalBytes: 5000,
		},
		{
			name: "multiple runners with latency merging",
			results: []*RunnerResult{
				{
					Total:          50,
					RequestsRun:    48,
					RequestsFailed: 2,
					Duration:       time.Second,
					ResponseStats: types.ResponseStats{
						Errors: []types.ResponseError{
							{Method: "GET", URL: "/api/v1/pods/p1", Type: types.ResponseErrorTypeHTTP, Code: 500},
						},
						LatenciesByURL: map[string][]float64{
							"GET /api/v1/pods/:name":  {0.1, 0.2},
							"LIST /api/v1/namespaces": {0.5},
						},
						TotalReceivedBytes: 3000,
					},
				},
				{
					Total:          50,
					RequestsRun:    50,
					RequestsFailed: 0,
					Duration:       time.Second,
					ResponseStats: types.ResponseStats{
						Errors:         []types.ResponseError{},
						LatenciesByURL: map[string][]float64{"GET /api/v1/pods/:name": {0.3, 0.4}},
						TotalReceivedBytes: 2000,
					},
				},
			},
			wantTotal:      100,
			wantRun:        98,
			wantFailed:     2,
			wantErrorCount: 1,
			wantURLCount:   2,
			wantTotalBytes: 5000,
		},
		{
			name: "mix of nil and valid results",
			results: []*RunnerResult{
				nil,
				{
					Total:          10,
					RequestsRun:    10,
					RequestsFailed: 0,
					Duration:       time.Second,
					ResponseStats: types.ResponseStats{
						Errors:             []types.ResponseError{},
						LatenciesByURL:     map[string][]float64{"GET /foo": {0.1}},
						TotalReceivedBytes: 100,
					},
				},
				nil,
			},
			wantTotal:      10,
			wantRun:        10,
			wantFailed:     0,
			wantErrorCount: 0,
			wantURLCount:   1,
			wantTotalBytes: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := aggregateResults(tt.results, 5*time.Second)

			assert.Equal(t, tt.wantTotal, result.TotalRequests)
			assert.Equal(t, tt.wantRun, result.TotalRun)
			assert.Equal(t, tt.wantFailed, result.TotalFailed)
			assert.Equal(t, tt.wantErrorCount, len(result.Aggregated.Errors))
			assert.Equal(t, tt.wantURLCount, len(result.Aggregated.LatenciesByURL))
			assert.Equal(t, tt.wantTotalBytes, result.Aggregated.TotalReceivedBytes)
			assert.Equal(t, 5*time.Second, result.Duration)
		})
	}
}

func TestAggregateResultsLatencyMerge(t *testing.T) {
	results := []*RunnerResult{
		{
			Total:       3,
			RequestsRun: 3,
			ResponseStats: types.ResponseStats{
				Errors:         []types.ResponseError{},
				LatenciesByURL: map[string][]float64{"GET /api/v1/pods/:name": {0.1, 0.2}},
			},
		},
		{
			Total:       2,
			RequestsRun: 2,
			ResponseStats: types.ResponseStats{
				Errors:         []types.ResponseError{},
				LatenciesByURL: map[string][]float64{"GET /api/v1/pods/:name": {0.3, 0.4, 0.5}},
			},
		},
	}

	result := aggregateResults(results, time.Second)

	latencies := result.Aggregated.LatenciesByURL["GET /api/v1/pods/:name"]
	assert.Equal(t, 5, len(latencies))
	assert.Equal(t, []float64{0.1, 0.2, 0.3, 0.4, 0.5}, latencies)
}

func TestValidateAndWarnConfig(t *testing.T) {
	// validateAndWarnConfig only logs warnings, so we verify it doesn't panic
	tests := []struct {
		name    string
		profile *types.ReplayProfile
		reqs    [][]types.ReplayRequest
	}{
		{
			name: "normal config",
			profile: &types.ReplayProfile{
				Spec: types.ReplayProfileSpec{
					RunnerCount:      2,
					ConnsPerRunner:   10,
					ClientsPerRunner: 30,
				},
				Requests: makeReplayRequests(100, 1000),
			},
			reqs: [][]types.ReplayRequest{
				makeReplayRequests(50, 1000),
				makeReplayRequests(50, 1000),
			},
		},
		{
			name: "high connections warning",
			profile: &types.ReplayProfile{
				Spec: types.ReplayProfileSpec{
					RunnerCount:      2,
					ConnsPerRunner:   100,
					ClientsPerRunner: 200,
				},
				Requests: makeReplayRequests(100, 1000),
			},
			reqs: [][]types.ReplayRequest{
				makeReplayRequests(50, 1000),
				makeReplayRequests(50, 1000),
			},
		},
		{
			name: "empty runner requests",
			profile: &types.ReplayProfile{
				Spec: types.ReplayProfileSpec{
					RunnerCount:    1,
					ConnsPerRunner: 5,
				},
				Requests: makeReplayRequests(100, 1000),
			},
			reqs: [][]types.ReplayRequest{{}},
		},
		{
			name: "zero ClientsPerRunner defaults to conns",
			profile: &types.ReplayProfile{
				Spec: types.ReplayProfileSpec{
					RunnerCount:      1,
					ConnsPerRunner:   5,
					ClientsPerRunner: 0,
				},
				Requests: makeReplayRequests(50, 1000),
			},
			reqs: [][]types.ReplayRequest{
				makeReplayRequests(50, 1000),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			assert.NotPanics(t, func() {
				validateAndWarnConfig(tt.profile, tt.reqs)
			})
		})
	}
}

// makeReplayRequests creates n replay requests spread over durationMs.
func makeReplayRequests(n int, durationMs int64) []types.ReplayRequest {
	reqs := make([]types.ReplayRequest, n)
	for i := range reqs {
		reqs[i] = types.ReplayRequest{
			Timestamp:    int64(i) * durationMs / int64(n),
			Verb:         "GET",
			ResourceKind: "Pod",
			APIPath:      "/api/v1/namespaces/default/pods/test",
			Name:         "test",
		}
	}
	return reqs
}
