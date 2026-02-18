// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"hash/fnv"

	"github.com/Azure/kperf/api/types"
)

// PartitionRequests partitions replay requests by deterministic hash partitioning.
// Each runner gets requests where FNV-1a(objectKey) % runnerCount == runnerIndex.
// This ensures per-object ordering while distributing load across runners.
func PartitionRequests(requests []types.ReplayRequest, runnerCount, runnerIndex int) []types.ReplayRequest {
	if runnerCount <= 0 {
		return nil
	}
	if runnerIndex < 0 || runnerIndex >= runnerCount {
		return nil
	}

	// Pre-allocate with estimated capacity
	result := make([]types.ReplayRequest, 0, len(requests)/runnerCount+1)

	for _, req := range requests {
		if assignRunner(req.ObjectKey(), runnerCount) == runnerIndex {
			result = append(result, req)
		}
	}

	return result
}

// assignRunner determines which runner should handle a request based on object key.
// Uses FNV-1a hash for consistent, fast hashing.
func assignRunner(objectKey string, runnerCount int) int {
	h := fnv.New64a()
	h.Write([]byte(objectKey))
	return int(h.Sum64() % uint64(runnerCount))
}

// CountByRunner returns a slice with the count of requests per runner.
// Useful for load distribution analysis.
func CountByRunner(requests []types.ReplayRequest, runnerCount int) []int {
	if runnerCount <= 0 {
		return nil
	}
	counts := make([]int, runnerCount)
	for _, req := range requests {
		idx := assignRunner(req.ObjectKey(), runnerCount)
		counts[idx]++
	}
	return counts
}

// AnalyzeDistribution returns statistics about request distribution across runners.
func AnalyzeDistribution(requests []types.ReplayRequest, runnerCount int) map[string]interface{} {
	if runnerCount <= 0 {
		return map[string]interface{}{
			"total":       len(requests),
			"runnerCount": runnerCount,
			"average":     0.0,
			"min":         0,
			"max":         0,
			"imbalance":   0.0,
			"perRunner":   []int{},
		}
	}

	counts := CountByRunner(requests, runnerCount)

	total := len(requests)
	avg := float64(total) / float64(runnerCount)

	maxCount := counts[0]
	minCount := counts[0]
	for _, c := range counts[1:] {
		maxCount = max(maxCount, c)
		minCount = min(minCount, c)
	}

	imbalance := 0.0
	if avg > 0 {
		imbalance = float64(maxCount-minCount) / avg * 100 // % imbalance
	}

	return map[string]interface{}{
		"total":       total,
		"runnerCount": runnerCount,
		"average":     avg,
		"min":         minCount,
		"max":         maxCount,
		"imbalance":   imbalance,
		"perRunner":   counts,
	}
}
