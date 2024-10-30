// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package helmcli

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// GetCli is a client to get helm chart from secret storage.
type GetCli struct {
	namespace string

	cfg *action.Configuration
}

// NewGetCli returns new GetCli instance.
func NewGetCli(kubeconfigPath string, namespace string) (*GetCli, error) {
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
	return &GetCli{
		namespace: namespace,
		cfg:       actionCfg,
	}, nil
}

// Get returns all the information about that given release.
func (cli *GetCli) Get(releaseName string) (*release.Release, error) {
	getCli := action.NewGet(cli.cfg)
	return getCli.Run(releaseName)
}
