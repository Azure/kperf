// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestTimeSeriesConfigGetOverridableFields(t *testing.T) {
	config := &TimeSeriesConfig{}
	fields := config.GetOverridableFields()

	// Time-series mode has no CLI-overridable fields
	assert.Len(t, fields, 0)
}

func TestTimeSeriesConfigApplyOverrides(t *testing.T) {
	tests := map[string]struct {
		initial   TimeSeriesConfig
		overrides map[string]interface{}
		err       bool
	}{
		"no overrides": {
			initial:   TimeSeriesConfig{},
			overrides: map[string]interface{}{},
			err:       false,
		},
		"any override fails": {
			initial: TimeSeriesConfig{},
			overrides: map[string]interface{}{
				"interval": "100ms",
			},
			err: true,
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
			}
		})
	}
}

func TestTimeSeriesConfigValidate(t *testing.T) {
	config := &TimeSeriesConfig{}
	err := config.Validate(nil)
	assert.NoError(t, err)
}

func TestTimeSeriesConfigConfigureClientOptions(t *testing.T) {
	config := &TimeSeriesConfig{}
	opts := config.ConfigureClientOptions()
	assert.Equal(t, float64(0), opts.QPS, "time-series should not use client-side rate limiting")
}

func TestLoadProfileTimeSeriesUnmarshalFromYAML(t *testing.T) {
	in := `
version: 1
description: time-series test
spec:
  conns: 5
  client: 10
  contentType: json
  mode: time-series
  modeConfig:
    buckets:
    - startTime: 0.0
      requests:
      - method: GET
        version: v1
        resource: pods
        namespace: default
        name: pod-1
      - method: LIST
        version: v1
        resource: configmaps
        namespace: kube-system
        limit: 100
    - startTime: 1.0
      requests:
      - method: POST
        version: v1
        resource: configmaps
        namespace: default
        name: cm-1
        body: '{"data": {"key": "value"}}'
`

	target := LoadProfile{}
	require.NoError(t, yaml.Unmarshal([]byte(in), &target))
	assert.Equal(t, 1, target.Version)
	assert.Equal(t, "time-series test", target.Description)
	assert.Equal(t, 5, target.Spec.Conns)
	assert.Equal(t, ModeTimeSeries, target.Spec.Mode)

	tsConfig, ok := target.Spec.ModeConfig.(*TimeSeriesConfig)
	require.True(t, ok, "ModeConfig should be *TimeSeriesConfig")
	require.NotNil(t, tsConfig)

	assert.Len(t, tsConfig.Buckets, 2)

	assert.Equal(t, 0.0, tsConfig.Buckets[0].StartTime)
	assert.Len(t, tsConfig.Buckets[0].Requests, 2)
	assert.Equal(t, "GET", tsConfig.Buckets[0].Requests[0].Method)
	assert.Equal(t, "pods", tsConfig.Buckets[0].Requests[0].Resource)
	assert.Equal(t, "default", tsConfig.Buckets[0].Requests[0].Namespace)
	assert.Equal(t, "pod-1", tsConfig.Buckets[0].Requests[0].Name)

	assert.Equal(t, "LIST", tsConfig.Buckets[0].Requests[1].Method)
	assert.Equal(t, "configmaps", tsConfig.Buckets[0].Requests[1].Resource)
	assert.Equal(t, "kube-system", tsConfig.Buckets[0].Requests[1].Namespace)
	assert.Equal(t, 100, tsConfig.Buckets[0].Requests[1].Limit)

	assert.Equal(t, 1.0, tsConfig.Buckets[1].StartTime)
	assert.Len(t, tsConfig.Buckets[1].Requests, 1)
	assert.Equal(t, "POST", tsConfig.Buckets[1].Requests[0].Method)
	assert.Equal(t, "configmaps", tsConfig.Buckets[1].Requests[0].Resource)
	assert.Equal(t, "cm-1", tsConfig.Buckets[1].Requests[0].Name)
	assert.Equal(t, `{"data": {"key": "value"}}`, tsConfig.Buckets[1].Requests[0].Body)

	assert.NoError(t, target.Validate())
}
