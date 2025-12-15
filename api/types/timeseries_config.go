// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import "fmt"

// TimeSeriesConfig defines configuration for time-series execution mode.
type TimeSeriesConfig struct {
	// Buckets contains the time-bucketed requests.
	Buckets []RequestBucket `json:"buckets" yaml:"buckets" mapstructure:"buckets"`
}

// RequestBucket represents requests for one time slot.
type RequestBucket struct {
	// StartTime is the relative time in seconds from benchmark start.
	StartTime float64 `json:"startTime" yaml:"startTime" mapstructure:"startTime"`
	// Requests are the exact requests to execute in this bucket.
	Requests []ExactRequest `json:"requests" yaml:"requests" mapstructure:"requests"`
}

// ExactRequest represents a single exact API request.
type ExactRequest struct {
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE, LIST).
	Method string `json:"method" yaml:"method" mapstructure:"method"`
	// Group is the API group.
	Group string `json:"group,omitempty" yaml:"group,omitempty" mapstructure:"group"`
	// Version is the API version.
	Version string `json:"version" yaml:"version" mapstructure:"version"`
	// Resource is the resource type.
	Resource string `json:"resource" yaml:"resource" mapstructure:"resource"`
	// Namespace is the object's namespace.
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty" mapstructure:"namespace"`
	// Name is the object's name.
	Name string `json:"name,omitempty" yaml:"name,omitempty" mapstructure:"name"`
	// Body is the request body for POST/PUT/PATCH.
	Body string `json:"body,omitempty" yaml:"body,omitempty" mapstructure:"body"`
	// PatchType is the patch type for PATCH requests.
	PatchType string `json:"patchType,omitempty" yaml:"patchType,omitempty" mapstructure:"patchType"`
	// LabelSelector for LIST requests.
	LabelSelector string `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty" mapstructure:"labelSelector"`
	// FieldSelector for LIST requests.
	FieldSelector string `json:"fieldSelector,omitempty" yaml:"fieldSelector,omitempty" mapstructure:"fieldSelector"`
	// Limit for LIST requests.
	Limit int `json:"limit,omitempty" yaml:"limit,omitempty" mapstructure:"limit"`
	// ResourceVersion for consistency.
	ResourceVersion string `json:"resourceVersion,omitempty" yaml:"resourceVersion,omitempty" mapstructure:"resourceVersion"`
}

// Ensure TimeSeriesConfig implements ModeConfig
func (*TimeSeriesConfig) isModeConfig() {}

// GetOverridableFields implements ModeConfig for TimeSeriesConfig
func (c *TimeSeriesConfig) GetOverridableFields() []OverridableField {
	// Time-series mode has no CLI-overridable fields
	// Bucket timing is defined in the load profile itself
	return []OverridableField{}
}

// ApplyOverrides implements ModeConfig for TimeSeriesConfig
func (c *TimeSeriesConfig) ApplyOverrides(overrides map[string]interface{}) error {
	// Time-series mode has no overridable fields
	if len(overrides) > 0 {
		return fmt.Errorf("time-series mode does not support CLI overrides")
	}
	return nil
}

// Validate implements ModeConfig for TimeSeriesConfig
func (c *TimeSeriesConfig) Validate(defaultOverrides map[string]interface{}) error {
	// Time-series mode doesn't have conflicting settings or defaults
	// Could add validation for bucket ordering, etc.
	return nil
}

// ConfigureClientOptions implements ModeConfig for TimeSeriesConfig
func (c *TimeSeriesConfig) ConfigureClientOptions() ClientOptions {
	// Time-series mode doesn't use client-side rate limiting
	// (rate is controlled by bucket timing)
	return ClientOptions{
		QPS: 0, // No limit
	}
}
