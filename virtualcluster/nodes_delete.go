// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/kperf/helmcli"

	"helm.sh/helm/v3/pkg/storage/driver"
)

// DeleteNodepool deletes a node pool with a given name.
func DeleteNodepool(_ context.Context, kubeconfigPath string, nodepoolName string) error {
	cfg := defaultNodepoolCfg
	cfg.name = nodepoolName

	if err := cfg.validate(); err != nil {
		return err
	}

	delCli, err := helmcli.NewDeleteCli(kubeconfigPath, virtualnodeReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm delete client: %w", err)
	}

	// delete virtual node controller first
	err = delCli.Delete(cfg.nodeControllerHelmReleaseName())
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return fmt.Errorf("failed to cleanup virtual node controller: %w", err)
	}
	return delCli.Delete(cfg.nodeHelmReleaseName())
}
