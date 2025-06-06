// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package configmaps

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"golang.org/x/sync/errgroup"

	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/urfave/cli"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var appLebel = "runkperf"

var Command = cli.Command{
	Name:      "configmap",
	ShortName: "cm",
	Usage:     "Manage configmaps",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: utils.DefaultKubeConfigPath,
		},
		cli.StringFlag{
			Name:  "namespace",
			Usage: "Namespace to use with commands. If the namespace does not exist, it will be created.",
			Value: "default",
		},
	},
	Subcommands: []cli.Command{
		configmapAddCommand,
		configmapDelCommand,
		configmapListCommand,
	},
}

var configmapAddCommand = cli.Command{
	Name:      "add",
	Usage:     "Add configmap set",
	ArgsUsage: "NAME of the configmaps set",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "size",
			Usage: "The size of each configmap (Unit: KiB)",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "The size of each configmap group",
			Value: 10,
		},
		cli.IntFlag{
			Name:  "total",
			Usage: "Total amount of configmaps",
			Value: 10,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as configmaps set name: %v", cliCtx.Args())
		}
		cmName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(cmName) == 0 {
			return fmt.Errorf("required non-empty configmap set name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		size := cliCtx.Int("size")
		groupSize := cliCtx.Int("group-size")
		total := cliCtx.Int("total")

		// Check if the flags are set correctly
		err := checkConfigmapParams(size, groupSize, total)
		if err != nil {
			return err
		}

		namespace := cliCtx.GlobalString("namespace")
		err = prepareNamespace(kubeCfgPath, namespace)
		if err != nil {
			return err
		}

		clientset, err := newClientsetWithRateLimiter(kubeCfgPath, 30, 10)
		if err != nil {
			return err
		}

		err = createConfigmaps(clientset, namespace, cmName, size, groupSize, total)
		if err != nil {
			return err
		}
		fmt.Printf("Created configmap %s with size %d KiB, group-size %d, total %d\n", cmName, size, groupSize, total)
		return nil
	},
}

var configmapDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete a configmaps set",
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one configmaps set name")
		}
		cmName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(cmName) == 0 {
			return fmt.Errorf("required non-empty configmaps set name")
		}

		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		labelSelector := fmt.Sprintf("app=%s,cmName=%s", appLebel, cmName)

		clientset, err := newClientsetWithRateLimiter(kubeCfgPath, 30, 10)
		if err != nil {
			return err
		}

		// Delete each configmap
		err = deleteConfigmaps(clientset, labelSelector, namespace)
		if err != nil {
			return err
		}

		fmt.Printf("Deleted configmap %s in %s namespace\n", cmName, namespace)
		return nil

	},
}

var configmapListCommand = cli.Command{
	Name:      "list",
	Usage:     "List generated configmaps",
	ArgsUsage: "NAME",
	Action: func(cliCtx *cli.Context) error {
		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		clientset, err := newClientsetWithRateLimiter(kubeCfgPath, 30, 10)
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
		fmt.Fprintln(tw, "NAME\tSIZE\tGROUP_SIZE\tTOTAL\t")

		// Build the label selector
		// If no args are provided, list all configmaps with the label app=runkperf
		// If args are provided, list all configmaps with the label app=runkperf and cmName in (args)
		var labelSelector string
		if cliCtx.NArg() == 0 {
			labelSelector = fmt.Sprintf("app=%s", appLebel)

		} else {
			args := cliCtx.Args()
			namesStr := strings.Join(args, ",")
			labelSelector = fmt.Sprintf("app=%s, cmName in (%s)", appLebel, namesStr)
		}
		cmMap := make(map[string][]int)
		err = listConfigmapsByName(clientset, labelSelector, namespace, cmMap)

		if err != nil {
			return err
		}

		for key, value := range cmMap {
			fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n",
				key,
				value[0],
				value[1],
				value[2],
			)
		}
		return tw.Flush()
	},
}

func prepareNamespace(kubeCfgPath string, namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	if namespace == "default" {
		return nil
	}

	clientset, err := newClientsetWithRateLimiter(kubeCfgPath, 30, 10)
	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		// If the namespace already exists, ignore the error
		if errors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create namespace %s: %v", namespace, err)
	}
	return nil
}

