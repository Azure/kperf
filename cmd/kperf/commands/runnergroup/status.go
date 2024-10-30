// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runnergroup

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/runner"

	"github.com/urfave/cli"
)

var statusCommand = cli.Command{
	Name:  "status",
	Usage: "show runner groups' current status",
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		ctx := context.Background()

		rgs, err := runner.ListRunnerGroups(ctx, kubeCfgPath)
		if err != nil {
			return err
		}

		return renderRunnerGroups(rgs)
	},
}

// renderRunnerGroups renders RunnerGroups into table format.
func renderRunnerGroups(rgs []*types.RunnerGroup) error {
	tw := tabwriter.NewWriter(os.Stdout, 1, 12, 3, ' ', 0)

	fmt.Fprintln(tw, "NAME\tCOUNT\tSUCCEEDED\tFAILED\tSTATE\tSTART\t")
	for _, rg := range rgs {
		startAt := "unknown"
		if st := rg.Status.StartTime; st != nil {
			startAt = st.Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%s\t%s\t\n",
			rg.Name,
			rg.Spec.Count,
			rg.Status.Succeeded,
			rg.Status.Failed,
			rg.Status.State,
			startAt,
		)
	}
	return tw.Flush()
}
