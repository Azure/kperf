// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

// HTTP2ErrorStats is the report about http2 error during testing.
type HTTP2ErrorStats struct {
	// ConnectionErrors represents connection level errors.
	ConnectionErrors map[string]int32 `json:"connectionErrors,omitempty"`
	// StreamErrors represents stream level errors.
	StreamErrors map[string]int32 `json:"streamErrors,omitempty"`
}

// NewHTTP2ErrorStats returns new instance of HTTP2ErrorStats.
func NewHTTP2ErrorStats() *HTTP2ErrorStats {
	return &HTTP2ErrorStats{
		ConnectionErrors: make(map[string]int32, 10),
		StreamErrors:     make(map[string]int32, 10),
	}
}

// ResponseErrorStats is the report about errors.
type ResponseErrorStats struct {
	// UnknownErrors is all unknown errors.
	UnknownErrors []string `json:"unknownErrors"`
	// NetErrors is to track errors from net.
	NetErrors map[string]int32 `json:"netErrors"`
	// ResponseCodes records request number grouped by response
	// code between 400 and 600.
	ResponseCodes map[int]int32 `json:"responseCodes"`
	// HTTP2Errors records http2 related errors.
	HTTP2Errors HTTP2ErrorStats `json:"http2Errors"`
}

// NewResponseErrorStats returns empty ResponseErrorStats.
func NewResponseErrorStats() *ResponseErrorStats {
	return &ResponseErrorStats{
		UnknownErrors: make([]string, 0, 1024),
		NetErrors:     make(map[string]int32, 10),
		ResponseCodes: map[int]int32{},
		HTTP2Errors:   *NewHTTP2ErrorStats(),
	}
}

// Copy clones self.
func (r *ResponseErrorStats) Copy() ResponseErrorStats {
	res := NewResponseErrorStats()

	res.UnknownErrors = make([]string, len(r.UnknownErrors))
	copy(res.UnknownErrors, r.UnknownErrors)
	res.NetErrors = cloneMap(r.NetErrors)
	res.ResponseCodes = cloneMap(r.ResponseCodes)
	res.HTTP2Errors.ConnectionErrors = cloneMap(r.HTTP2Errors.ConnectionErrors)
	res.HTTP2Errors.StreamErrors = cloneMap(r.HTTP2Errors.StreamErrors)
	return *res
}

// Merge merges two ResponseErrorStats.
func (r *ResponseErrorStats) Merge(from *ResponseErrorStats) {
	r.UnknownErrors = append(r.UnknownErrors, from.UnknownErrors...)
	mergeMap(r.NetErrors, from.NetErrors)
	mergeMap(r.ResponseCodes, from.ResponseCodes)
	mergeMap(r.HTTP2Errors.ConnectionErrors, from.HTTP2Errors.ConnectionErrors)
	mergeMap(r.HTTP2Errors.StreamErrors, from.HTTP2Errors.StreamErrors)
}

// ResponseStats is the report about benchmark result.
type ResponseStats struct {
	// ErrorStats means summary of errors.
	ErrorStats ResponseErrorStats
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
	// ErrorStats means summary of errors.
	ErrorStats ResponseErrorStats `json:"errorStats"`
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

func mergeMap[K comparable, V int32](to, from map[K]V) {
	for key, value := range from {
		to[key] += value
	}
}

func cloneMap[K comparable, V int32](src map[K]V) map[K]V {
	res := map[K]V{}
	for key, value := range src {
		res[key] = value
	}
	return res
}
