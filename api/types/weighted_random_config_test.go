// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestWeightedRandomConfigGetOverridableFields(t *testing.T) {
	config := &WeightedRandomConfig{}
	fields := config.GetOverridableFields()

	assert.Len(t, fields, 3)

	fieldMap := make(map[string]OverridableField)
	for _, f := range fields {
		fieldMap[f.Name] = f
	}

	assert.Equal(t, FieldTypeFloat64, fieldMap["rate"].Type)
	assert.Contains(t, fieldMap["rate"].Description, "requests per second")

	assert.Equal(t, FieldTypeInt, fieldMap["total"].Type)
	assert.Contains(t, fieldMap["total"].Description, "Total number")

	assert.Equal(t, FieldTypeInt, fieldMap["duration"].Type)
	assert.Contains(t, fieldMap["duration"].Description, "Duration")
}

func TestWeightedRandomConfigApplyOverrides(t *testing.T) {
	tests := map[string]struct {
		initial   WeightedRandomConfig
		overrides map[string]interface{}
		expected  WeightedRandomConfig
		err       bool
	}{
		"rate override": {
			initial: WeightedRandomConfig{Rate: 100, Total: 1000},
			overrides: map[string]interface{}{
				"rate": float64(200),
			},
			expected: WeightedRandomConfig{Rate: 200, Total: 1000},
			err:      false,
		},
		"total override": {
			initial: WeightedRandomConfig{Rate: 100, Total: 1000},
			overrides: map[string]interface{}{
				"total": 2000,
			},
			expected: WeightedRandomConfig{Rate: 100, Total: 2000},
			err:      false,
		},
		"duration override": {
			initial: WeightedRandomConfig{Rate: 100, Duration: 60},
			overrides: map[string]interface{}{
				"duration": 120,
			},
			expected: WeightedRandomConfig{Rate: 100, Duration: 120},
			err:      false,
		},
		"multiple overrides": {
			initial: WeightedRandomConfig{Rate: 100, Total: 1000, Duration: 60},
			overrides: map[string]interface{}{
				"rate":     float64(300),
				"total":    3000,
				"duration": 180,
			},
			expected: WeightedRandomConfig{Rate: 300, Total: 3000, Duration: 180},
			err:      false,
		},
		"invalid rate type": {
			initial: WeightedRandomConfig{Rate: 100},
			overrides: map[string]interface{}{
				"rate": "invalid",
			},
			expected: WeightedRandomConfig{Rate: 100},
			err:      true,
		},
		"invalid total type": {
			initial: WeightedRandomConfig{Total: 1000},
			overrides: map[string]interface{}{
				"total": "invalid",
			},
			expected: WeightedRandomConfig{Total: 1000},
			err:      true,
		},
		"unknown key": {
			initial: WeightedRandomConfig{Rate: 100},
			overrides: map[string]interface{}{
				"unknown": 123,
			},
			expected: WeightedRandomConfig{Rate: 100},
			err:      true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			config := tc.initial
			err := config.ApplyOverrides(tc.overrides)
			if tc.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected.Rate, config.Rate)
				assert.Equal(t, tc.expected.Total, config.Total)
				assert.Equal(t, tc.expected.Duration, config.Duration)
			}
		})
	}
}

func TestWeightedRandomConfigValidate(t *testing.T) {
	tests := map[string]struct {
		config           WeightedRandomConfig
		defaultOverrides map[string]interface{}
		expectedTotal    int
		expectedDuration int
		err              bool
	}{
		"total and duration set - duration ignored": {
			config:           WeightedRandomConfig{Total: 1000, Duration: 60},
			defaultOverrides: nil,
			expectedTotal:    1000,
			expectedDuration: 0,
			err:              false,
		},
		"only total set": {
			config:           WeightedRandomConfig{Total: 1000},
			defaultOverrides: nil,
			expectedTotal:    1000,
			expectedDuration: 0,
			err:              false,
		},
		"only duration set": {
			config:           WeightedRandomConfig{Duration: 60},
			defaultOverrides: nil,
			expectedTotal:    0,
			expectedDuration: 60,
			err:              false,
		},
		"neither set - default total applied": {
			config:           WeightedRandomConfig{},
			defaultOverrides: map[string]interface{}{"total": 500},
			expectedTotal:    500,
			expectedDuration: 0,
			err:              false,
		},
		"neither set - no default": {
			config:           WeightedRandomConfig{},
			defaultOverrides: nil,
			expectedTotal:    0,
			expectedDuration: 0,
			err:              false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			config := tc.config
			err := config.Validate(tc.defaultOverrides)
			if tc.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedTotal, config.Total)
				assert.Equal(t, tc.expectedDuration, config.Duration)
			}
		})
	}
}

