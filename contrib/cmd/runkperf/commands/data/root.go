// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package data

import (
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data/configmaps"
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data/daemonsets"

	"github.com/urfave/cli"
)

var Command = cli.Command{
	Name:  "data",
	Usage: "Create data for runkperf",
	Subcommands: []cli.Command{
		configmaps.Command,
		daemonsets.Command,
	},
}
