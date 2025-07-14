// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/kperf/helmcli"
	"github.com/Azure/kperf/manifests"
)

func installNodeLifecycleDef(ctx context.Context, kubeCfgPath string) error {
	err := installNodeLifecycleCRD(ctx, kubeCfgPath)
	if err != nil {
		return fmt.Errorf("failed to install node lifecycle CRD: %w", err)
	}

	ch, err := manifests.LoadChart(virtualnodeLifecycleDefChartName)
	if err != nil {
		return fmt.Errorf("failed to load virtual node lifecycle def chart: %w", err)
	}

	releaseCli, err := helmcli.NewReleaseCli(
		kubeCfgPath,
		virtualnodeReleaseNamespace,
		virtualnodeLifecycleDefRelName,
		ch,
		virtualnodeReleaseLabels,
	)
	if err != nil {
		return fmt.Errorf("failed to create helm release client: %w", err)
	}
	return releaseCli.Deploy(ctx, 30*time.Minute)
}

func installNodeLifecycleCRD(ctx context.Context, kubeCfgPath string) error {
	crdCh, err := manifests.LoadChart(virtualnodeLifecycleCRDChartName)
	if err != nil {
		return fmt.Errorf("failed to load virtual node lifecycle CRD chart: %w", err)
	}

	releaseCli, err := helmcli.NewReleaseCli(
		kubeCfgPath,
		virtualnodeReleaseNamespace,
		virtualnodeLifecycleCRDRelName,
		crdCh,
		virtualnodeReleaseLabels,
	)
	if err != nil {
		return fmt.Errorf("failed to create helm release client: %w", err)
	}
	return releaseCli.Deploy(ctx, 30*time.Minute)
}
