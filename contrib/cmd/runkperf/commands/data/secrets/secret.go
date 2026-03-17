// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package secrets

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
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data"

	"github.com/urfave/cli"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var appLabel = "runkperf"

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
			Usage: "Namespace to use with commands. If the namespace does not exist, it will be created.",
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
		fmt.Printf("Created secret %s with size %d B, group-size %d, total %d\n", secName, size, groupSize, total)
		return nil
	},
}

var secretDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete a secrets set",
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
		labelSelector := fmt.Sprintf("app=%s,secName=%s", appLabel, secName)

		clientset, err := data.NewClientset(kubeCfgPath)
		if err != nil {
			return err
		}

		err = deleteSecrets(clientset, labelSelector, namespace)
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
	ArgsUsage: "NAME",
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
			labelSelector = fmt.Sprintf("app=%s,secName", appLabel)
		} else {
			args := cliCtx.Args()
			namesStr := strings.Join(args, ",")
			labelSelector = fmt.Sprintf("app=%s, secName in (%s)", appLabel, namesStr)
		}

		secMap := make(map[string][]int)
		err = listSecretsByName(clientset, labelSelector, namespace, secMap)
		if err != nil {
			return err
		}

		for key, value := range secMap {
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

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

func randBytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}

	b := make([]byte, n)
	for i := range b {
		random, err := rand.Int(rand.Reader, big.NewInt(int64(len(letterRunes))))
		if err != nil {
			return nil, fmt.Errorf("error generating random number: %w", err)
		}
		b[i] = byte(letterRunes[int(random.Int64())])
	}
	return b, nil
}

func createSecrets(clientset *kubernetes.Clientset, namespace string, secName string, size int, groupSize int, total int) error {
	for i := 0; i < total; i += groupSize {
		ownerID := i
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < total; j++ {
			idx := j
			g.Go(func() error {
				name := fmt.Sprintf("%s-sec-%s-%d", appLabel, secName, idx)

				data, err := randBytes(size)
				if err != nil {
					return fmt.Errorf("failed to generate random data for secret %s: %v", name, err)
				}

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
						Labels: map[string]string{
							"ownerID": strconv.Itoa(ownerID),
							"app":     appLabel,
							"secName": secName,
						},
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"data": data,
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

func deleteSecrets(clientset *kubernetes.Clientset, labelSelector string, namespace string) error {
	secrets, err := listSecrets(clientset, labelSelector, namespace)
	if err != nil {
		return err
	}

	if len(secrets.Items) == 0 {
		return fmt.Errorf("no secrets set found in namespace: %s", namespace)
	}

	n, batch := len(secrets.Items), 300
	for i := 0; i < n; i += batch {
		g := new(errgroup.Group)
		for j := i; j < i+batch && j < n; j++ {
			idx := j
			g.Go(func() error {
				err := clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), secrets.Items[idx].Name, metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete secret %s: %v", secrets.Items[idx].Name, err)
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

func listSecrets(clientset *kubernetes.Clientset, labelSelector string, namespace string) (*corev1.SecretList, error) {
	secrets, err := clientset.CoreV1().Secrets(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %v", err)
	}
	return secrets, nil
}

func listSecretsByName(clientset *kubernetes.Clientset, labelSelector string, namespace string, secMap map[string][]int) error {
	secrets, err := listSecrets(clientset, labelSelector, namespace)
	if err != nil {
		return err
	}

	for _, sec := range secrets.Items {
		name, ok := sec.Labels["secName"]
		if !ok {
			return fmt.Errorf("failed to find the secName of secret %s", sec.Name)
		}

		_, ok = secMap[name]
		if !ok {
			// Initialize: size, group-size, total
			secMap[name] = []int{0, 0, 0}

			if data, exists := sec.Data["data"]; exists {
				secMap[name][0] = len(data)
			}
		}

		secMap[name][2]++

		if secMap[name][1] != 0 {
			continue
		}

		ownerID, ok := sec.Labels["ownerID"]
		if !ok {
			return fmt.Errorf("failed to find the ownerID of secret %s", name)
		}

		if ownerIDInt, err := strconv.Atoi(ownerID); err == nil {
			if ownerIDInt > secMap[name][1] {
				secMap[name][1] = ownerIDInt
			}
		} else {
			return fmt.Errorf("failed to convert ownerID %s to int: %v", ownerID, err)
		}
	}
	return nil
}
