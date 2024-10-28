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
	"github.com/Azure/kperf/contrib/internal/utils"

	"github.com/urfave/cli"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
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
			klog.ErrorS(ferr, "failed to fetch apiserver cores")
		}

		report, err := handler(cliCtx)
		if err != nil {
			return nil, err
		}

		afterCores, ferr := utils.FetchAPIServerCores(ctx, kubeCfgPath)
		if ferr != nil {
			klog.ErrorS(ferr, "failed to fetch apiserver cores")
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
	klog.V(0).InfoS("Deploying virtual nodepool", "name", target)

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

	klog.V(0).InfoS("Trying to delete nodepool if necessary", "name", target)
	if err = kr.DeleteNodepool(ctx, 0, target); err != nil {
		klog.V(0).ErrorS(err, "failed to delete nodepool", "name", target)
	}

	err = kr.NewNodepool(ctx, 0, target, nodes, cpu, memory, maxPods, virtualNodeAffinity, sharedProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to create nodepool %s: %w", target, err)
	}

	return func() error {
		return kr.DeleteNodepool(ctx, 0, target)
	}, nil
}

// newLoadProfileFromEmbed loads load profile from embed and tweaks that load
// profile.
func newLoadProfileFromEmbed(cliCtx *cli.Context, name string) (_name string, _spec *types.RunnerGroupSpec, _cleanup func() error, _err error) {
	var rgSpec types.RunnerGroupSpec
	rgCfgFile, rgCfgFileDone, err := utils.NewLoadProfileFromEmbed(
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

			spec.Profile.Spec.Total = reqs
			spec.NodeAffinity = affinityLabels
			spec.Profile.Spec.ContentType = types.ContentType(cliCtx.String("content-type"))
			data, _ := yaml.Marshal(spec)
			klog.V(2).InfoS("Load Profile", "config", string(data))

			rgSpec = *spec
			return nil
		},
	)
	if err != nil {
		return "", nil, nil, err
	}
	return rgCfgFile, &rgSpec, rgCfgFileDone, nil
}
