// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"fmt"
	"sync"
)

// deployRunnerGroups deploys runner groups.
//
// FIXME(weifu): should decouple URL from runner group.
func (s *Server) deployRunnerGroups() error {
	targetAddr, err := s.firstNonLocalAddr()
	if err != nil {
		return err
	}

	uploadURL := fmt.Sprintf("http://%s/v1/runnergroups/$(POD_NAME)/result", targetAddr)

	var wg sync.WaitGroup
	errCh := make(chan error, len(s.groups))
	for idx := range s.groups {
		wg.Add(1)
		g := s.groups[idx]
		go func() {
			defer wg.Done()

			errCh <- g.Deploy(context.Background(), uploadURL)
		}()
	}
	wg.Wait()

	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// waitForRunnerGroups watches all runner groups and marks summary ready until
// all runner groups finish.
func (s *Server) waitForRunnerGroups() {
	var wg sync.WaitGroup

	for idx := range s.groups {
		wg.Add(1)
		g := s.groups[idx]
		go func() {
			defer wg.Done()

			// FIXME(weifu): remove panic here
			if err := g.Wait(context.TODO()); err != nil {
				panic(err)
			}
		}()
	}
	wg.Wait()

	s.report = buildRunnerGroupSummary(s.store, s.groups)
	close(s.readyCh)
}

// firstNoLocalAddr returns first non-local address.
func (s *Server) firstNonLocalAddr() (string, error) {
	for _, lis := range s.listeners {
		addr := lis.Addr().String()

		local, err := isLocalhost(addr)
		if err != nil {
			return "", err
		}

		if !local {
			return addr, nil
		}
	}
	return "", fmt.Errorf("there is no non-local address")
}
