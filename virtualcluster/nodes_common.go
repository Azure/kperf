package virtualcluster

import (
	"fmt"

	"github.com/Azure/kperf/helmcli"
)

var (
	defaultNodepoolCfg = nodepoolConfig{
		count:  10,
		cpu:    8,
		memory: 16, // GiB
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

	// virtualnodeReleaseNamespace is used to host virtual nodes.
	//
	// NOTE: The Node resource is cluster-scope. Just in case that new node
	// name is conflict with existing one, we should use fixed namespace
	// to store all the resources related to virtual nodes.
	virtualnodeReleaseNamespace = "virtualnodes-kperf-io"
)

type nodepoolConfig struct {
	// count represents the desired number of node.
	count int
	// cpu represents a logical CPU resource provided by virtual node.
	cpu int
	// memory represents a logical memory resource provided by virtual node.
	// The unit is GiB.
	memory int
	// labels is to be applied to each virtual node.
	labels []string
	// nodeSelectors forces virtual node's controller to nodes with that specific labels.
	nodeSelectors map[string][]string
}

func (cfg *nodepoolConfig) validate() error {
	if cfg.count <= 0 || cfg.cpu <= 0 || cfg.memory <= 0 {
		return fmt.Errorf("invalid count=%d or cpu=%d or memory=%d",
			cfg.count, cfg.cpu, cfg.memory)
	}
	return nil
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

// WithNodepoolLabelsOpt updates node's labels.
func WithNodepoolLabelsOpt(labels []string) NodepoolOpt {
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

// toHelmValuesAppliers creates ValuesAppliers.
//
// NOTE: Please align with ../manifests/virtualcluster/nodes/values.yaml
//
// TODO: Add YAML ValuesAppliers to support array type.
func (cfg *nodepoolConfig) toHelmValuesAppliers(nodepoolName string) []helmcli.ValuesApplier {
	res := make([]string, 0, 4)

	res = append(res, fmt.Sprintf("name=%s", nodepoolName))
	res = append(res, fmt.Sprintf("replicas=%d", cfg.count))
	res = append(res, fmt.Sprintf("cpu=%d", cfg.cpu))
	res = append(res, fmt.Sprintf("memory=%d", cfg.memory))
	return []helmcli.ValuesApplier{helmcli.StringPathValuesApplier(res...)}
}
