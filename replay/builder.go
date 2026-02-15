// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/kperf/api/types"

	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

// ReplayRequester builds and executes replay requests using rest.Interface.
// This is consistent with how existing kperf handles requests.
type ReplayRequester struct {
	method              string
	verb                string
	url                 *url.URL
	maskedURL           *url.URL
	body                []byte
	timeout             time.Duration
	restCli             rest.Interface
	apiPath             string
	connectionLatency   float64 // Time to establish connection (for WATCH)
}

// NewReplayRequester creates a new ReplayRequester from a ReplayRequest.
func NewReplayRequester(req types.ReplayRequest, restCli rest.Interface, baseURL string) (*ReplayRequester, error) {
	// Build full URL for metrics tracking
	apiPath := req.APIPath
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}

	// Fix malformed URLs: replace first & with ? if no ? is present
	if !strings.Contains(apiPath, "?") && strings.Contains(apiPath, "&") {
		apiPath = strings.Replace(apiPath, "&", "?", 1)
	}

	baseURL = strings.TrimSuffix(baseURL, "/")
	fullURL := baseURL + apiPath

	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL %q: %w", fullURL, err)
	}

	// Add query parameters for LIST/WATCH
	q := parsedURL.Query()
	if req.LabelSelector != "" && (req.Verb == "LIST" || req.Verb == "WATCH") {
		q.Set("labelSelector", req.LabelSelector)
	}
	if req.Verb == "WATCH" {
		q.Set("watch", "true")
	}
	if len(q) > 0 {
		parsedURL.RawQuery = q.Encode()
	}

	// Create masked URL for metrics aggregation.
	// Mask the last path segment (object name) for mutation verbs that target specific objects.
	// Consistent with request/requester.go: only DELETE, PATCH, and APPLY are masked.
	// GET is not masked to preserve per-object latency metrics in replay analysis.
	// CREATE uses the collection path (e.g. /api/v1/pods), so no masking needed.
	maskedURL := *parsedURL
	if req.Verb == "DELETE" || req.Verb == "PATCH" || req.Verb == "APPLY" {
		maskedURL.Path = maskLastPathSegment(maskedURL.Path)
	}

	return &ReplayRequester{
		method:    verbToHTTPMethod(req.Verb),
		verb:      req.Verb,
		url:       parsedURL,
		maskedURL: &maskedURL,
		body:      []byte(req.Body),
		restCli:   restCli,
		apiPath:   apiPath,
	}, nil
}

// Method returns the HTTP method.
func (r *ReplayRequester) Method() string {
	return r.method
}

// URL returns the request URL.
func (r *ReplayRequester) URL() *url.URL {
	return r.url
}

// MaskedURL returns the masked URL for metrics aggregation.
func (r *ReplayRequester) MaskedURL() *url.URL {
	return r.maskedURL
}

// Timeout sets the request timeout.
func (r *ReplayRequester) Timeout(timeout time.Duration) {
	r.timeout = timeout
}

// ConnectionLatency returns the time to establish the connection.
// For WATCH operations, this is the time until the first response is received.
// For other operations, this equals the total latency.
func (r *ReplayRequester) ConnectionLatency() float64 {
	return r.connectionLatency
}

// Do executes the request and returns the bytes received.
func (r *ReplayRequester) Do(ctx context.Context) (int64, error) {
	// Build the request using rest.Interface (same pattern as existing kperf)
	var req *rest.Request

	// Parse path components from URL path (without query string)
	pathParts := strings.Split(strings.Trim(r.url.Path, "/"), "/")

	switch r.verb {
	case "GET", "LIST", "WATCH":
		req = r.restCli.Get().AbsPath(pathParts...)

	case "CREATE":
		req = r.restCli.Post().AbsPath(pathParts...).Body(r.body)

	case "DELETE":
		req = r.restCli.Delete().AbsPath(pathParts...)

	case "DELETECOLLECTION":
		// DeleteCollection is a DELETE with collection-level path (e.g., /api/v1/namespaces/foo/pods)
		req = r.restCli.Delete().AbsPath(pathParts...)

	case "PATCH":
		// Choose patch type based on resource type to avoid 415 errors.
		// Built-in K8s resources support strategic merge patch.
		// CRDs only support merge patch and json patch.
		patchType := apitypes.MergePatchType // Default: works for everything

		if isBuiltinAPIPath(r.url.Path) {
			patchType = apitypes.StrategicMergePatchType
		}

		req = r.restCli.Patch(patchType).AbsPath(pathParts...).Body(r.body)

	case "APPLY":
		// Server-side apply uses apply patch type
		req = r.restCli.Patch(apitypes.ApplyPatchType).AbsPath(pathParts...).
			Body(r.body).
			Param("fieldManager", "kperf-replay")

	default:
		req = r.restCli.Get().AbsPath(pathParts...)
	}

	// Add all query parameters from the original URL
	for key, values := range r.url.Query() {
		for _, value := range values {
			req = req.Param(key, value)
		}
	}

	// Set timeout (this may override timeout from query params, which is fine)
	if r.timeout > 0 {
		req = req.Timeout(r.timeout)
	}

	// Execute and read response
	// For WATCH operations, track connection establishment time separately
	connectionStart := time.Now()
	respBody, err := req.Stream(ctx)
	connectionEstablished := time.Now()

	// Store connection latency (time to get first response)
	r.connectionLatency = connectionEstablished.Sub(connectionStart).Seconds()

	if err != nil {
		return 0, err
	}
	defer respBody.Close()

	// Discard body but count bytes
	return io.Copy(io.Discard, respBody)
}

// verbToHTTPMethod converts a replay verb to HTTP method.
func verbToHTTPMethod(verb string) string {
	switch verb {
	case "CREATE":
		return "POST"
	case "GET":
		return "GET"
	case "LIST":
		return "LIST"
	case "APPLY":
		return "PATCH"
	case "DELETE", "DELETECOLLECTION":
		return "DELETE"
	case "WATCH":
		return "WATCH"
	case "PATCH":
		return "PATCH"
	default:
		klog.Warningf("Unknown verb %q, defaulting to GET", verb)
		return "GET"
	}
}

// maskLastPathSegment replaces the last path segment with :name for aggregation.
func maskLastPathSegment(path string) string {
	if path == "" {
		return path
	}

	// Remove trailing slash if present
	path = strings.TrimSuffix(path, "/")

	lastSlash := strings.LastIndex(path, "/")
	if lastSlash == -1 {
		return ":name"
	}

	return path[:lastSlash+1] + ":name"
}

// isBuiltinAPIPath returns true if the API path targets a built-in Kubernetes resource
// (which supports StrategicMergePatch). CRD paths use custom domain groups (e.g.,
// /apis/custom.example.com/v1/...) and only support MergePatch.
func isBuiltinAPIPath(path string) bool {
	// Core API: /api/v1/...
	if strings.HasPrefix(path, "/api/") {
		return true
	}
	// Extension APIs: /apis/<group>/...
	// Built-in groups either have no dots (apps, batch, policy) or end with .k8s.io
	// CRDs use custom domains like custom.example.com
	if !strings.HasPrefix(path, "/apis/") {
		return false
	}
	// Extract the API group from /apis/<group>/...
	rest := strings.TrimPrefix(path, "/apis/")
	slashIdx := strings.Index(rest, "/")
	if slashIdx == -1 {
		return false
	}
	group := rest[:slashIdx]
	return !strings.Contains(group, ".") || strings.HasSuffix(group, ".k8s.io")
}
