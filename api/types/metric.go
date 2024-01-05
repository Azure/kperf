package types

import "time"

// ResponseStats is the report about benchmark result.
type ResponseStats struct {
	// Total represents total number of requests.
	Total int
	// List of failures
	FailureList []error
	// Duration means the time of benchmark.
	Duration time.Duration
	// PercentileLatencies represents the latency distribution in seconds.
	//
	// NOTE: The key represents quantile.
	PercentileLatencies map[float64]float64
	// total bytes read from apiserver
	TotalReceivedBytes int64
	// TODO:
	// 1. Support failures partitioned by http code and verb
	// 2. Support to dump all latency data
}
