// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import "time"

// ResponseErrorType is error type of response.
type ResponseErrorType string

const (
	// ResponseErrorTypeUnknown indicates we don't have correct category for errors.
	ResponseErrorTypeUnknown ResponseErrorType = "unknown"
	// ResponseErrorTypeHTTP indicates that the response returns http code >= 400.
	ResponseErrorTypeHTTP ResponseErrorType = "http"
	// ResponseErrorTypeHTTP2Protocol indicates that error comes from http2 layer.
	ResponseErrorTypeHTTP2Protocol ResponseErrorType = "http2-protocol"
	// ResponseErrorTypeConnection indicates that error is related to connection.
	// For instance, connection refused caused by server down.
	ResponseErrorTypeConnection ResponseErrorType = "connection"
)

// ResponseError is the record about that error.
type ResponseError struct {
	// Timestamp indicates when this error was received.
	Timestamp time.Time `json:"timestamp"`
	// Duration records timespan in seconds.
	Duration float64 `json:"duration"`
	// Type indicates that category to which the error belongs.
	Type ResponseErrorType `json:"type"`
	// Code only works when Type is http.
	Code int `json:"code,omitempty"`
	// Message shows error message for this error.
	//
	// NOTE: When Type is http, this field will be empty.
	Message string `json:"message,omitempty"`
}

// ResponseStats is the report about benchmark result.
type ResponseStats struct {
	// Errors stores all the observed errors.
	Errors []ResponseError
	// LatenciesByURL stores all the observed latencies for each request.
	LatenciesByURL map[string][]float64
	// TotalReceivedBytes is total bytes read from apiserver.
	TotalReceivedBytes int64
}

type RunnerMetricReport struct {
	// Total represents total number of requests.
	Total int `json:"total"`
	// Duration means the time of benchmark.
	Duration string `json:"duration"`
	// Errors stores all the observed errors.
	Errors []ResponseError `json:"errors,omitempty"`
	// ErrorStats means summary of errors group by type.
	ErrorStats map[string]int32 `json:"errorStats,omitempty"`
	// TotalReceivedBytes is total bytes read from apiserver.
	TotalReceivedBytes int64 `json:"totalReceivedBytes"`
	// LatenciesByURL stores all the observed latencies.
	LatenciesByURL map[string][]float64 `json:"latenciesByURL,omitempty"`
	// PercentileLatencies represents the latency distribution in seconds.
	PercentileLatencies [][2]float64 `json:"percentileLatencies,omitempty"`
	// PercentileLatenciesByURL represents the latency distribution in seconds per request.
	PercentileLatenciesByURL map[string][][2]float64 `json:"percentileLatenciesByURL,omitempty"`
}

// TODO(weifu): build brand new struct for RunnerGroupsReport to include more
// information, like how many runner groups, service account and flow control.
type RunnerGroupsReport = RunnerMetricReport
