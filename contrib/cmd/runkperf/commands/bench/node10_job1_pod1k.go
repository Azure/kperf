// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

// SLI Read Only Benchmark
// This benchmark is to test the read-only performance of a Kubernetes cluster
// deploying a job with 1000 pods on 10 virtual nodes.
package bench

import (
	"context"
	"fmt"
	"sync"
	"time"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/utils"

	"github.com/urfave/cli"
)

var benchNode10Job1Pod1kCase = cli.Command{
	Name: "node10_job1_pod1k",
	Usage: `
	The test suite is to setup 10 virtual nodes and deploy one job with 1000 pods on
	that nodes. It repeats to create and delete job. The load profile is fixed.
	`,
	Flags: append(
		[]cli.Flag{
			cli.IntFlag{
				Name:  "total",
				Usage: "Total requests per runner (There are 10 runners totally and runner's rate is 1)",
				Value: 1000,
			},
		},
		commonFlags...,
	),
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchNode10Job1Pod1kCaseRun),
		)(cliCtx)
		return err
	},
}

func benchNode10Job1Pod1kCaseRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")

	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/node10_job1_pod1k.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	vcDone, err := deployVirtualNodepool(ctx, cliCtx, "node10job1pod1k",
		10,
		cliCtx.Int("cpu"),
		cliCtx.Int("memory"),
		cliCtx.Int("max-pods"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy virtual node: %w", err)
	}
	defer func() { _ = vcDone() }()

	var wg sync.WaitGroup
	wg.Add(1)

	jobInterval := 5 * time.Second
	jobCtx, jobCancel := context.WithCancel(ctx)
	go func() {
		defer wg.Done()

		utils.RepeatJobWithPod(jobCtx, kubeCfgPath, "job1pod1k", "workload/1kpod.job.yaml", jobInterval)
	}()

	rgResult, derr := utils.DeployRunnerGroup(ctx,
		cliCtx.GlobalString("kubeconfig"),
		cliCtx.GlobalString("runner-image"),
		rgCfgFile,
		cliCtx.GlobalString("runner-flowcontrol"),
		cliCtx.GlobalString("rg-affinity"),
	)
	jobCancel()
	wg.Wait()

	if derr != nil {
		return nil, derr
	}

	return &internaltypes.BenchmarkReport{
		Description: fmt.Sprintf(`
		Environment: 10 virtual nodes managed by kwok-controller,
		Workload: Deploy 1 job with 1,000 pods repeatedly. The parallelism is 100. The interval is %v`, jobInterval),
		LoadSpec: *rgSpec,
		Result:   *rgResult,
		Info:     make(map[string]interface{}),
	}, nil
}
