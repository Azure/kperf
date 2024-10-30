// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package virtualcluster

import "github.com/urfave/cli"

// const namespace = "kperf-virtualcluster"

// Command represents virtualcluster subcommand.
var Command = cli.Command{
	Name:      "virtualcluster",
	ShortName: "vc",
	Usage:     "Setup virtual cluster and run workload on that",
	Subcommands: []cli.Command{
		nodepoolCommand,
	},
}
