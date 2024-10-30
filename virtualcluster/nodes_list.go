// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import (
	"context"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/release"

	"github.com/Azure/kperf/helmcli"
)

// ListNodeppol lists nodepools added by the vc nodeppool add command.
func ListNodepools(_ context.Context, kubeconfigPath string) ([]*release.Release, error) {
	listCli, err := helmcli.NewListCli(kubeconfigPath, virtualnodeReleaseNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create helm list client: %w", err)
	}

	releases, err := listCli.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list nodepool: %w", err)
	}

	// NOTE: Skip node controllers
	res := make([]*release.Release, 0, len(releases)/2)
	for idx := range releases {
		r := releases[idx]
		if strings.HasSuffix(r.Name, reservedNodepoolSuffixName) {
			continue
		}
		res = append(res, r)
	}
	return res, nil
}
