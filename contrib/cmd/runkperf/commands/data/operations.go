// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package data

import (
	"fmt"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DefaultPageSize is the list page size for Event, Secret, and Pod data commands.
const DefaultPageSize int64 = 300

// ListPaginated visits items from successive pages until the continuation token
// is empty. List and visit errors stop traversal immediately.
func ListPaginated[T any](opts metav1.ListOptions, list func(metav1.ListOptions) ([]T, string, error), visit func(T) error) error {
	if opts.Limit <= 0 {
		opts.Limit = DefaultPageSize
	}
	for {
		items, next, err := list(opts)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := visit(item); err != nil {
				return err
			}
		}
		if next == "" {
			return nil
		}
		opts.Continue = next
	}
}

// BatchDelete collects all names before deleting in fixed concurrent batches.
// resource is the singular resource name used in messages. Missing objects are
// ignored; other errors stop deletion after the current batch finishes.
func BatchDelete(resource, namespace string, groupSize int, list func(func(string) error) error, deleteFn func(string) error) error {
	if groupSize <= 0 {
		return fmt.Errorf("group-size must be greater than 0")
	}
	var names []string
	if err := list(func(name string) error {
		names = append(names, name)
		return nil
	}); err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Printf("No %ss found in namespace %s, nothing to delete\n", resource, namespace)
		return nil
	}
	for start := 0; start < len(names); start += groupSize {
		g := new(errgroup.Group)
		for i := start; i < len(names) && i-start < groupSize; i++ {
			name := names[i]
			g.Go(func() error {
				if err := deleteFn(name); err != nil && !errors.IsNotFound(err) {
					return fmt.Errorf("failed to delete %s %s: %w", resource, name, err)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
	}
	return nil
}
