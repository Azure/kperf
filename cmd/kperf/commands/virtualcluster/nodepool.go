// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/virtualcluster"
	"helm.sh/helm/v3/pkg/release"

	"github.com/urfave/cli"
)

var nodepoolCommand = cli.Command{
	Name:  "nodepool",
	Usage: "Manage virtual node pools",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: utils.DefaultKubeConfigPath,
		},
	},
	Subcommands: []cli.Command{
		nodepoolAddCommand,
		nodepoolDelCommand,
		nodepoolListCommand,
	},
}

var nodepoolAddCommand = cli.Command{
	Name:      "add",
	Usage:     "Add a virtual node pool",
	ArgsUsage: "NAME",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "nodes",
			Usage: "The number of virtual nodes",
			Value: 10,
		},
		cli.IntFlag{
			Name:  "cpu",
			Usage: "The allocatable CPU resource per node",
			Value: 8,
		},
		cli.IntFlag{
			Name:  "memory",
			Usage: "The allocatable Memory resource per node (GiB)",
			Value: 16,
		},
		cli.IntFlag{
			Name:  "max-pods",
			Usage: "The maximum Pods per node",
			Value: 110,
		},
		cli.StringSliceFlag{
			Name:  "affinity",
			Usage: "Deploy controllers to the nodes with a specific labels (FORMAT: KEY=VALUE[,VALUE])",
		},
		cli.StringSliceFlag{
			Name:  "node-labels",
			Usage: "Additional labels to node (FORMAT: KEY=VALUE)",
		},
		cli.StringFlag{
			Name:   "shared-provider-id",
			Usage:  "Force all the virtual nodes using one provider ID",
			Hidden: true,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as nodepool name: %v", cliCtx.Args())
		}
		nodepoolName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(nodepoolName) == 0 {
			return fmt.Errorf("required non-empty nodepool name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")

		affinityLabels, err := utils.KeyValuesMap(cliCtx.StringSlice("affinity"))
		if err != nil {
			return fmt.Errorf("failed to parse affinity: %w", err)
		}

		nodeLabels, err := utils.KeyValueMap(cliCtx.StringSlice("node-labels"))
		if err != nil {
			return fmt.Errorf("failed to parse node-labels: %w", err)
		}

		return virtualcluster.CreateNodepool(context.Background(),
			kubeCfgPath,
			nodepoolName,
			virtualcluster.WithNodepoolCPUOpt(cliCtx.Int("cpu")),
			virtualcluster.WithNodepoolMemoryOpt(cliCtx.Int("memory")),
			virtualcluster.WithNodepoolCountOpt(cliCtx.Int("nodes")),
			virtualcluster.WithNodepoolMaxPodsOpt(cliCtx.Int("max-pods")),
			virtualcluster.WithNodepoolNodeControllerAffinity(affinityLabels),
			virtualcluster.WithNodepoolLabelsOpt(nodeLabels),
			virtualcluster.WithNodepoolSharedProviderID(cliCtx.String("shared-provider-id")),
		)
	},
}

var nodepoolDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete a virtual node pool",
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as nodepool name")
		}
		nodepoolName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(nodepoolName) == 0 {
			return fmt.Errorf("required non-empty nodepool name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")

		return virtualcluster.DeleteNodepool(context.Background(), kubeCfgPath, nodepoolName)
	},
}

var nodepoolListCommand = cli.Command{
	Name:  "list",
	Usage: "List virtual node pools",
	Action: func(cliCtx *cli.Context) error {
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		nodepools, err := virtualcluster.ListNodepools(context.Background(), kubeCfgPath)
		if err != nil {
			return err
		}
		return renderRunnerGroups(nodepools)

	},
}

func renderRunnerGroups(nodepools []*release.Release) error {
	tw := tabwriter.NewWriter(os.Stdout, 1, 12, 3, ' ', 0)

	fmt.Fprintln(tw, "NAME\tNODES\tCPU\tMEMORY (GiB)\tMAX PODS\tSTATUS\t")
	for _, nodepool := range nodepools {
		fmt.Fprintf(tw, "%s\t%v\t%v\t%v\t%v\t%s\t\n",
			nodepool.Name,
			// TODO(weifu): show the number of read nodes
			fmt.Sprintf("? / %v", nodepool.Config["replicas"]),
			nodepool.Config["cpu"],
			nodepool.Config["memory"],
			nodepool.Config["maxPods"],
			nodepool.Info.Status,
		)
	}
	return tw.Flush()
}
