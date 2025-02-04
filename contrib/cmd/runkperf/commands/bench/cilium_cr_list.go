// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package bench

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/log"
	"github.com/Azure/kperf/contrib/utils"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/urfave/cli"
)

const (
	numCRApplyWorkers      = 50
	maxNumCRApplyAttempts  = 5
	kubectlApplyTimeout    = 30 * time.Second
	progressReportInterval = 10 * time.Second
	installCiliumCRDsFlag  = "install-cilium-crds"
	numCEPFlag             = "num-cilium-endpoints"
	numCIDFlag             = "num-cilium-identities"
)

var benchCiliumCustomResourceListCase = cli.Command{
	Name: "cilium_cr_list",
	Usage: `

	Simulate workload with stale list requests for Cilium custom resources.

	This benchmark MUST be run in a cluster *without* Cilium installed, so Cilium doesn't
	delete or modify the synthetic CiliumEndpoint and CiliumIdentity resources created in this test.
	`,
	Flags: append(
		[]cli.Flag{
			cli.BoolTFlag{
				Name:  installCiliumCRDsFlag,
				Usage: "Install Cilium CRDs if they don't already exist (default: true)",
			},
			cli.IntFlag{
				Name:  numCIDFlag,
				Usage: "Number of CiliumIdentities to generate (default: 1000)",
				Value: 1000,
			},
			cli.IntFlag{
				Name:  numCEPFlag,
				Usage: "Number of CiliumEndpoints to generate (default: 1000)",
				Value: 1000,
			},
		},
		commonFlags...,
	),
	Action: func(cliCtx *cli.Context) error {
		_, err := renderBenchmarkReportInterceptor(ciliumCustomResourceListRun)(cliCtx)
		return err
	},
}

// ciliumCustomResourceListRun runs a benchmark that:
// (1) creates many Cilium custom resources (CiliumIdentity and CiliumEndpoint).
// (2) executes stale list requests against those resources.
// This simulates a "worst case" scenario in which Cilium performs many expensive list requests.
func ciliumCustomResourceListRun(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
	ctx := context.Background()

	rgCfgFile, rgSpec, rgCfgFileDone, err := newLoadProfileFromEmbed(cliCtx, "loadprofile/cilium_cr_list.yaml")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rgCfgFileDone() }()

	kubeCfgPath := cliCtx.GlobalString("kubeconfig")
	kr := utils.NewKubectlRunner(kubeCfgPath, "")

	if cliCtx.BoolT(installCiliumCRDsFlag) {
		if err := installCiliumCRDs(ctx, kr); err != nil {
			return nil, fmt.Errorf("failed to install Cilium CRDs: %w", err)
		}
	}

	numCID := cliCtx.Int(numCIDFlag)
	numCEP := cliCtx.Int(numCEPFlag)
	if err := loadCiliumData(ctx, kr, numCID, numCEP); err != nil {
		return nil, fmt.Errorf("failed to load Cilium data: %w", err)
	}

	rgResult, err := utils.DeployRunnerGroup(ctx,
		cliCtx.GlobalString("kubeconfig"),
		cliCtx.GlobalString("runner-image"),
		rgCfgFile,
		cliCtx.GlobalString("runner-flowcontrol"),
		cliCtx.GlobalString("rg-affinity"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to deploy runner group: %w", err)
	}

	return &internaltypes.BenchmarkReport{
		Description: fmt.Sprintf(`Deploy %d CiliumIdentities and %d CiliumEndpoints, then run stale list requests against them`, numCID, numCEP),
		LoadSpec:    *rgSpec,
		Result:      *rgResult,
		Info: map[string]interface{}{
			"numCiliumIdentities": numCID,
			"numCiliumEndpoints":  numCEP,
		},
	}, nil
}

var ciliumCRDs = []string{
	"https://raw.githubusercontent.com/cilium/cilium/refs/tags/v1.16.6/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumendpoints.yaml",
	"https://raw.githubusercontent.com/cilium/cilium/refs/tags/v1.16.6/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumidentities.yaml",
}

func installCiliumCRDs(ctx context.Context, kr *utils.KubectlRunner) error {
	log.GetLogger(ctx).WithKeyValues("level", "info").LogKV("msg", "Installing Cilium CRDs...")
	for _, crdURL := range ciliumCRDs {
		err := kr.Apply(ctx, kubectlApplyTimeout, crdURL)
		if err != nil {
			return fmt.Errorf("failed to apply CRD %s: %v", crdURL, err)
		}
	}
	return nil
}

