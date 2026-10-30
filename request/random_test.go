// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Azure/kperf/api/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest/fake"
)

func TestRenderPatchBody(t *testing.T) {
	const tmpl = `{"metadata":{"labels":{"bench-tick":"{{.Random}}"}}}`

	// Placeholder is replaced with a random string, so the result no longer
	// contains the placeholder and differs from the raw template.
	rendered := string(renderPatchBody(tmpl))
	assert.NotContains(t, rendered, patchRandomPlaceholder)
	assert.NotEqual(t, tmpl, rendered)

	// Each render produces a distinct body.
	assert.NotEqual(t, string(renderPatchBody(tmpl)), string(renderPatchBody(tmpl)))

	// Every occurrence is replaced.
	multi := string(renderPatchBody(`{"a":"{{.Random}}","b":"{{.Random}}"}`))
	assert.NotContains(t, multi, patchRandomPlaceholder)

	// A body without the placeholder is returned unchanged (backward compatible).
	const static = `{"metadata":{"labels":{"bench-tick":"1"}}}`
	assert.Equal(t, static, string(renderPatchBody(static)))
}

func TestRandomPayload(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 8, 62, 4095, 4096, 4097, 16384} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			s := RandomPayload(n)
			if n <= 0 {
				assert.Empty(t, s)
				return
			}
			assert.Len(t, s, n)
			assert.Regexp(t, `^[a-zA-Z0-9]+$`, s)
		})
	}
	assert.NotEqual(t, RandomPayload(32), RandomPayload(32))
}

func TestRandBytes(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 8, 62, 4095, 4096, 4097, 16384} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			b, err := RandBytes(n)
			if n <= 0 {
				assert.EqualError(t, err, "length must be positive")
				assert.Nil(t, b)
				return
			}
			if !assert.NoError(t, err) {
				return
			}
			assert.Len(t, b, n)
			assert.Regexp(t, `^[a-zA-Z0-9]+$`, string(b))
		})
	}
}

func TestRequestPatchBuilderRendersUniqueBodies(t *testing.T) {
	b := newRequestPatchBuilder(&types.RequestPatch{
		KubeGroupVersionResource: types.KubeGroupVersionResource{
			Version:  "v1",
			Resource: "configmaps",
		},
		Namespace:    "bench",
		Name:         "cm",
		KeySpaceSize: 100,
		PatchType:    "merge",
		Body:         `{"metadata":{"labels":{"bench-tick":"{{.Random}}"}}}`,
	}, "", 0)

	// Each call renders a fresh random body, producing a distinct result.
	first := string(renderPatchBody(b.body))
	second := string(renderPatchBody(b.body))
	assert.NotEqual(t, first, second)
	assert.NotContains(t, first, patchRandomPlaceholder)
	assert.NotContains(t, second, patchRandomPlaceholder)
}

func TestRequestPatchBuilderRendersConcurrencySafe(t *testing.T) {
	b := newRequestPatchBuilder(&types.RequestPatch{
		KubeGroupVersionResource: types.KubeGroupVersionResource{
			Version:  "v1",
			Resource: "configmaps",
		},
		Name:      "cm",
		PatchType: "merge",
		Body:      `{"n":"{{.Random}}"}`,
	}, "", 0)

	const goroutines = 16
	const perGoroutine = 100

	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := make(map[string]struct{}, goroutines*perGoroutine)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				body := string(renderPatchBody(b.body))
				assert.NotContains(t, body, patchRandomPlaceholder)
				mu.Lock()
				seen[body] = struct{}{}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Random bodies are effectively unique across all requests.
	assert.Equal(t, goroutines*perGoroutine, len(seen))
}

func TestPodOptions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options *types.PostDelPodOptions
		labels  map[string]string
	}{
		{name: "defaults", labels: map[string]string{"app": "fake-pod"}},
		{name: "empty app", options: &types.PostDelPodOptions{
			Labels: map[string]string{"app": ""},
		}, labels: map[string]string{"app": ""}},
		{name: "other labels", options: &types.PostDelPodOptions{
			Labels: map[string]string{"team": "benchmark"},
		}, labels: map[string]string{"app": "fake-pod", "team": "benchmark"}},
		{name: "string labels", options: &types.PostDelPodOptions{
			Labels: map[string]string{"true": "false", "123": "456", "null": "null"},
		}, labels: map[string]string{"app": "fake-pod", "true": "false", "123": "456", "null": "null"}},
		{name: "custom", options: &types.PostDelPodOptions{
			Labels: map[string]string{"app": "custom-app", "team": "benchmark"}, NodeSelector: map[string]string{"agentpool": "benchmark"},
			ServiceAccountName: "custom-account", SchedulerName: "custom-scheduler",
		}, labels: map[string]string{"app": "custom-app", "team": "benchmark"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := newRequestPostDelBuilder(&types.RequestPostDel{
				KubeGroupVersionResource: types.KubeGroupVersionResource{Version: "v1", Resource: "pods"},
				Namespace:                "default",
				Pod:                      tt.options,
			}, "", 0)
			var pod corev1.Pod
			cli := &fake.RESTClient{
				GroupVersion:         schema.GroupVersion{Version: "v1"},
				NegotiatedSerializer: scheme.Codecs.WithoutConversion(),
				Client: fake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
					require.Equal(t, "POST", r.Method)
					require.NoError(t, json.NewDecoder(r.Body).Decode(&pod))
					return &http.Response{
						StatusCode: http.StatusCreated, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader("{}")),
					}, nil
				}),
			}
			_, err := builder.Build(cli).Do(context.Background())
			require.NoError(t, err)
			require.Equal(t, "default", pod.Namespace)
			options := tt.options
			if options == nil {
				options = &types.PostDelPodOptions{}
			}
			require.Equal(t, tt.labels, pod.Labels)
			require.Equal(t, options.NodeSelector, pod.Spec.NodeSelector)
			require.Equal(t, options.ServiceAccountName, pod.Spec.ServiceAccountName)
			require.Equal(t, options.SchedulerName, pod.Spec.SchedulerName)
		})
	}
}
