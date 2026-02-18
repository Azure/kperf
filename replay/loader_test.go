// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseProfile(t *testing.T) {
	yamlContent := `
version: 1
description: "Test profile"
spec:
  runnerCount: 2
  connsPerRunner: 1
  clientsPerRunner: 5
  contentType: json
requests:
  - timestamp: 0
    verb: GET
    namespace: default
    resourceKind: Pod
    name: nginx
    apiPath: /api/v1/namespaces/default/pods/nginx
  - timestamp: 100
    verb: LIST
    namespace: default
    resourceKind: Pod
    apiPath: /api/v1/namespaces/default/pods
`

	profile, err := parseProfile(bytes.NewReader([]byte(yamlContent)))
	if err != nil {
		t.Fatalf("parseProfile() error = %v", err)
	}

	if profile.Version != 1 {
		t.Errorf("Version = %d, want 1", profile.Version)
	}
	if profile.Description != "Test profile" {
		t.Errorf("Description = %s, want 'Test profile'", profile.Description)
	}
	if profile.Spec.RunnerCount != 2 {
		t.Errorf("RunnerCount = %d, want 2", profile.Spec.RunnerCount)
	}
	if len(profile.Requests) != 2 {
		t.Errorf("len(Requests) = %d, want 2", len(profile.Requests))
	}
}

func TestParseProfileGzip(t *testing.T) {
	yamlContent := `
version: 1
description: "Gzip test profile"
spec:
  runnerCount: 1
  connsPerRunner: 1
  contentType: json
requests:
  - timestamp: 0
    verb: GET
    namespace: default
    resourceKind: Pod
    name: nginx
    apiPath: /api/v1/namespaces/default/pods/nginx
`

	// Gzip the content
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte(yamlContent))
	if err != nil {
		t.Fatalf("gzip write error: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close error: %v", err)
	}

	profile, err := parseProfile(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("parseProfile() error = %v", err)
	}

	if profile.Description != "Gzip test profile" {
		t.Errorf("Description = %s, want 'Gzip test profile'", profile.Description)
	}
}

func TestLoadProfileFromFile(t *testing.T) {
	yamlContent := `
version: 1
description: "File test profile"
spec:
  runnerCount: 1
  connsPerRunner: 1
  contentType: json
requests:
  - timestamp: 0
    verb: GET
    namespace: default
    resourceKind: Pod
    name: nginx
    apiPath: /api/v1/namespaces/default/pods/nginx
`

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	profile, err := LoadProfile(context.Background(), tmpFile)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}

	if profile.Description != "File test profile" {
		t.Errorf("Description = %s, want 'File test profile'", profile.Description)
	}
}

func TestLoadProfileFromGzipFile(t *testing.T) {
	yamlContent := `
version: 1
description: "Gzip file test"
spec:
  runnerCount: 1
  connsPerRunner: 1
  contentType: json
requests:
  - timestamp: 0
    verb: GET
    namespace: default
    resourceKind: Pod
    name: nginx
    apiPath: /api/v1/namespaces/default/pods/nginx
`

	// Gzip the content
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	_, err := gzWriter.Write([]byte(yamlContent))
	if err != nil {
		t.Fatalf("gzip write error: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("gzip close error: %v", err)
	}

	// Create temp file (note: extension doesn't matter, we detect by magic bytes)
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "profile.yaml")
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	profile, err := LoadProfile(context.Background(), tmpFile)
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}

	if profile.Description != "Gzip file test" {
		t.Errorf("Description = %s, want 'Gzip file test'", profile.Description)
	}
}

func TestLoadProfileInvalidFile(t *testing.T) {
	_, err := LoadProfile(context.Background(), "/nonexistent/path/profile.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestParseProfileInvalidYAML(t *testing.T) {
	invalidYAML := `
this is not valid yaml: [
`
	_, err := parseProfile(bytes.NewReader([]byte(invalidYAML)))
	if err == nil {
		t.Error("Expected error for invalid YAML, got nil")
	}
}

func TestParseProfileValidationError(t *testing.T) {
	// Missing required fields
	invalidProfile := `
version: 1
spec:
  runnerCount: 0
  connsPerRunner: 1
  contentType: json
requests:
  - timestamp: 0
    verb: GET
    namespace: default
    resourceKind: Pod
    name: nginx
    apiPath: /api/v1/namespaces/default/pods/nginx
`
	_, err := parseProfile(bytes.NewReader([]byte(invalidProfile)))
	if err == nil {
		t.Error("Expected validation error, got nil")
	}
}
