// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"fmt"

	"github.com/Azure/kperf/portforward"
)

var (
	// runnerGroupReleaseLabels is used to mark that helm chart release
	// is managed by kperf.
	runnerGroupReleaseLabels = map[string]string{
		"runnergroups.kperf.io/managed": "true",
	}
)

const (
	// runnerGroupServerChartName should be aligned with ../manifests/runnergroup/server.
	runnerGroupServerChartName = "runnergroup/server"

	// runnerGroupServerReleaseName is the helm releas name for runner groups's server.
	runnerGroupServerReleaseName = "runnergroup-server"

	// runnerGroupServerPort should be aligned with ../manifests/runnergroup/server/templates/pod.yaml.
	runnerGroupServerPort uint16 = 8080

	// runnerGroupReleaseNamespace is used to host runner groups.
	runnerGroupReleaseNamespace = "runnergroups-kperf-io"
)

// initPortForwardToServer creates local listener to forward traffic to runner
// groups' server.
func initPortForwardToServer(kubecfgPath string) (_localhost string, _cleanup func(), retErr error) {
	pf, err := portforward.NewPodPortForwarder(
		kubecfgPath,
		runnerGroupReleaseNamespace,
		runnerGroupServerReleaseName,
		runnerGroupServerPort,
	)
	if err != nil {
		return "", nil, fmt.Errorf("failed to init pod portforward: %w", err)
	}
	defer func() {
		if retErr != nil {
			pf.Stop()
		}
	}()

	if err = pf.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to start pod port forward: %w", err)
	}

	localPort, err := pf.GetLocalPort()
	if err != nil {
		return "", nil, fmt.Errorf("failed to get local port: %w", err)
	}
	return fmt.Sprintf("localhost:%d", localPort), pf.Stop, nil
}
