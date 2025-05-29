// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/kperf/contrib/utils"
	"helm.sh/helm/v3/pkg/release"

	"github.com/Azure/kperf/helmcli"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListNodeppol lists nodepools added by the vc nodeppool add command.
func ListNodepools(_ context.Context, kubeconfigPath string, prefix string) ([]*release.Release, error) {
	// get all namespaces with the prefix of virtualnodeReleaseNamespace
	nsList, err := listNodepoolNamespacesWithPrefix(kubeconfigPath, prefix)

	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	res := make([]*release.Release, 0, len(nsList))
	for _, ns := range nsList {
		listCli, err := helmcli.NewListCli(kubeconfigPath, ns)
		if err != nil {
			return nil, fmt.Errorf("failed to create helm list client: %w", err)
		}

		releases, err := listCli.List()
		if err != nil {
			return nil, fmt.Errorf("failed to list nodepool: %w", err)
		}

		for idx := range releases {
			r := releases[idx]
			if strings.HasSuffix(r.Name, reservedNodepoolSuffixName) {
				continue
			}
			res = append(res, r)
		}
	}
	return res, nil
}

// listNodepoolNamespacesWithPrefix lists Kperf noodpools' namespaces with a given prefix.
func listNodepoolNamespacesWithPrefix(kubeconfigPath string, prefix string) ([]string, error) {
	clientset, err := utils.BuildClientset(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build clientset in listNodepoolNamespacesWithPrefix: %w", err)
	}

	nsList, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces in listNodepoolNamespacesWithPrefix: %w", err)
	}

	var res []string
	// Kperf nodepools' namespaces are prefixed with virtualnodeReleaseNamespace
	nodepoolPrefix := fmt.Sprintf("%s-%s", virtualnodeReleaseNamespace, prefix)
	for _, ns := range nsList.Items {
		if strings.HasPrefix(ns.Name, nodepoolPrefix) {
			res = append(res, ns.Name)
		}
	}
	return res, nil
}
