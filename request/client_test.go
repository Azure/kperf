// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/metrics"
)

type transportCacheTracker struct{}

// Increment implements k8s.io/client-go/tools/metrics.TransportCreateCallsMetric interface.
func (t *transportCacheTracker) Increment(result string) {
	if result != "uncacheable" {
		panic(fmt.Errorf("unexpected use cache transport: %s", result))
	}
	fmt.Printf("transport cache: %s\n", result)
}

func init() {
	metrics.Register(metrics.RegisterOpts{
		TransportCreateCalls: &transportCacheTracker{},
	})
}

func TestNewClientShouldNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("should not reuse transport: %v", r)
		}
	}()
	_, err := NewClients("testdata/dummy_nonexistent_kubeconfig.yaml", 10)
	assert.NoError(t, err)
}
