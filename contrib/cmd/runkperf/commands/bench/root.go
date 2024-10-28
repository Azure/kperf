package bench

import (
	kperfcmdutils "github.com/Azure/kperf/cmd/kperf/commands/utils"

	"github.com/urfave/cli"
)

// Command represents bench subcommand.
var Command = cli.Command{
	Name:  "bench",
	Usage: "Run benchmark test cases",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: kperfcmdutils.DefaultKubeConfigPath,
		},
		cli.StringFlag{
			Name:  "runner-image",
			Usage: "The runner's conainer image",
			// TODO(weifu):
			//
			// We should build release pipeline so that we can
			// build with fixed public release image as default value.
			// Right now, we need to set image manually.
			Required: true,
		},
		cli.StringFlag{
			Name:  "runner-flowcontrol",
			Usage: "Apply flowcontrol to runner group. (FORMAT: PriorityLevel:MatchingPrecedence)",
			Value: "workload-low:1000",
		},
		cli.StringFlag{
			Name:  "vc-affinity",
			Usage: "Deploy virtualnode's controller with a specific labels (FORMAT: KEY=VALUE[,VALUE])",
			Value: "node.kubernetes.io/instance-type=Standard_D8s_v3,m4.2xlarge",
		},
		cli.StringFlag{
			Name:  "rg-affinity",
			Usage: "Deploy runner group with a specific labels (FORMAT: KEY=VALUE[,VALUE])",
			Value: "node.kubernetes.io/instance-type=Standard_D16s_v3,m4.4xlarge",
		},
		cli.BoolFlag{
			Name:   "eks",
			Usage:  "Indicates the target kubernetes cluster is EKS",
			Hidden: true,
		},
		cli.StringFlag{
			Name:  "result",
			Usage: "Path to the file which stores results",
		},
	},
	Subcommands: []cli.Command{
		benchNode10Job1Pod100Case,
		benchNode100Job1Pod3KCase,
		benchNode100DeploymentNPod10KCase,
	},
}

// commonFlags is used as subcommand's option instead of global options.
//
// NOTE: The format of global options, like `--option xyz subcommand`, is not
// easy to extend existing configuration. If the subcommand extends it with
// its own option, the user can just append options, like `subcommand --options
// xyz.
var commonFlags = []cli.Flag{
	cli.IntFlag{
		Name:  "cpu",
		Usage: "the allocatable cpu resource per node",
		Value: 32,
	},
	cli.IntFlag{
		Name:  "memory",
		Usage: "The allocatable Memory resource per node (GiB)",
		Value: 96,
	},
	cli.IntFlag{
		Name:  "max-pods",
		Usage: "The maximum Pods per node",
		Value: 110,
	},
	cli.StringFlag{
		Name:  "content-type",
		Usage: "Content type (json or protobuf)",
		Value: "json",
	},
}
