// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RunnerGroup defines a set of runners with same load profile.
type RunnerGroup struct {
	// Name is the name of runner group.
	Name string `json:"name" yaml:"name"`
	// Spec is specification of the desired behavior of the runner group.
	Spec *RunnerGroupSpec `json:"spec" yaml:"spec"`
	// Status is current state.
	Status *RunnerGroupStatus `json:"status,omitempty" yaml:"status,omitempty"`
}

// RunnerGroupSpec is to descibe how the runner group works.
type RunnerGroupSpec struct {
	// Count is the number of runners.
	Count int32 `json:"count" yaml:"count"`
	// Profile defines what the load traffic looks like.
	Profile *LoadProfile `json:"loadProfile,omitempty" yaml:"loadProfile"`
	// NodeAffinity defines how to deploy runners into dedicated nodes
	// which have specific labels.
	NodeAffinity map[string][]string `json:"nodeAffinity,omitempty" yaml:"nodeAffinity,omitempty"`
	// ServiceAccount is the name of the ServiceAccount to use to run runners.
	ServiceAccount *string `json:"serviceAccount,omitempty" yaml:"serviceAccount,omitempty"`
	// OwnerReference is to mark the runner group depending on this object.
	//
	// FORMAT: APIVersion:Kind:Name:UID
	OwnerReference *string `json:"ownerReference,omitempty" yaml:"ownerReference,omitempty"`
}

// RunnerGroupStatus represents current state of RunnerGroup.
type RunnerGroupStatus struct {
	// State is the current state of RunnerGroup.
	State string `json:"state" yaml:"state"`
	// StartTime represents time when RunnerGroup has been started.
	StartTime *metav1.Time `json:"startTime,omitempty" yaml:"startTime,omitempty"`
	// The number of runners which reached phase Succeeded.
	Succeeded int32 `json:"succeeded" yaml:"succeeded"`
	// The number of runners which reached phase Failed.
	Failed int32 `json:"failed" yaml:"failed"`
}

// RunnerGroupStatusState is current state of RunnerGroup.
type RunnerGroupStatusState string

const (
	// RunnerGroupStatusStateUnknown represents unknown state.
	RunnerGroupStatusStateUnknown = "unknown"
	// RunnerGroupStatusStateRunning represents runner group is still running.
	RunnerGroupStatusStateRunning = "running"
	// RunnerGroupStatusStateFinished represents all runners finished.
	RunnerGroupStatusStateFinished = "finished"
)
