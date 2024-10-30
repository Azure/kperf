// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	rootmainfests "github.com/Azure/kperf/manifests"

	"helm.sh/helm/v3/pkg/chart"
)

// LoadChart returns chart from current package's embed filesystem.
func LoadChart(componentName string) (*chart.Chart, error) {
	return rootmainfests.LoadChartFromEmbedFS(FS, componentName)
}
