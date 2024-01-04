package manifests

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// LoadChart returns chart from embed filesystem.
func LoadChart(componentName string) (*chart.Chart, error) {
	files, err := getFilesFromFSRecursively(componentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get chart files: %w", err)
	}

	topDir := componentName + string(filepath.Separator)
	bufFiles := make([]*loader.BufferedFile, 0, len(files))
	for _, f := range files {
		data, err := fs.ReadFile(FS, f)
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

func getFilesFromFSRecursively(componentName string) ([]string, error) {
	files := make([]string, 0)

	err := fs.WalkDir(FS, componentName,
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
