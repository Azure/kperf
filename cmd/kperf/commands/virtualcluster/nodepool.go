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
	"k8s.io/klog/v2"
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
		nodepoolBatchAddCommand,
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

		err := utils.ApplyPriorityLevelConfiguration(kubeCfgPath)
		if err != nil {
			return fmt.Errorf("failed to apply priority level configuration: %w", err)
		}

		affinityLabels, err := utils.KeyValuesMap(cliCtx.StringSlice("affinity"))
		if err != nil {
			return fmt.Errorf("failed to parse affinity: %w", err)
		}

		nodeLabels, err := utils.KeyValueMap(cliCtx.StringSlice("node-labels"))
		if err != nil {
			return fmt.Errorf("failed to parse node-labels: %w", err)
		}

		nodes := cliCtx.Int("nodes")
		if nodes > 300 {
			klog.Warningf("Creating a node pool with a large number of nodes may cause performance issues. Consider using batch-add command for large node pools.")
		}

		return virtualcluster.CreateNodepool(context.Background(),
			kubeCfgPath,
			nodepoolName,
			virtualcluster.WithNodepoolCPUOpt(cliCtx.Int("cpu")),
			virtualcluster.WithNodepoolMemoryOpt(cliCtx.Int("memory")),
			virtualcluster.WithNodepoolCountOpt(nodes),
			virtualcluster.WithNodepoolMaxPodsOpt(cliCtx.Int("max-pods")),
			virtualcluster.WithNodepoolNodeControllerAffinity(affinityLabels),
			virtualcluster.WithNodepoolLabelsOpt(nodeLabels),
			virtualcluster.WithNodepoolSharedProviderID(cliCtx.String("shared-provider-id")),
		)
	},
}

var nodepoolBatchAddCommand = cli.Command{
	Name:      "batch-add",
	Usage:     "Add nodes in batch to multiple virtual node pools instead of one node pool with a large number of nodes",
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
		cli.IntFlag{
			Name:  "batch-size",
			Usage: "Maximum number of nodes to create in one batch, default is 300",
			Value: 300,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("expected exactly one argument as name prefix for nodepool: %v", cliCtx.Args())
		}
		nodepoolName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(nodepoolName) == 0 {
			return fmt.Errorf("nodepool name cannot be empty")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")

		if err := utils.ApplyPriorityLevelConfiguration(kubeCfgPath); err != nil {
			return fmt.Errorf("failed to apply priority level configuration: %w", err)
		}

		affinityLabels, err := utils.KeyValuesMap(cliCtx.StringSlice("affinity"))
		if err != nil {
			return fmt.Errorf("failed to parse affinity labels: %w", err)
		}

		nodeLabels, err := utils.KeyValueMap(cliCtx.StringSlice("node-labels"))
		if err != nil {
			return fmt.Errorf("failed to parse node labels: %w", err)
		}

		totalNodes := cliCtx.Int("nodes")
		batchSize := cliCtx.Int("batch-size")
		if batchSize <= 0 {
			return fmt.Errorf("batch-size must be greater than zero")
		}

		for i := 0; i < totalNodes; i += batchSize {
			currentBatchSize := batchSize
			if i+currentBatchSize > totalNodes {
				currentBatchSize = totalNodes - i
			}

			batchNodepoolName := fmt.Sprintf("%s-batch-%d", nodepoolName, i/batchSize)
			if err := virtualcluster.CreateNodepool(context.Background(),
				kubeCfgPath,
				batchNodepoolName,
				virtualcluster.WithNodepoolCPUOpt(cliCtx.Int("cpu")),
				virtualcluster.WithNodepoolMemoryOpt(cliCtx.Int("memory")),
				virtualcluster.WithNodepoolCountOpt(currentBatchSize),
				virtualcluster.WithNodepoolMaxPodsOpt(cliCtx.Int("max-pods")),
				virtualcluster.WithNodepoolNodeControllerAffinity(affinityLabels),
				virtualcluster.WithNodepoolLabelsOpt(nodeLabels),
				virtualcluster.WithNodepoolSharedProviderID(cliCtx.String("shared-provider-id")),
			); err != nil {
				return fmt.Errorf("failed to create nodepool batch %s: %w", batchNodepoolName, err)
			}
			klog.Infof("Created nodepool batch %s with %d nodes", batchNodepoolName, currentBatchSize)
		}

		return nil
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
		return renderNodepoolList(nodepools)

	},
}

func renderNodepoolList(nodepools []*release.Release) error {
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