func checkConfigmapParams(size int, groupSize int, total int) error {
	if size <= 0 {
		return fmt.Errorf("size must be greater than 0")
	}
	if groupSize <= 0 {
		return fmt.Errorf("group-size must be greater than 0")
	}
	if total <= 0 {
		return fmt.Errorf("total amount must be greater than 0")
	}
	if groupSize > total {
		return fmt.Errorf("group-size must be less than or equal to total")
	}
	return nil
}

func newClientsetWithRateLimiter(kubeCfgPath string, qps float32, burst int) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, err
	}

	config.QPS = qps
	config.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(qps, burst)
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randString(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("length must be positive")
	}

	b := make([]rune, n)
	for i := range b {
		random, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			return "", fmt.Errorf("error generating random number: %w", err)
		}
		b[i] = letterRunes[int(random.Int64())]
	}
	return string(b), nil
}

func createConfigmaps(clientset *kubernetes.Clientset, namespace string, cmName string, size int, groupSize int, total int) error {
	// Generate configmaps in parallel with fixed group size
	// and random data
	for i := 0; i < total; i = i + groupSize {
		ownerID := i
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < total; j++ {
			g.Go(func() error {
				cli := clientset.CoreV1().ConfigMaps(namespace)

				name := fmt.Sprintf("%s-cm-%s-%d", appLebel, cmName, j)

				cm := &corev1.ConfigMap{}
				cm.Name = name
				// Set the labels for the configmap to easily identify in delete or list commands
				cm.Labels = map[string]string{
					"ownerID": strconv.Itoa(ownerID),
					"app":     appLebel,
					"cmName":  cmName,
				}
				data, err := randString(size * 1024)
				if err != nil {
					return fmt.Errorf("failed to generate random string for configmap %s: %v", name, err)
				}
				cm.Data = map[string]string{
					"data": data,
				}

				_, err = cli.Create(context.TODO(), cm, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create configmap %s: %v", name, err)
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

func deleteConfigmaps(clientset *kubernetes.Clientset, labelSelector string, namespace string) error {
	// List all configmaps with the label selector
	configMaps, err := listConfigmaps(clientset, labelSelector, namespace)
	if err != nil {
		return err
	}

	if len(configMaps.Items) == 0 {
		return fmt.Errorf("no configmaps set found in namespace: %s", namespace)
	}
	// Delete each configmap in parallel with fixed group size
	n, batch := len(configMaps.Items), 10
	for i := 0; i < n; i = i + batch {
		g := new(errgroup.Group)
		for j := i; j < i+batch && j < n; j++ {
			g.Go(func() error {
				err := clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), configMaps.Items[j].Name, metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					// Ignore not found errors
					return fmt.Errorf("failed to delete configmap %s: %v", configMaps.Items[j].Name, err)
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

func listConfigmaps(clientset *kubernetes.Clientset, labelSelector string, namespace string) (*corev1.ConfigMapList, error) {
	configMaps, err := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list configmaps: %v", err)
	}

	return configMaps, nil
}

// Get info of configmaps by name
func listConfigmapsByName(clientset *kubernetes.Clientset, labelSelector string, namespace string, cmMap map[string][]int) error {
	configMaps, err := listConfigmaps(clientset, labelSelector, namespace)

	if err != nil {
		return err
	}

	for _, cm := range configMaps.Items {
		name, ok := cm.Labels["cmName"]
		if !ok {
			return fmt.Errorf("failed to find the cmName of configmap %s", cm.Name)
		}

		_, ok = cmMap[name]
		if !ok {
			// Initialize the map with default values
			// size, group-size, total in int list
			cmMap[name] = []int{0, 0, 0}

			// Get the size of the configmap
			_, ok = cm.Data["data"]
			if ok {
				cmMap[name][0] = len(cm.Data["data"])
			}
		}

		// Increment the total count of configmaps
		cmMap[name][2]++

		if cmMap[name][1] != 0 {
			continue
		}

		ownerID, ok := cm.Labels["ownerID"]
		if !ok {
			return fmt.Errorf("failed to find the ownerID of configmap %s", name)
		}

		if ownerIDInt, err := strconv.Atoi(ownerID); err == nil {
			// Use the ownerID to get the group size
			if ownerIDInt > cmMap[name][1] {
				cmMap[name][1] = ownerIDInt
			}
		} else {
			return fmt.Errorf("failed to convert ownerID %s to int: %v", ownerID, err)
		}

	}
	return nil
}
