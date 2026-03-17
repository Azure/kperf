// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package data

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
)

// NewClientset creates a Kubernetes clientset with default rate limiting.
func NewClientset(kubeCfgPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// NewClientsetWithRateLimiter creates a Kubernetes clientset with custom QPS and burst rate limiting.
func NewClientsetWithRateLimiter(kubeCfgPath string, qps float32, burst int) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, err
	}

	config.QPS = qps
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(qps, burst)
	return kubernetes.NewForConfig(config)
}

// PrepareNamespace creates the namespace if it does not already exist.
// It skips creation for the "default" namespace.
func PrepareNamespace(clientset *kubernetes.Clientset, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	if namespace == "default" {
		return nil
	}

	_, err := clientset.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create namespace %s: %v", namespace, err)
	}
	return nil
}
