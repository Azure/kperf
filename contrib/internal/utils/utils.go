package utils

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/contrib/internal/manifests"
	"github.com/Azure/kperf/helmcli"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	// EKSIdleNodepoolInstanceType is the instance type of idle nodepool.
	//
	// NOTE: The EKS cloud provider will delete all the NOT-READY nodes
	// which aren't managed by it. When kwok-controller fails to update
	// virtual node's lease, the EKS cloud provider would delete that
	// virtual node. It's unexpected behavior. In order to avoid this case,
	// we should create a idle nodepool with one node and use that node's
	// provider ID for all the virtual nodes so that EKS cloud provider
	// won't delete our virtual nodes.
	EKSIdleNodepoolInstanceType = "m4.large"

	// EKSRunnerNodepoolInstanceType is the instance type of nodes for kperf
	// runners.
	//
	// NOTE: This is default type. Please align it with ../manifests/loadprofile/ekswarmup.yaml.
	EKSRunnerNodepoolInstanceType = "m4.4xlarge"
)

// RepeatJobWithPod repeats to deploy 3k pods.
func RepeatJobWithPod(ctx context.Context, kubeCfgPath string, namespace string, target string, internal time.Duration) {
	klog.V(0).Info("Repeat to create job with 3k pods")

	data, err := manifests.FS.ReadFile(target)
	if err != nil {
		panic(fmt.Errorf("unexpected error when read %s from embed memory: %v",
			target, err))
	}

	jobFile, cleanup, err := CreateTempFileWithContent(data)
	if err != nil {
		panic(fmt.Errorf("unexpected error when create job yaml: %v", err))
	}
	defer func() { _ = cleanup() }()

	kr := NewKubectlRunner(kubeCfgPath, namespace)

	klog.V(0).Infof("Creating namespace %s", namespace)
	err = kr.CreateNamespace(ctx, 5*time.Minute, namespace)
	if err != nil {
		panic(fmt.Errorf("failed to create a new namespace %s: %v", namespace, err))
	}

	defer func() {
		klog.V(0).Infof("Cleanup namespace %s", namespace)
		err := kr.DeleteNamespace(context.TODO(), 60*time.Minute, namespace)
		if err != nil {
			klog.V(0).ErrorS(err, "failed to cleanup", "namespace", namespace)
		}
	}()

	retryInterval := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			klog.V(0).Info("Stop creating job")
			return
		default:
		}

		time.Sleep(retryInterval)

		aerr := kr.Apply(ctx, 5*time.Minute, jobFile)
		if aerr != nil {
			klog.V(0).ErrorS(aerr, "failed to apply, retry after 5 seconds", "job", target)
			continue
		}

		werr := kr.Wait(ctx, 15*time.Minute, "condition=complete", "15m", "job/batchjobs")
		if werr != nil {
			klog.V(0).ErrorS(werr, "failed to wait", "job", target)
		}

		derr := kr.Delete(ctx, 5*time.Minute, jobFile)
		if derr != nil {
			klog.V(0).ErrorS(derr, "failed to delete", "job", target)
		}
		time.Sleep(internal)
	}
}

