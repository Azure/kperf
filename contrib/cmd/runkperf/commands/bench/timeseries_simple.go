// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

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

var benchTimeSeriesSimpleCase = cli.Command{
	Name: "timeseries_simple",
	Usage: `
Test time-series replay mode with a simple 3-bucket pattern.
This benchmark replays exact API requests in time-bucketed intervals to simulate
real production traffic patterns captured from audit logs.
	`,
	Flags: commonFlags,
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchTimeSeriesSimpleCaseRun),
		)(cliCtx)
		return err
	},
}

// benchTimeSeriesSimpleCaseRun is for benchTimeSeriesSimpleCase subcommand.
func benchTimeSeriesSimpleCaseRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")

	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/timeseries_simple.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	vcDone, err := deployVirtualNodepool(ctx, cliCtx, "timeseriestest",
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

		utils.RepeatJobWithPod(jobCtx, kubeCfgPath, "job1pod100", "workload/100pod.job.yaml",
			utils.WithJobIntervalOpt(jobInterval))
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
Environment: 10 virtual nodes managed by kwok-controller
Workload: Deploy 1 job with 100 pods repeatedly. The parallelism is 100. The interval is %v
Mode: time-series replay with 3 time buckets`, jobInterval),
		LoadSpec: *rgSpec,
		Result:   *rgResult,
		Info:     make(map[string]interface{}),
	}, nil
}
