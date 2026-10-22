// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package pod

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/sync/errgroup"

	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data"
	"github.com/Azure/kperf/request"

	"github.com/urfave/cli"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// defaultBatchSize is the number of pods to list or delete per batch.
// It is used as the page size for paginated API calls.
const defaultBatchSize int64 = 300

// paddingAnnotation holds random bytes used to inflate each pod's object size.
const paddingAnnotation = "runkperf.io/padding"

// Use an unhandled scheduler name to keep ingestion pods unscheduled.
const dataOnlySchedulerName = "runkperf-data-only"

// pauseImage is the placeholder image used for the bare ingestion pods.
const pauseImage = "registry.k8s.io/pause:3.9"

var Command = cli.Command{
	Name:      "pod",
	ShortName: "po",
	Usage:     "Manage pods",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: utils.DefaultKubeConfigPath,
		},
		cli.StringFlag{
			Name:  "namespace",
			Usage: "Namespace to use with commands. If the namespace does not exist, it will be created when executing add subcommand.",
			Value: "default",
		},
	},
	Subcommands: []cli.Command{
		podAddCommand,
		podDelCommand,
		podListCommand,
	},
}

var podAddCommand = cli.Command{
	Name:      "add",
	Usage:     "Add pods set",
	ArgsUsage: "NAME of the pods set",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "total",
			Usage: "Total number of pods to create",
			Value: 10,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "Number of pods to create in parallel per batch",
			Value: 30,
		},
		cli.IntFlag{
			Name:  "size",
			Usage: "The size of the padding stored in each pod annotation (Unit: Byte)",
			Value: 10240,
		},
		cli.Float64Flag{
			Name:  "qps",
			Usage: "QPS for the Kubernetes client rate limiter to control pod operations",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "burst",
			Usage: "Burst for the Kubernetes client rate limiter to control pod operations",
			Value: 200,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as pods set name: %v", cliCtx.Args())
		}
		podSetName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(podSetName) == 0 {
			return fmt.Errorf("required non-empty pod set name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		namespace := cliCtx.GlobalString("namespace")
		total := cliCtx.Int("total")
		groupSize := cliCtx.Int("group-size")
		size := cliCtx.Int("size")
		qps := float32(cliCtx.Float64("qps"))
		burst := cliCtx.Int("burst")

		if err := checkPodParams(size, groupSize, total); err != nil {
			return err
		}

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = data.PrepareNamespace(clientset, namespace)
		if err != nil {
			return err
		}

		err = createPods(clientset, namespace, podSetName, total, groupSize, size)
		if err != nil {
			return err
		}
		fmt.Printf("Created %d pods (set=%s, size=%d B) in namespace %s\n",
			total, podSetName, size, namespace)
		return nil
	},
}

var podDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete a pods set",
	Flags: []cli.Flag{
		cli.Float64Flag{
			Name:  "qps",
			Usage: "QPS for the Kubernetes client rate limiter to control pod deletion",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "burst",
			Usage: "Burst for the Kubernetes client rate limiter to control pod deletion",
			Value: 200,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "Number of pods to delete in parallel per batch",
			Value: 30,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one pods set name")
		}
		podSetName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(podSetName) == 0 {
			return fmt.Errorf("required non-empty pods set name")
		}

		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		qps := float32(cliCtx.Float64("qps"))
		burst := cliCtx.Int("burst")
		groupSize := cliCtx.Int("group-size")
		if groupSize <= 0 {
			return fmt.Errorf("group-size must be greater than 0")
		}
		labelSelector := fmt.Sprintf("app=%s,podSetName=%s", data.AppLabel, podSetName)

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = deletePods(clientset, labelSelector, namespace, groupSize)
		if err != nil {
			return err
		}

		fmt.Printf("Deleted pods set %s in %s namespace\n", podSetName, namespace)
		return nil
	},
}

