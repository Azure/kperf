// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package data

import (
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
