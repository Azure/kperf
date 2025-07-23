// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

// SLI Read Only Benchmark
// This benchmark is to test the read-only performance of a Kubernetes cluster
// deploying jobs with 10k pods on 100 virtual nodes.
package bench

import (
	"context"
	"fmt"
	"time"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/utils"

	"github.com/urfave/cli"
)

var benchNode100Job10Pod10kCase = cli.Command{
	Name: "node100_job10_pod10k",
	Usage: `
	The test suite is to setup SLI read-only performance on 100 virtual nodes and deploy 10 jobs with 10k pods on
	those nodes. It creates jobs once and measures read-only performance. The load profile is fixed.
	`,
	Flags: append(
		[]cli.Flag{
			cli.IntFlag{
				Name:  "total",
				Usage: "Total requests per runner (There are 10 runners totally and runner's rate is 10)",
				Value: 1000,
			},
			cli.IntFlag{
				Name:  "job-count",
				Usage: "Number of jobs to deploy",
				Value: 10,
			},
			cli.IntFlag{
				Name:  "pods-per-job",
				Usage: "Number of pods per job",
				Value: 1000,
			},
			cli.IntFlag{
				Name:  "parallelism",
				Usage: "Parallelism for each job",
				Value: 100,
			},
		},
		commonFlags...,
	),
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchNode100Job10Pod10kCaseRun),
		)(cliCtx)
		return err
	},
}

func benchNode100Job10Pod10kCaseRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")

	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/node100_job10_pod10k.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	// Deploy virtual nodes
	vcDone, err := deployVirtualNodepool(ctx, cliCtx, "node100job10pod10k",
		100,
		cliCtx.Int("cpu"),
		cliCtx.Int("memory"),
		cliCtx.Int("max-pods"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy virtual node: %w", err)
	}
	defer func() { _ = vcDone() }()

	// Deploy jobs
	jobCount := cliCtx.Int("job-count")
	podsPerJob := cliCtx.Int("pods-per-job")
	parallelism := cliCtx.Int("parallelism")

	jobsCleanup, err := utils.DeployJobs(
		ctx,
		kubeCfgPath,
		"benchmark-jobs",
		jobCount,
		podsPerJob,
		parallelism,
		"job10pod10k",
		10*time.Minute, // deployTimeout
	)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy jobs: %w", err)
	}
	defer jobsCleanup()

	err = utils.WaitForJobsCompletion(
		ctx,
		kubeCfgPath,
		"job10pod10k",
		"benchmark-jobs",
		jobCount,
		30*time.Minute,
	)
	if err != nil {
		return nil, fmt.Errorf("jobs did not complete: %w", err)
	}

	// Deploy runner group to measure read-only performance
	rgResult, err := utils.DeployRunnerGroup(ctx,
		cliCtx.GlobalString("kubeconfig"),
		cliCtx.GlobalString("runner-image"),
		rgCfgFile,
		cliCtx.GlobalString("runner-flowcontrol"),
		cliCtx.GlobalString("rg-affinity"),
	)
	if err != nil {
		return nil, err
	}

	return &internaltypes.BenchmarkReport{
		Description: fmt.Sprintf(`
		Environment: 100 virtual nodes managed by kwok-controller,
		Workload: Deploy %d jobs with %d pods each (total %d pods) with parallelism %d.
		Measures read-only performance against stable workload.`,
			jobCount, podsPerJob, jobCount*podsPerJob, parallelism),
		LoadSpec: *rgSpec,
		Result:   *rgResult,
		Info:     make(map[string]interface{}),
	}, nil
}
