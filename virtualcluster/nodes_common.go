// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import (
	"fmt"
	"strings"

	"github.com/Azure/kperf/helmcli"

	"sigs.k8s.io/yaml"
)

var (
	defaultNodepoolCfg = nodepoolConfig{
		count:   10,
		cpu:     8,
		memory:  16, // GiB
		maxPods: 110,
	}

	// virtualnodeReleaseLabels is used to mark that helm chart release
	// is managed by kperf.
	virtualnodeReleaseLabels = map[string]string{
		"virtualnodes.kperf.io/managed": "true",
	}
)

const (
	// virtualnodeChartName should be aligned with ../manifests/virtualcluster/nodes.
	virtualnodeChartName = "virtualcluster/nodes"

	// virtualnodeControllerChartName should be aligned with ../manifests/virtualcluster/nodecontrollers.
	virtualnodeControllerChartName = "virtualcluster/nodecontrollers"

	// virtualnodeReleaseNamespace is used to host virtual nodes.
	//
	// NOTE: The Node resource is cluster-scope. Just in case that new node
	// name is conflict with existing one, we should use fixed namespace
	// to store all the resources related to virtual nodes.
	virtualnodeReleaseNamespace = "virtualnodes-kperf-io"

	// reservedNodepoolSuffixName is used to render virtualnodes/nodecontrollers.
	//
	// NOTE: Please check the details in ./nodes_create.go.
	reservedNodepoolSuffixName = "-controller"
)

type nodepoolConfig struct {
	// name represents the name of node pool.
	name string
	// count represents the desired number of node.
	count int
	// cpu represents a logical CPU resource provided by virtual node.
	cpu int
	// memory represents a logical memory resource provided by virtual node.
	// The unit is GiB.
	memory int
	// maxPods represents maximum Pods per node.
	maxPods int
	// labels is to be applied to each virtual node.
	labels map[string]string
	// sharedProviderID is to force all the virtual nodes sharing one providerID.
	//
	// FIXME(weifu):
	//
	// EKS cloud provider will delete all the non-ready virtual nodes
	// if they aren't managed by their cloud provider. The sharedProviderID
	// can be linked to existing node provided by EKS so that cloud provider
	// won't delete the virtual nodes. It's hack.
	sharedProviderID string
	// nodeSelectors forces virtual node's controller to nodes with that specific labels.
	nodeSelectors map[string][]string
}

func (cfg *nodepoolConfig) validate() error {
	if cfg.count <= 0 || cfg.cpu <= 0 || cfg.memory <= 0 {
		return fmt.Errorf("invalid count=%d or cpu=%d or memory=%d",
			cfg.count, cfg.cpu, cfg.memory)
	}

	if cfg.maxPods <= 0 {
		return fmt.Errorf("required max pods > 0, but got %d", cfg.maxPods)
	}

	if cfg.name == "" {
		return fmt.Errorf("required non-empty name")
	}

	if strings.HasSuffix(cfg.name, reservedNodepoolSuffixName) {
		return fmt.Errorf("name can't contain %s as suffix", reservedNodepoolSuffixName)
	}
	return nil
}

func (cfg *nodepoolConfig) nodeHelmReleaseName() string {
	return cfg.name
}

func (cfg *nodepoolConfig) nodeControllerHelmReleaseName() string {
	return cfg.name + reservedNodepoolSuffixName
}

// NodepoolOpt is used to update default node pool's setting.
type NodepoolOpt func(*nodepoolConfig)

// WithNodepoolCountOpt updates node count.
func WithNodepoolCountOpt(count int) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.count = count
	}
}

// WithNodepoolCPUOpt updates CPU resource.
func WithNodepoolCPUOpt(cpu int) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.cpu = cpu
	}
}

// WithNodepoolMemoryOpt updates Memory resource.
func WithNodepoolMemoryOpt(memory int) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.memory = memory
	}
}

// WithNodepoolMaxPodsOpt updates max pods.
func WithNodepoolMaxPodsOpt(maxPods int) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.maxPods = maxPods
	}
}

