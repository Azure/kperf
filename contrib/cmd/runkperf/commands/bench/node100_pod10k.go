// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/internal/utils"

	"github.com/urfave/cli"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var benchNode100DeploymentNPod10KCase = cli.Command{
	Name: "node100_pod10k",
	Usage: `

The test suite is to setup 100 virtual nodes and deploy N deployments for 10k
pods on that nodes. It repeats to rolling-update deployments one by one during
benchmark.
	`,
	Flags: append(
		[]cli.Flag{
			cli.IntFlag{
				Name:  "deployments",
				Usage: "The total number of deployments for 10k pods",
				Value: 20,
			},
			cli.IntFlag{
				Name:  "total",
				Usage: "Total requests per runner (There are 10 runners totally and runner's rate is 10)",
				Value: 36000,
			},
			cli.IntFlag{
				Name:  "padding-bytes",
				Usage: "Add <key=data, value=randomStringByLen(padding-bytes)> in pod's annotation to increase pod size",
				Value: 0,
			},
			cli.DurationFlag{
				Name:  "interval",
				Usage: "Interval to restart deployments",
				Value: time.Second * 10,
			},
		},
		commonFlags...,
	),
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchNode100DeploymentNPod10KRun),
		)(cliCtx)
		return err
	},
}

// benchNode100DeploymentNPod10KCase is for subcommand benchNode100DeploymentNPod10KCase.
func benchNode100DeploymentNPod10KRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")

	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/node100_pod10k.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	// NOTE: The nodepool name should be aligned with ../../../../internal/manifests/loadprofile/node100_pod10k.yaml.
	vcDone, err := deployVirtualNodepool(ctx, cliCtx, "node100pod10k",
		100,
		cliCtx.Int("cpu"),
		cliCtx.Int("memory"),
		cliCtx.Int("max-pods"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy virtual node: %w", err)
	}
	defer func() { _ = vcDone() }()

	dpCtx, dpCancel := context.WithCancel(ctx)
	defer dpCancel()

	var wg sync.WaitGroup
	wg.Add(1)

	restartInterval := cliCtx.Duration("interval")
	klog.V(0).Infof("The interval is %v for restaring deployments", restartInterval)

	paddingBytes := cliCtx.Int("padding-bytes")
	total := cliCtx.Int("deployments")
	replica := 10000 / total

	// NOTE: The name pattern should be aligned with ../../../../internal/manifests/loadprofile/node100_pod10k.yaml.
	deploymentNamePattern := "benchmark"

	rollingUpdateFn, ruCleanupFn, err := utils.DeployAndRepeatRollingUpdateDeployments(dpCtx,
		kubeCfgPath, deploymentNamePattern, total, replica, paddingBytes, restartInterval)
	if err != nil {
		dpCancel()
		return nil, fmt.Errorf("failed to setup workload: %w", err)
	}
	defer ruCleanupFn()

	err = dumpDeploymentReplicas(ctx, kubeCfgPath, deploymentNamePattern, total)
	if err != nil {
		return nil, err
	}

	podSize, err := getDeploymentPodSize(ctx, kubeCfgPath, deploymentNamePattern)
	if err != nil {
		return nil, err
	}

	podSize = (podSize / 1024) * 1024

	go func() {
		defer wg.Done()

		// FIXME(weifu):
		//
		// DeployRunnerGroup should return ready notification.
		// The rolling update should run after runners.
		rollingUpdateFn()
	}()

	rgResult, derr := utils.DeployRunnerGroup(ctx,
		cliCtx.GlobalString("kubeconfig"),
		cliCtx.GlobalString("runner-image"),
		rgCfgFile,
		cliCtx.GlobalString("runner-flowcontrol"),
		cliCtx.GlobalString("rg-affinity"),
	)
	dpCancel()
	wg.Wait()

	if derr != nil {
		return nil, derr
	}

	return &internaltypes.BenchmarkReport{
		Description: fmt.Sprintf(`
Environment: 100 virtual nodes managed by kwok-controller,
Workload: Deploy %d deployments with %d pods. Rolling-update deployments one by one and the interval is %v`,
			total, total*replica, restartInterval),

		LoadSpec: *rgSpec,
		Result:   *rgResult,
		Info: map[string]interface{}{
			"podSizeInBytes": podSize,
			"interval":       restartInterval.String(),
		},
	}, nil
}

// dumpDeploymentReplicas dumps deployment's replica.
func dumpDeploymentReplicas(ctx context.Context, kubeCfgPath string, namePattern string, total int) error {
	klog.V(0).Info("Dump deployment's replica information")

	cli, err := utils.BuildClientset(kubeCfgPath)
	if err != nil {
		return err
	}

	for i := 0; i < total; i++ {
		name := fmt.Sprintf("%s-%d", namePattern, i)
		ns := name

		dp, err := cli.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment %s in namespace %s: %w",
				name, ns, err)
		}

		klog.V(0).InfoS("Deployment", "name", name, "ns", ns,
			"replica", *dp.Spec.Replicas, "readyReplicas", dp.Status.ReadyReplicas)
	}
	return nil
}

// getDeploymentPodSize gets the size of pod created by deployment.
func getDeploymentPodSize(ctx context.Context, kubeCfgPath string, namePattern string) (int, error) {
	ns := fmt.Sprintf("%s-0", namePattern)
	labelSelector := fmt.Sprintf("app=%s", namePattern)

	klog.V(0).InfoS("Get the size of pod", "labelSelector", labelSelector, "namespace", ns)

	cli, err := utils.BuildClientset(kubeCfgPath)
	if err != nil {
		return 0, err
	}

	resp, err := cli.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		Limit:         1,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list pods with labelSelector %s: %w",
			labelSelector, err)
	}
	if len(resp.Items) == 0 {
		return 0, fmt.Errorf("no pod with labelSelector %s in namespace %s: %w",
			labelSelector, ns, err)
	}

	pod := resp.Items[0]
	data, err := json.Marshal(pod)
	if err != nil {
		return 0, fmt.Errorf("failed to json.Marshal pod: %w", err)
	}
	return len(data), nil
}
