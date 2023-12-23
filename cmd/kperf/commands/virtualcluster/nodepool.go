package virtualcluster

import (
	"fmt"

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
		return fmt.Errorf("nodepool add - not implemented")
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
