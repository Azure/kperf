// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/helmcli"
	"github.com/Azure/kperf/manifests"

	"gopkg.in/yaml.v3"
)

var (
	defaultRunCmdCfg = runCmdConfig{
		runnerGroupFlowcontrol: struct {
			priorityLevel      string
			matchingPrecedence int
		}{
			priorityLevel:      "workload-low",
			matchingPrecedence: 1000,
		},
	}
)

// CreateRunnerGroupServer creates a long running server to deploy runner groups.
//
// TODO:
// 1. create a new package to define ErrNotFound, ErrAlreadyExists, ... errors.
// 2. support configurable timeout.
func CreateRunnerGroupServer(ctx context.Context,
	kubeconfigPath string,
	runnerImage string,
	rgSpec *types.RunnerGroupSpec,
	opts ...RunCmdOpt,
) error {
	specInStr, err := tweakAndMarshalSpec(rgSpec)
	if err != nil {
		return err
	}

	cfg := defaultRunCmdCfg
	for _, opt := range opts {
		opt(&cfg)
	}

	appiler, err := cfg.toServerHelmValuesAppiler()
	if err != nil {
		return err
	}

	getCli, err := helmcli.NewGetCli(kubeconfigPath, runnerGroupReleaseNamespace)
	if err != nil {
		return fmt.Errorf("failed to create helm get client: %w", err)
	}

	_, err = getCli.Get(runnerGroupServerReleaseName)
	if err == nil {
		return fmt.Errorf("runner group server already exists")
	}

	ch, err := manifests.LoadChart(runnerGroupServerChartName)
	if err != nil {
		return fmt.Errorf("failed to load runner group server chart: %w", err)
	}

	releaseCli, err := helmcli.NewReleaseCli(
		kubeconfigPath,
		runnerGroupReleaseNamespace,
		runnerGroupServerReleaseName,
		ch,
		runnerGroupReleaseLabels,
		helmcli.StringPathValuesApplier(
			"name="+runnerGroupServerReleaseName,
			"image="+runnerImage,
			"runnerGroupSpec="+specInStr,
		),
		appiler,
	)
	if err != nil {
		return fmt.Errorf("failed to create helm release client: %w", err)
	}
	return releaseCli.Deploy(ctx, 120*time.Second)
}

// tweakAndMarshalSpec updates spec's service account if not set and marshals
// it into string.
func tweakAndMarshalSpec(spec *types.RunnerGroupSpec) (string, error) {
	// NOTE: It should be aligned with ../manifests/runnergroup/server/templates/pod.yaml.
	if spec.ServiceAccount == nil {
		var sa = runnerGroupServerReleaseName
		spec.ServiceAccount = &sa
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec: %w", err)
	}
	return string(data), nil
}

type runCmdConfig struct {
	// serverNodeSelectors forces to schedule server to nodes with that specific labels.
	serverNodeSelectors map[string][]string
	// runnerGroupFlowcontrol applies flowcontrol settings to runners.
	//
	// NOTE: Please align with ../manifests/runnergroup/server/values.yaml
	//
	// FIXME(weifu): before v1.0.0, we should define type in ../manifests.
	runnerGroupFlowcontrol struct {
		priorityLevel      string
		matchingPrecedence int
	}

	// TODO(weifu): merge name/image/specs into this
}

// RunCmdOpt is used to update default run command's setting.
type RunCmdOpt func(*runCmdConfig)

// WithRunCmdServerNodeSelectorsOpt updates server's node selectors.
func WithRunCmdServerNodeSelectorsOpt(labels map[string][]string) RunCmdOpt {
	return func(cfg *runCmdConfig) {
		cfg.serverNodeSelectors = labels
	}
}

// WithRunCmdRunnerGroupFlowControl updates runner groups' flowcontrol.
func WithRunCmdRunnerGroupFlowControl(priorityLevel string, matchingPrecedence int) RunCmdOpt {
	return func(cfg *runCmdConfig) {
		cfg.runnerGroupFlowcontrol.priorityLevel = priorityLevel
		cfg.runnerGroupFlowcontrol.matchingPrecedence = matchingPrecedence
	}
}

// toServerHelmValuesAppiler creates ValuesApplier.
//
// NOTE: It should be aligned with ../manifests/runnergroup/server/values.yaml.
func (cfg *runCmdConfig) toServerHelmValuesAppiler() (helmcli.ValuesApplier, error) {
	values := map[string]interface{}{
		"nodeSelectors": cfg.serverNodeSelectors,
		"flowcontrol": map[string]interface{}{
			"priorityLevelConfiguration": cfg.runnerGroupFlowcontrol.priorityLevel,
			"matchingPrecedence":         cfg.runnerGroupFlowcontrol.matchingPrecedence,
		},
	}

	rawData, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to render run command config into YAML: %w", err)
	}

	appiler, err := helmcli.YAMLValuesApplier(string(rawData))
	if err != nil {
		return nil, fmt.Errorf("failed to prepare value appiler for run command config: %w", err)
	}
	return appiler, nil
}
