// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/runner/group"
	"github.com/Azure/kperf/runner/localstore"

	"github.com/gorilla/mux"
)

// Server is to deploy runner groups and expose endpoints for runner report.
type Server struct {
	store     *localstore.Store
	listeners []net.Listener
	groups    []*group.Handler
	readyCh   chan struct{}
	report    *types.RunnerMetricReport
}

// NewServer returns new instance of server.
func NewServer(dataDir string, addrs []string, groups ...*group.Handler) (*Server, error) {
	s, err := localstore.NewStore(dataDir)
	if err != nil {
		return nil, err
	}

	listeners, err := buildNetListeners(addrs)
	if err != nil {
		return nil, err
	}

	return &Server{
		listeners: listeners,
		groups:    groups,
		store:     s,
		readyCh:   make(chan struct{}),
	}, nil
}

// Run is to expose endpoints.
func (s *Server) Run() error {
	if err := s.deployRunnerGroups(); err != nil {
		return fmt.Errorf("failed to deploy runner group %w", err)
	}

	go s.waitForRunnerGroups()

	r := mux.NewRouter()
	// NOTE: Please update ./runnergroup_list.go if endpoint has been changed.
	r.HandleFunc("/v1/runnergroups", s.listRunnerGroupsHandler).Methods("GET")
	// NOTE: Please update ./runnergroup_result.go if endpoint has been changed.
	r.HandleFunc("/v1/runnergroups/summary", s.getRunnerGroupsSummary).Methods("GET")
	r.HandleFunc("/v1/runnergroups/{runner_name}/result", s.postRunnerGroupsRunnerResult).Methods("POST")

	errCh := make(chan error, len(s.listeners))
	var wg sync.WaitGroup
	for _, lis := range s.listeners {
		wg.Add(1)
		go func(l net.Listener) {
			defer wg.Done()
			//nolint:gosec
			errCh <- http.Serve(l, r)
		}(lis)
	}
	wg.Wait()

	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// listRunnerGroupsHandler lists all the runner groups.
func (s *Server) listRunnerGroupsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	res := make([]*types.RunnerGroup, 0, len(s.groups))
	for _, g := range s.groups {
		res = append(res, g.Info(ctx))
	}

	data, _ := json.Marshal(res)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// getRunnerGroupsSummary returns summary report.
func (s *Server) getRunnerGroupsSummary(w http.ResponseWriter, r *http.Request) {
	wait := r.URL.Query().Has("wait")

	select {
	case <-s.readyCh:
	default:
		if !wait {
			renderErrorResponse(w, http.StatusNotFound, fmt.Errorf("summary is not ready"))
			return
		}
	}

	ctx := r.Context()
	select {
	case <-s.readyCh:
	case <-ctx.Done():
		renderErrorResponse(w, http.StatusRequestTimeout, fmt.Errorf("request has been canceled"))
		return
	}

	data, _ := json.Marshal(s.report)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// postRunnerGroupsRunnerResult receives summary result from runner.
func (s *Server) postRunnerGroupsRunnerResult(w http.ResponseWriter, r *http.Request) {
	runnerName := mux.Vars(r)["runner_name"]
	ctx := r.Context()

	var found = false
	var err error
	for _, g := range s.groups {
		found, err = g.IsControlled(ctx, runnerName)
		if err != nil {
			renderErrorResponse(w, http.StatusInternalServerError, err)
			return
		}
		if found {
			break
		}
	}

	if !found {
		renderErrorResponse(w, http.StatusNotFound, fmt.Errorf("no such runner %s", runnerName))
		return
	}

	writer, err := s.store.OpenWriter()
	if err != nil {
		renderErrorResponse(w, http.StatusInternalServerError, err)
		return
	}
	defer writer.Close()

	_, err = io.Copy(writer, r.Body)
	if err != nil {
		renderErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	err = writer.Commit(runnerName)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, localstore.ErrAlreadyExists) {
			code = http.StatusConflict
		}
		renderErrorResponse(w, code, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
