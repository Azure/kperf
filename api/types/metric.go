package types

import "time"

// ResponseStats is the report about benchmark result.
type ResponseStats struct {
	// Total represents total number of requests.
	Total int
	// Failures represents number of failure request.
	Failures int
	// Duration means the time of benchmark.
	Duration time.Duration
	// PercentileLatencies represents the latency distribution in seconds.
	//
	// NOTE: The key represents quantile.
	PercentileLatencies map[float64]float64
	// TODO:
	// 1. Support total read/upload bytes
	// 2. Support failures partitioned by http code and verb
	// 3. Support to dump all latency data
}
