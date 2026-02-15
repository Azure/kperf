// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"testing"
)

func TestReplayRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     ReplayRequest
		wantErr bool
	}{
		{
			name: "valid GET request",
			req: ReplayRequest{
				Timestamp:    0,
				Verb:         "GET",
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/namespaces/default/pods/nginx",
			},
			wantErr: false,
		},
		{
			name: "valid LIST request",
			req: ReplayRequest{
				Timestamp:    100,
				Verb:         "LIST",
				Namespace:    "default",
				ResourceKind: "Pod",
				APIPath:      "/api/v1/namespaces/default/pods",
			},
			wantErr: false,
		},
		{
			name: "valid CREATE request",
			req: ReplayRequest{
				Timestamp:    200,
				Verb:         "CREATE",
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/namespaces/default/pods",
				Body:         `{"apiVersion":"v1","kind":"Pod"}`,
			},
			wantErr: false,
		},
		{
			name: "invalid verb",
			req: ReplayRequest{
				Timestamp:    0,
				Verb:         "INVALID",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/pods/nginx",
			},
			wantErr: true,
		},
		{
			name: "negative timestamp",
			req: ReplayRequest{
				Timestamp:    -1,
				Verb:         "GET",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/pods/nginx",
			},
			wantErr: true,
		},
		{
			name: "missing resourceKind",
			req: ReplayRequest{
				Timestamp: 0,
				Verb:      "GET",
				Name:      "nginx",
				APIPath:   "/api/v1/pods/nginx",
			},
			wantErr: true,
		},
		{
			name: "missing apiPath",
			req: ReplayRequest{
				Timestamp:    0,
				Verb:         "GET",
				ResourceKind: "Pod",
				Name:         "nginx",
			},
			wantErr: true,
		},
		{
			name: "missing name for GET",
			req: ReplayRequest{
				Timestamp:    0,
				Verb:         "GET",
				ResourceKind: "Pod",
				APIPath:      "/api/v1/pods",
			},
			wantErr: true,
		},
		{
			name: "missing body for CREATE",
			req: ReplayRequest{
				Timestamp:    0,
				Verb:         "CREATE",
				ResourceKind: "Pod",
				Name:         "nginx",
				APIPath:      "/api/v1/pods",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReplayRequestObjectKey(t *testing.T) {
	tests := []struct {
		name string
		req  ReplayRequest
		want string
	}{
		{
			name: "namespaced resource",
			req: ReplayRequest{
				Namespace:    "default",
				ResourceKind: "Pod",
				Name:         "nginx",
			},
			want: "default/Pod/nginx",
		},
		{
			name: "cluster-scoped resource",
			req: ReplayRequest{
				ResourceKind: "Node",
				Name:         "node-1",
			},
			want: "/Node/node-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.req.ObjectKey()
			if got != tt.want {
				t.Errorf("ObjectKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplayProfileSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    ReplayProfileSpec
		wantErr bool
	}{
		{
			name: "valid spec",
			spec: ReplayProfileSpec{
				RunnerCount:      4,
				ConnsPerRunner:   2,
				ClientsPerRunner: 10,
				ContentType:      ContentTypeJSON,
			},
			wantErr: false,
		},
		{
			name: "zero runner count",
			spec: ReplayProfileSpec{
				RunnerCount:    0,
				ConnsPerRunner: 2,
				ContentType:    ContentTypeJSON,
			},
			wantErr: true,
		},
		{
			name: "zero conns per runner",
			spec: ReplayProfileSpec{
				RunnerCount:    4,
				ConnsPerRunner: 0,
				ContentType:    ContentTypeJSON,
			},
			wantErr: true,
		},
		{
			name: "negative clients per runner",
			spec: ReplayProfileSpec{
				RunnerCount:      4,
				ConnsPerRunner:   2,
				ClientsPerRunner: -1,
				ContentType:      ContentTypeJSON,
			},
			wantErr: true,
		},
		{
			name: "invalid content type",
			spec: ReplayProfileSpec{
				RunnerCount:    4,
				ConnsPerRunner: 2,
				ContentType:    "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReplayProfileValidate(t *testing.T) {
	validRequest := ReplayRequest{
		Timestamp:    0,
		Verb:         "GET",
		ResourceKind: "Pod",
		Name:         "nginx",
		APIPath:      "/api/v1/pods/nginx",
	}

	validSpec := ReplayProfileSpec{
		RunnerCount:    4,
		ConnsPerRunner: 2,
		ContentType:    ContentTypeJSON,
	}

	tests := []struct {
		name    string
		profile ReplayProfile
		wantErr bool
	}{
		{
			name: "valid profile",
			profile: ReplayProfile{
				Version:  1,
				Spec:     validSpec,
				Requests: []ReplayRequest{validRequest},
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			profile: ReplayProfile{
				Version:  2,
				Spec:     validSpec,
				Requests: []ReplayRequest{validRequest},
			},
			wantErr: true,
		},
		{
			name: "empty requests",
			profile: ReplayProfile{
				Version:  1,
				Spec:     validSpec,
				Requests: []ReplayRequest{},
			},
			wantErr: true,
		},
		{
			name: "unsorted requests",
			profile: ReplayProfile{
				Version: 1,
				Spec:    validSpec,
				Requests: []ReplayRequest{
					{Timestamp: 100, Verb: "GET", ResourceKind: "Pod", Name: "nginx", APIPath: "/api/v1/pods/nginx"},
					{Timestamp: 50, Verb: "GET", ResourceKind: "Pod", Name: "nginx2", APIPath: "/api/v1/pods/nginx2"},
				},
			},
			wantErr: true,
		},
		{
			name: "sorted requests",
			profile: ReplayProfile{
				Version: 1,
				Spec:    validSpec,
				Requests: []ReplayRequest{
					{Timestamp: 0, Verb: "GET", ResourceKind: "Pod", Name: "nginx", APIPath: "/api/v1/pods/nginx"},
					{Timestamp: 50, Verb: "GET", ResourceKind: "Pod", Name: "nginx2", APIPath: "/api/v1/pods/nginx2"},
					{Timestamp: 100, Verb: "GET", ResourceKind: "Pod", Name: "nginx3", APIPath: "/api/v1/pods/nginx3"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReplayProfileDuration(t *testing.T) {
	tests := []struct {
		name     string
		requests []ReplayRequest
		want     int64
	}{
		{
			name:     "empty requests",
			requests: []ReplayRequest{},
			want:     0,
		},
		{
			name: "single request",
			requests: []ReplayRequest{
				{Timestamp: 100},
			},
			want: 100,
		},
		{
			name: "multiple requests",
			requests: []ReplayRequest{
				{Timestamp: 0},
				{Timestamp: 500},
				{Timestamp: 1000},
			},
			want: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := ReplayProfile{Requests: tt.requests}
			got := profile.Duration()
			if got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}
