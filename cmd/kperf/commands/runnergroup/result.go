// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runnergroup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/runner"

	"github.com/urfave/cli"
)

var resultCommand = cli.Command{
	Name:  "result",
	Usage: "show the runner groups' result",
	Flags: []cli.Flag{
		cli.DurationFlag{
			Name:  "timeout",
			Usage: "Timeout for waiting result. Only valid when --wait",
			Value: time.Hour,
		},
		cli.BoolTFlag{
			Name:  "wait",
			Usage: "Wait until result is ready",
		},
	},
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		wait := cliCtx.Bool("wait")

		ctx := context.Background()
		to := cliCtx.Duration("timeout")
		if to > 0 && wait {
			tctx, tcancel := context.WithTimeout(ctx, to)
			defer tcancel()
			ctx = tctx
		}

		res, err := runner.GetRunnerGroupResult(ctx, kubeCfgPath, wait)
		if err != nil {
			return err
		}

		return renderRunnerGroupsReport(res)
	},
}

// renderRunnerGroupsReport renders runner groups' report into stdio.
func renderRunnerGroupsReport(res *types.RunnerGroupsReport) error {
	encoder := json.NewEncoder(os.Stdout)

	encoder.SetIndent("", "  ")
	err := encoder.Encode(res)
	if err != nil {
		return fmt.Errorf("failed to encode json: %w", err)
	}
	return nil
}
