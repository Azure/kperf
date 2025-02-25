//go:build linux

// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/kperf/contrib/internal/mountns"

	"golang.org/x/sys/unix"
	"k8s.io/klog/v2"
)

// Metrics returns the metrics for a specific kube-apiserver.
func (kr *KubectlRunner) Metrics(ctx context.Context, timeout time.Duration, fqdn, ip string) ([]byte, error) {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	args = append(args, "get", "--raw", "/metrics")

	var result []byte

	merr := mountns.Executes(func() error {
		newETCHostFile, cleanup, err := CreateTempFileWithContent([]byte(fmt.Sprintf("%s %s\n", ip, fqdn)))
		if err != nil {
			return err
		}
		defer func() { _ = cleanup() }()

		target := "/etc/hosts"

		err = unix.Mount(newETCHostFile, target, "none", unix.MS_BIND, "")
		if err != nil {
			return fmt.Errorf("failed to mount %s on %s: %w",
				newETCHostFile, target, err)
		}
		defer func() {
			derr := unix.Unmount(target, 0)
			if derr != nil {
				klog.Warningf("failed umount %s", target)
			}
		}()

		result, err = runCommand(ctx, timeout, "kubectl", args)
		return err
	})
	return result, merr
}
