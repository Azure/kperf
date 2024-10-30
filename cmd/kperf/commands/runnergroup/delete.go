// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runnergroup

import (
	"context"

	"github.com/Azure/kperf/runner"

	"github.com/urfave/cli"
)

var deleteCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	Usage:     "delete runner groups",
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")

		return runner.DeleteRunnerGroupServer(context.Background(), kubeCfgPath)
	},
}