// DeployAndRepeatRollingUpdateDeployments deploys and repeats to rolling-update deployments.
func DeployAndRepeatRollingUpdateDeployments(
	ctx context.Context,
	kubeCfgPath string,
	releaseName string,
	total, replica, paddingBytes int,
	internal time.Duration,
) (rollingUpdateFn, cleanupFn func(), retErr error) {

	target := "workload/deployments"
	ch, err := manifests.LoadChart(target)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load %s chart: %w", target, err)
	}

	namePattern := releaseName

	releaseCli, err := helmcli.NewReleaseCli(
		kubeCfgPath,
		// NOTE: The deployments have fixed namespace name so here
		// it's used to fill the required argument for NewReleaseCli.
		"default",
		releaseName,
		ch,
		nil,
		helmcli.StringPathValuesApplier(
			fmt.Sprintf("namePattern=%s", namePattern),
			fmt.Sprintf("total=%d", total),
			fmt.Sprintf("replica=%d", replica),
			fmt.Sprintf("paddingBytes=%d", paddingBytes),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create a new helm release cli: %w", err)
	}

	klog.V(0).InfoS("Deploying deployments",
		"total", total,
		"replica", replica,
		"paddingBytes", paddingBytes,
	)
	err = releaseCli.Deploy(ctx, 10*time.Minute)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			klog.V(0).Info("Deploy is canceled")
			return func() {}, func() {}, nil
		}
		return nil, nil, fmt.Errorf("failed to deploy helm chart %s: %w", target, err)
	}
	klog.V(0).InfoS("Deployed deployments")

	cleanupFn = func() {
		klog.V(0).Infof("Cleanup helm chart %s", target)
		err := releaseCli.Uninstall()
		if err != nil {
			klog.V(0).ErrorS(err, "failed to cleanup", "chart", target)
		}
	}

	rollingUpdateFn = func() {
		for {
			select {
			case <-ctx.Done():
				klog.V(0).Info("Stop rolling-updating")
				return
			case <-time.After(internal):
			}

			klog.V(0).Info("Start to rolling-update deployments")
			for i := 0; i < total; i++ {
				name := fmt.Sprintf("%s-%d", namePattern, i)
				ns := name

				klog.V(0).InfoS("Rolling-update", "deployment", name, "namespace", ns)
				err := func() error {
					kr := NewKubectlRunner(kubeCfgPath, ns)

					err := kr.DeploymentRestart(ctx, 2*time.Minute, name)
					if err != nil {
						return fmt.Errorf("failed to restart deployment %s: %w", name, err)
					}

					err = kr.DeploymentRolloutStatus(ctx, 10*time.Minute, name)
					if err != nil {
						return fmt.Errorf("failed to watch the rollout status of deployment %s: %w", name, err)
					}
					return nil
				}()
				if err != nil {
					klog.V(0).ErrorS(err, "failed to rolling-update",
						"deployment", name, "namespace", ns)
				}
			}
		}
	}
	return rollingUpdateFn, cleanupFn, nil
}

// NewLoadProfileFromEmbed reads load profile from embed memory.
func NewLoadProfileFromEmbed(target string, tweakFn func(*types.RunnerGroupSpec) error) (_name string, _cleanup func() error, _ error) {
	data, err := manifests.FS.ReadFile(target)
	if err != nil {
		return "", nil, fmt.Errorf("unexpected error when read %s from embed memory: %v", target, err)
	}

	if tweakFn != nil {
		var spec types.RunnerGroupSpec
		if err = yaml.Unmarshal(data, &spec); err != nil {
			return "", nil, fmt.Errorf("failed to unmarshal into runner group spec:\n (data: %s)\n: %w",
				string(data), err)
		}

		if err = tweakFn(&spec); err != nil {
			return "", nil, err
		}

		data, err = yaml.Marshal(spec)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal runner group spec after tweak: %w", err)
		}
	}

	return CreateTempFileWithContent(data)
}

// DeployRunnerGroup deploys runner group for benchmark.
func DeployRunnerGroup(ctx context.Context,
	kubeCfgPath, runnerImage, rgCfgFile string,
	runnerFlowControl, runnerGroupAffinity string) (*types.RunnerGroupsReport, error) {

	klog.InfoS("Deploying runner group", "config", rgCfgFile)

	kr := NewKperfRunner(kubeCfgPath, runnerImage)

	klog.Info("Deleting existing runner group")
	derr := kr.RGDelete(ctx, 0)
	if derr != nil {
		klog.ErrorS(derr, "failed to delete existing runner group")
	}

	rerr := kr.RGRun(ctx, 0, rgCfgFile, runnerFlowControl, runnerGroupAffinity)
	if rerr != nil {
		return nil, fmt.Errorf("failed to deploy runner group: %w", rerr)
	}

	klog.Info("Waiting runner group")
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// NOTE: The result subcommand will hold the long connection
		// until runner-group's server replies. However, there is no
		// data transport before runners finish. If the apiserver
		// has been restarted, the proxy tunnel will be broken and
		// the client won't be notified. So, the client will hang forever.
		// Using 1 min as timeout is to ensure we can get result in time.
		data, err := kr.RGResult(ctx, 1*time.Minute)
		if err != nil {
			klog.ErrorS(err, "failed to fetch runner group's result")
			continue
		}
		klog.InfoS("Runner group's result", "data", data)

		var rgResult types.RunnerGroupsReport
		if err = json.Unmarshal([]byte(data), &rgResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal into RunnerGroupsReport: %w", err)
		}

		klog.Info("Deleting runner group")
		if derr := kr.RGDelete(ctx, 0); derr != nil {
			klog.ErrorS(err, "failed to delete runner group")
		}
		return &rgResult, nil
	}
}

