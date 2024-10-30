// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runnergroup

import (
	"fmt"
	"strings"

	"github.com/Azure/kperf/runner"
	runnergroup "github.com/Azure/kperf/runner/group"

	"github.com/urfave/cli"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var serverCommand = cli.Command{
	Name:      "server",
	ArgsUsage: "NAME",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "namespace",
			Usage: "The namespace scope for runners",
			Value: "default",
		},
		cli.StringSliceFlag{
			Name:     "runnergroup",
			Usage:    "The runner group spec's URI",
			Required: true,
		},
		cli.StringFlag{
			Name:     "runner-image",
			Usage:    "The runner's conainer image",
			Required: true,
		},
		cli.StringFlag{
			Name:  "runner-owner",
			Usage: "The runners depend on this object (FORMAT: APIVersion:Kind:Name:UID)",
		},
		cli.StringFlag{
			Name:  "runner-sa",
			Usage: "Override runner group's service account",
		},
		cli.StringSliceFlag{
			Name:     "address",
			Usage:    "Address for the server",
			Required: true,
		},
		cli.StringFlag{
			Name:     "data",
			Usage:    "The runner result should be stored in that path",
			Required: true,
		},
	},
	Hidden: true,
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as server name")
		}

		name := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(name) == 0 {
			return fmt.Errorf("required non-empty server name")
		}

		groupHandlers, err := buildRunnerGroupHandlers(cliCtx, name)
		if err != nil {
			return fmt.Errorf("failed to create runner group handlers: %w", err)
		}

		dataDir := cliCtx.String("data")
		addrs := cliCtx.StringSlice("address")

		srv, err := runner.NewServer(dataDir, addrs, groupHandlers...)
		if err != nil {
			return err
		}
		return srv.Run()
	},
}

// buildRunnerGroupHandlers creates a slice of runner group handlers.
func buildRunnerGroupHandlers(cliCtx *cli.Context, serverName string) ([]*runnergroup.Handler, error) {
	clientset, err := buildKubernetesClientset(cliCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubernetes clientset: %w", err)
	}

	specURIs := cliCtx.StringSlice("runnergroup")
	imgRef := cliCtx.String("runner-image")
	namespace := cliCtx.String("namespace")

	ownerRef := ""
	if cliCtx.IsSet("runner-owner") {
		ownerRef = cliCtx.String("runner-owner")
	}

	sa := ""
	if cliCtx.IsSet("runner-sa") {
		sa = cliCtx.String("runner-sa")
	}

	groups := make([]*runnergroup.Handler, 0, len(specURIs))
	for idx, specURI := range specURIs {
		spec, err := runnergroup.NewRunnerGroupSpecFromURI(clientset, specURI)
		if err != nil {
			return nil, err
		}

		if ownerRef != "" {
			spec.OwnerReference = &ownerRef
		}

		if sa != "" {
			spec.ServiceAccount = &sa
		}

		groupName := fmt.Sprintf("%s-%d", serverName, idx)
		g, err := runnergroup.NewHandler(clientset, namespace, groupName, spec, imgRef)
		if err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}

	return groups, nil
}

// buildKubernetesClientset builds kubernetes clientset from global flag.
func buildKubernetesClientset(cliCtx *cli.Context) (kubernetes.Interface, error) {
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")
	config, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}
