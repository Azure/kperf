package virtualcluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/kperf/virtualcluster"

	"github.com/urfave/cli"
)

var nodepoolCommand = cli.Command{
	Name:  "nodepool",
	Usage: "Manage virtual node pools",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
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
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as nodepool name")
		}
		nodepoolName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(nodepoolName) == 0 {
			return fmt.Errorf("required non-empty nodepool name")
		}

		kubeCfgPath := cliCtx.String("kubeconfig")

		return virtualcluster.CreateNodepool(context.Background(),
			kubeCfgPath,
			nodepoolName,
			virtualcluster.WithNodepoolCPUOpt(cliCtx.Int("cpu")),
			virtualcluster.WithNodepoolMemoryOpt(cliCtx.Int("memory")),
			virtualcluster.WithNodepoolCountOpt(cliCtx.Int("nodes")),
		)
	},
}

var nodepoolDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete a virtual node pool",
	Action: func(cliCtx *cli.Context) error {
		return fmt.Errorf("nodepool delete - not implemented")
	},
}

var nodepoolListCommand = cli.Command{
	Name:  "list",
	Usage: "List virtual node pools",
	Action: func(cliCtx *cli.Context) error {
		return fmt.Errorf("nodepool list - not implemented")
	},
}
