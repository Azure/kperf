// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"context"
	"fmt"
	"time"
)

// KperfRunner is the wrapper of exec.Command to execute kperf command.
type KperfRunner struct {
	kubeCfgPath string
	runnerImage string
}

func NewKperfRunner(kubeCfgPath string, runnerImage string) *KperfRunner {
	return &KperfRunner{
		kubeCfgPath: kubeCfgPath,
		runnerImage: runnerImage,
	}
}

// NewNodepool creates new virtual nodepool.
func (kr *KperfRunner) NewNodepool(
	ctx context.Context,
	timeout time.Duration,
	name string, nodes int, cpu, memory, maxPods int,
	affinity string,
	sharedProviderID string,
) error {
	args := []string{"vc", "nodepool"}
	if kr.kubeCfgPath != "" {
		args = append(args, fmt.Sprintf("--kubeconfig=%s", kr.kubeCfgPath))
	}
	args = append(args, "add", name,
		fmt.Sprintf("--nodes=%v", nodes),
		fmt.Sprintf("--cpu=%v", cpu),
		fmt.Sprintf("--memory=%v", memory),
		fmt.Sprintf("--max-pods=%v", maxPods),
	)
	if affinity != "" {
		args = append(args, fmt.Sprintf("--affinity=%v", affinity))
	}
	if sharedProviderID != "" {
		args = append(args, fmt.Sprintf("--shared-provider-id=%v", sharedProviderID))
	}

	_, err := runCommand(ctx, timeout, "kperf", args)
	return err
}

// DeleteNodepool deletes a virtual nodepool by a given name.
func (kr *KperfRunner) DeleteNodepool(ctx context.Context, timeout time.Duration, name string) error {
	args := []string{"vc", "nodepool"}
	if kr.kubeCfgPath != "" {
		args = append(args, fmt.Sprintf("--kubeconfig=%s", kr.kubeCfgPath))
	}
	args = append(args, "delete", name)

	_, err := runCommand(ctx, timeout, "kperf", args)
	return err
}

// RGRun deploys runner group into kubernetes cluster.
func (kr *KperfRunner) RGRun(ctx context.Context, timeout time.Duration, rgCfgPath, flowcontrol, affinity string) error {
	args := []string{"rg"}
	if kr.kubeCfgPath != "" {
		args = append(args, fmt.Sprintf("--kubeconfig=%s", kr.kubeCfgPath))
	}
	args = append(args, "run",
		fmt.Sprintf("--runnergroup=file://%v", rgCfgPath),
		fmt.Sprintf("--runner-image=%v", kr.runnerImage),
	)
	if affinity != "" {
		args = append(args, fmt.Sprintf("--affinity=%v", affinity))
	}
	if flowcontrol != "" {
		args = append(args, fmt.Sprintf("--runner-flowcontrol=%v", flowcontrol))
	}

	_, err := runCommand(ctx, timeout, "kperf", args)
	return err
}

// RGResult fetches runner group's result.
func (kr *KperfRunner) RGResult(ctx context.Context, timeout time.Duration) (string, error) {
	args := []string{"rg"}
	if kr.kubeCfgPath != "" {
		args = append(args, fmt.Sprintf("--kubeconfig=%s", kr.kubeCfgPath))
	}
	args = append(args, "result")

	data, err := runCommand(ctx, timeout, "kperf", args)
	return string(data), err
}

// RGDelete deletes runner group.
func (kr *KperfRunner) RGDelete(ctx context.Context, timeout time.Duration) error {
	args := []string{"rg"}
	if kr.kubeCfgPath != "" {
		args = append(args, fmt.Sprintf("--kubeconfig=%s", kr.kubeCfgPath))
	}
	args = append(args, "delete")

	_, err := runCommand(ctx, timeout, "kperf", args)
	return err
}
