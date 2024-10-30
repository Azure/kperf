// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package helmcli

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// ListCli is a client to get helm charts from secret storage.
type ListCli struct {
	namespace string

	cfg *action.Configuration
}

// NewGetCli returns new GetCli instance.
func NewListCli(kubeconfigPath string, namespace string) (*ListCli, error) {
	actionCfg := new(action.Configuration)
	if err := actionCfg.Init(
		&genericclioptions.ConfigFlags{
			KubeConfig: &kubeconfigPath,
		},
		namespace,
		"secret",
		debugLog,
	); err != nil {
		return nil, fmt.Errorf("failed to init action config: %w", err)
	}
	return &ListCli{
		namespace: namespace,
		cfg:       actionCfg,
	}, nil
}

func (cli *ListCli) List() ([]*release.Release, error) {
	listCli := action.NewList(cli.cfg)
	return listCli.Run()
}
