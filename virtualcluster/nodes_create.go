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

// CreateNodepool creates a new node pool.
//
// TODO:
// 1. create a new package to define ErrNotFound, ErrAlreadyExists, ... errors.
// 2. support configurable timeout.
//
// FIXME:
//
// Some cloud providers will delete unknown or not-ready nodes. If we render
// both nodes and controllers in one helm release, helm won't wait for
// controller ready before creating nodes. The nodes will be deleted by cloud
// providers. The helm's post-install or post-upgrade hook can ensure that it
// won't deploy nodes until controllers ready. However, resources created by
// helm hook aren't part of helm release. We need extra step to cleanup nodes
// resources when we delete nodepool's helm release. Based on this fact, we
// separate one helm release into two. One is for controllers and other one
// is for nodes.
//
// However, it's not a guarantee. When controller was deleted and it takes long
// time to restart, the node will be marked NotReady and deleted by cloud providers.
// Maybe we can consider to contribute to difference cloud providers with
// workaround. For example, if node.Spec.ProviderID contains `?ignore=virtual`,
// the cloud providers should ignore this kind of nodes.
func CreateNodepool(ctx context.Context, kubeCfgPath string, nodepoolName string, opts ...NodepoolOpt) (retErr error) {
	cfg := defaultNodepoolCfg
	for _, opt := range opts {
		opt(&cfg)
	}
	cfg.name = nodepoolName

	if err := cfg.validate(); err != nil {
		return err
	}

	getCli, err := helmcli.NewGetCli(kubeCfgPath, virtualnodeReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm get client: %w", err)
	}

	_, err = getCli.Get(cfg.nodeHelmReleaseName())
	if err == nil {
		return fmt.Errorf("nodepool %s already exists", cfg.nodeHelmReleaseName())
	}

	cleanupFn, err := createNodepoolController(ctx, kubeCfgPath, &cfg)
	if err != nil {
		return err
	}
	defer func() {
		// NOTE: Try best to cleanup. If there is leaky resources after
		// force stop, like kill process, it needs cleanup manually.
		if retErr != nil {
			_ = cleanupFn()
		}
	}()

	ch, err := manifests.LoadChart(virtualnodeChartName)
	if err != nil {
		return fmt.Errorf("failed to load virtual node chart: %w", err)
	}

	valueAppliers, err := cfg.toNodeHelmValuesAppliers()
	if err != nil {
		return err
	}

	releaseCli, err := helmcli.NewReleaseCli(
		kubeCfgPath,
		virtualnodeReleaseNamespace,
		cfg.nodeHelmReleaseName(),
		ch,
		virtualnodeReleaseLabels,
		valueAppliers...,
	)
	if err != nil {
		return fmt.Errorf("failed to create helm release client: %w", err)
	}
	return releaseCli.Deploy(ctx, 30*time.Minute)
}

// createNodepoolController creates node controller release.
func createNodepoolController(ctx context.Context, kubeCfgPath string, cfg *nodepoolConfig) (_cleanup func() error, _ error) {
	ch, err := manifests.LoadChart(virtualnodeControllerChartName)
	if err != nil {
		return nil, fmt.Errorf("failed to load virtual node controller chart: %w", err)
	}

	appliers, err := cfg.toNodeControllerHelmValuesAppliers()
	if err != nil {
		return nil, err
	}

	releaseCli, err := helmcli.NewReleaseCli(
		kubeCfgPath,
		virtualnodeReleaseNamespace,
		cfg.nodeControllerHelmReleaseName(),
		ch,
		virtualnodeReleaseLabels,
		appliers...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create helm release client: %w", err)
	}

	if err := releaseCli.Deploy(ctx, 30*time.Minute); err != nil {
		return nil, fmt.Errorf("failed to deploy virtual node controller: %w", err)
	}
	return releaseCli.Uninstall, nil
}
