// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package log

import (
	"context"
)

type Logger interface {
	Logf(msg string, args ...any)

	LogKV(kvs ...any)

	// WithKeyValues returns new logger with default key values
	WithKeyValues(kvs ...any) Logger
}

type loggerKey struct{}

// WithLogger returns a context with provided logger.
func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// GetLogger returns logger from context if applicable. Or it will returns
// builtin logger.
func GetLogger(ctx context.Context) Logger {
	if logger := ctx.Value(loggerKey{}); logger != nil {
		return logger.(Logger)
	}
	return NewLogger(2)
}
