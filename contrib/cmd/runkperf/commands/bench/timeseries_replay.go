// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package bench

import (
	"context"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/utils"

	"github.com/urfave/cli"
)

var benchTimeseriesReplayCase = cli.Command{
	Name: "timeseries_replay",
	Usage: `
The test suite demonstrates time-series replay functionality by executing multiple load specs sequentially.
It simulates a traffic pattern with three phases: baseline load, traffic spike, and recovery period.
This allows benchmarking API server performance under varying load conditions over time.
`,
	Flags: append(
		[]cli.Flag{
			cli.IntFlag{
				Name:  "duration",
				Usage: "Duration for each phase in seconds (overrides default)",
			},
		},
		commonFlags...,
	),
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchTimeseriesReplayRun),
		)(cliCtx)
		return err
	},
}

// benchTimeseriesReplayRun executes the timeseries replay benchmark.
func benchTimeseriesReplayRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()

	// Load the load profile
	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/timeseries_replay.yaml")

	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	// Deploy the runner group
	rgResult, derr := utils.DeployRunnerGroup(ctx,
		cliCtx.GlobalString("kubeconfig"),
		cliCtx.GlobalString("runner-image"),
		rgCfgFile,
		cliCtx.GlobalString("runner-flowcontrol"),
		cliCtx.GlobalString("rg-affinity"),
	)

	if derr != nil {
		return nil, derr
	}

	return &internaltypes.BenchmarkReport{
		Description: "Time-series replay: Baseline (10 QPS, 30s) → Spike (100 QPS, 20s) → Recovery (25 QPS, 30s)",
		LoadSpec:    *rgSpec,
		Result:      *rgResult,
		Info:        map[string]interface{}{},
	}, nil
}
