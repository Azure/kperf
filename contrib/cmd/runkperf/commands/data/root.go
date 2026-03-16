// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package data

import (
	"github.com/urfave/cli"
)

var Command = cli.Command{
	Name:  "data",
	Usage: "Create data for runkperf",
}

// RegisterSubcommands adds subcommands to the data command.
// This is called from the parent package to avoid import cycles,
// allowing sub-packages to import the data package for shared utilities.
func RegisterSubcommands(cmds ...cli.Command) {
	Command.Subcommands = append(Command.Subcommands, cmds...)
}
