// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Azure/kperf/api/types"
)

// ListRunnerGroups lists RunnerGroups from server.
func ListRunnerGroups(ctx context.Context, kubeCfgPath string) ([]*types.RunnerGroup, error) {
	host, done, err := initPortForwardToServer(kubeCfgPath)
	if err != nil {
		return nil, err
	}
	defer done()

	targetURL := fmt.Sprintf("http://%s/v1/runnergroups", host)

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to init GET request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to access %s by portforward: %w", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errInRaw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read error message when http code = %v: %w",
				resp.Status, err)
		}

		herr := types.HTTPError{}
		err = json.Unmarshal(errInRaw, &herr)
		if err != nil {
			return nil, fmt.Errorf("failed to get error when http code = %v: %w",
				resp.Status, err)
		}
		return nil, herr
	}

	dataInRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	res := []*types.RunnerGroup{}
	err = json.Unmarshal(dataInRaw, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to get RunnerGroup slice: %w\n\n%s",
			err, string(dataInRaw))
	}
	return res, nil
}
