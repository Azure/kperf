// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package secrets

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"golang.org/x/sync/errgroup"

	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data"

	"github.com/urfave/cli"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// defaultBatchSize is the number of secrets to list or delete per batch.
// It is used as the page size for paginated API calls.
const defaultBatchSize int64 = 300

var Command = cli.Command{
	Name:      "secret",
	ShortName: "sec",
	Usage:     "Manage secrets",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "kubeconfig",
			Usage: "Path to the kubeconfig file",
			Value: utils.DefaultKubeConfigPath,
		},
		cli.StringFlag{
			Name:  "namespace",
			Usage: "Namespace to use with commands. If the namespace does not exist, it will be created when executing add subcommand",
			Value: "default",
		},
	},
	Subcommands: []cli.Command{
		secretAddCommand,
		secretDelCommand,
		secretListCommand,
	},
}

var secretAddCommand = cli.Command{
	Name:      "add",
	Usage:     "Add secrets set",
	ArgsUsage: "NAME of the secrets set",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "size",
			Usage: "The size of each secret value (Unit: Byte)",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "The number of secrets to create in parallel per batch",
			Value: 30,
		},
		cli.IntFlag{
			Name:  "total",
			Usage: "Total number of secrets to create",
			Value: 10,
		},
		cli.Float64Flag{
			Name:  "qps",
			Usage: "QPS for the Kubernetes client rate limiter to control secret operations",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "burst",
			Usage: "Burst for the Kubernetes client rate limiter to control secret operations",
			Value: 200,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as secrets set name: %v", cliCtx.Args())
		}
		secName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(secName) == 0 {
			return fmt.Errorf("required non-empty secret set name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		size := cliCtx.Int("size")
		groupSize := cliCtx.Int("group-size")
		total := cliCtx.Int("total")

		err := checkSecretParams(size, groupSize, total)
		if err != nil {
			return err
		}

		namespace := cliCtx.GlobalString("namespace")
		qps := float32(cliCtx.Float64("qps"))
		burst := cliCtx.Int("burst")

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = data.PrepareNamespace(clientset, namespace)
		if err != nil {
			return err
		}

		err = createSecrets(clientset, namespace, secName, size, groupSize, total)
		if err != nil {
			return err
		}
		fmt.Printf("Created secret set %s with size %d B, group-size %d, total %d\n", secName, size, groupSize, total)
		return nil
	},
}

var secretDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete a secrets set",
	Flags: []cli.Flag{
		cli.Float64Flag{
			Name:  "qps",
			Usage: "QPS for the Kubernetes client rate limiter to control secret deletion",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "burst",
			Usage: "Burst for the Kubernetes client rate limiter to control secret deletion",
			Value: 200,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "Number of secrets to delete in parallel per batch",
			Value: 30,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one secrets set name")
		}
		secName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(secName) == 0 {
			return fmt.Errorf("required non-empty secrets set name")
		}

		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		qps := float32(cliCtx.Float64("qps"))
		burst := cliCtx.Int("burst")
		groupSize := cliCtx.Int("group-size")
		if groupSize <= 0 {
			return fmt.Errorf("group-size must be greater than 0")
		}
		labelSelector := fmt.Sprintf("app=%s,secName=%s", data.AppLabel, secName)

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = deleteSecrets(clientset, labelSelector, namespace, groupSize)
		if err != nil {
			return err
		}

		fmt.Printf("Deleted secrets set %s in %s namespace\n", secName, namespace)
		return nil
	},
}

var secretListCommand = cli.Command{
	Name:      "list",
	Usage:     "List generated secrets. Lists all if no arguments are given; otherwise, provide secret set names separated by spaces.",
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
		fmt.Fprintln(tw, "NAME\tSIZE\tGROUP_SIZE\tTOTAL\t")

		var labelSelector string
		if cliCtx.NArg() == 0 {
			labelSelector = fmt.Sprintf("app=%s", data.AppLabel)
		} else {
			args := cliCtx.Args()
			namesStr := strings.Join(args, ",")
			labelSelector = fmt.Sprintf("app=%s, secName in (%s)", data.AppLabel, namesStr)
		}

		type secretSetInfo struct {
			size    int
			total   int
			ownerID map[string]int // ownerID -> count of secrets in that group
		}
		secMap := make(map[string]*secretSetInfo)
		err = listSecrets(clientset, labelSelector, namespace, defaultBatchSize, func(sec corev1.Secret) error {
			name, ok := sec.Labels["secName"]
			if !ok {
				return fmt.Errorf("failed to find the secName of secret %s", sec.Name)
			}

			info, ok := secMap[name]
			if !ok {
				info = &secretSetInfo{
					ownerID: make(map[string]int),
				}
				if d, exists := sec.Data["data"]; exists {
					info.size = len(d)
				}
				secMap[name] = info
			}

			info.total++

			ownerID, ok := sec.Labels["ownerID"]
			if !ok {
				return fmt.Errorf("failed to find the ownerID of secret %s", sec.Name)
			}
			info.ownerID[ownerID]++
			return nil
		})
		if err != nil {
			return err
		}

		for name, info := range secMap {
			// Group size should be the max count across all ownerID groups
			groupSize := 0
			for _, count := range info.ownerID {
				if count > groupSize {
					groupSize = count
				}
			}
			fmt.Fprintf(tw, "%s\t%d\t%d\t%d\n",
				name,
				info.size,
				groupSize,
				info.total,
			)
		}
		return tw.Flush()
	},
}

func checkSecretParams(size int, groupSize int, total int) error {
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

func createSecrets(clientset *kubernetes.Clientset, namespace string, secName string, size int, groupSize int, total int) error {
	for i := 0; i < total; i += groupSize {
		ownerID := i
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < total; j++ {
			idx := j
			g.Go(func() error {
				name := fmt.Sprintf("%s-sec-%s-%d", data.AppLabel, secName, idx)

				randData, err := data.RandBytes(size)
				if err != nil {
					return fmt.Errorf("failed to generate random data for secret %s: %v", name, err)
				}

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
						Labels: map[string]string{
							"ownerID": strconv.Itoa(ownerID),
							"app":     data.AppLabel,
							"secName": secName,
						},
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"data": randData,
					},
				}

				_, err = clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create secret %s: %v", name, err)
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

func deleteSecrets(clientset *kubernetes.Clientset, labelSelector string, namespace string, groupSize int) error {
	var names []string
	err := listSecrets(clientset, labelSelector, namespace, defaultBatchSize, func(sec corev1.Secret) error {
		names = append(names, sec.Name)
		return nil
	})
	if err != nil {
		return err
	}

	if len(names) == 0 {
		fmt.Printf("No secrets found in namespace %s, nothing to delete\n", namespace)
		return nil
	}

	n := len(names)
	for i := 0; i < n; i += groupSize {
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < n; j++ {
			idx := j
			g.Go(func() error {
				err := clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), names[idx], metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete secret %s: %v", names[idx], err)
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

func listSecrets(clientset *kubernetes.Clientset, labelSelector string, namespace string, limit int64, fn func(corev1.Secret) error) error {
	if limit <= 0 {
		limit = defaultBatchSize
	}
	var continueToken string
	for {
		secrets, err := clientset.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labelSelector,
			Limit:         limit,
			Continue:      continueToken,
		})
		if err != nil {
			return fmt.Errorf("failed to list secrets: %v", err)
		}

		for _, sec := range secrets.Items {
			if err := fn(sec); err != nil {
				return err
			}
		}

		continueToken = secrets.Continue
		if continueToken == "" {
			break
		}
	}
	return nil
}
