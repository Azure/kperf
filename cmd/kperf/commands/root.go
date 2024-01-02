package commands

import (
	"github.com/Azure/kperf/cmd/kperf/commands/multirunners"
	"github.com/Azure/kperf/cmd/kperf/commands/runner"
	"github.com/Azure/kperf/cmd/kperf/commands/virtualcluster"

	"github.com/urfave/cli"
)

// App returns kperf application.
func App() *cli.App {
	return &cli.App{
		Name: "kperf",
		// TODO: add more fields
		Commands: []cli.Command{
			runner.Command,
			multirunners.Command,
			virtualcluster.Command,
		},
	}
}