func TestWeightedRandomConfigConfigureClientOptions(t *testing.T) {
	tests := map[string]struct {
		config      WeightedRandomConfig
		expectedQPS float64
	}{
		"rate set": {
			config:      WeightedRandomConfig{Rate: 100},
			expectedQPS: 100,
		},
		"rate zero": {
			config:      WeightedRandomConfig{Rate: 0},
			expectedQPS: 0,
		},
		"high rate": {
			config:      WeightedRandomConfig{Rate: 10000},
			expectedQPS: 10000,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			opts := tc.config.ConfigureClientOptions()
			assert.Equal(t, tc.expectedQPS, opts.QPS)
		})
	}
}

func TestLoadProfileWeightedRandomUnmarshalFromYAML(t *testing.T) {
	in := `
version: 1
description: test
spec:
  conns: 2
  client: 1
  contentType: json
  mode: weighted-random
  modeConfig:
    rate: 100
    total: 10000
    requests:
    - staleGet:
        group: core
        version: v1
        resource: pods
        namespace: default
        name: x1
      shares: 100
    - quorumGet:
        group: core
        version: v1
        resource: configmaps
        namespace: default
        name: x2
      shares: 150
    - staleList:
        group: core
        version: v1
        resource: pods
        namespace: default
        selector: app=x2
        fieldSelector: spec.nodeName=x
      shares: 200
    - quorumList:
        group: core
        version: v1
        resource: configmaps
        namespace: default
        limit: 10000
        selector: app=x3
      shares: 400
    - put:
        group: core
        version: v1
        resource: configmaps
        namespace: kperf
        name: kperf-
        keySpaceSize: 1000
        valueSize: 1024
      shares: 1000
    - getPodLog:
        namespace: default
        name: hello
        container: main
        tailLines: 1000
        limitBytes: 1024
      shares: 10
    - watchList:
        group: core
        version: v1
        resource: pods
        namespace: default
        selector: app=x7
        fieldSelector: spec.nodeName=x
      shares: 25
`

	target := LoadProfile{}
	require.NoError(t, yaml.Unmarshal([]byte(in), &target))
	assert.Equal(t, 1, target.Version)
	assert.Equal(t, "test", target.Description)
	assert.Equal(t, 2, target.Spec.Conns)
	assert.Equal(t, ModeWeightedRandom, target.Spec.Mode)

	wrConfig, ok := target.Spec.ModeConfig.(*WeightedRandomConfig)
	require.True(t, ok, "ModeConfig should be *WeightedRandomConfig")
	require.NotNil(t, wrConfig)

	assert.Equal(t, float64(100), wrConfig.Rate)
	assert.Equal(t, 10000, wrConfig.Total)
	assert.Len(t, wrConfig.Requests, 7)

	assert.Equal(t, 100, wrConfig.Requests[0].Shares)
	assert.NotNil(t, wrConfig.Requests[0].StaleGet)
	assert.Equal(t, "pods", wrConfig.Requests[0].StaleGet.Resource)
	assert.Equal(t, "v1", wrConfig.Requests[0].StaleGet.Version)
	assert.Equal(t, "core", wrConfig.Requests[0].StaleGet.Group)
	assert.Equal(t, "default", wrConfig.Requests[0].StaleGet.Namespace)
	assert.Equal(t, "x1", wrConfig.Requests[0].StaleGet.Name)

	assert.NotNil(t, wrConfig.Requests[1].QuorumGet)
	assert.Equal(t, 150, wrConfig.Requests[1].Shares)

	assert.Equal(t, 200, wrConfig.Requests[2].Shares)
	assert.NotNil(t, wrConfig.Requests[2].StaleList)
	assert.Equal(t, "pods", wrConfig.Requests[2].StaleList.Resource)
	assert.Equal(t, "v1", wrConfig.Requests[2].StaleList.Version)
	assert.Equal(t, "core", wrConfig.Requests[2].StaleList.Group)
	assert.Equal(t, "default", wrConfig.Requests[2].StaleList.Namespace)
	assert.Equal(t, 0, wrConfig.Requests[2].StaleList.Limit)
	assert.Equal(t, "app=x2", wrConfig.Requests[2].StaleList.Selector)
	assert.Equal(t, "spec.nodeName=x", wrConfig.Requests[2].StaleList.FieldSelector)

	assert.NotNil(t, wrConfig.Requests[3].QuorumList)
	assert.Equal(t, 400, wrConfig.Requests[3].Shares)

	assert.Equal(t, 1000, wrConfig.Requests[4].Shares)
	assert.NotNil(t, wrConfig.Requests[4].Put)
	assert.Equal(t, "configmaps", wrConfig.Requests[4].Put.Resource)
	assert.Equal(t, "v1", wrConfig.Requests[4].Put.Version)
	assert.Equal(t, "core", wrConfig.Requests[4].Put.Group)
	assert.Equal(t, "kperf", wrConfig.Requests[4].Put.Namespace)
	assert.Equal(t, "kperf-", wrConfig.Requests[4].Put.Name)
	assert.Equal(t, 1000, wrConfig.Requests[4].Put.KeySpaceSize)
	assert.Equal(t, 1024, wrConfig.Requests[4].Put.ValueSize)

	assert.Equal(t, 10, wrConfig.Requests[5].Shares)
	assert.NotNil(t, wrConfig.Requests[5].GetPodLog)
	assert.Equal(t, "default", wrConfig.Requests[5].GetPodLog.Namespace)
	assert.Equal(t, "hello", wrConfig.Requests[5].GetPodLog.Name)
	assert.Equal(t, "main", wrConfig.Requests[5].GetPodLog.Container)
	assert.Equal(t, int64(1000), *wrConfig.Requests[5].GetPodLog.TailLines)
	assert.Equal(t, int64(1024), *wrConfig.Requests[5].GetPodLog.LimitBytes)

	assert.Equal(t, 25, wrConfig.Requests[6].Shares)
	assert.NotNil(t, wrConfig.Requests[6].WatchList)

	assert.NoError(t, target.Validate())
}

