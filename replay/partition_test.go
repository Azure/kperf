// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"testing"

	"github.com/Azure/kperf/api/types"
)

func TestPartitionRequests(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
		{Timestamp: 100, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns1/pods/pod-2"},
		{Timestamp: 200, Verb: "GET", Namespace: "ns2", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns2/pods/pod-1"},
		{Timestamp: 300, Verb: "GET", Namespace: "ns2", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns2/pods/pod-2"},
	}

	runnerCount := 2

	// Partition for runner 0
	partition0 := PartitionRequests(requests, runnerCount, 0)
	// Partition for runner 1
	partition1 := PartitionRequests(requests, runnerCount, 1)

	// Ensure no request is lost
	totalPartitioned := len(partition0) + len(partition1)
	if totalPartitioned != len(requests) {
		t.Errorf("Total partitioned = %d, want %d", totalPartitioned, len(requests))
	}

	// Ensure each request is in exactly one partition
	seenRequests := make(map[string]int)
	for _, req := range partition0 {
		seenRequests[req.ObjectKey()]++
	}
	for _, req := range partition1 {
		seenRequests[req.ObjectKey()]++
	}

	for key, count := range seenRequests {
		if count != 1 {
			t.Errorf("Request %s appears %d times, want 1", key, count)
		}
	}
}

func TestPartitionRequestsSingleRunner(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
		{Timestamp: 100, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns1/pods/pod-2"},
	}

	// With single runner, all requests should be assigned to runner 0
	partition := PartitionRequests(requests, 1, 0)
	if len(partition) != len(requests) {
		t.Errorf("Single runner partition = %d, want %d", len(partition), len(requests))
	}
}

func TestPartitionRequestsInvalidIndex(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
	}

	// Invalid runner index should return nil
	partition := PartitionRequests(requests, 2, -1)
	if partition != nil {
		t.Errorf("Expected nil for negative index, got %v", partition)
	}

	partition = PartitionRequests(requests, 2, 2)
	if partition != nil {
		t.Errorf("Expected nil for index >= runnerCount, got %v", partition)
	}

	partition = PartitionRequests(requests, 0, 0)
	if partition != nil {
		t.Errorf("Expected nil for zero runnerCount, got %v", partition)
	}
}

func TestPartitionRequestsPerObjectOrdering(t *testing.T) {
	// Multiple requests for the same object should go to the same runner
	// and maintain their relative ordering
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "CREATE", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods", Body: "{}"},
		{Timestamp: 100, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
		{Timestamp: 200, Verb: "DELETE", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
	}

	runnerCount := 4

	// Find which runner handles ns1/Pod/pod-1
	var foundRunner int
	for i := 0; i < runnerCount; i++ {
		partition := PartitionRequests(requests, runnerCount, i)
		if len(partition) > 0 {
			foundRunner = i
			break
		}
	}

	// All requests for the same object should be in the same partition
	partition := PartitionRequests(requests, runnerCount, foundRunner)
	if len(partition) != 3 {
		t.Errorf("Expected all 3 requests for same object in one partition, got %d", len(partition))
	}

	// Verify ordering is preserved
	for i := 1; i < len(partition); i++ {
		if partition[i].Timestamp < partition[i-1].Timestamp {
			t.Errorf("Ordering not preserved: timestamp %d < %d",
				partition[i].Timestamp, partition[i-1].Timestamp)
		}
	}
}

func TestCountByRunner(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
		{Timestamp: 100, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns1/pods/pod-2"},
		{Timestamp: 200, Verb: "GET", Namespace: "ns2", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns2/pods/pod-1"},
		{Timestamp: 300, Verb: "GET", Namespace: "ns2", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns2/pods/pod-2"},
	}

	runnerCount := 2
	counts := CountByRunner(requests, runnerCount)

	if len(counts) != runnerCount {
		t.Errorf("Expected %d count entries, got %d", runnerCount, len(counts))
	}

	totalCount := 0
	for _, c := range counts {
		totalCount += c
	}
	if totalCount != len(requests) {
		t.Errorf("Total count = %d, want %d", totalCount, len(requests))
	}
}

func TestCountByRunnerInvalid(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
	}

	if counts := CountByRunner(requests, 0); counts != nil {
		t.Errorf("Expected nil for zero runnerCount, got %v", counts)
	}
	if counts := CountByRunner(requests, -1); counts != nil {
		t.Errorf("Expected nil for negative runnerCount, got %v", counts)
	}
}

func TestAnalyzeDistribution(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
		{Timestamp: 100, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns1/pods/pod-2"},
		{Timestamp: 200, Verb: "GET", Namespace: "ns2", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns2/pods/pod-1"},
		{Timestamp: 300, Verb: "GET", Namespace: "ns2", ResourceKind: "Pod", Name: "pod-2", APIPath: "/api/v1/namespaces/ns2/pods/pod-2"},
	}

	dist := AnalyzeDistribution(requests, 2)

	if dist["total"] != 4 {
		t.Errorf("Expected total=4, got %v", dist["total"])
	}
	if dist["runnerCount"] != 2 {
		t.Errorf("Expected runnerCount=2, got %v", dist["runnerCount"])
	}
	if dist["average"] != 2.0 {
		t.Errorf("Expected average=2.0, got %v", dist["average"])
	}
}

func TestAnalyzeDistributionInvalid(t *testing.T) {
	requests := []types.ReplayRequest{
		{Timestamp: 0, Verb: "GET", Namespace: "ns1", ResourceKind: "Pod", Name: "pod-1", APIPath: "/api/v1/namespaces/ns1/pods/pod-1"},
	}

	dist := AnalyzeDistribution(requests, 0)
	if dist["total"] != 1 {
		t.Errorf("Expected total=1 for zero runnerCount, got %v", dist["total"])
	}
	if dist["average"] != 0.0 {
		t.Errorf("Expected average=0.0 for zero runnerCount, got %v", dist["average"])
	}

	dist = AnalyzeDistribution([]types.ReplayRequest{}, 2)
	if dist["total"] != 0 {
		t.Errorf("Expected total=0 for empty requests, got %v", dist["total"])
	}
}
