// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

// ModeConfig is a discriminated union for mode-specific configuration.
// It automatically deserializes to the correct concrete type based on the Mode field.
type ModeConfig interface {
	isModeConfig()
	// ApplyOverrides applies string key-value overrides to the config.
	// This allows each mode to define its own override logic without
	// coupling CLI/util code to specific config fields.
	// Returns error if override key is invalid for this mode.
	ApplyOverrides(overrides map[string]interface{}) error
	// GetOverridableFields returns metadata about fields that can be overridden via CLI.
	// This allows CLI tools to automatically extract and apply overrides without
	// hardcoding field names and types.
	GetOverridableFields() []OverridableField
	// Validate performs mode-specific validation and normalization.
	// This includes checking for conflicting settings and applying defaults.
	// defaultOverrides provides default values from CLI (e.g., default total).
	Validate(defaultOverrides map[string]interface{}) error
	// ConfigureClientOptions returns mode-specific client configuration.
	// This allows each mode to customize REST client behavior (e.g., QPS limiting).
	ConfigureClientOptions() ClientOptions
}

// ClientOptions contains mode-specific REST client configuration
type ClientOptions struct {
	// QPS is the queries per second limit (0 means no limit)
	QPS float64
}

// OverridableField describes a config field that can be overridden via CLI flags.
type OverridableField struct {
	// Name is the field name (e.g., "rate", "total", "interval")
	Name string
	// Type describes the field type for CLI parsing
	Type FieldType
	// Description is help text for CLI flags
	Description string
}

// FieldType indicates the type of a field for CLI flag parsing
type FieldType string

const (
	FieldTypeFloat64 FieldType = "float64"
	FieldTypeInt     FieldType = "int"
	FieldTypeString  FieldType = "string"
	FieldTypeBool    FieldType = "bool"
)

// CLIContext is an interface for CLI flag access (wraps urfave/cli.Context)
// This allows the types package to extract overrides without depending on urfave/cli
type CLIContext interface {
	IsSet(name string) bool
	Float64(name string) float64
	Int(name string) int
	String(name string) string
	Bool(name string) bool
}

// BuildOverridesFromCLI automatically builds an override map from CLI flags
// based on the mode config's declared overridable fields.
func BuildOverridesFromCLI(config ModeConfig, cliCtx CLIContext) map[string]interface{} {
	overrides := make(map[string]interface{})

	for _, field := range config.GetOverridableFields() {
		if !cliCtx.IsSet(field.Name) {
			continue
		}

		switch field.Type {
		case FieldTypeFloat64:
			overrides[field.Name] = cliCtx.Float64(field.Name)
		case FieldTypeInt:
			overrides[field.Name] = cliCtx.Int(field.Name)
		case FieldTypeString:
			overrides[field.Name] = cliCtx.String(field.Name)
		case FieldTypeBool:
			overrides[field.Name] = cliCtx.Bool(field.Name)
		}
	}

	return overrides
}
