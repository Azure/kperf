// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package executor

import (
	"fmt"

	"github.com/Azure/kperf/api/types"
)

// ExecutorConstructor creates an executor from a LoadProfileSpec.
type ExecutorConstructor func(spec *types.LoadProfileSpec) (Executor, error)

// ExecutorFactory creates executors for different execution modes.
type ExecutorFactory struct {
	constructors map[string]ExecutorConstructor
}

var defaultFactory = NewExecutorFactory()

// NewExecutorFactory creates a new factory with built-in modes registered.
func NewExecutorFactory() *ExecutorFactory {
	f := &ExecutorFactory{
		constructors: make(map[string]ExecutorConstructor),
	}

	f.Register(string(types.ModeWeightedRandom), NewWeightedRandomExecutor)
	f.Register(string(types.ModeTimeSeries), NewTimeSeriesExecutor)

	return f
}

// Register registers a new mode constructor.
func (f *ExecutorFactory) Register(mode string, constructor ExecutorConstructor) {
	f.constructors[mode] = constructor
}

// RegisterMode registers a mode constructor using the ExecutionMode type.
func (f *ExecutorFactory) RegisterMode(mode types.ExecutionMode, constructor ExecutorConstructor) {
	f.Register(string(mode), constructor)
}

// Create creates an executor for the given mode.
func (f *ExecutorFactory) Create(spec *types.LoadProfileSpec) (Executor, error) {
	modeStr := string(spec.Mode)
	constructor, ok := f.constructors[modeStr]
	if !ok {
		return nil, fmt.Errorf("unknown executor mode: %s (available modes: %v)",
			spec.Mode, f.AvailableModes())
	}
	return constructor(spec)
}

// AvailableModes returns a list of registered mode names.
func (f *ExecutorFactory) AvailableModes() []string {
	modes := make([]string, 0, len(f.constructors))
	for mode := range f.constructors {
		modes = append(modes, mode)
	}
	return modes
}

// CreateExecutor is a global convenience function that uses the default factory.
func CreateExecutor(spec *types.LoadProfileSpec) (Executor, error) {
	return defaultFactory.Create(spec)
}

// RegisterMode allows external packages to register custom executors.
func RegisterMode(mode types.ExecutionMode, constructor ExecutorConstructor) {
	defaultFactory.RegisterMode(mode, constructor)
}
