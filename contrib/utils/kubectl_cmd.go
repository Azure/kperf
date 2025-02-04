// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/kperf/contrib/internal/mountns"
	"golang.org/x/sys/unix"

	"k8s.io/klog/v2"
)

// KubectlRunner is the wrapper of exec.Command to execute kubectl command.
type KubectlRunner struct {
	kubeCfgPath string
	namespace   string
}

func NewKubectlRunner(kubeCfgPath string, namespace string) *KubectlRunner {
	return &KubectlRunner{
		kubeCfgPath: kubeCfgPath,
		namespace:   namespace,
	}
}

// FQDN returns the FQDN of the cluster.
func (kr *KubectlRunner) FQDN(ctx context.Context, timeout time.Duration) (string, error) {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	args = append(args, "cluster-info")

	data, err := runCommand(ctx, timeout, "kubectl", args)
	if err != nil {
		return "", err
	}

	line := strings.Split(string(data), "\n")[0]
	items := strings.Fields(line)

	rawFqdn := items[len(items)-1]
	rawFqdn = strings.TrimPrefix(rawFqdn, "\x1b[0;33m")
	rawFqdn = strings.TrimSuffix(rawFqdn, "\x1b[0m")

	fqdn, err := url.Parse(rawFqdn)
	if err != nil {
		return "", err
	}
	host := strings.Split(fqdn.Host, ":")[0]
	return strings.ToLower(host), nil
}

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

// Wait runs wait subcommand.
func (kr *KubectlRunner) Wait(ctx context.Context, timeout time.Duration, condition, waitTimeout, target string) error {
	if condition == "" {
		return fmt.Errorf("condition is required")
	}

	if target == "" {
		return fmt.Errorf("target is required")
	}

	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	if kr.namespace != "" {
		args = append(args, "-n", kr.namespace)
	}

	args = append(args, "wait", "--for="+condition)
	if waitTimeout != "" {
		args = append(args, "--timeout="+waitTimeout)
	}
	args = append(args, target)

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}

// CreateNamespace creates a new namespace.
func (kr *KubectlRunner) CreateNamespace(ctx context.Context, timeout time.Duration, name string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	args = append(args, "create", "namespace", name)

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}

// DeleteNamespace delete a namespace.
func (kr *KubectlRunner) DeleteNamespace(ctx context.Context, timeout time.Duration, name string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	args = append(args, "delete", "namespace", name)

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}

// Apply runs apply subcommand.
func (kr *KubectlRunner) Apply(ctx context.Context, timeout time.Duration, filePath string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	if kr.namespace != "" {
		args = append(args, "-n", kr.namespace)
	}
	args = append(args, "apply", "-f", filePath)

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}

// ServerSideApplyWithData runs kubectl apply with --server-side=true, with input data piped through stdin.
func (kr *KubectlRunner) ServerSideApplyWithData(ctx context.Context, timeout time.Duration, data string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	if kr.namespace != "" {
		args = append(args, "-n", kr.namespace)
	}
	args = append(args, "apply", "--server-side=true", "--validate=ignore", "-f", "-")

	_, err := runCommandWithInput(ctx, timeout, "kubectl", args, data)
	return err
}

// Delete runs delete subcommand.
func (kr *KubectlRunner) Delete(ctx context.Context, timeout time.Duration, filePath string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	if kr.namespace != "" {
		args = append(args, "-n", kr.namespace)
	}
	args = append(args, "delete", "-f", filePath)

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}

// DeploymentRestart restats a deployment.
func (kr *KubectlRunner) DeploymentRestart(ctx context.Context, timeout time.Duration, name string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	if kr.namespace != "" {
		args = append(args, "-n", kr.namespace)
	}
	args = append(args, "rollout", "restart", "deployment", name)

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}

// DeploymentRolloutStatus watches the rollout status of a deployment.
func (kr *KubectlRunner) DeploymentRolloutStatus(ctx context.Context, timeout time.Duration, name string) error {
	args := []string{}
	if kr.kubeCfgPath != "" {
		args = append(args, "--kubeconfig", kr.kubeCfgPath)
	}
	if kr.namespace != "" {
		args = append(args, "-n", kr.namespace)
	}
	args = append(args, "rollout", "status", fmt.Sprintf("deployment/%s", name))

	_, err := runCommand(ctx, timeout, "kubectl", args)
	return err
}
