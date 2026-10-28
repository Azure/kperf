// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomPayload(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 8, 62, 4095, 4096, 4097, 16384} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			s := RandomPayload(n)
			if n <= 0 {
				assert.Empty(t, s)
				return
			}
			assert.Len(t, s, n)
			assert.Regexp(t, `^[a-zA-Z0-9]+$`, s)
		})
	}
	assert.NotEqual(t, RandomPayload(32), RandomPayload(32))
}

func TestRandBytes(t *testing.T) {
	for _, n := range []int{-1, 0, 1, 8, 62, 4095, 4096, 4097, 16384} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()
			b, err := RandBytes(n)
			if n <= 0 {
				assert.EqualError(t, err, "length must be positive")
				assert.Nil(t, b)
				return
			}
			if !assert.NoError(t, err) {
				return
			}
			assert.Len(t, b, n)
			assert.Regexp(t, `^[a-zA-Z0-9]+$`, string(b))
		})
	}
}