// WithNodepoolLabelsOpt updates node's labels.
func WithNodepoolLabelsOpt(labels map[string]string) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.labels = labels
	}
}

// WithNodepoolNodeControllerAffinity forces virtual node's controller to
// nodes with that specific labels.
func WithNodepoolNodeControllerAffinity(nodeSelectors map[string][]string) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.nodeSelectors = nodeSelectors
	}
}

// WithNodepoolSharedProviderID forces virtual nodes to share unique providerID.
func WithNodepoolSharedProviderID(providerID string) NodepoolOpt {
	return func(cfg *nodepoolConfig) {
		cfg.sharedProviderID = providerID
	}
}

// toNodeHelmValuesAppliers creates ValuesAppliers.
//
// NOTE: Please align with ../manifests/virtualcluster/nodes/values.yaml
func (cfg *nodepoolConfig) toNodeHelmValuesAppliers() ([]helmcli.ValuesApplier, error) {
	res := make([]string, 0, 6)

	res = append(res, fmt.Sprintf("name=%s", cfg.name))
	res = append(res, fmt.Sprintf("cpu=%d", cfg.cpu))
	res = append(res, fmt.Sprintf("memory=%d", cfg.memory))
	res = append(res, fmt.Sprintf("replicas=%d", cfg.count))
	res = append(res, fmt.Sprintf("maxPods=%d", cfg.maxPods))
	res = append(res, fmt.Sprintf("sharedProviderID=%s", cfg.sharedProviderID))

	nodeLabelsYaml, err := cfg.renderNodeLabels()
	if err != nil {
		return nil, err
	}

	nodeLabelsApplier, err := helmcli.YAMLValuesApplier(nodeLabelsYaml)
	if err != nil {
		return nil, err
	}

	return []helmcli.ValuesApplier{
		helmcli.StringPathValuesApplier(res...),
		nodeLabelsApplier,
	}, nil
}

// renderNodeLabels renders virtual node's labels into YAML string
//
// NOTE: Please align with ../manifests/virtualcluster/nodes/values.yaml
func (cfg *nodepoolConfig) renderNodeLabels() (string, error) {
	target := map[string]interface{}{
		"nodeLabels": cfg.labels,
	}

	rawData, err := yaml.Marshal(target)
	if err != nil {
		return "", fmt.Errorf("failed to render nodeLabels: %w", err)
	}
	return string(rawData), nil
}

// toNodeControllerHelmValuesAppliers creates ValuesAppliers.
//
// NOTE: Please align with ../manifests/virtualcluster/nodecontrollers/values.yaml
func (cfg *nodepoolConfig) toNodeControllerHelmValuesAppliers() ([]helmcli.ValuesApplier, error) {
	res := make([]string, 0, 2)

	res = append(res, fmt.Sprintf("name=%s", cfg.name))
	res = append(res, fmt.Sprintf("replicas=%d", cfg.count))

	stringPathApplier := helmcli.StringPathValuesApplier(res...)
	nodeSelectorsYaml, err := cfg.renderNodeControllerNodeSelectors()
	if err != nil {
		return nil, err
	}

	nodeSelectorsApplier, err := helmcli.YAMLValuesApplier(nodeSelectorsYaml)
	if err != nil {
		return nil, err
	}
	return []helmcli.ValuesApplier{stringPathApplier, nodeSelectorsApplier}, nil
}

// renderNodeControllerNodeSelectors renders node controller's nodeSelectors
// config into YAML string.
//
// NOTE: Please align with ../manifests/virtualcluster/nodecontrollers/values.yaml
func (cfg *nodepoolConfig) renderNodeControllerNodeSelectors() (string, error) {
	target := map[string]interface{}{
		"nodeSelectors": cfg.nodeSelectors,
	}

	rawData, err := yaml.Marshal(target)
	if err != nil {
		return "", fmt.Errorf("failed to render nodeSelectors: %w", err)
	}
	return string(rawData), nil
}
