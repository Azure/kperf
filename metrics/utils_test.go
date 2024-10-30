// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

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
	res := BuildPercentileLatencies(ls)
	assert.Equal(t, [2]float64{0, 0}, res[0])
	assert.Equal(t, [2]float64{0.5, 0}, res[1])
	assert.Equal(t, [2]float64{0.9, 0}, res[2])
	assert.Equal(t, [2]float64{0.95, 0}, res[3])
	assert.Equal(t, [2]float64{0.99, 49}, res[4])
	assert.Equal(t, [2]float64{1, 50}, res[5])

	ls = make([]float64, 1000)
	ls[0] = 50
	ls[1] = 49
	ls[2] = -1
	res = BuildPercentileLatencies(ls)
	assert.Equal(t, [2]float64{0, -1}, res[0])
	assert.Equal(t, [2]float64{0.5, 0}, res[1])
	assert.Equal(t, [2]float64{0.9, 0}, res[2])
	assert.Equal(t, [2]float64{0.95, 0}, res[3])
	assert.Equal(t, [2]float64{0.99, 0}, res[4])
	assert.Equal(t, [2]float64{1, 50}, res[5])
}
