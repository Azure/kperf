package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildPercentileLatencies(t *testing.T) {
	ls := make([]float64, 100)
	ls[0] = 50
	ls[1] = 49
	ls[2] = 1
	res := buildPercentileLatencies(ls)
	assert.Equal(t, float64(0), res[0])
	assert.Equal(t, float64(0), res[50])
	assert.Equal(t, float64(0), res[90])
	assert.Equal(t, float64(0), res[95])
	assert.Equal(t, float64(49), res[99])
	assert.Equal(t, float64(50), res[100])

	ls = make([]float64, 1000)
	ls[0] = 50
	ls[1] = 49
	ls[2] = -1
	res = buildPercentileLatencies(ls)
	assert.Equal(t, float64(-1), res[0])
	assert.Equal(t, float64(0), res[50])
	assert.Equal(t, float64(0), res[90])
	assert.Equal(t, float64(0), res[95])
	assert.Equal(t, float64(0), res[99])
	assert.Equal(t, float64(50), res[100])
}

func TestResponseMetric(t *testing.T) {
	c := NewResponseMetric()
	for i := 100; i > 0; i-- {
		c.ObserveLatency(float64(i))
	}

	_, res, _ := c.Gather()
	assert.Equal(t, float64(1), res[0])
	assert.Equal(t, float64(50), res[50])
	assert.Equal(t, float64(90), res[90])
	assert.Equal(t, float64(95), res[95])
	assert.Equal(t, float64(99), res[99])
	assert.Equal(t, float64(100), res[100])
}
