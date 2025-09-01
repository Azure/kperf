// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package bench

import (
	"context"
	"fmt"
	"sync"
	"time"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"

	"github.com/urfave/cli"
)

var appLabel = "runkperf"

var benchReadUpdateCase = cli.Command{
	Name: "read_update",
	Usage: `
The test suite sets up a benchmark that simulates a mix of read, watch, and update operations on ConfigMaps. 
It creates ConfigMaps, establishes watch connections, and then issues concurrent read and update requests based on a specified ratio to evaluate API server performance under combined load.
`,
	Flags: append(
		[]cli.Flag{
			cli.IntFlag{
				Name:  "total",
				Usage: "Total requests per runner (There are 10 runners totally and runner's rate is 10)",
				Value: 3600,
			},
			cli.StringFlag{
				Name:  "read-update-namespace",
				Usage: "Kubernetes namespace to use. If not specified, it will use the default namespace.",
				Value: "default",
			},
			cli.IntFlag{
				Name:  "read-update-configmap-total",
				Usage: "Total ConfigMaps need to create",
				Value: 100,
			},
			cli.IntFlag{
				Name:  "read-update-configmap-size",
				Usage: "Size of each ConfigMap, unit: KiB. ConfigMap must not exceed 1 MiB.",
				Value: 1024, // 1 KiB
			},
			cli.StringFlag{
				Name:  "read-update-name-pattern",
				Usage: "Name pattern for the resources to create",
				Value: "kperf-read-update",
			},
			cli.Float64Flag{
				Name:  "read-ratio",
				Usage: "Proportion of read requests among all requests (range: 0.0 to 1.0). For example, 0.5 indicates 50% of the requests are reads.",
				Value: 0.5,
			},
		},
		commonFlags...,
	),
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(
			addAPIServerCoresInfoInterceptor(benchReadUpdateRun),
		)(cliCtx)
		return err
	},
}

// benchReadUpdateRun is for subcommand benchReadUpdateRun.
func benchReadUpdateRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()
	kubeCfgPath := cliCtx.GlobalString("kubeconfig")

	// Load the load profile
	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx,
		"loadprofile/read_update.yaml")

	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	total := cliCtx.Int("read-update-configmap-total")
	size := cliCtx.Int("read-update-configmap-size")
	namespace := cliCtx.String("read-update-namespace")
	namePattern := cliCtx.String("read-update-name-pattern")
	if total <= 0 || size <= 0 || total*size > 2*1024*1024*1024 || size > 1024*1024 {
		return nil, fmt.Errorf("invalid total (%d) or size (%d) for configmaps: total must be > 0, size must be > 0, and total*size must not exceed 2 GiB, size must not exceed 1 MiB", total, size)
	}

	// Create configmaps with specified name pattern
	client, err := utils.BuildClientset(kubeCfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build clientset: %w", err)
	}

	err = utils.CreateConfigmaps(ctx, kubeCfgPath, namespace, namePattern, total, size, 30, 0)

	if err != nil {
		return nil, fmt.Errorf("failed to create configmaps: %w", err)
	}

	defer func() {
		// Delete the configmaps after the benchmark
		err = utils.DeleteConfigmaps(ctx, kubeCfgPath, namespace, namePattern, 0)
		if err != nil {
			klog.Errorf("Failed to delete configmaps: %v", err)
		}
	}()

	// Stop all the watches when the function returns
	var wg sync.WaitGroup
	defer wg.Wait()

	dpCtx, dpCancel := context.WithCancel(ctx)
	defer dpCancel()

	// Start to watch the configmaps
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(ii int) {
			defer wg.Done()
			timeoutSeconds := int64(100000)

			var watchReq watch.Interface = nil
			var err error
			defer func() {
				if watchReq != nil {
					watchReq.Stop()
				}
			}()

			for {
				if watchReq == nil {
					watchReq, err = client.CoreV1().ConfigMaps(namespace).
						Watch(context.TODO(), metav1.ListOptions{
							Watch:          true,
							TimeoutSeconds: &timeoutSeconds,
							FieldSelector:  fmt.Sprintf("metadata.name=%s-cm-%s-%d", appLabel, namePattern, ii),
						})

					if err != nil {
						fmt.Printf("Error starting watch for configmap %s: %v\n", fmt.Sprintf("%s-cm-%s-%d", appLabel, namePattern, ii), err)
						time.Sleep(5 * time.Second)
						continue
					}
					klog.V(5).Infof("Starting watch for configmap: %s\n", fmt.Sprintf("%s-cm-%s-%d", appLabel, namePattern, ii))
				}

				// Process watch events proactively to prevent cache oversizing.
				select {
				case <-dpCtx.Done():
					klog.V(5).Infof("Stopping watch for configmap: %s\n", fmt.Sprintf("%s-cm-%s-%d", appLabel, namePattern, ii))
					return
				case event, ok := <-watchReq.ResultChan():
					if !ok {
						klog.V(2).Infof("Watch channel closed for configmap: %s", fmt.Sprintf("%s-cm-%s-%d", appLabel, namePattern, ii))
						watchReq.Stop()
						watchReq = nil
					}
					if event.Type == watch.Error {
						klog.Errorf("Error event received for configmap %s: %v", fmt.Sprintf("%s-cm-%s-%d", appLabel, namePattern, ii), event.Object)
						watchReq.Stop()
						watchReq = nil
					}
					klog.V(5).Infof("Event received for configmap %s: %v", fmt.Sprintf("%s-cm-%s-%d", appLabel, namePattern, ii), event.Type)
				case <-time.After(5 * time.Second):
				}
			}

		}(i)
	}

	// Deploy the runner group
	rgResult, derr := utils.DeployRunnerGroup(ctx,
		cliCtx.GlobalString("kubeconfig"),
		cliCtx.GlobalString("runner-image"),
		rgCfgFile,
		cliCtx.GlobalString("runner-flowcontrol"),
		cliCtx.GlobalString("rg-affinity"),
	)

	if derr != nil {
		return nil, fmt.Errorf("failed to deploy runner group: %w", derr)
	}

	return &internaltypes.BenchmarkReport{
		Description: fmt.Sprintf(`
Environment: Combine %d%% read requests and %d%% update requests during benchmarking. Workload: Deploy %d configmaps in %d KiB`,
			int(100*cliCtx.Float64("read-ratio")), 100-int(100*cliCtx.Float64("read-ratio")), total, size*total),
		LoadSpec: *rgSpec,
		Result:   *rgResult,
		Info:     map[string]interface{}{},
	}, nil
}