var podListCommand = cli.Command{
	Name:      "list",
	Usage:     "List generated pod sets. Lists all if no arguments are given; otherwise, provide pod set names separated by spaces.",
	ArgsUsage: "[NAME ...]",
	Action: func(cliCtx *cli.Context) error {
		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		clientset, err := data.NewClientset(kubeCfgPath)
		if err != nil {
			return err
		}

		const (
			minWidth = 1
			tabWidth = 12
			padding  = 3
			padChar  = ' '
			flags    = 0
		)
		tw := tabwriter.NewWriter(os.Stdout, minWidth, tabWidth, padding, padChar, flags)
		fmt.Fprintln(tw, "NAME\tSIZE\tTOTAL\t")

		var labelSelector string
		if cliCtx.NArg() == 0 {
			labelSelector = fmt.Sprintf("app=%s", data.AppLabel)
		} else {
			args := cliCtx.Args()
			namesStr := strings.Join(args, ",")
			labelSelector = fmt.Sprintf("app=%s, podSetName in (%s)", data.AppLabel, namesStr)
		}

		podMap := make(map[string]*podSetInfo)
		err = listPods(clientset, labelSelector, namespace, defaultBatchSize, func(pod corev1.Pod) error {
			name, ok := pod.Labels["podSetName"]
			if !ok {
				return nil
			}

			info, ok := podMap[name]
			if !ok {
				info = &podSetInfo{
					size: len(pod.Annotations[paddingAnnotation]),
				}
				podMap[name] = info
			}
			info.total++
			return nil
		})
		if err != nil {
			return err
		}

		for name, info := range podMap {
			fmt.Fprintf(tw, "%s\t%d\t%d\n",
				name,
				info.size,
				info.total,
			)
		}
		return tw.Flush()
	},
}

type podSetInfo struct {
	size  int
	total int
}

func checkPodParams(size, groupSize, total int) error {
	if size <= 0 {
		return fmt.Errorf("size must be greater than 0")
	}
	if groupSize <= 0 {
		return fmt.Errorf("group-size must be greater than 0")
	}
	if total <= 0 {
		return fmt.Errorf("total must be greater than 0")
	}
	return nil
}

func createPods(clientset kubernetes.Interface, namespace, podSetName string, total, groupSize, size int) error {
	for i := 0; i < total; i += groupSize {
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < total; j++ {
			idx := j
			g.Go(func() error {
				name := fmt.Sprintf("%s-pod-%s-%d", data.AppLabel, podSetName, idx)

				padding := request.RandomPayload(size)

				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels: map[string]string{
							"app":        data.AppLabel,
							"podSetName": podSetName,
						},
						Annotations: map[string]string{
							paddingAnnotation: padding,
						},
					},
					Spec: corev1.PodSpec{
						SchedulerName: dataOnlySchedulerName,
						Containers: []corev1.Container{
							{
								Name:  "main",
								Image: pauseImage,
							},
						},
					},
				}

				_, err := clientset.CoreV1().Pods(namespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create pod %s: %v", name, err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
	}
	return nil
}

func deletePods(clientset kubernetes.Interface, labelSelector, namespace string, groupSize int) error {
	var names []string
	err := listPods(clientset, labelSelector, namespace, defaultBatchSize, func(pod corev1.Pod) error {
		names = append(names, pod.Name)
		return nil
	})
	if err != nil {
		return err
	}

	if len(names) == 0 {
		fmt.Printf("No pods found in namespace %s, nothing to delete\n", namespace)
		return nil
	}

	n := len(names)
	for i := 0; i < n; i += groupSize {
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < n; j++ {
			idx := j
			g.Go(func() error {
				err := clientset.CoreV1().Pods(namespace).Delete(context.TODO(), names[idx], metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete pod %s: %v", names[idx], err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
	}
	return nil
}

func listPods(clientset kubernetes.Interface, labelSelector, namespace string, limit int64, fn func(corev1.Pod) error) error {
	if limit <= 0 {
		limit = defaultBatchSize
	}
	var continueToken string
	for {
		podList, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
			Limit:         limit,
			Continue:      continueToken,
		})
		if err != nil {
			return fmt.Errorf("failed to list pods: %v", err)
		}

		for _, pod := range podList.Items {
			if err := fn(pod); err != nil {
				return err
			}
		}

		continueToken = podList.Continue
		if continueToken == "" {
			break
		}
	}
	return nil
}
