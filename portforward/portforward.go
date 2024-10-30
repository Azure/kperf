// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package portforward

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	kubepf "k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"k8s.io/klog/v2"
)

// PodPortForwarder is used to forward traffic to specific pod's TCP port from
// local listener.
type PodPortForwarder struct {
	// targetPort is the target TCP port.
	targetPort uint16
	// portforwardURL is the pod's portforward URL.
	portforwardURL *url.URL
	// restCfg is used to create spdy transport.
	restCfg *rest.Config

	portForwarder *kubepf.PortForwarder
}

// NewPodPortForwarder return a new instance of PodPortForwarder.
func NewPodPortForwarder(kubeCfgPath string, namespace, podName string, targetPort uint16) (*PodPortForwarder, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, err
	}
	restCfg.ContentType = "application/vnd.kubernetes.protobuf"

	restCli, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}

	if err := ensurePodIsRunning(restCli, namespace, podName); err != nil {
		return nil, err
	}

	u := restCli.CoreV1().RESTClient().Post().
		Namespace(namespace).
		Resource("pods").
		Name(podName).
		SubResource("portforward").URL()

	return &PodPortForwarder{
		targetPort:     targetPort,
		portforwardURL: u,
		restCfg:        restCfg,
	}, nil
}

// Start is to start local listener to forward traffic.
func (pf *PodPortForwarder) Start() error {
	transport, upgrader, err := spdy.RoundTripperFor(pf.restCfg)
	if err != nil {
		return fmt.Errorf("failed to create spdy transport: %w", err)
	}

	dialer := spdy.NewDialer(
		upgrader,
		&http.Client{Transport: transport},
		"POST",
		pf.portforwardURL,
	)

	startCh := make(chan struct{})

	// pick available local port randomly.
	kubePortForwarder, err := kubepf.New(
		dialer,
		[]string{fmt.Sprintf("0:%d", pf.targetPort)},
		nil,
		startCh,
		&debugLogger{},
		&debugLogger{},
	)
	if err != nil {
		return fmt.Errorf("failed to init kube port forward: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- kubePortForwarder.ForwardPorts()
	}()

	select {
	case <-startCh:
	case err := <-errCh:
		return fmt.Errorf("failed to start kube port forward: %w", err)
	case <-time.After(120 * time.Second):
		return fmt.Errorf("timeout to start kube port forward")
	}

	pf.portForwarder = kubePortForwarder
	return nil
}

// GetLocalPort returns the local listener's port.
func (pf *PodPortForwarder) GetLocalPort() (uint16, error) {
	if pf.portForwarder == nil {
		return 0, fmt.Errorf("kube port forwarder doesn't start")
	}

	ports, err := pf.portForwarder.GetPorts()
	if err != nil {
		return 0, fmt.Errorf("failed to get local port: %w", err)
	}
	return ports[0].Local, nil
}

// Stop stops port forward.
func (pf *PodPortForwarder) Stop() {
	defer klog.Flush()
	if pf.portForwarder != nil {
		pf.portForwarder.Close()
	}
}

// ensurePodIsRunning is to check if the target pod is still running.
func ensurePodIsRunning(restCli kubernetes.Interface, namespace, podName string) error {
	pod, err := restCli.CoreV1().
		Pods(namespace).
		Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to ensure if %s in %s exists: %w",
			podName, namespace, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("unable to forward port because pod is not running (status=%s)", pod.Status.Phase)
	}
	return nil
}

type debugLogger struct{}

func (l *debugLogger) Write(data []byte) (int, error) {
	klog.V(2).InfoS(string(data))
	return len(data), nil
}
