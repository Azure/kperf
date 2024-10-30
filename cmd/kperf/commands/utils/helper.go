// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/util/homedir"
)

// DefaultKubeConfigPath is default kubeconfig path if there is home dir.
var DefaultKubeConfigPath string

func init() {
	if !inCluster() {
		if home := homedir.HomeDir(); home != "" {
			DefaultKubeConfigPath = filepath.Join(home, ".kube", "config")
		}
	}
}

// KeyValuesMap converts key=value[,value] into map[string][]string.
func KeyValuesMap(strs []string) (map[string][]string, error) {
	res := make(map[string][]string, len(strs))
	for _, str := range strs {
		key, valuesInStr, ok := strings.Cut(str, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value[,value] format, but got %s", str)
		}
		values := strings.Split(valuesInStr, ",")
		res[key] = values
	}
	return res, nil
}

// KeyValuesMap converts key=value into map[string]string.
func KeyValueMap(strs []string) (map[string]string, error) {
	res := make(map[string]string, len(strs))
	for _, str := range strs {
		key, value, ok := strings.Cut(str, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value format, but got %s", str)
		}
		res[key] = value
	}
	return res, nil
}

// inCluster is to check if current process is in pod.
func inCluster() bool {
	f, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil || f.IsDir() {
		return false
	}

	return os.Getenv("KUBERNETES_SERVICE_HOST") != "" &&
		os.Getenv("KUBERNETES_SERVICE_PORT") != ""
}
