// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"sync"
	"testing"

	"github.com/Azure/kperf/api/types"

	"github.com/stretchr/testify/assert"
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
	const n = 8
	s := randomPayload(n)
	assert.Len(t, s, n)
	for _, r := range s {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		assert.True(t, isAlnum, "unexpected char %q", r)
	}
	// n <= 0 returns an empty string.
	assert.Equal(t, "", randomPayload(0))
	// Two calls should not collide.
	assert.NotEqual(t, randomPayload(n), randomPayload(n))
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
	first := string(renderPatchBody(b.rawBody))
	second := string(renderPatchBody(b.rawBody))
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
				body := string(renderPatchBody(b.rawBody))
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
