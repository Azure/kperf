// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"fmt"

	"github.com/Azure/kperf/helmcli"
)

// DeleteRunnerGroupServer delete existing long running server.
func DeleteRunnerGroupServer(_ context.Context, kubeconfigPath string) error {
	delCli, err := helmcli.NewDeleteCli(kubeconfigPath, runnerGroupReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm delete client: %w", err)
	}

	return delCli.Delete(runnerGroupServerReleaseName)
}
