// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"fmt"

	"github.com/Azure/kperf/contrib/utils"
	"github.com/Azure/kperf/helmcli"
)

// DeleteRunnerGroupServer delete existing long running server.
func DeleteRunnerGroupServer(_ context.Context, kubeconfigPath string) error {
	delCli, err := helmcli.NewDeleteCli(kubeconfigPath, runnerGroupReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm delete client: %w", err)
	}

	err = delCli.Delete(runnerGroupServerReleaseName)
	if err != nil {
		return fmt.Errorf("failed to delete runner group server: %w", err)
	}

	// Delete the namespace after deleting the release.
	kr := utils.NewKubectlRunner(kubeconfigPath, runnerGroupReleaseNamespace)
	err = kr.DeleteNamespace(context.Background(), 0, runnerGroupReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to delete runner group namespace %s: %w", runnerGroupReleaseNamespace, err)
	}
	return nil
}
