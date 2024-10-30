// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package group

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/Azure/kperf/api/types"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// SpecURIType is the scheme of RunnerGroupSpec's URI.
type SpecURIType string

const (
	// SpecURITypeFile is file scheme.
	SpecURITypeFile SpecURIType = "file"

	// SpecURITypeConfigMap is configmap scheme.
	SpecURITypeConfigMap SpecURIType = "configmap"
)

// NewRunnerGroupSpecFromURI builds RunnerGroupSpec via URI. Current supported
// schemes are:
//
//   - file      - The spec is stored in filesystem.
//
//   - configmap - The spec is stored in kubernetes as configmap.
//
// For configmap, current supported query parameters:
//
//   - namespace: The namespace scope for the configmap. Using `default` if not set or empty.
//
//   - specName: The name of data which stores RunnerGroupSpec. Using `spec` if not set or empty.
func NewRunnerGroupSpecFromURI(clientset kubernetes.Interface, specURI string) (*types.RunnerGroupSpec, error) {
	u, err := url.Parse(specURI)
	if err != nil {
		return nil, fmt.Errorf("invalid runner group uri %s: %w", specURI, err)
	}

	switch typ := SpecURIType(u.Scheme); typ {
	case SpecURITypeFile:
		return parseRunnerGroupSpecFromFile(u.Path)
	case SpecURITypeConfigMap:
		var (
			namespace = "default"
			specName  = "spec"
		)

		if ns := u.Query().Get("namespace"); len(ns) > 0 {
			namespace = ns
		}

		if name := u.Query().Get("specName"); len(name) > 0 {
			specName = name
		}
		return parseRunnerGroupSpecFromConfigMap(clientset, namespace, u.Host, specName)
	default:
		return nil, fmt.Errorf("unsupported RunnerGroupSpec's URI scheme: %v", typ)
	}
}

func parseRunnerGroupSpecFromFile(specPath string) (*types.RunnerGroupSpec, error) {
	specInRaw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read runner group spec from %s: %w", specPath, err)
	}

	return parseRunnerGroupSpecFromBinary(specInRaw)
}

func parseRunnerGroupSpecFromConfigMap(clientset kubernetes.Interface, namespace, name, specName string) (*types.RunnerGroupSpec, error) {
	ctx := context.Background()

	cli := clientset.CoreV1().ConfigMaps(namespace)

	cm, err := cli.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to load configmap %s from namespace %s: %w",
			name, namespace, err)
	}

	specInStr, ok := cm.Data[specName]
	if !ok {
		return nil, fmt.Errorf("no such data (%s) in configmap %s from namespace %s",
			specName, name, namespace)
	}

	return parseRunnerGroupSpecFromBinary([]byte(specInStr))
}

func parseRunnerGroupSpecFromBinary(data []byte) (*types.RunnerGroupSpec, error) {
	var spec types.RunnerGroupSpec

	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse RunnerGroupSpec from YAML: %s\nerror: %w", string(data), err)
	}
	return &spec, nil
}
