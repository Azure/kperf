// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package metrics

import (
	"container/list"
	"sync"
	"sync/atomic"

	"github.com/Azure/kperf/api/types"
)

// ResponseMetric is a measurement related to http response.
type ResponseMetric interface {
	// ObserveLatency observes latency.
	ObserveLatency(url string, seconds float64)
	// ObserveFailure observes failure response.
	ObserveFailure(err error)
	// ObserveReceivedBytes observes the bytes read from apiserver.
	ObserveReceivedBytes(bytes int64)
	// Gather returns the summary.
	Gather() types.ResponseStats
}

type responseMetricImpl struct {
	mu              sync.Mutex
	errorStats      *types.ResponseErrorStats
	receivedBytes   int64
	latenciesByURLs map[string]*list.List
}

func NewResponseMetric() ResponseMetric {
	return &responseMetricImpl{
		errorStats:      types.NewResponseErrorStats(),
		latenciesByURLs: map[string]*list.List{},
	}
}

// ObserveLatency implements ResponseMetric.
func (m *responseMetricImpl) ObserveLatency(url string, seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	l, ok := m.latenciesByURLs[url]
	if !ok {
		m.latenciesByURLs[url] = list.New()
		l = m.latenciesByURLs[url]
	}
	l.PushBack(seconds)
}

// ObserveFailure implements ResponseMetric.
func (m *responseMetricImpl) ObserveFailure(err error) {
	if err == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// HTTP2 -> TCP/TLS -> Unknown
	code := codeFromHTTP(err)
	switch {
	case code != 0:
		m.errorStats.ResponseCodes[code]++
	case isHTTP2Error(err):
		updateHTTP2ErrorStats(m.errorStats, err)
	case isNetRelatedError(err):
		updateNetErrors(m.errorStats, err)
	default:
		m.errorStats.UnknownErrors = append(m.errorStats.UnknownErrors, err.Error())
	}
}

// ObserveReceivedBytes implements ResponseMetric.
func (m *responseMetricImpl) ObserveReceivedBytes(bytes int64) {
	atomic.AddInt64(&m.receivedBytes, bytes)
}

// Gather implements ResponseMetric.
func (m *responseMetricImpl) Gather() types.ResponseStats {
	return types.ResponseStats{
		ErrorStats:         m.dumpErrorStats(),
		LatenciesByURL:     m.dumpLatencies(),
		TotalReceivedBytes: atomic.LoadInt64(&m.receivedBytes),
	}
}

func (m *responseMetricImpl) dumpLatencies() map[string][]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	res := make(map[string][]float64)
	for u, latencies := range m.latenciesByURLs {
		res[u] = make([]float64, 0, latencies.Len())

		for e := latencies.Front(); e != nil; e = e.Next() {
			res[u] = append(res[u], e.Value.(float64))
		}
	}
	return res
}

func (m *responseMetricImpl) dumpErrorStats() types.ResponseErrorStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.errorStats.Copy()
}
