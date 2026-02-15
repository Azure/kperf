// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package replay

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/kperf/api/types"

	"gopkg.in/yaml.v2"
)

// LoadProfile loads a replay profile from file path or URL.
// Supports:
//   - Local file: /path/to/profile.yaml or /path/to/profile.yaml.gz
//   - PVC mount: /mnt/profile/replay.yaml.gz (Azure Files, etc.)
//   - HTTP URL: https://storage.blob.core.windows.net/profiles/replay.yaml.gz
//
// Compression is auto-detected by gzip magic bytes (0x1f 0x8b), NOT file extension.
func LoadProfile(ctx context.Context, source string) (*types.ReplayProfile, error) {
	var reader io.ReadCloser
	var err error

	// Detect source type by prefix
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		reader, err = fetchFromURL(ctx, source)
	} else {
		// Local file or PVC mount path
		reader, err = os.Open(source)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open source %s: %w", source, err)
	}
	defer reader.Close()

	return parseProfile(reader)
}

// fetchFromURL fetches content from an HTTP/HTTPS URL.
func fetchFromURL(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	return resp.Body, nil
}

// parseProfile parses a replay profile from a reader.
// Auto-detects gzip compression by magic bytes.
func parseProfile(reader io.Reader) (*types.ReplayProfile, error) {
	// Use buffered reader to peek at the first bytes
	bufReader := bufio.NewReader(reader)

	// Peek to check for gzip magic bytes (0x1f 0x8b)
	header, err := bufReader.Peek(2)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to peek header: %w", err)
	}

	var finalReader io.Reader = bufReader

	// Check for gzip magic bytes
	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		gzReader, err := gzip.NewReader(bufReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		finalReader = gzReader
	}

	var profile types.ReplayProfile
	if err := yaml.NewDecoder(finalReader).Decode(&profile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile: %w", err)
	}

	return &profile, nil
}
