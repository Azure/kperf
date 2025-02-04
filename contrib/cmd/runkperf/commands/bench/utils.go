// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Azure/kperf/api/types"
	kperfcmdutils "github.com/Azure/kperf/cmd/kperf/commands/utils"
	internaltypes "github.com/Azure/kperf/contrib/internal/types"
	"github.com/Azure/kperf/contrib/log"
	"github.com/Azure/kperf/contrib/utils"

	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
)

// subcmdActionFunc is to unify each subcommand's interface. They should return
// benchmark report as result.
type subcmdActionFunc func(*cli.Context) (*internaltypes.BenchmarkReport, error)

// addAPIServerCoresInfoInterceptor adds apiserver's cores into benchmark report.
func addAPIServerCoresInfoInterceptor(handler subcmdActionFunc) subcmdActionFunc {
	return func(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
		ctx := context.Background()
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")

		beforeCores, ferr := utils.FetchAPIServerCores(ctx, kubeCfgPath)
		if ferr != nil {
			log.GetLogger(ctx).
				WithKeyValues("level", "warn").
				LogKV("msg", "failed to fetch apiserver cores", "error", ferr)
		}

		report, err := handler(cliCtx)
		if err != nil {
			return nil, err
		}

		afterCores, ferr := utils.FetchAPIServerCores(ctx, kubeCfgPath)
		if ferr != nil {
			log.GetLogger(ctx).
				WithKeyValues("level", "warn").
				LogKV("msg", "failed to fetch apiserver cores", "error", ferr)
		}

		report.Info["apiserver"] = map[string]interface{}{
			"cores": map[string]interface{}{
				"before": beforeCores,
				"after":  afterCores,
			},
		}
		return report, nil
	}
}

// renderBenchmarkReportInterceptor renders benchmark report into file or stdout.
func renderBenchmarkReportInterceptor(handler subcmdActionFunc) subcmdActionFunc {
	return func(cliCtx *cli.Context) (*internaltypes.BenchmarkReport, error) {
		report, err := handler(cliCtx)
		if err != nil {
			return nil, err
		}

		outF := os.Stdout
		if targetFile := cliCtx.GlobalString("result"); targetFile != "" {
			targetFileDir := filepath.Dir(targetFile)

			_, err = os.Stat(targetFileDir)
			if err != nil && os.IsNotExist(err) {
				err = os.MkdirAll(targetFileDir, 0750)
			}
			if err != nil {
				return nil, fmt.Errorf("failed to ensure output's dir %s: %w", targetFileDir, err)
			}

			outF, err = os.Create(targetFile)
			if err != nil {
				return nil, err
			}
			defer outF.Close()
		}

		encoder := json.NewEncoder(outF)
		encoder.SetIndent("", "  ")

		if err := encoder.Encode(report); err != nil {
			return nil, fmt.Errorf("failed to encode json: %w", err)
		}
		return report, nil
	}
}

// deployVirtualNodepool deploys virtual nodepool.
func deployVirtualNodepool(ctx context.Context, cliCtx *cli.Context, target string, nodes, cpu, memory, maxPods int) (func() error, error) {
	log.GetLogger(ctx).
		WithKeyValues("level", "info").
		LogKV("msg", "deploying virtual nodepool", "name", target)

	kubeCfgPath := cliCtx.GlobalString("kubeconfig")
	virtualNodeAffinity := cliCtx.GlobalString("vc-affinity")

	kr := utils.NewKperfRunner(kubeCfgPath, "")

	var sharedProviderID string
	var err error

	if cliCtx.GlobalBool("eks") {
		sharedProviderID, err = utils.FetchNodeProviderIDByType(ctx, kubeCfgPath, utils.EKSIdleNodepoolInstanceType)
		if err != nil {
			return nil, fmt.Errorf("failed to get EKS idle node (type: %s) providerID: %w",
				utils.EKSIdleNodepoolInstanceType, err)
		}
	}

	log.GetLogger(ctx).
		WithKeyValues("level", "info").
		LogKV("msg", "trying to delete nodepool if necessary", "name", target)
	if err = kr.DeleteNodepool(ctx, 0, target); err != nil {
		log.GetLogger(ctx).
			WithKeyValues("level", "warn").
			LogKV("msg", "failed to delete nodepool", "name", target, "error", err)
	}

	err = kr.NewNodepool(ctx, 0, target, nodes, cpu, memory, maxPods, virtualNodeAffinity, sharedProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to create nodepool %s: %w", target, err)
	}

	return func() error {
		return kr.DeleteNodepool(ctx, 0, target)
	}, nil
}

func NewRunnerGroupSpecFromYamlFile() {}

// newLoadProfileFromEmbed loads load profile from embed and tweaks that load
// profile.
func newLoadProfileFromEmbed(cliCtx *cli.Context, name string) (_name string, _spec *types.RunnerGroupSpec, _cleanup func() error, _err error) {
	var rgSpec types.RunnerGroupSpec
	rgCfgFile, rgCfgFileDone, err := utils.NewRunnerGroupSpecFileFromEmbed(
		name,
		func(spec *types.RunnerGroupSpec) error {
			reqs := cliCtx.Int("total")
			if reqs < 0 {
				return fmt.Errorf("invalid total-requests value: %v", reqs)
			}

			rgAffinity := cliCtx.GlobalString("rg-affinity")
			affinityLabels, err := kperfcmdutils.KeyValuesMap([]string{rgAffinity})
			if err != nil {
				return fmt.Errorf("failed to parse %s affinity: %w", rgAffinity, err)
			}

			if reqs != 0 {
				spec.Profile.Spec.Total = reqs
			}
			spec.NodeAffinity = affinityLabels
			spec.Profile.Spec.ContentType = types.ContentType(cliCtx.String("content-type"))
			data, _ := yaml.Marshal(spec)

			log.GetLogger(context.TODO()).
				WithKeyValues("level", "info").
				LogKV("msg", "dump load profile", "config", string(data))

			rgSpec = *spec
			return nil
		},
	)
	if err != nil {
		return "", nil, nil, err
	}
	return rgCfgFile, &rgSpec, rgCfgFileDone, nil
}
