package metrics

import (
	"container/list"
	"math"
	"sort"
	"sync"
)

// ResponseMetric is a measurement related to http response.
type ResponseMetric interface {
	// ObserveLatency observes latency.
	ObserveLatency(seconds float64)
	// ObserveFailure observes failure response.
	ObserveFailure(err error)
	// Gather returns the summary.
	Gather() (latencies []float64, percentileLatencies map[float64]float64, failureList []error)
}

type responseMetricImpl struct {
	mu          sync.Mutex
	failureList []error
	latencies   *list.List
}

func NewResponseMetric() ResponseMetric {
	errList := make([]error, 0, 1024)
	return &responseMetricImpl{
		latencies:   list.New(),
		failureList: errList,
	}
}

// ObserveLatency implements ResponseMetric.
func (m *responseMetricImpl) ObserveLatency(seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latencies.PushBack(seconds)
}

// ObserveFailure implements ResponseMetric.
func (m *responseMetricImpl) ObserveFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureList = append(m.failureList, err)
}

// Gather implements ResponseMetric.
func (m *responseMetricImpl) Gather() ([]float64, map[float64]float64, []error) {
	latencies := m.dumpLatencies()
	return latencies, buildPercentileLatencies(latencies), m.failureList
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
