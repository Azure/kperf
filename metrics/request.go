// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package metrics

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Azure/kperf/api/types"
)

// ResponseMetric is a measurement related to http response.
type ResponseMetric interface {
	// ObserveLatency observes latency.
	ObserveLatency(url string, seconds float64)
	// ObserveFailure observes failure response.
	ObserveFailure(now time.Time, seconds float64, err error)
	// ObserveReceivedBytes observes the bytes read from apiserver.
	ObserveReceivedBytes(bytes int64)
	// Gather returns the summary.
	Gather() types.ResponseStats
}

type responseMetricImpl struct {
	mu              sync.Mutex
	errors          *list.List
	receivedBytes   int64
	latenciesByURLs map[string]*list.List
}

func NewResponseMetric() ResponseMetric {
	return &responseMetricImpl{
		errors:          list.New(),
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
func (m *responseMetricImpl) ObserveFailure(now time.Time, seconds float64, err error) {
	if err == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	oerr := types.ResponseError{
		Timestamp: now,
		Duration:  seconds,
	}

	// HTTP Code -> HTTP2 -> Connection -> Unknown
	code := codeFromHTTP(err)
	http2Err, isHTTP2Err := isHTTP2Error(err)
	connErr, isConnErr := isConnectionError(err)
	switch {
	case code != 0:
		oerr.Type = types.ResponseErrorTypeHTTP
		oerr.Code = code
	case isHTTP2Err:
		oerr.Type = types.ResponseErrorTypeHTTP2Protocol
		oerr.Message = http2Err
	case isConnErr:
		oerr.Type = types.ResponseErrorTypeConnection
		oerr.Message = connErr
	default:
		oerr.Type = types.ResponseErrorTypeUnknown
		oerr.Message = err.Error()
	}
	m.errors.PushBack(oerr)
}

// ObserveReceivedBytes implements ResponseMetric.
func (m *responseMetricImpl) ObserveReceivedBytes(bytes int64) {
	atomic.AddInt64(&m.receivedBytes, bytes)
}

// Gather implements ResponseMetric.
func (m *responseMetricImpl) Gather() types.ResponseStats {
	return types.ResponseStats{
		Errors:             m.dumpErrors(),
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

func (m *responseMetricImpl) dumpErrors() []types.ResponseError {
	m.mu.Lock()
	defer m.mu.Unlock()

	res := make([]types.ResponseError, 0, m.errors.Len())
	for e := m.errors.Front(); e != nil; e = e.Next() {
		res = append(res, e.Value.(types.ResponseError))
	}
	return res
}
