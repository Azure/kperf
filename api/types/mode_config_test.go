// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestBuildOverridesFromCLI(t *testing.T) {
	tests := map[string]struct {
		config          ModeConfig
		cliValues       map[string]interface{}
		expectedResult  map[string]interface{}
	}{
		"weighted-random with rate and total": {
			config: &WeightedRandomConfig{},
			cliValues: map[string]interface{}{
				"rate":  float64(250),
				"total": 5000,
			},
			expectedResult: map[string]interface{}{
				"rate":  float64(250),
				"total": 5000,
			},
		},
		"weighted-random with duration only": {
			config: &WeightedRandomConfig{},
			cliValues: map[string]interface{}{
				"duration": 120,
			},
			expectedResult: map[string]interface{}{
				"duration": 120,
			},
		},
		"time-series with interval": {
			config: &TimeSeriesConfig{},
			cliValues: map[string]interface{}{
				"interval": "500ms",
			},
			expectedResult: map[string]interface{}{
				"interval": "500ms",
			},
		},
		"no overrides set": {
			config:         &WeightedRandomConfig{},
			cliValues:      map[string]interface{}{},
			expectedResult: map[string]interface{}{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockCLI := &mockCLIContext{values: tc.cliValues}
			result := BuildOverridesFromCLI(tc.config, mockCLI)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestPolymorphicDeserialization(t *testing.T) {
	tests := map[string]struct {
		yaml         string
		expectedMode ExecutionMode
		validateFunc func(*testing.T, ModeConfig)
	}{
		"weighted-random mode": {
			yaml: `
version: 1
spec:
  mode: weighted-random
  conns: 10
  client: 5
  contentType: json
  modeConfig:
    rate: 150
    total: 2000
    requests:
    - shares: 100
      staleGet:
        version: v1
        resource: pods
        namespace: default
        name: test-pod
`,
			expectedMode: ModeWeightedRandom,
			validateFunc: func(t *testing.T, mc ModeConfig) {
				wrConfig, ok := mc.(*WeightedRandomConfig)
				require.True(t, ok)
				assert.Equal(t, float64(150), wrConfig.Rate)
				assert.Equal(t, 2000, wrConfig.Total)
				assert.Len(t, wrConfig.Requests, 1)
			},
		},
		"time-series mode": {
			yaml: `
version: 1
spec:
  mode: time-series
  conns: 10
  client: 5
  contentType: json
  modeConfig:
    interval: "2s"
    buckets:
    - startTime: 0.0
      requests:
      - method: GET
        version: v1
        resource: pods
        namespace: default
`,
			expectedMode: ModeTimeSeries,
			validateFunc: func(t *testing.T, mc ModeConfig) {
				tsConfig, ok := mc.(*TimeSeriesConfig)
				require.True(t, ok)
				assert.Equal(t, "2s", tsConfig.Interval)
				assert.Len(t, tsConfig.Buckets, 1)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var profile LoadProfile
			err := yaml.Unmarshal([]byte(tc.yaml), &profile)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedMode, profile.Spec.Mode)
			tc.validateFunc(t, profile.Spec.ModeConfig)
		})
	}
}

type mockCLIContext struct {
	values map[string]interface{}
}

func (m *mockCLIContext) IsSet(name string) bool {
	_, exists := m.values[name]
	return exists
}

func (m *mockCLIContext) Float64(name string) float64 {
	if v, ok := m.values[name].(float64); ok {
		return v
	}
	return 0
}

func (m *mockCLIContext) Int(name string) int {
	if v, ok := m.values[name].(int); ok {
		return v
	}
	return 0
}

func (m *mockCLIContext) String(name string) string {
	if v, ok := m.values[name].(string); ok {
		return v
	}
	return ""
}

func (m *mockCLIContext) Bool(name string) bool {
	if v, ok := m.values[name].(bool); ok {
		return v
	}
	return false
}
