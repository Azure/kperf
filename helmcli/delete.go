package helmcli

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// DeleteCli is a client to delete helm release.
type DeleteCli struct {
	namespace string

	cfg *action.Configuration
}

// NewDeleteCli returns new DeleteCli instance.
func NewDeleteCli(kubeconfigPath string, namespace string) (*DeleteCli, error) {
	actionCfg := new(action.Configuration)
	if err := actionCfg.Init(
		&genericclioptions.ConfigFlags{
			KubeConfig: &kubeconfigPath,
		},
		namespace,
		"secret",
		noopLog,
	); err != nil {
		return nil, fmt.Errorf("failed to init action config: %w", err)
	}
	return &DeleteCli{
		namespace: namespace,
		cfg:       actionCfg,
	}, nil
}

// Delete deletes existing helm release.
func (cli *DeleteCli) Delete(releaseName string) error {
	delCli := action.NewUninstall(cli.cfg)
	_, err := delCli.Run(releaseName)
	return err
}
