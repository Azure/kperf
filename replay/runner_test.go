// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"os"
	"testing"

	"github.com/Azure/kperf/api/types"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
)

func TestGetRunnerIndex(t *testing.T) {
	// Test without environment variable
	assert.Equal(t, 5, GetRunnerIndex(5))

	// Test with environment variable
	os.Setenv("JOB_COMPLETION_INDEX", "3")
	defer os.Unsetenv("JOB_COMPLETION_INDEX")

	assert.Equal(t, 3, GetRunnerIndex(5))

	// Test with invalid environment variable
	os.Setenv("JOB_COMPLETION_INDEX", "invalid")
	assert.Equal(t, 7, GetRunnerIndex(7))
}

func TestGroupIntoTimeBuckets(t *testing.T) {
	tests := []struct {
		name     string
		requests []types.ReplayRequest
		bucketMs int64
		want     int // expected bucket count
	}{
		{
			name:     "empty requests",
			requests: []types.ReplayRequest{},
			bucketMs: 10,
			want:     0,
		},
		{
			name: "single request",
			requests: []types.ReplayRequest{
				{Timestamp: 100, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
			},
			bucketMs: 10,
			want:     1,
		},
		{
			name: "multiple requests in same bucket",
			requests: []types.ReplayRequest{
				{Timestamp: 100, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 105, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 109, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
			},
			bucketMs: 10,
			want:     1,
		},
		{
			name: "multiple requests across buckets",
			requests: []types.ReplayRequest{
				{Timestamp: 100, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 105, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 110, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 125, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
			},
			bucketMs: 10,
			want:     3, // buckets at 100, 110, 120
		},
		{
			name: "larger bucket size",
			requests: []types.ReplayRequest{
				{Timestamp: 0, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 25, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 50, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 75, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
				{Timestamp: 100, Verb: "GET", ResourceKind: "Pod", APIPath: "/api/v1/pods"},
			},
			bucketMs: 50,
			want:     3, // buckets at 0, 50, 100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buckets := groupIntoTimeBuckets(tt.requests, tt.bucketMs)
			assert.Equal(t, tt.want, len(buckets))

			// Verify all requests are accounted for using indices
			totalReqs := 0
			for _, bucket := range buckets {
				totalReqs += bucket.endIdx - bucket.startIdx
			}
			assert.Equal(t, len(tt.requests), totalReqs, "all requests should be accounted for")

			// Verify buckets are ordered
			for i := 1; i < len(buckets); i++ {
				assert.Greater(t, buckets[i].timestamp, buckets[i-1].timestamp,
					"buckets should be ordered by timestamp")
			}

			// Verify indices are valid and non-overlapping
			for i, bucket := range buckets {
				assert.GreaterOrEqual(t, bucket.startIdx, 0, "bucket[%d] startIdx should be >= 0", i)
				assert.LessOrEqual(t, bucket.endIdx, len(tt.requests), "bucket[%d] endIdx should be <= len(requests)", i)
				assert.LessOrEqual(t, bucket.startIdx, bucket.endIdx, "bucket[%d] startIdx should be <= endIdx", i)
			}
		})
	}
}

func TestCalculateBucketSize(t *testing.T) {
	toPointers := func(reqs []types.ReplayRequest) []*types.ReplayRequest {
		ptrs := make([]*types.ReplayRequest, len(reqs))
		for i := range reqs {
			ptrs[i] = &reqs[i]
		}
		return ptrs
	}

	tests := []struct {
		name     string
		requests []*types.ReplayRequest
		want     int64
	}{
		{
			name:     "empty requests",
			requests: []*types.ReplayRequest{},
			want:     10,
		},
		{
			name: "low QPS (50 req/s)",
			requests: toPointers(func() []types.ReplayRequest {
				reqs := make([]types.ReplayRequest, 100)
				for i := range reqs {
					reqs[i] = types.ReplayRequest{
						Timestamp:    int64(i * 20), // 50 req/s
						Verb:         "GET",
						ResourceKind: "Pod",
						APIPath:      "/api/v1/pods",
					}
				}
				return reqs
			}()),
			want: 10, // Low QPS = 10ms buckets
		},
		{
			name: "medium QPS (200 req/s)",
			requests: toPointers(func() []types.ReplayRequest {
				reqs := make([]types.ReplayRequest, 1000)
				for i := range reqs {
					reqs[i] = types.ReplayRequest{
						Timestamp:    int64(i * 5), // 200 req/s
						Verb:         "GET",
						ResourceKind: "Pod",
						APIPath:      "/api/v1/pods",
					}
				}
				return reqs
			}()),
			want: 20, // Medium QPS = 20ms buckets
		},
		{
			name: "high QPS (1000 req/s)",
			requests: toPointers(func() []types.ReplayRequest {
				reqs := make([]types.ReplayRequest, 10000)
				for i := range reqs {
					reqs[i] = types.ReplayRequest{
						Timestamp:    int64(i), // 1000 req/s
						Verb:         "GET",
						ResourceKind: "Pod",
						APIPath:      "/api/v1/pods",
					}
				}
				return reqs
			}()),
			want: 50, // High QPS = 50ms buckets
		},
		{
			name: "very high QPS (5000 req/s)",
			requests: toPointers(func() []types.ReplayRequest {
				reqs := make([]types.ReplayRequest, 50000)
				for i := range reqs {
					reqs[i] = types.ReplayRequest{
						Timestamp:    int64(i / 5), // 5000 req/s
						Verb:         "GET",
						ResourceKind: "Pod",
						APIPath:      "/api/v1/pods",
					}
				}
				return reqs
			}()),
			want: 100, // Very high QPS = 100ms buckets
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBucketSize(tt.requests)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewRunner(t *testing.T) {
	tests := []struct {
		name            string
		workerCount     int
		connsCount      int
		expectedWorkers int
	}{
		{
			name:            "default workers to connections",
			workerCount:     0,
			connsCount:      5,
			expectedWorkers: 5,
		},
		{
			name:            "explicit worker count",
			workerCount:     10,
			connsCount:      5,
			expectedWorkers: 10,
		},
		{
			name:            "no connections defaults to 1",
			workerCount:     0,
			connsCount:      0,
			expectedWorkers: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use nil rest.Interface values — safe since we don't call Run()
			restClis := make([]rest.Interface, tt.connsCount)

			runner := NewRunner(0, nil, restClis, "https://k8s.example.com", tt.workerCount, "", nil)

			assert.Equal(t, tt.expectedWorkers, runner.workerCount)
		})
	}
}


// BenchmarkGroupIntoTimeBuckets benchmarks the time bucket grouping with different request counts.
func BenchmarkGroupIntoTimeBuckets(b *testing.B) {
	// Create sample requests
	makeRequests := func(count int) []types.ReplayRequest {
		reqs := make([]types.ReplayRequest, count)
		for i := range reqs {
			reqs[i] = types.ReplayRequest{
				Timestamp:    int64(i),
				Verb:         "GET",
				ResourceKind: "Pod",
				APIPath:      "/api/v1/pods",
			}
		}
		return reqs
	}

	benchmarks := []struct {
		name  string
		count int
	}{
		{"100_requests", 100},
		{"1000_requests", 1000},
		{"10000_requests", 10000},
	}

	for _, bm := range benchmarks {
		requests := makeRequests(bm.count)
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = groupIntoTimeBuckets(requests, 10)
			}
		})
	}
}
