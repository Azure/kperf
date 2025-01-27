// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	flowcontrolv1 "k8s.io/api/flowcontrol/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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

// ApplyPriorityLevelConfiguration applies the PriorityLevelConfiguration manifest using kubectl.
func ApplyPriorityLevelConfiguration(kubeconfigPath string) error {
	// Load the kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %v", err)
	}

	// Create a Kubernetes client
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %v", err)
	}

	// Define the PriorityLevelConfiguration
	lendablePercent := int32(30)
	plc := &flowcontrolv1.PriorityLevelConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "flowcontrol.apiserver.k8s.io/v1",
			Kind:       "PriorityLevelConfiguration",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-system",
		},
		Spec: flowcontrolv1.PriorityLevelConfigurationSpec{
			Type: flowcontrolv1.PriorityLevelEnablementLimited,
			Limited: &flowcontrolv1.LimitedPriorityLevelConfiguration{
				LendablePercent: &lendablePercent,
				LimitResponse: flowcontrolv1.LimitResponse{
					Type: flowcontrolv1.LimitResponseTypeQueue,
					Queuing: &flowcontrolv1.QueuingConfiguration{
						Queues:           64,
						HandSize:         6,
						QueueLengthLimit: 50,
					},
				},
			},
		},
	}

	plcCli := clientset.FlowcontrolV1().PriorityLevelConfigurations()

	// Apply the PriorityLevelConfiguration
	_, err = plcCli.Create(context.TODO(), plc, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, err = plcCli.Update(context.TODO(), plc, metav1.UpdateOptions{})
		}
	}
	if err != nil {
		return fmt.Errorf("failed to apply PriorityLevelConfiguration: %v", err)
	}

	fmt.Printf("Successfully applied PriorityLevelConfiguration: %s\n", plc.Name)
	return nil
}