func TestLoadProfileWeightedRandomUnmarshalFromYAML_LegacyFormat(t *testing.T) {
	// Test backward compatibility with legacy format (no mode field)
	in := `
version: 1
description: legacy format test
spec:
  rate: 50
  total: 5000
  duration: 120
  conns: 4
  client: 2
  contentType: json
  requests:
  - staleGet:
      group: core
      version: v1
      resource: pods
      namespace: default
      name: test-pod
    shares: 50
  - quorumList:
      group: core
      version: v1
      resource: configmaps
      namespace: default
      limit: 100
    shares: 100
`

	target := LoadProfile{}
	require.NoError(t, yaml.Unmarshal([]byte(in), &target))

	assert.Equal(t, 1, target.Version)
	assert.Equal(t, "legacy format test", target.Description)
	assert.Equal(t, 4, target.Spec.Conns)
	assert.Equal(t, 2, target.Spec.Client)

	// Should auto-migrate to weighted-random mode
	assert.Equal(t, ModeWeightedRandom, target.Spec.Mode)

	wrConfig, ok := target.Spec.ModeConfig.(*WeightedRandomConfig)
	require.True(t, ok, "ModeConfig should be *WeightedRandomConfig for legacy format")
	require.NotNil(t, wrConfig)

	// Verify legacy fields are migrated
	assert.Equal(t, float64(50), wrConfig.Rate)
	assert.Equal(t, 5000, wrConfig.Total)
	assert.Equal(t, 120, wrConfig.Duration)
	assert.Len(t, wrConfig.Requests, 2)

	assert.Equal(t, 50, wrConfig.Requests[0].Shares)
	assert.NotNil(t, wrConfig.Requests[0].StaleGet)
	assert.Equal(t, "pods", wrConfig.Requests[0].StaleGet.Resource)
	assert.Equal(t, "v1", wrConfig.Requests[0].StaleGet.Version)
	assert.Equal(t, "default", wrConfig.Requests[0].StaleGet.Namespace)
	assert.Equal(t, "test-pod", wrConfig.Requests[0].StaleGet.Name)

	assert.Equal(t, 100, wrConfig.Requests[1].Shares)
	assert.NotNil(t, wrConfig.Requests[1].QuorumList)
	assert.Equal(t, "configmaps", wrConfig.Requests[1].QuorumList.Resource)
	assert.Equal(t, 100, wrConfig.Requests[1].QuorumList.Limit)

	assert.NoError(t, target.Validate())
}
