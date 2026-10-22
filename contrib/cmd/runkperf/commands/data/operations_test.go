// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package data

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestListPaginated(t *testing.T) {
	for _, limit := range []int64{-1, 0, 2} {
		t.Run(strconv.FormatInt(limit, 10), func(t *testing.T) {
			expectedLimit := limit
			if limit <= 0 {
				expectedLimit = DefaultPageSize
			}
			pages := []struct {
				items []int
				next  string
			}{{[]int{1, 2}, "second"}, {nil, "third"}, {[]int{3}, ""}}
			tokens := []string{"", "second", "third"}
			calls := 0
			var visited []int
			err := ListPaginated(metav1.ListOptions{LabelSelector: "app=runkperf", Limit: limit},
				func(opts metav1.ListOptions) ([]int, string, error) {
					require.Less(t, calls, len(pages))
					assert.Equal(t, "app=runkperf", opts.LabelSelector)
					assert.Equal(t, expectedLimit, opts.Limit)
					assert.Equal(t, tokens[calls], opts.Continue)
					page := pages[calls]
					calls++
					return page.items, page.next, nil
				}, func(item int) error {
					visited = append(visited, item)
					return nil
				})
			require.NoError(t, err)
			assert.Equal(t, 3, calls)
			assert.Equal(t, []int{1, 2, 3}, visited)
		})
	}
}

func TestListPaginatedErrors(t *testing.T) {
	sentinel := errors.New("failed")
	t.Run("list", func(t *testing.T) {
		err := ListPaginated(metav1.ListOptions{}, func(metav1.ListOptions) ([]int, string, error) {
			return nil, "", sentinel
		}, func(int) error {
			t.Fatal("must not visit items after a list error")
			return nil
		})
		assert.ErrorIs(t, err, sentinel)
	})
	t.Run("visit", func(t *testing.T) {
		calls, visits := 0, 0
		err := ListPaginated(metav1.ListOptions{}, func(metav1.ListOptions) ([]int, string, error) {
			calls++
			return []int{1, 2}, "next", nil
		}, func(int) error {
			visits++
			return sentinel
		})
		assert.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, calls)
		assert.Equal(t, 1, visits)
	})
}

func enumerateNames(names ...string) func(func(string) error) error {
	return func(visit func(string) error) error {
		for _, name := range names {
			if err := visit(name); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestBatchDeleteWaves(t *testing.T) {
	for _, groupSize := range []int{1, 2, 10} {
		t.Run(strconv.Itoa(groupSize), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			names := []string{"0", "1", "2", "3", "4"}
			releases := make([]chan struct{}, (len(names)+groupSize-1)/groupSize)
			for i := range releases {
				releases[i] = make(chan struct{})
			}
			started := make(chan int, len(names))
			done := make(chan error, 1)
			var completed, active atomic.Int64
			listed := false
			go func() {
				done <- BatchDelete("pod", "default", groupSize, func(visit func(string) error) error {
					err := enumerateNames(names...)(visit)
					listed = true
					return err
				}, func(name string) error {
					assert.True(t, listed, "listing must finish before deletion")
					idx, err := strconv.Atoi(name)
					if err != nil {
						return err
					}
					wave := idx / groupSize
					assert.GreaterOrEqual(t, completed.Load(), int64(wave*groupSize), "previous wave must finish")
					assert.LessOrEqual(t, active.Add(1), int64(groupSize))
					defer active.Add(-1)
					started <- idx
					select {
					case <-releases[wave]:
						completed.Add(1)
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
			}()
			for wave := range releases {
				count := groupSize
				if remaining := len(names) - wave*groupSize; remaining < count {
					count = remaining
				}
				for i := 0; i < count; i++ {
					select {
					case idx := <-started:
						assert.Equal(t, wave, idx/groupSize)
					case <-ctx.Done():
						t.Fatal("timed out waiting for deletion batch")
					}
				}
				close(releases[wave])
			}
			require.NoError(t, <-done)
			assert.Equal(t, int64(len(names)), completed.Load())
		})
	}
}

func TestBatchDeleteErrors(t *testing.T) {
	sentinel := errors.New("denied")
	for _, groupSize := range []int{-1, 0} {
		err := BatchDelete("pod", "default", groupSize, func(func(string) error) error {
			t.Fatal("invalid batch size must be rejected before listing")
			return nil
		}, nil)
		assert.EqualError(t, err, "group-size must be greater than 0")
	}
	t.Run("list failure", func(t *testing.T) {
		err := BatchDelete("pod", "default", 1, func(visit func(string) error) error {
			require.NoError(t, visit("partial"))
			return sentinel
		}, func(string) error {
			t.Fatal("must not delete after a list failure")
			return nil
		})
		assert.ErrorIs(t, err, sentinel)
	})
	t.Run("empty", func(t *testing.T) {
		require.NoError(t, BatchDelete("pod", "default", 1, enumerateNames(), func(string) error {
			t.Fatal("must not delete when the list is empty")
			return nil
		}))
	})
	t.Run("missing", func(t *testing.T) {
		var calls int
		require.NoError(t, BatchDelete("pod", "default", 1, enumerateNames("gone", "present"), func(name string) error {
			calls++
			if name == "gone" {
				return apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
			}
			return nil
		}))
		assert.Equal(t, 2, calls)
	})
	t.Run("delete failure finishes current wave", func(t *testing.T) {
		var calls atomic.Int32
		err := BatchDelete("pod", "default", 2, enumerateNames("bad", "current", "later"), func(name string) error {
			calls.Add(1)
			assert.NotEqual(t, "later", name)
			if name == "bad" {
				return sentinel
			}
			return nil
		})
		assert.ErrorIs(t, err, sentinel)
		assert.EqualError(t, err, "failed to delete pod bad: denied")
		assert.Equal(t, int32(2), calls.Load())
	})
}