func loadCiliumData(ctx context.Context, kr *utils.KubectlRunner, numCID int, numCEP int) error {
	totalNumResources := numCID + numCEP
	log.GetLogger(ctx).WithKeyValues("level", "info").LogKV("msg", "Loading Cilium data",
		"CiliumIdentities", numCID, "CiliumEndpoints", numCEP)

	// Parallelize kubectl apply to speed it up. Achieves ~80 inserts/sec.
	taskChan := make(chan string, numCRApplyWorkers*2)
	var appliedCount atomic.Uint64
	g, ctx := errgroup.WithContext(ctx)
	for i := 0; i < numCRApplyWorkers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case ciliumResourceData, ok := <-taskChan:
					if !ok {
						return nil // taskChan closed
					}
					var err error
					for i := 0; i < maxNumCRApplyAttempts; i++ {
						err = kr.ServerSideApplyWithData(ctx, kubectlApplyTimeout, ciliumResourceData)
						if err == nil {
							appliedCount.Add(1)
							break
						} else if i < maxNumCRApplyAttempts-1 {
							log.GetLogger(ctx).WithKeyValues("level", "warn").LogKV("msg", "failed to apply cilium resource data, will retry", "error", err)
						}
					}
					if err != nil { // last retry failed, so give up.
						return fmt.Errorf("failed to apply cilium resource data: %w", err)
					}
				}
			}
		})
	}

	// Report progress periodically.
	reporterDoneChan := make(chan struct{})
	g.Go(func() error {
		timer := time.NewTicker(progressReportInterval)
		defer timer.Stop()
		for {
			select {
			case <-reporterDoneChan:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				c := appliedCount.Load()
				percent := int(float64(c) / float64(totalNumResources) * 100)
				log.GetLogger(ctx).WithKeyValues("level", "info").LogKV("msg", fmt.Sprintf("applied %d/%d cilium resources (%d%%)", c, totalNumResources, percent))
			}
		}
	})

	// Generate CiliumIdentity and CiliumEndpoint CRs to be applied by the worker goroutines.
	g.Go(func() error {
		for i := 0; i < numCID; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case taskChan <- generateCiliumIdentity():
			}
		}

		for i := 0; i < numCEP; i++ {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case taskChan <- generateCiliumEndpoint():
			}
		}

		close(taskChan) // signal to consumer goroutines that we're done.
		close(reporterDoneChan)
		return nil
	})

	if err := g.Wait(); err != nil {
		return err
	}

	log.GetLogger(ctx).WithKeyValues("level", "info").LogKV("msg", fmt.Sprintf("loaded %d CiliumIdentities and %d CiliumEndpoints\n", numCID, numCEP))

	return nil
}

func generateCiliumIdentity() string {
	identityName := uuid.New().String()
	return fmt.Sprintf(`
apiVersion: cilium.io/v2
kind: CiliumIdentity
metadata:
  name: "%s"
security-labels:
  k8s:io.cilium.k8s.namespace.labels.control-plane: "true"
  k8s:io.cilium.k8s.namespace.labels.kubernetes.azure.com/managedby: aks
  k8s:io.cilium.k8s.namespace.labels.kubernetes.io/cluster-service: "true"
  k8s:io.cilium.k8s.namespace.labels.kubernetes.io/metadata.name: kube-system
  k8s:io.cilium.k8s.policy.cluster: default
  k8s:io.cilium.k8s.policy.serviceaccount: coredns
  k8s:io.kubernetes.pod.namespace: kube-system
  k8s:k8s-app: kube-dns
  k8s:kubernetes.azure.com/managedby: aks
  k8s:version: v20`, identityName)
}

func generateCiliumEndpoint() string {
	cepName := uuid.New().String()
	return fmt.Sprintf(`
apiVersion: cilium.io/v2
kind: CiliumEndpoint
metadata:
  name: "%s"
status:
  encryption: {}
  external-identifiers:
    container-id: 790d85075c394a8384f8b1a0fec62e2316c2556d175dab0c1fe676e5a6d92f33
    k8s-namespace: kube-system
    k8s-pod-name: coredns-54b69f46b8-dbcdl
    pod-name: kube-system/coredns-54b69f46b8-dbcdl
  id: 1453
  identity:
    id: 0000001
    labels:
    - k8s:io.cilium.k8s.namespace.labels.control-plane=true
    - k8s:io.cilium.k8s.namespace.labels.kubernetes.azure.com/managedby=aks
    - k8s:io.cilium.k8s.namespace.labels.kubernetes.io/cluster-service=true
    - k8s:io.cilium.k8s.namespace.labels.kubernetes.io/metadata.name=kube-system
    - k8s:io.cilium.k8s.policy.cluster=default
    - k8s:io.cilium.k8s.policy.serviceaccount=coredns
    - k8s:io.kubernetes.pod.namespace=kube-system
    - k8s:k8s-app=kube-dns
    - k8s:kubernetes.azure.com/managedby=aks
    - k8s:version=v20
  named-ports:
  - name: dns
    port: 53
    protocol: UDP
  - name: dns-tcp
    port: 53
    protocol: TCP
  - name: metrics
    port: 9153
    protocol: TCP
  networking:
    addressing:
    - ipv4: 10.244.1.38
    node: 10.224.0.4
  policy:
    egress:
      enforcing: false
      state: <status disabled>
    ingress:
      enforcing: false
      state: <status disabled>
  state: ready
  visibility-policy-status: <status disabled>
`, cepName)
}
