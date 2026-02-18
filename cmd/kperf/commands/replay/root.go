// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import "github.com/urfave/cli"

// Command represents replay subcommand.
var Command = cli.Command{
	Name:  "replay",
	Usage: "Replay captured Kubernetes API requests at their recorded timestamps",
	Subcommands: []cli.Command{
		runCommand,
	},
}
