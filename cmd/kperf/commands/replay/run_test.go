// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"testing"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/replay"
	"github.com/stretchr/testify/assert"
)

func TestBuildReplayReport(t *testing.T) {
	result := &replay.ScheduleResult{
		RunnerResults: []*replay.RunnerResult{
			{Total: 50, RequestsRun: 48, RequestsFailed: 2, Duration: time.Second},
			{Total: 50, RequestsRun: 50, RequestsFailed: 0, Duration: time.Second},
		},
		Duration:      2 * time.Second,
		TotalRequests: 100,
		TotalRun:      98,
		TotalFailed:   2,
		Aggregated: types.ResponseStats{
			Errors: []types.ResponseError{
				{Method: "GET", URL: "/api/v1/pods/p1", Type: types.ResponseErrorTypeHTTP, Code: 500},
			},
			LatenciesByURL: map[string][]float64{
				"GET /api/v1/pods/:name":  {0.1, 0.2, 0.3, 0.4},
				"LIST /api/v1/namespaces": {0.5, 0.6},
			},
			TotalReceivedBytes: 5000,
		},
	}

	report := buildReplayReport(result, false)

	assert.Equal(t, 100, report.Total)
	assert.Equal(t, 2, report.RunnerCount)
	assert.Equal(t, 98, report.TotalRun)
	assert.Equal(t, 2, report.TotalFailed)
	assert.Equal(t, int64(5000), report.TotalReceivedBytes)
	assert.NotEmpty(t, report.PercentileLatencies)
	assert.Equal(t, 2, len(report.PercentileLatenciesByURL))

	// Raw data should not be included when includeRawData is false
	assert.Nil(t, report.LatenciesByURL)
	assert.Nil(t, report.Errors)
}

func TestBuildReplayReportWithRawData(t *testing.T) {
	result := &replay.ScheduleResult{
		RunnerResults: []*replay.RunnerResult{},
		Duration:      time.Second,
		Aggregated: types.ResponseStats{
			Errors: []types.ResponseError{
				{Method: "GET", URL: "/api/v1/pods/p1", Type: types.ResponseErrorTypeHTTP, Code: 404},
			},
			LatenciesByURL: map[string][]float64{
				"GET /api/v1/pods/:name": {0.1},
			},
			TotalReceivedBytes: 100,
		},
	}

	report := buildReplayReport(result, true)

	assert.NotNil(t, report.LatenciesByURL)
	assert.NotNil(t, report.Errors)
}
