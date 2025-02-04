// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

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
	"github.com/Azure/kperf/contrib/log"
	"github.com/Azure/kperf/helmcli"

	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
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
)

// RepeatJobWithPod repeats to deploy 3k pods.
func RepeatJobWithPod(ctx context.Context, kubeCfgPath string, namespace string, target string, internal time.Duration) {
	infoLogger := log.GetLogger(ctx).WithKeyValues("level", "info")
	warnLogger := log.GetLogger(ctx).WithKeyValues("level", "warn")

	infoLogger.LogKV("msg", "repeat to create job with 3k pods")

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

	infoLogger.LogKV("msg", "creating namespace", "name", namespace)
	err = kr.CreateNamespace(ctx, 5*time.Minute, namespace)
	if err != nil {
		panic(fmt.Errorf("failed to create a new namespace %s: %v", namespace, err))
	}

	defer func() {
		infoLogger.LogKV("msg", "cleanup namespace", "name", namespace)
		err := kr.DeleteNamespace(context.TODO(), 60*time.Minute, namespace)
		if err != nil {
			warnLogger.LogKV("msg", "failed to cleanup namespace", "name", namespace, "error", err)
		}
	}()

	retryInterval := 5 * time.Second
	for {
		select {
		case <-ctx.Done():
			infoLogger.LogKV("msg", "stop creating job")
			return
		default:
		}

		time.Sleep(retryInterval)

		aerr := kr.Apply(ctx, 5*time.Minute, jobFile)
		if aerr != nil {
			warnLogger.LogKV("msg", "failed to apply job, retry after 5 seconds", "job", target, "error", aerr)
			continue
		}

		werr := kr.Wait(ctx, 15*time.Minute, "condition=complete", "15m", "job/batchjobs")
		if werr != nil {
			warnLogger.LogKV("msg", "failed to wait job finish", "job", target, "error", werr)
		}

		derr := kr.Delete(ctx, 5*time.Minute, jobFile)
		if derr != nil {
			warnLogger.LogKV("msg", "failed to delete job", "job", target, "error", derr)
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
	infoLogger := log.GetLogger(ctx).WithKeyValues("level", "info")
	warnLogger := log.GetLogger(ctx).WithKeyValues("level", "warn")

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

	infoLogger.LogKV(
		"msg", "deploying deployments",
		"total", total,
		"replica", replica,
		"paddingBytes", paddingBytes,
	)

	err = releaseCli.Deploy(ctx, 10*time.Minute)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			infoLogger.LogKV("msg", "deploy is canceled")
			return func() {}, func() {}, nil
		}
		return nil, nil, fmt.Errorf("failed to deploy helm chart %s: %w", target, err)
	}
	infoLogger.LogKV("msg", "deployed deployments")

	cleanupFn = func() {
		infoLogger.LogKV("msg", "cleanup helm chart", "target", target)
		err := releaseCli.Uninstall()
		if err != nil {
			warnLogger.LogKV("msg", "failed to cleanup helm chart",
				"target", target,
				"error", err)
		}
	}

	rollingUpdateFn = func() {
		for {
			select {
			case <-ctx.Done():
				infoLogger.LogKV("msg", "stop rolling-updating")
				return
			case <-time.After(internal):
			}

			infoLogger.LogKV("msg", "start to rolling-update deployments")
			for i := 0; i < total; i++ {
				name := fmt.Sprintf("%s-%d", namePattern, i)
				ns := name

				infoLogger.LogKV("msg", "rolling-update deployment", "name", name, "namespace", ns)
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
					warnLogger.LogKV("msg", "failed to rolling-update",
						"error", err,
						"deployment", name,
						"namespace", ns)
				}
			}
		}
	}
	return rollingUpdateFn, cleanupFn, nil
}

// NewRunnerGroupSpecFromYAML returns RunnerGroupSpec instance from yaml data.
func NewRunnerGroupSpecFromYAML(data []byte, tweakFn func(*types.RunnerGroupSpec) error) (*types.RunnerGroupSpec, error) {
	var spec types.RunnerGroupSpec

	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into RunnerGroupSpec:\n (data: %s)\n: %w",
			string(data), err)
	}

	if tweakFn != nil {
		if err := tweakFn(&spec); err != nil {
			return nil, fmt.Errorf("failed to tweak RunnerGroupSpec: %w", err)
		}
	}
	return &spec, nil
}

