// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package runner

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/metrics"
	"github.com/Azure/kperf/runner/group"
	"github.com/Azure/kperf/runner/localstore"

	"k8s.io/klog/v2"
)

// renderErrorResponse renders error into types.HTTPError format.
func renderErrorResponse(w http.ResponseWriter, code int, err error) {
	if err == nil {
		panic("unexpected error")
	}

	w.WriteHeader(code)

	data, _ := json.Marshal(types.HTTPError{
		ErrorMessage: err.Error(),
	})
	_, _ = w.Write(data)
}

// buildNetListeners returns slice of net.Listeners.
func buildNetListeners(addrs []string) (_ []net.Listener, retErr error) {
	res := make([]net.Listener, 0, len(addrs))

	defer func() {
		if retErr != nil {
			for _, l := range res {
				l.Close()
			}
		}
	}()

	for _, addr := range addrs {
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
		res = append(res, lis)
	}
	return res, nil
}

// buildRunnerGroupSummary returns aggrecated summary from runner groups' report.
func buildRunnerGroupSummary(s *localstore.Store, groups []*group.Handler) *types.RunnerMetricReport {
	totalBytes := int64(0)
	totalResp := 0
	latenciesByURL := map[string]*list.List{}
	errs := []types.ResponseError{}
	errStats := map[string]int32{}
	maxDuration := 0 * time.Second

	for idx := range groups {
		g := groups[idx]

		pods, err := g.Pods(context.TODO())
		if err != nil {
			klog.V(2).ErrorS(err, "failed to list runners", "runner-group", g.Name())
			continue
		}

		for _, pod := range pods {
			data, err := readBlob(s, pod.Name)
			if err != nil {
				klog.V(2).ErrorS(err, "failed to read report", "runner", pod.Name)
				continue
			}

			report := types.RunnerMetricReport{}

			err = json.Unmarshal(data, &report)
			if err != nil {
				klog.V(2).ErrorS(err, "failed to unmarshal", "runner", pod.Name)
				continue
			}

			// update totalReceivedBytes
			totalBytes += report.TotalReceivedBytes

			// update latencies
			for u, l := range report.LatenciesByURL {
				latencies, ok := latenciesByURL[u]
				if !ok {
					latenciesByURL[u] = list.New()
					latencies = latenciesByURL[u]
				}
				for _, v := range l {
					totalResp++
					latencies.PushBack(v)
				}
			}

			// update error stats
			mergeErrorStat(errStats, report.ErrorStats)
			errs = append(errs, report.Errors...)
			report.Errors = nil

			// update max duration
			rDur, err := time.ParseDuration(report.Duration)
			if err != nil {
				klog.V(2).ErrorS(err, "failed to parse duration", "runner",
					pod.Name, "duration", report.Duration)
			}
			if rDur > maxDuration {
				maxDuration = rDur
			}
		}
	}

	percentileLatenciesByURL := map[string][][2]float64{}

	latencies := make([]float64, 0, totalResp)
	for u, l := range latenciesByURL {
		lInSlice := listToSliceFloat64(l)

		latencies = append(latencies, lInSlice...)
		percentileLatenciesByURL[u] = metrics.BuildPercentileLatencies(lInSlice)
	}

	return &types.RunnerMetricReport{
		Total:                    totalResp,
		Errors:                   errs,
		ErrorStats:               errStats,
		Duration:                 maxDuration.String(),
		TotalReceivedBytes:       totalBytes,
		PercentileLatencies:      metrics.BuildPercentileLatencies(latencies),
		PercentileLatenciesByURL: percentileLatenciesByURL,
	}
}

// listToSliceFloat64 converts list.List into []float64.
func listToSliceFloat64(l *list.List) []float64 {
	res := make([]float64, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		res = append(res, e.Value.(float64))
	}
	return res
}

// mergeErrorStat merges two error stats.
func mergeErrorStat(s, d map[string]int32) {
	for e, n := range d {
		s[e] += n
	}
}

// readBlob reads blob data from localstore.
func readBlob(s *localstore.Store, ref string) ([]byte, error) {
	r, err := s.OpenReader(ref)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return io.ReadAll(r)
}

// isLocalhost returns true if addr is local address.
func isLocalhost(addr string) (bool, error) {
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return false, fmt.Errorf("invalid address %s: %w", addr, err)
		}
		h = addr
	}

	if len(p) == 0 {
		return false, fmt.Errorf("invalid host name format %s", addr)
	}

	if h == "localhost" {
		h = "127.0.0.1"
	}

	ip := net.ParseIP(h)
	return ip.IsLoopback(), nil
}
