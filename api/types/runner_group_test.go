// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"testing"
)

func TestRunnerGroupSpecValidate(t *testing.T) {
	replayProfile := "https://example.com/replay.yaml"
	tests := []struct {
		name    string
		spec    RunnerGroupSpec
		wantErr bool
	}{
		{
			name: "valid load profile",
			spec: RunnerGroupSpec{
				Profile: &LoadProfile{},
			},
			wantErr: false,
		},
		{
			name: "valid replay profile",
			spec: RunnerGroupSpec{
				ReplayProfile: &replayProfile,
			},
			wantErr: false,
		},
		{
			name: "both profiles set",
			spec: RunnerGroupSpec{
				Profile:       &LoadProfile{},
				ReplayProfile: &replayProfile,
			},
			wantErr: true,
		},
		{
			name:    "neither profile set",
			spec:    RunnerGroupSpec{},
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
