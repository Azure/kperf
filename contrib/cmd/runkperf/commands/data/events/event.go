// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package events

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"strings"
	"text/tabwriter"
	"time"

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
	Name:      "event",
	ShortName: "ev",
	Usage:     "Manage events",
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
		eventAddCommand,
		eventDelCommand,
		eventListCommand,
	},
}

var eventAddCommand = cli.Command{
	Name:      "add",
	Usage:     "Add events set",
	ArgsUsage: "NAME of the events set",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "total",
			Usage: "Total number of events to create",
			Value: 10,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "Number of events to create in parallel per batch",
			Value: 30,
		},
		cli.StringFlag{
			Name:  "reason",
			Usage: "The reason string for the events (e.g. ScaleUp, FailedScheduling)",
			Value: "BenchmarkEvent",
		},
		cli.StringFlag{
			Name:  "type",
			Usage: "The type of the events: Normal or Warning",
			Value: "Normal",
		},
		cli.IntFlag{
			Name:  "message-size",
			Usage: "The size of the event message in bytes (default targets ~5KB total event size; k8s metadata overhead is ~500-700 bytes)",
			Value: 5000,
		},
		cli.StringFlag{
			Name:  "involved-object-kind",
			Usage: "The kind of the involved object (e.g. Pod, Node, Deployment)",
			Value: "Pod",
		},
		cli.StringFlag{
			Name:  "involved-object-name",
			Usage: "The name of the involved object. If empty, a name will be generated.",
			Value: "",
		},
		cli.Float64Flag{
			Name:  "qps",
			Usage: "QPS for the Kubernetes client rate limiter to control event operations",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "burst",
			Usage: "Burst for the Kubernetes client rate limiter to control event operations",
			Value: 200,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one argument as events set name: %v", cliCtx.Args())
		}
		evName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(evName) == 0 {
			return fmt.Errorf("required non-empty event set name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		namespace := cliCtx.GlobalString("namespace")
		total := cliCtx.Int("total")
		groupSize := cliCtx.Int("group-size")
		reason := cliCtx.String("reason")
		eventType := cliCtx.String("type")
		messageSize := cliCtx.Int("message-size")
		involvedObjectKind := cliCtx.String("involved-object-kind")
		involvedObjectName := cliCtx.String("involved-object-name")
		qps := float32(cliCtx.Float64("qps"))
		burst := cliCtx.Int("burst")

		if total <= 0 {
			return fmt.Errorf("total must be greater than 0")
		}
		if groupSize <= 0 {
			return fmt.Errorf("group-size must be greater than 0")
		}
		if groupSize > total {
			return fmt.Errorf("group-size must be less than or equal to total")
		}
		if eventType != "Normal" && eventType != "Warning" {
			return fmt.Errorf("type must be either Normal or Warning")
		}
		if messageSize <= 0 {
			return fmt.Errorf("message-size must be greater than 0")
		}

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = data.PrepareNamespace(clientset, namespace)
		if err != nil {
			return err
		}

		err = createEvents(clientset, namespace, evName, total, groupSize, reason, eventType, messageSize, involvedObjectKind, involvedObjectName)
		if err != nil {
			return err
		}
		fmt.Printf("Created %d events (set=%s, reason=%s, type=%s) in namespace %s\n",
			total, evName, reason, eventType, namespace)
		return nil
	},
}

var eventDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete an events set",
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one events set name")
		}
		evName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(evName) == 0 {
			return fmt.Errorf("required non-empty events set name")
		}

		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		labelSelector := fmt.Sprintf("app=%s,evName=%s", appLabel, evName)

		clientset, err := data.NewClientset(kubeCfgPath)
		if err != nil {
			return err
		}

		err = deleteEvents(clientset, labelSelector, namespace)
		if err != nil {
			return err
		}

		fmt.Printf("Deleted events set %s in %s namespace\n", evName, namespace)
		return nil
	},
}

