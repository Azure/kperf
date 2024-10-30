// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import apitypes "github.com/Azure/kperf/api/types"

// BenchmarkReport represents runkperf-bench's result.
type BenchmarkReport struct {
	// Description describes test case.
	Description string `json:"description" yaml:"description"`
	// LoadSpec represents what the load profile looks like.
	LoadSpec apitypes.RunnerGroupSpec `json:"loadSpec" yaml:"loadSpec"`
	// Result represents runner group's report.
	Result apitypes.RunnerGroupsReport `json:"result" yaml:"result"`
	// Info is additional information.
	//
	// FIXME(weifu): Use struct after finialized.
	Info map[string]interface{} `json:"info" yaml:"info"`
}
