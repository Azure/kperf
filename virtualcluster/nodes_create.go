package virtualcluster

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/kperf/helmcli"
	"github.com/Azure/kperf/manifests"
)

// CreateNodepool creates a new node pool.
//
// TODO:
// 1. create a new package to define ErrNotFound, ErrAlreadyExists, ... errors.
// 2. support configurable timeout.
func CreateNodepool(ctx context.Context, kubeconfigPath string, nodepoolName string, opts ...NodepoolOpt) error {
	cfg := defaultNodepoolCfg
	for _, opt := range opts {
		opt(&cfg)
	}

	if err := cfg.validate(); err != nil {
		return err
	}

	getCli, err := helmcli.NewGetCli(kubeconfigPath, virtualnodeReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm get client: %w", err)
	}

	_, err = getCli.Get(nodepoolName)
	if err == nil {
		return fmt.Errorf("nodepool %s already exists", nodepoolName)
	}

	ch, err := manifests.LoadChart(virtualnodeChartName)
	if err != nil {
		return fmt.Errorf("failed to load virtual node chart: %w", err)
	}

	releaseCli, err := helmcli.NewReleaseCli(
		kubeconfigPath,
		virtualnodeReleaseNamespace,
		nodepoolName,
		ch,
		virtualnodeReleaseLabels,
		cfg.toHelmValuesAppliers(nodepoolName)...,
	)
	if err != nil {
		return fmt.Errorf("failed to create helm release client: %w", err)
	}
	return releaseCli.Deploy(ctx, 120*time.Second)
}
