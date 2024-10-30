// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package commands

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/bench"
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/warmup"

	"github.com/urfave/cli"
	"k8s.io/klog/v2"
)

// App returns kperf application.
func App() *cli.App {
	return &cli.App{
		Name: "runkperf",
		// TODO: add more fields
		Commands: []cli.Command{
			warmup.Command,
			bench.Command,
		},
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "v",
				Usage: "log level for V logs",
				Value: "0",
			},
		},
		Before: func(cliCtx *cli.Context) error {
			return initKlog(cliCtx)
		},
	}
}

// initKlog initializes klog.
func initKlog(cliCtx *cli.Context) error {
	klogFlagset := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(klogFlagset)

	vInStr := cliCtx.GlobalString("v")
	if vFlag, err := strconv.Atoi(vInStr); err != nil || vFlag < 0 {
		return fmt.Errorf("invalid value \"%v\" for flag -v: value must be a non-negative integer", vInStr)
	}

	if err := klogFlagset.Set("v", vInStr); err != nil {
		return fmt.Errorf("failed to set log level: %w", err)
	}
	return nil
}
