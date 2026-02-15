// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"testing"

	"github.com/Azure/kperf/api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerbToHTTPMethod(t *testing.T) {
	tests := []struct {
		verb string
		want string
	}{
		{"CREATE", "POST"},
		{"GET", "GET"},
		{"LIST", "LIST"},
		{"APPLY", "PATCH"},
		{"DELETE", "DELETE"},
		{"DELETECOLLECTION", "DELETE"},
		{"WATCH", "WATCH"},
		{"PATCH", "PATCH"},
		{"UNKNOWN", "GET"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			got := verbToHTTPMethod(tt.verb)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMaskLastPathSegment(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/v1/namespaces/default/pods/nginx", "/api/v1/namespaces/default/pods/:name"},
		{"/api/v1/namespaces/default/pods", "/api/v1/namespaces/default/:name"},
		{"/api/v1/nodes/node-1", "/api/v1/nodes/:name"},
		{"/single", "/:name"},
		{"", ""},
		{"/trailing/", "/:name"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := maskLastPathSegment(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewReplayRequesterURLBuilding(t *testing.T) {
	tests := []struct {
		name       string
		req        types.ReplayRequest
		baseURL    string
		wantMethod string
	}{
		{
			name: "GET request",
			req: types.ReplayRequest{
				Verb:         "GET",
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/namespaces/default/pods/nginx",
			},
			baseURL:    "https://kubernetes.default.svc",
			wantMethod: "GET",
		},
		{
			name: "CREATE request",
			req: types.ReplayRequest{
				Verb:         "CREATE",
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/namespaces/default/pods",
				Body:         `{"apiVersion":"v1","kind":"Pod"}`,
			},
			baseURL:    "https://kubernetes.default.svc",
			wantMethod: "POST",
		},
		{
			name: "DELETE request",
			req: types.ReplayRequest{
				Verb:         "DELETE",
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/namespaces/default/pods/nginx",
			},
			baseURL:    "https://kubernetes.default.svc",
			wantMethod: "DELETE",
		},
		{
			name: "LIST request",
			req: types.ReplayRequest{
				Verb:         "LIST",
				Namespace:    "default",
				ResourceKind: "Pod",
				APIPath:      "/api/v1/namespaces/default/pods",
			},
			baseURL:    "https://kubernetes.default.svc",
			wantMethod: "LIST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requester, err := NewReplayRequester(tt.req, nil, tt.baseURL)
			require.NoError(t, err)

			assert.Equal(t, tt.wantMethod, requester.Method())

			expectedURL := tt.baseURL + tt.req.APIPath
			assert.Equal(t, expectedURL, requester.URL().String())
		})
	}
}

func TestNewReplayRequesterPATCH(t *testing.T) {
	tests := []struct {
		name       string
		apiPath    string
		wantMethod string
	}{
		{
			name:       "PATCH built-in resource",
			apiPath:    "/api/v1/namespaces/default/pods/nginx",
			wantMethod: "PATCH",
		},
		{
			name:       "PATCH CRD resource",
			apiPath:    "/apis/custom.example.com/v1/namespaces/default/widgets/widget-1",
			wantMethod: "PATCH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := types.ReplayRequest{
				Verb:         "PATCH",
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      tt.apiPath,
				Body:         `{"metadata":{"labels":{"app":"test"}}}`,
			}
			requester, err := NewReplayRequester(req, nil, "https://k8s.example.com")
			require.NoError(t, err)
			assert.Equal(t, tt.wantMethod, requester.Method())
		})
	}
}

func TestNewReplayRequesterWATCHWithLabelSelector(t *testing.T) {
	req := types.ReplayRequest{
		Verb:          "WATCH",
		Namespace:     "default",
		ResourceKind:  "Pod",
		APIPath:       "/api/v1/namespaces/default/pods",
		LabelSelector: "app=nginx",
	}
	requester, err := NewReplayRequester(req, nil, "https://k8s.example.com")
	require.NoError(t, err)

	assert.Equal(t, "WATCH", requester.Method())

	q := requester.URL().Query()
	assert.Equal(t, "true", q.Get("watch"))
	assert.Equal(t, "app=nginx", q.Get("labelSelector"))
}

func TestNewReplayRequesterWATCHWithoutLabelSelector(t *testing.T) {
	req := types.ReplayRequest{
		Verb:         "WATCH",
		Namespace:    "default",
		ResourceKind: "Pod",
		APIPath:      "/api/v1/namespaces/default/pods",
	}
	requester, err := NewReplayRequester(req, nil, "https://k8s.example.com")
	require.NoError(t, err)

	// watch=true should still be set even without labelSelector
	q := requester.URL().Query()
	assert.Equal(t, "true", q.Get("watch"))
}

func TestNewReplayRequesterDELETECOLLECTION(t *testing.T) {
	req := types.ReplayRequest{
		Verb:         "DELETECOLLECTION",
		Namespace:    "default",
		ResourceKind: "Pod",
		APIPath:      "/api/v1/namespaces/default/pods",
	}
	requester, err := NewReplayRequester(req, nil, "https://k8s.example.com")
	require.NoError(t, err)
	assert.Equal(t, "DELETE", requester.Method())
}

func TestNewReplayRequesterAPPLY(t *testing.T) {
	req := types.ReplayRequest{
		Verb:         "APPLY",
		Namespace:    "default",
		ResourceKind: "Deployment",
		Name:         "nginx",
		APIPath:      "/apis/apps/v1/namespaces/default/deployments/nginx",
		Body:         `{"apiVersion":"apps/v1","kind":"Deployment"}`,
	}
	requester, err := NewReplayRequester(req, nil, "https://k8s.example.com")
	require.NoError(t, err)
	assert.Equal(t, "PATCH", requester.Method())

	// APPLY should mask the object name for metrics aggregation
	assert.Contains(t, requester.MaskedURL().Path, ":name")
}

func TestNewReplayRequesterMalformedURL(t *testing.T) {
	// Test that & is replaced with ? when no ? is present
	req := types.ReplayRequest{
		Verb:         "LIST",
		Namespace:    "default",
		ResourceKind: "Pod",
		APIPath:      "/api/v1/namespaces/default/pods&limit=100&continue=abc",
	}
	requester, err := NewReplayRequester(req, nil, "https://k8s.example.com")
	require.NoError(t, err)

	q := requester.URL().Query()
	assert.Equal(t, "100", q.Get("limit"))
	assert.Equal(t, "abc", q.Get("continue"))
}

func TestIsBuiltinAPIPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Core API
		{"/api/v1/namespaces/default/pods/nginx", true},
		{"/api/v1/nodes/node-1", true},
		// Built-in extension APIs (no dots in group)
		{"/apis/apps/v1/namespaces/default/deployments/nginx", true},
		{"/apis/batch/v1/namespaces/default/jobs/my-job", true},
		{"/apis/policy/v1/poddisruptionbudgets", true},
		// Built-in extension APIs (*.k8s.io)
		{"/apis/networking.k8s.io/v1/ingresses", true},
		{"/apis/rbac.authorization.k8s.io/v1/clusterroles/admin", true},
		{"/apis/storage.k8s.io/v1/storageclasses", true},
		// CRDs (custom domains)
		{"/apis/stable.example.com/v1/crontabs", false},
		{"/apis/custom.mycompany.io/v1alpha1/widgets", false},
		// Edge cases
		{"/other/path", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isBuiltinAPIPath(tt.path)
			assert.Equal(t, tt.want, got, "isBuiltinAPIPath(%q)", tt.path)
		})
	}
}

