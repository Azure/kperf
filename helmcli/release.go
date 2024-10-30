// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package helmcli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/klog/v2"
)

var debugLog action.DebugLog = func(fmt string, args ...interface{}) {
	klog.V(2).Infof(fmt, args...)
}

// ValuesApplier is to apply new key/values to existing chart's values.
type ValuesApplier func(values map[string]interface{}) error

// StringPathValuesApplier applies key/values by string path.
//
// For instance, x.y.z=1 is the same to that YAML value:
//
// ```yaml
//
//	x:
//	  y:
//	    z: 1
//
// ```
func StringPathValuesApplier(values ...string) ValuesApplier {
	return func(to map[string]interface{}) error {
		for _, v := range values {
			if err := strvals.ParseInto(v, to); err != nil {
				return fmt.Errorf("failed to parse (%s) into values: %w", v, err)
			}
		}
		return nil
	}
}

// YAMLValuesApplier applies key/values by YAML.
func YAMLValuesApplier(yamlValues string) (ValuesApplier, error) {
	values := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(yamlValues), &values)
	if err != nil {
		return nil, err
	}

	return func(to map[string]interface{}) error {
		return applyValues(to, values)
	}, nil
}

func applyValues(to, from map[string]interface{}) error {
	for k, v := range from {
		// If 'to' doesn't have key 'k'
		if _, checkKey := to[k]; !checkKey {
			to[k] = v
			continue
		}

		// If 'to' has key 'k'
		switch v := v.(type) {
		case map[string]interface{}:
			// If 'v' is of type map[string]interface{}
			if toMap, checkKey := to[k].(map[string]interface{}); checkKey {
				if err := applyValues(toMap, v); err != nil {
					return err
				}
			} else {
				to[k] = v

			}
		default:
			// If 'v' is not of type map[string]interface{}
			to[k] = v
		}
	}
	return nil
}

// ReleaseCli is a client to deploy helm chart with secret storage.
type ReleaseCli struct {
	namespace string
	name      string

	cfg    *action.Configuration
	ch     *chart.Chart
	values map[string]interface{}
	labels map[string]string
}

// NewReleaseCli returns new ReleaseCli instance.
//
// TODO:
// 1. add flag to disable Wait
func NewReleaseCli(
	kubeconfigPath string,
	namespace string,
	name string,
	ch *chart.Chart,
	labels map[string]string,
	valuesAppliers ...ValuesApplier,
) (*ReleaseCli, error) {
	// build default values
	values, err := copyValues(ch.Values)
	if err != nil {
		return nil, err
	}

	for _, applier := range valuesAppliers {
		if err := applier(values); err != nil {
			return nil, fmt.Errorf("failed to apply: %w", err)
		}
	}

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

	return &ReleaseCli{
		namespace: namespace,
		name:      name,
		cfg:       actionCfg,
		ch:        ch,
		values:    values,
		labels:    labels,
	}, nil
}

// Deploy will install or upgrade that release.
func (cli *ReleaseCli) Deploy(ctx context.Context, timeout time.Duration, valuesAppliers ...ValuesApplier) error {
	values, err := cli.initValues(valuesAppliers...)
	if err != nil {
		return err
	}

	// NOTE: Maintain only one history record just in case that there are
	// too many secret records which causes ETCD OutOfSpace.
	histCli := action.NewHistory(cli.cfg)
	histCli.Max = 1
	if _, err = histCli.Run(cli.name); err == driver.ErrReleaseNotFound {
		installCli := action.NewInstall(cli.cfg)
		installCli.CreateNamespace = true
		installCli.Atomic = true
		installCli.Namespace = cli.namespace
		installCli.ReleaseName = cli.name
		installCli.IsUpgrade = true
		installCli.Timeout = timeout
		installCli.Labels = cli.labels
		installCli.Wait = true

		release, err := installCli.RunWithContext(ctx, cli.ch, values)
		if err != nil {
			return fmt.Errorf("failed to install that release %s: %w", cli.name, err)
		}
		cli.values = release.Config
		return nil
	}

	upgradeCli := action.NewUpgrade(cli.cfg)
	upgradeCli.Namespace = cli.namespace
	upgradeCli.Atomic = true
	upgradeCli.Timeout = timeout
	upgradeCli.MaxHistory = 1
	upgradeCli.Wait = true
	upgradeCli.Labels = cli.labels

	release, err := upgradeCli.RunWithContext(ctx, cli.name, cli.ch, values)
	if err != nil {
		return fmt.Errorf("failed to upgrade that release %s: %w", cli.name, err)
	}

	cli.values = release.Config
	return nil
}

// Uninstall deletes that release.
func (cli *ReleaseCli) Uninstall() error {
	uninstallCli := action.NewUninstall(cli.cfg)
	_, err := uninstallCli.Run(cli.name)
	return err
}

// initValues is to apply valuesAppliers into copied values. Just in case that
// we can rollback if valuesApplier returns error.
func (cli *ReleaseCli) initValues(valuesAppliers ...ValuesApplier) (map[string]interface{}, error) {
	values, err := copyValues(cli.values)
	if err != nil {
		return nil, fmt.Errorf("failed to copy values: %w", err)
	}

	for _, applier := range valuesAppliers {
		if err := applier(values); err != nil {
			return nil, fmt.Errorf("failed to apply: %w", err)
		}
	}
	return values, nil
}

func copyValues(src map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to json.Marshal original values: %w", err)
	}

	newValues := make(map[string]interface{})
	if err := json.Unmarshal(data, &newValues); err != nil {
		return nil, fmt.Errorf("failed to use json.Unmarshal to copy values: %w", err)
	}
	return newValues, nil
}
