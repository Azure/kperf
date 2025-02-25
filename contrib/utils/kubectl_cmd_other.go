//go:build !linux

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"context"
	"fmt"
	"time"
)

// Metrics returns the metrics for a specific kube-apiserver.
func (kr *KubectlRunner) Metrics(ctx context.Context, timeout time.Duration, fqdn, ip string) ([]byte, error) {
	return nil, fmt.Errorf("not supported")
}
