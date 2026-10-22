// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package events

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/Azure/kperf/cmd/kperf/commands/utils"
	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data"
	"github.com/Azure/kperf/request"

	"github.com/urfave/cli"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

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
			Usage: "Namespace to use with commands. If the namespace does not exist, it will be created when executing add subcommand.",
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
			Name:  "size",
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
		eventSetName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(eventSetName) == 0 {
			return fmt.Errorf("required non-empty event set name")
		}

		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		namespace := cliCtx.GlobalString("namespace")
		total := cliCtx.Int("total")
		groupSize := cliCtx.Int("group-size")
		reason := cliCtx.String("reason")
		eventType := cliCtx.String("type")
		messageSize := cliCtx.Int("size")
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
		if eventType != "Normal" && eventType != "Warning" {
			return fmt.Errorf("type must be either Normal or Warning")
		}
		if messageSize <= 0 {
			return fmt.Errorf("size must be greater than 0")
		}

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = data.PrepareNamespace(clientset, namespace)
		if err != nil {
			return err
		}

		err = createEvents(clientset, createOptions{
			Namespace:          namespace,
			EventSetName:       eventSetName,
			Total:              total,
			GroupSize:          groupSize,
			Reason:             reason,
			EventType:          eventType,
			MessageSize:        messageSize,
			InvolvedObjectKind: involvedObjectKind,
			InvolvedObjectName: involvedObjectName,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Created %d events (set=%s, reason=%s, type=%s) in namespace %s\n",
			total, eventSetName, reason, eventType, namespace)
		return nil
	},
}

var eventDelCommand = cli.Command{
	Name:      "delete",
	ShortName: "del",
	ArgsUsage: "NAME",
	Usage:     "Delete an events set",
	Flags: []cli.Flag{
		cli.Float64Flag{
			Name:  "qps",
			Usage: "QPS for the Kubernetes client rate limiter to control event deletion",
			Value: 100,
		},
		cli.IntFlag{
			Name:  "burst",
			Usage: "Burst for the Kubernetes client rate limiter to control event deletion",
			Value: 200,
		},
		cli.IntFlag{
			Name:  "group-size",
			Usage: "Number of events to delete in parallel per batch",
			Value: 30,
		},
	},
	Action: func(cliCtx *cli.Context) error {
		if cliCtx.NArg() != 1 {
			return fmt.Errorf("required only one events set name")
		}
		eventSetName := strings.TrimSpace(cliCtx.Args().Get(0))
		if len(eventSetName) == 0 {
			return fmt.Errorf("required non-empty events set name")
		}

		namespace := cliCtx.GlobalString("namespace")
		kubeCfgPath := cliCtx.GlobalString("kubeconfig")
		qps := float32(cliCtx.Float64("qps"))
		burst := cliCtx.Int("burst")
		groupSize := cliCtx.Int("group-size")
		if groupSize <= 0 {
			return fmt.Errorf("group-size must be greater than 0")
		}
		labelSelector := fmt.Sprintf("app=%s,eventSetName=%s", data.AppLabel, eventSetName)

		clientset, err := data.NewClientsetWithRateLimiter(kubeCfgPath, qps, burst)
		if err != nil {
			return err
		}

		err = deleteEvents(clientset, labelSelector, namespace, groupSize)
		if err != nil {
			return err
		}

		fmt.Printf("Deleted events set %s in %s namespace\n", eventSetName, namespace)
		return nil
	},
}

var eventListCommand = cli.Command{
	Name:      "list",
	Usage:     "List generated event sets. Lists all if no arguments are given; otherwise, provide event set names separated by spaces.",
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
		fmt.Fprintln(tw, "NAME\tTYPE\tREASON\tTOTAL\t")

		var labelSelector string
		if cliCtx.NArg() == 0 {
			labelSelector = fmt.Sprintf("app=%s", data.AppLabel)
		} else {
			args := cliCtx.Args()
			namesStr := strings.Join(args, ",")
			labelSelector = fmt.Sprintf("app=%s, eventSetName in (%s)", data.AppLabel, namesStr)
		}

		evMap := make(map[string]*eventSetInfo)
		err = listEvents(clientset, labelSelector, namespace, data.DefaultPageSize, func(ev corev1.Event) error {
			name, ok := ev.Labels["eventSetName"]
			if !ok {
				return nil
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
			return nil
		})
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

type createOptions struct {
	Namespace          string
	EventSetName       string
	Total              int
	GroupSize          int
	Reason             string
	EventType          string
	MessageSize        int
	InvolvedObjectKind string
	InvolvedObjectName string
}

func createEvents(clientset kubernetes.Interface, opts createOptions) error {
	now := time.Now()

	for i := 0; i < opts.Total; i += opts.GroupSize {
		g := new(errgroup.Group)
		for j := i; j < i+opts.GroupSize && j < opts.Total; j++ {
			idx := j
			g.Go(func() error {
				name := fmt.Sprintf("%s-ev-%s-%d", data.AppLabel, opts.EventSetName, idx)

				message := request.RandomPayload(opts.MessageSize)

				objName := opts.InvolvedObjectName
				if objName == "" {
					objName = fmt.Sprintf("%s-obj-%d", opts.EventSetName, idx)
				}

				event := &corev1.Event{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: opts.Namespace,
						Labels: map[string]string{
							"app":          data.AppLabel,
							"eventSetName": opts.EventSetName,
						},
					},
					InvolvedObject: corev1.ObjectReference{
						Kind:      opts.InvolvedObjectKind,
						Name:      objName,
						Namespace: opts.Namespace,
					},
					Reason:         opts.Reason,
					Message:        message,
					Type:           opts.EventType,
					Count:          1,
					FirstTimestamp: metav1.NewTime(now),
					LastTimestamp:  metav1.NewTime(now),
					Source: corev1.EventSource{
						Component: "runkperf-bench",
					},
				}

				_, err := clientset.CoreV1().Events(opts.Namespace).Create(context.TODO(), event, metav1.CreateOptions{})
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

func deleteEvents(clientset kubernetes.Interface, labelSelector, namespace string, groupSize int) error {
	return data.BatchDelete("event", namespace, groupSize, func(visit func(string) error) error {
		return listEvents(clientset, labelSelector, namespace, data.DefaultPageSize, func(ev corev1.Event) error {
			return visit(ev.Name)
		})
	}, func(name string) error {
		return clientset.CoreV1().Events(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
	})
}

func listEvents(clientset kubernetes.Interface, labelSelector, namespace string, limit int64, fn func(corev1.Event) error) error {
	return data.ListPaginated(metav1.ListOptions{LabelSelector: labelSelector, Limit: limit}, func(opts metav1.ListOptions) ([]corev1.Event, string, error) {
		eventList, err := clientset.CoreV1().Events(namespace).List(context.TODO(), opts)
		if err != nil {
			return nil, "", fmt.Errorf("failed to list events: %w", err)
		}
		return eventList.Items, eventList.Continue, nil
	}, fn)
}
