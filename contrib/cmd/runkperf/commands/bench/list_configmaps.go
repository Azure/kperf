// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package bench

import (
	"context"
	"fmt"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/log"
	"github.com/Azure/kperf/contrib/utils"

	"github.com/urfave/cli"
)

var benchListConfigmapsCase = cli.Command{
	Name: "list_configmaps",
	Usage: `

The test suite is to generate configmaps in a namespace and list them. The load profile is fixed.
	`,
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "size",
			Usage: "The size of each configmap (Unit: KiB)",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "The size of each configmap group",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "configmap-amount",
			Usage: "Total amount of configmaps",
			Value: 1024,
		},
		cli.IntFlag{
			Name:  "total",
			Usage: "Total requests per runner (There are 10 runners totally and runner's rate is 10)",
			Value: 1000,
		},
		cli.IntFlag{
			Name:  "duration-sec",
			Usage: "Duration of the benchmark in seconds. It will be ignored if --total is set.",
			Value: 0,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchListConfigmapsRun),
		)(cliCtx)
		return err
	},
}

var benchConfigmapNamespace = "kperf-configmaps-bench"

// benchfigmapsCase is for subcommand benchConfigmapsCase.
func benchListConfigmapsRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")

	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/list_configmaps.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	// Create a namespace for the benchmark
	cmAmount := cliCtx.Int("configmap-amount")
	cmSize := cliCtx.Int("size")
	cmGroupSize := cliCtx.Int("group-size")

	err = utils.CreateConfigmaps(ctx, kubeCfgPath, cmAmount, cmSize, cmGroupSize, benchConfigmapNamespace, 0)
	if err != nil {
		return nil, err
	}

	defer func() {
		// Delete the configmaps after the benchmark
		err = utils.DeleteConfigmaps(ctx, kubeCfgPath, benchConfigmapNamespace, 0)
		if err != nil {
			log.GetLogger(ctx).WithKeyValues("level", "error").
				LogKV("msg", fmt.Sprintf("Failed to delete configmaps: %v", err))
		}

		// Delete the namespace after the benchmark
		kr := utils.NewKubectlRunner(kubeCfgPath, benchConfigmapNamespace)
		err := kr.DeleteNamespace(ctx, 0, benchConfigmapNamespace)
		if err != nil {
			log.GetLogger(ctx).WithKeyValues("level", "error").
				LogKV("msg", fmt.Sprintf("Failed to delete namespace: %v", err))
		}
	}()

	dpCtx, dpCancel := context.WithCancel(ctx)
	defer dpCancel()

	duration := cliCtx.Duration("duration-sec")
	if duration != 0 {
		log.GetLogger(dpCtx).
			WithKeyValues("level", "info").
			LogKV("msg", fmt.Sprintf("Running for %v seconds", duration.Seconds()))
	}

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
		Description: fmt.Sprintf(`
Environment: Generate %v configmaps with %v bytes each in a namespace.
Workload: List all configmaps in the namespace and get the percentile latency.`,
			cmAmount, cmSize),

		LoadSpec: *rgSpec,
		Result:   *rgResult,
		Info: map[string]interface{}{
			"configmapSizeInBytes": cmSize,
			"runningTime":          duration.String(),
		},
	}, nil
}