// FetchAPIServerCores fetchs core number for each kube-apiserver.
func FetchAPIServerCores(ctx context.Context, kubeCfgPath string) (map[string]int, error) {
	klog.V(0).Info("Fetching apiserver's cores")

	kr := NewKubectlRunner(kubeCfgPath, "")
	fqdn, err := kr.FQDN(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster fqdn: %w", err)
	}

	ips, nerr := NSLookup(fqdn)
	if nerr != nil {
		return nil, fmt.Errorf("failed get dns records of fqdn %s: %w", fqdn, nerr)
	}

	res := map[string]int{}
	for _, ip := range ips {
		cores, err := func() (int, error) {
			data, err := kr.Metrics(ctx, 0, fqdn, ip)
			if err != nil {
				return 0, fmt.Errorf("failed to get metrics for ip %s: %w", ip, err)
			}

			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "go_sched_gomaxprocs_threads") {
					vInStr := strings.Fields(line)[1]
					v, err := strconv.Atoi(vInStr)
					if err != nil {
						return 0, fmt.Errorf("failed to parse go_sched_gomaxprocs_threads %s: %w", line, err)
					}
					return v, nil
				}
			}
			return 0, fmt.Errorf("failed to get go_sched_gomaxprocs_threads")
		}()
		if err != nil {
			klog.V(0).ErrorS(err, "failed to get cores", "ip", ip)
			continue
		}
		klog.V(0).InfoS("apiserver cores", ip, cores)
		res[ip] = cores
	}
	return res, nil
}

// FetchNodeProviderIDByType is used to get one node's provider id with a given
// instance type.
func FetchNodeProviderIDByType(ctx context.Context, kubeCfgPath string, instanceType string) (string, error) {
	clientset, err := BuildClientset(kubeCfgPath)
	if err != nil {
		return "", err
	}

	label := fmt.Sprintf("node.kubernetes.io/instance-type=%v", instanceType)

	nodeCli := clientset.CoreV1().Nodes()
	listResp, err := nodeCli.List(ctx, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return "", fmt.Errorf("failed to list nodes with label %s: %w", label, err)
	}

	if len(listResp.Items) == 0 {
		return "", fmt.Errorf("there is no such node with label %s", label)
	}
	return listResp.Items[0].Spec.ProviderID, nil
}

// BuildClientset returns kubernetes clientset.
func BuildClientset(kubeCfgPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build client-go config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to build client-go rest client: %w", err)
	}
	return clientset, nil
}

// NSLookup returns ips for URL.
func NSLookup(domainURL string) ([]string, error) {
	ips, err := net.LookupHost(domainURL)
	if err != nil {
		return nil, err
	}
	sort.Strings(ips)
	return ips, nil
}

// runCommand runs command with Pdeathsig.
func runCommand(ctx context.Context, timeout time.Duration, cmd string, args []string) ([]byte, error) {
	var cancel context.CancelFunc
	if timeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}

	klog.V(2).Infof("[CMD] %s", c.String())
	output, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to invoke %s:\n (output: %s): %w",
			c.String(), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}

// CreateTempFileWithContent creates temporary file with data.
func CreateTempFileWithContent(data []byte) (_name string, _cleanup func() error, retErr error) {
	f, err := os.CreateTemp("", "temp*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	fName := f.Name()
	defer func() {
		if retErr != nil {
			_ = os.RemoveAll(fName)
		}
	}()

	_, err = f.Write(data)
	f.Close()
	if err != nil {
		return "", nil, fmt.Errorf("failed to write temporary in %s: %w",
			fName, err)
	}

	return fName, func() error {
		return os.RemoveAll(fName)
	}, nil
}
