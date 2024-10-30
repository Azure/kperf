// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package helmcli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyValues(t *testing.T) {
	for idx, tc := range []struct {
		from     map[string]interface{}
		to       map[string]interface{}
		expected map[string]interface{}
	}{
		{
			from: map[string]interface{}{
				"foo": "bar1",
				"baz": map[string]interface{}{
					"name": "alice",
				},
			},
			to: map[string]interface{}{
				"foo": "bar2",
				"baz": map[string]interface{}{
					"name": "bob",
					"age":  "18",
				},
			},
			expected: map[string]interface{}{
				"foo": "bar1",
				"baz": map[string]interface{}{
					"name": "alice",
					"age":  "18",
				},
			},
		},
		{
			from: map[string]interface{}{
				"foo": "bar1",
				"baz": "profile",
			},
			to: map[string]interface{}{
				"foo": "bar1",
				"baz": map[string]interface{}{
					"name": "alice",
				},
			},
			expected: map[string]interface{}{
				"foo": "bar1",
				"baz": "profile",
			},
		},
		{
			from: map[string]interface{}{
				"foo": "bar1",
				"baz": map[string]interface{}{
					"name": "alice",
				},
			},
			to: map[string]interface{}{
				"version": "alpha",
			},
			expected: map[string]interface{}{
				"foo": "bar1",
				"baz": map[string]interface{}{
					"name": "alice",
				},
				"version": "alpha",
			},
		},
		{
			from: map[string]interface{}{
				"baz": map[string]interface{}{
					"name": map[string]interface{}{
						"last":  "unknown",
						"first": "bob",
					},
				},
				"version": "beta",
			},
			to: map[string]interface{}{
				"foo": "bar2",
				"baz": map[string]interface{}{
					"name": "bob",
					"age":  "18",
				},
			},
			expected: map[string]interface{}{
				"foo": "bar2",
				"baz": map[string]interface{}{
					"name": map[string]interface{}{
						"last":  "unknown",
						"first": "bob",
					},
					"age": "18",
				},
				"version": "beta",
			},
		},
	} {
		err := applyValues(tc.to, tc.from)
		assert.NoError(t, err, "#%v", idx)
		assert.Equal(t, tc.expected, tc.to, "#%v", idx)
	}
}
