// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import "fmt"

// ReplayRequest represents a single API request to replay.
type ReplayRequest struct {
	// Timestamp is milliseconds from the start of the replay.
	Timestamp int64 `json:"timestamp" yaml:"timestamp"`
	// Verb is the HTTP verb: CREATE, GET, LIST, APPLY, DELETE, WATCH.
	Verb string `json:"verb" yaml:"verb"`
	// Namespace is the object's namespace. Empty for cluster-scoped resources.
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	// ResourceKind is the resource kind (Pod, Deployment, etc.).
	ResourceKind string `json:"resourceKind" yaml:"resourceKind"`
	// Name is the object name. Empty for LIST operations.
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// APIPath is the full HTTP path for the request.
	APIPath string `json:"apiPath" yaml:"apiPath"`
	// Body is the request body for CREATE/APPLY operations.
	Body string `json:"body,omitempty" yaml:"body,omitempty"`
	// LabelSelector is for LIST/WATCH operations.
	LabelSelector string `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
}

// Validate verifies the fields of ReplayRequest.
func (r ReplayRequest) Validate() error {
	if r.Timestamp < 0 {
		return fmt.Errorf("timestamp must be >= 0: %d", r.Timestamp)
	}

	switch r.Verb {
	case "CREATE", "GET", "LIST", "APPLY", "DELETE", "DELETECOLLECTION", "WATCH", "PATCH":
		// valid verbs
	default:
		return fmt.Errorf("unsupported verb: %s", r.Verb)
	}

	if r.ResourceKind == "" {
		return fmt.Errorf("resourceKind is required")
	}

	if r.APIPath == "" {
		return fmt.Errorf("apiPath is required")
	}

	// Name is required for specific operations (GET, DELETE, PATCH)
	// CREATE, LIST, WATCH, DELETECOLLECTION, APPLY can have empty names
	if (r.Verb == "GET" || r.Verb == "DELETE" || r.Verb == "PATCH") && r.Name == "" {
		return fmt.Errorf("name is required for %s operation", r.Verb)
	}

	// Body is required for CREATE/APPLY/PATCH operations
	if (r.Verb == "CREATE" || r.Verb == "APPLY" || r.Verb == "PATCH") && r.Body == "" {
		return fmt.Errorf("body is required for %s operation", r.Verb)
	}

	return nil
}

// ObjectKey returns a key for partitioning: "namespace/kind/name".
// For cluster-scoped resources, namespace is empty.
// For LIST/WATCH operations (empty name), includes labelSelector for better distribution.
func (r ReplayRequest) ObjectKey() string {
	if r.Name == "" && r.LabelSelector != "" && (r.Verb == "LIST" || r.Verb == "WATCH") {
		return fmt.Sprintf("%s/%s/?labelSelector=%s", r.Namespace, r.ResourceKind, r.LabelSelector)
	}
	return fmt.Sprintf("%s/%s/%s", r.Namespace, r.ResourceKind, r.Name)
}

// ReplayProfileSpec defines replay execution parameters.
type ReplayProfileSpec struct {
	// RunnerCount is the number of runners/partitions for distributed execution.
	RunnerCount int `json:"runnerCount" yaml:"runnerCount"`

	// ConnsPerRunner is the number of HTTP connections (REST clients) per runner.
	// Each connection maintains its own HTTP connection pool.
	// For HTTP/2 (default), each connection can handle ~100 concurrent requests efficiently.
	// Recommended: 5-20 for most workloads, cap at 50 per runner to avoid overwhelming API server.
	ConnsPerRunner int `json:"connsPerRunner" yaml:"connsPerRunner"`

	// ClientsPerRunner is the number of concurrent worker goroutines per runner.
	// Workers pull from the request queue and execute requests using round-robin
	// connection selection. If 0 or unspecified, defaults to ConnsPerRunner.
	// This controls concurrency and should be tuned based on expected QPS:
	//   - Low QPS (< 100): ClientsPerRunner = ConnsPerRunner (1 worker per conn)
	//   - Medium QPS (100-500): ClientsPerRunner = 3x ConnsPerRunner
	//   - High QPS (500-1000): ClientsPerRunner = 4x ConnsPerRunner
	//   - Very High QPS (> 1000): ClientsPerRunner = 5x ConnsPerRunner, or increase runnerCount
	// Workers are lightweight (goroutines), while connections are expensive (network resources).
	ClientsPerRunner int `json:"clientsPerRunner" yaml:"clientsPerRunner"`

	// ContentType defines response's content type (json or protobuf).
	ContentType ContentType `json:"contentType" yaml:"contentType"`

	// DisableHTTP2 means client will use HTTP/1.1 protocol if true.
	// Default is false (use HTTP/2 for better multiplexing).
	DisableHTTP2 bool `json:"disableHTTP2" yaml:"disableHTTP2"`
}

// Validate verifies the fields of ReplayProfileSpec.
func (s ReplayProfileSpec) Validate() error {
	if s.RunnerCount <= 0 {
		return fmt.Errorf("runnerCount must be > 0: %d", s.RunnerCount)
	}

	if s.ConnsPerRunner <= 0 {
		return fmt.Errorf("connsPerRunner must be > 0: %d", s.ConnsPerRunner)
	}

	if s.ClientsPerRunner < 0 {
		return fmt.Errorf("clientsPerRunner must be >= 0: %d", s.ClientsPerRunner)
	}

	if err := s.ContentType.Validate(); err != nil {
		return err
	}

	return nil
}

// ReplayProfile defines a replay workload.
type ReplayProfile struct {
	// Version defines the version of this object.
	Version int `json:"version" yaml:"version"`
	// Description is a string value to describe this object.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Spec defines the replay execution parameters.
	Spec ReplayProfileSpec `json:"spec" yaml:"spec"`
	// Requests is the list of requests to replay.
	Requests []ReplayRequest `json:"requests" yaml:"requests"`
}

// Validate verifies fields of ReplayProfile.
func (p ReplayProfile) Validate() error {
	if p.Version != 1 {
		return fmt.Errorf("version should be 1")
	}

	if err := p.Spec.Validate(); err != nil {
		return fmt.Errorf("spec: %w", err)
	}

	if len(p.Requests) == 0 {
		return fmt.Errorf("requests must not be empty")
	}

	for i, req := range p.Requests {
		if err := req.Validate(); err != nil {
			return fmt.Errorf("requests[%d]: %w", i, err)
		}
	}

	// Verify requests are sorted by timestamp
	for i := 1; i < len(p.Requests); i++ {
		if p.Requests[i].Timestamp < p.Requests[i-1].Timestamp {
			return fmt.Errorf("requests must be sorted by timestamp: request[%d] timestamp %d < request[%d] timestamp %d",
				i, p.Requests[i].Timestamp, i-1, p.Requests[i-1].Timestamp)
		}
	}

	return nil
}

// Duration returns the total duration of the replay in milliseconds.
func (p ReplayProfile) Duration() int64 {
	if len(p.Requests) == 0 {
		return 0
	}
	return p.Requests[len(p.Requests)-1].Timestamp
}
