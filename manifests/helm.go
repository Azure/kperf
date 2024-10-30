// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package manifests

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// LoadChart returns chart from current package's embed filesystem.
func LoadChart(componentName string) (*chart.Chart, error) {
	return loadChart(FS, componentName)
}

// LoadChartFromEmbedFS returns chart from a given embed filesystem.
func LoadChartFromEmbedFS(targetFS embed.FS, componentName string) (*chart.Chart, error) {
	return loadChart(targetFS, componentName)
}

func loadChart(targetFS embed.FS, componentName string) (*chart.Chart, error) {
	files, err := getFilesFromFSRecursively(targetFS, componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chart files: %w", err)
	}

	topDir := componentName + string(filepath.Separator)
	bufFiles := make([]*loader.BufferedFile, 0, len(files))
	for _, f := range files {
		data, err := fs.ReadFile(targetFS, f)
		if err != nil {
			return nil, fmt.Errorf("failed to read file (%s): %w", f, err)
		}

		fname := filepath.ToSlash(strings.TrimPrefix(f, topDir))
		bufFiles = append(bufFiles,
			&loader.BufferedFile{
				Name: fname,
				Data: data,
			},
		)
	}
	return loader.LoadFiles(bufFiles)
}

func getFilesFromFSRecursively(targetFS embed.FS, componentName string) ([]string, error) {
	files := make([]string, 0)

	err := fs.WalkDir(targetFS, componentName,
		func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				return nil
			}
			files = append(files, path)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	return files, nil
}