// NewRunnerGroupSpecFileFromEmbed reads load profile (RunnerGroupSpec) from
// embed memory and marshals it into temporary file. Use it when invoking
// kperf binary instead of package.
func NewRunnerGroupSpecFileFromEmbed(target string, tweakFn func(*types.RunnerGroupSpec) error) (_name string, _cleanup func() error, _ error) {
	data, err := manifests.FS.ReadFile(target)
	if err != nil {
		return "", nil, fmt.Errorf("unexpected error when read %s from embed memory: %v", target, err)
	}

	if tweakFn != nil {
		var spec types.RunnerGroupSpec
		if err = yaml.Unmarshal(data, &spec); err != nil {
			return "", nil, fmt.Errorf("failed to unmarshal into RunnerGroupSpec:\n (data: %s)\n: %w",
				string(data), err)
		}

		if err = tweakFn(&spec); err != nil {
			return "", nil, err
		}

		data, err = yaml.Marshal(spec)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal RunnerGroupSpec after tweak: %w", err)
		}
	}
	return CreateTempFileWithContent(data)
}

// DeployRunnerGroup deploys runner group for benchmark.
func DeployRunnerGroup(ctx context.Context,
	kubeCfgPath, runnerImage, rgCfgFile string,
	runnerFlowControl, runnerGroupAffinity string) (*types.RunnerGroupsReport, error) {

	infoLogger := log.GetLogger(ctx).WithKeyValues("level", "info")
	warnLogger := log.GetLogger(ctx).WithKeyValues("level", "warn")

	kr := NewKperfRunner(kubeCfgPath, runnerImage)

	infoLogger.LogKV("msg", "deleting existing runner group")
	derr := kr.RGDelete(ctx, 0)
	if derr != nil {
		return nil, fmt.Errorf("failed to delete existing runner group: %w", derr)
	}

	infoLogger.LogKV("msg", "deploying runner group")
	rerr := kr.RGRun(ctx, 0, rgCfgFile, runnerFlowControl, runnerGroupAffinity)
	if rerr != nil {
		return nil, fmt.Errorf("failed to deploy runner group: %w", rerr)
	}

	infoLogger.LogKV("msg", "start to wait runner group")
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
			// FIXME(weifu): If the pod is not found, we should fast
			// return. However, it's hard to maintain error string
			// match. We should use specific commandline error code
			// or use package instead of binary call.
			if strings.Contains(err.Error(), `pods "runnergroup-server" not found`) {
				return nil, err
			}

			warnLogger.LogKV("msg", fmt.Errorf("failed to fetch runner group's result: %w", err))
			continue
		}

		infoLogger.LogKV("msg", "dump RunnerGroupsReport", "data", data)

		var rgResult types.RunnerGroupsReport
		if err = json.Unmarshal([]byte(data), &rgResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal into RunnerGroupsReport: %w", err)
		}

		infoLogger.LogKV("msg", "deleting runner group")
		if derr := kr.RGDelete(ctx, 0); derr != nil {
			warnLogger.LogKV("msg", "failed to delete runner group", "err", err)
		}
		return &rgResult, nil
	}
}

// FetchAPIServerCores fetchs core number for each kube-apiserver.
func FetchAPIServerCores(ctx context.Context, kubeCfgPath string) (map[string]int, error) {
	logger := log.GetLogger(ctx)

	logger.WithKeyValues("level", "info").LogKV("msg", "fetching apiserver's cores")

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
			logger.WithKeyValues("level", "warn").LogKV("msg", "failed to get cores", "ip", ip, "error", err)
			continue
		}
		logger.LogKV(ip, cores)
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
	logger := log.GetLogger(ctx)

	var cancel context.CancelFunc
	if timeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}

	logger.WithKeyValues("level", "info").LogKV("msg", "start command", "cmd", c.String())
	output, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to invoke %s:\n (output: %s): %w",
			c.String(), strings.TrimSpace(string(output)), err)
	}
	return output, nil
}

// runCommandWithInput executes a command with `input` piped through stdin.
func runCommandWithInput(ctx context.Context, timeout time.Duration, cmd string, args []string, input string) ([]byte, error) {
	logger := log.GetLogger(ctx)

	var cancel context.CancelFunc
	if timeout != 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	c := exec.CommandContext(ctx, cmd, args...)
	c.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	c.Stdin = strings.NewReader(input)

	logger.WithKeyValues("level", "info").LogKV("msg", "start command", "cmd", c.String())
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