var eventListCommand = cli.Command{
	Name:  "list",
	Usage: "List generated event sets. Lists all if no arguments are given; otherwise, provide event set names separated by spaces.",
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
		fmt.Fprintln(tw, "NAME\tTYPE\tREASON\tTOTAL\t")

		var labelSelector string
		if cliCtx.NArg() == 0 {
			labelSelector = fmt.Sprintf("app=%s,evName", appLabel)
		} else {
			args := cliCtx.Args()
			namesStr := strings.Join(args, ",")
			labelSelector = fmt.Sprintf("app=%s, evName in (%s)", appLabel, namesStr)
		}

		evMap, err := listEventsByName(clientset, labelSelector, namespace)
		if err != nil {
			return err
		}

		for name, info := range evMap {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n",
				name,
				info.eventType,
				info.reason,
				info.total,
			)
		}
		return tw.Flush()
	},
}

type eventSetInfo struct {
	eventType string
	reason    string
	total     int
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

func createEvents(clientset *kubernetes.Clientset, namespace, evName string, total, groupSize int, reason, eventType string, messageSize int, involvedObjectKind, involvedObjectName string) error {
	now := time.Now()

	for i := 0; i < total; i += groupSize {
		g := new(errgroup.Group)
		for j := i; j < i+groupSize && j < total; j++ {
			idx := j
			g.Go(func() error {
				name := fmt.Sprintf("%s-ev-%s-%d", appLabel, evName, idx)

				message, err := randString(messageSize)
				if err != nil {
					return fmt.Errorf("failed to generate random message for event %s: %v", name, err)
				}

				objName := involvedObjectName
				if objName == "" {
					objName = fmt.Sprintf("%s-obj-%d", evName, idx)
				}

				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels: map[string]string{
							"app":    appLabel,
							"evName": evName,
						},
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      involvedObjectKind,
						Name:      objName,
						Namespace: namespace,
					},
					Reason:         reason,
					Message:        message,
					Type:           eventType,
					Count:          1,
					FirstTimestamp: metav1.NewTime(now),
					LastTimestamp:  metav1.NewTime(now),
					Source: corev1.EventSource{
						Component: "runkperf-bench",
					},
				}

				_, err = clientset.CoreV1().Events(namespace).Create(context.TODO(), event, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("failed to create event %s: %v", name, err)
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

func deleteEvents(clientset *kubernetes.Clientset, labelSelector, namespace string) error {
	eventList, err := listEvents(clientset, labelSelector, namespace)
	if err != nil {
		return err
	}

	if len(eventList.Items) == 0 {
		return fmt.Errorf("no events set found in namespace: %s", namespace)
	}

	n, batch := len(eventList.Items), 300
	for i := 0; i < n; i += batch {
		g := new(errgroup.Group)
		for j := i; j < i+batch && j < n; j++ {
			idx := j
			g.Go(func() error {
				err := clientset.CoreV1().Events(namespace).Delete(context.TODO(), eventList.Items[idx].Name, metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete event %s: %v", eventList.Items[idx].Name, err)
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

func listEvents(clientset *kubernetes.Clientset, labelSelector, namespace string) (*corev1.EventList, error) {
	eventList, err := clientset.CoreV1().Events(namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %v", err)
	}
	return eventList, nil
}

func listEventsByName(clientset *kubernetes.Clientset, labelSelector, namespace string) (map[string]*eventSetInfo, error) {
	eventList, err := listEvents(clientset, labelSelector, namespace)
	if err != nil {
		return nil, err
	}

	evMap := make(map[string]*eventSetInfo)
	for _, ev := range eventList.Items {
		name, ok := ev.Labels["evName"]
		if !ok {
			continue
		}

		info, ok := evMap[name]
		if !ok {
			info = &eventSetInfo{
				eventType: ev.Type,
				reason:    ev.Reason,
			}
			evMap[name] = info
		}
		info.total++
	}
	return evMap, nil
}
