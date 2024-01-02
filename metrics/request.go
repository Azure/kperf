package metrics

import (
	"container/list"
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// ResponseMetric is a measurement related to http response.
type ResponseMetric interface {
	// ObserveLatency observes latency.
	ObserveLatency(seconds float64)
	// ObserveFailure observes failure response.
	ObserveFailure()
	// Gather returns the summary.
	Gather() (latencies []float64, percentileLatencies map[float64]float64, failure int)
}

type responseMetricImpl struct {
	mu           sync.Mutex
	failureCount int64
	latencies    *list.List
}

func NewResponseMetric() ResponseMetric {
	return &responseMetricImpl{
		latencies: list.New(),
	}
}

// ObserveLatency implements ResponseMetric.
func (m *responseMetricImpl) ObserveLatency(seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencies.PushBack(seconds)
}

// ObserveFailure implements ResponseMetric.
func (m *responseMetricImpl) ObserveFailure() {
	atomic.AddInt64(&m.failureCount, 1)
}

// Gather implements ResponseMetric.
func (m *responseMetricImpl) Gather() ([]float64, map[float64]float64, int) {
	latencies := m.dumpLatencies()

	return latencies, buildPercentileLatencies(latencies), int(atomic.LoadInt64(&m.failureCount))
}

func (m *responseMetricImpl) dumpLatencies() []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]float64, 0, m.latencies.Len())
	for e := m.latencies.Front(); e != nil; e = e.Next() {
		res = append(res, e.Value.(float64))
	}
	return res
}

var percentiles = []float64{0, 50, 90, 95, 99, 100}

func buildPercentileLatencies(latencies []float64) map[float64]float64 {
	if len(latencies) == 0 {
		return nil
	}

	res := make(map[float64]float64, len(percentiles))

	n := len(latencies)
	sort.Float64s(latencies)
	for _, p := range percentiles {
		idx := int(math.Ceil(float64(n) * p / 100))
		if idx > 0 {
			idx--
		}
		res[p] = latencies[idx]
	}
	return res
}
