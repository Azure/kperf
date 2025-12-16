// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWeightedRequest(t *testing.T) {
	tests := map[string]struct {
		req WeightedRequest
		err bool
	}{
		"shares < 0": {
			req: WeightedRequest{
				Shares: -1,
			},
			err: true,
		},
		"no request setting": {
			req: WeightedRequest{
				Shares: 100,
			},
			err: true,
		},
		"empty version": {
			req: WeightedRequest{
				Shares: 100,
				StaleList: &RequestList{
					KubeGroupVersionResource: KubeGroupVersionResource{
						Resource: "pods",
					},
				},
			},
			err: true,
		},
		"empty resource": {
			req: WeightedRequest{
				Shares: 100,
				StaleList: &RequestList{
					KubeGroupVersionResource: KubeGroupVersionResource{
						Version: "v1",
					},
				},
			},
			err: true,
		},
		"wrong limit": {
			req: WeightedRequest{
				Shares: 100,
				StaleList: &RequestList{
					KubeGroupVersionResource: KubeGroupVersionResource{
						Resource: "pods",
						Version:  "v1",
					},
					Limit: -1,
				},
			},
			err: true,
		},
		"no error": {
			req: WeightedRequest{
				Shares: 100,
				StaleList: &RequestList{
					KubeGroupVersionResource: KubeGroupVersionResource{
						Resource: "pods",
						Version:  "v1",
					},
					Limit: 0,
				},
			},
			err: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
