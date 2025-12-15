// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import "fmt"

// WeightedRandomConfig defines configuration for weighted-random execution mode.
type WeightedRandomConfig struct {
	// Rate defines the maximum requests per second (zero is no limit).
	Rate float64 `json:"rate" yaml:"rate" mapstructure:"rate"`
	// Total defines the total number of requests.
	Total int `json:"total" yaml:"total" mapstructure:"total"`
	// Duration defines the running time in seconds.
	Duration int `json:"duration" yaml:"duration" mapstructure:"duration"`
	// Requests defines the different kinds of requests with weights.
	Requests []*WeightedRequest `json:"requests" yaml:"requests" mapstructure:"requests"`
}

// Ensure WeightedRandomConfig implements ModeConfig
func (*WeightedRandomConfig) isModeConfig() {}

// GetOverridableFields implements ModeConfig for WeightedRandomConfig
func (c *WeightedRandomConfig) GetOverridableFields() []OverridableField {
	return []OverridableField{
		{
			Name:        "rate",
			Type:        FieldTypeFloat64,
			Description: "Maximum requests per second (0 means no limit)",
		},
		{
			Name:        "total",
			Type:        FieldTypeInt,
			Description: "Total number of requests to execute",
		},
		{
			Name:        "duration",
			Type:        FieldTypeInt,
			Description: "Duration in seconds (ignored if total is set)",
		},
	}
}

// ApplyOverrides implements ModeConfig for WeightedRandomConfig
func (c *WeightedRandomConfig) ApplyOverrides(overrides map[string]interface{}) error {
	for key, value := range overrides {
		switch key {
		case "rate":
			if v, ok := value.(float64); ok {
				c.Rate = v
			} else {
				return fmt.Errorf("rate must be float64, got %T", value)
			}
		case "total":
			if v, ok := value.(int); ok {
				c.Total = v
			} else {
				return fmt.Errorf("total must be int, got %T", value)
			}
		case "duration":
			if v, ok := value.(int); ok {
				c.Duration = v
			} else {
				return fmt.Errorf("duration must be int, got %T", value)
			}
		default:
			return fmt.Errorf("unknown override key for weighted-random mode: %s", key)
		}
	}
	return nil
}

// Validate implements ModeConfig for WeightedRandomConfig
func (c *WeightedRandomConfig) Validate(defaultOverrides map[string]interface{}) error {
	// Check for conflicting Total and Duration settings
	if c.Total > 0 && c.Duration > 0 {
		// Both set - Duration is ignored
		c.Duration = 0
	}

	// Apply defaults if both are zero
	if c.Total == 0 && c.Duration == 0 {
		if defaultTotal, ok := defaultOverrides["total"].(int); ok {
			c.Total = defaultTotal
		}
	}

	return nil
}

// ConfigureClientOptions implements ModeConfig for WeightedRandomConfig
func (c *WeightedRandomConfig) ConfigureClientOptions() ClientOptions {
	return ClientOptions{
		QPS: c.Rate,
	}
}
