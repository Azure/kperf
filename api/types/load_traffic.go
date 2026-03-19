// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package types

import (
	"fmt"

	apitypes "k8s.io/apimachinery/pkg/types"
)

// ContentType represents the format of response.
type ContentType string

const (
	// ContentTypeJSON means the format is json.
	ContentTypeJSON ContentType = "json"
	// ContentTypeProtobuffer means the format is protobuf.
	ContentTypeProtobuffer = "protobuf"
)

// Validate returns error if ContentType is not supported.
func (ct ContentType) Validate() error {
	switch ct {
	case ContentTypeJSON, ContentTypeProtobuffer:
		return nil
	default:
		return fmt.Errorf("unsupported content type %s", ct)
	}
}

// LoadProfile defines how to create load traffic from one host to kube-apiserver.
type LoadProfile struct {
	// Version defines the version of this object.
	Version int `json:"version" yaml:"version"`
	// Description is a string value to describe this object.
	Description string `json:"description,omitempty" yaml:"description"`
	// Specs defines behaviors of load profile for time-series replay support.
	Specs []LoadProfileSpec `json:"specs" yaml:"specs"`
}

// LoadProfileSpec defines the load traffic for traget resource.
type LoadProfileSpec struct {
	// Rate defines the maximum requests per second (zero is no limit).
	Rate float64 `json:"rate" yaml:"rate"`
	// Total defines the total number of requests.
	Total int `json:"total" yaml:"total"`
	// Duration defines the running time in seconds.
	Duration int `json:"duration" yaml:"duration"`
	// Conns defines total number of long connections used for traffic.
	Conns int `json:"conns" yaml:"conns"`
	// Client defines total number of HTTP clients.
	Client int `json:"client" yaml:"client"`
	// ContentType defines response's content type.
	ContentType ContentType `json:"contentType" yaml:"contentType"`
	// DisableHTTP2 means client will use HTTP/1.1 protocol if it's true.
	DisableHTTP2 bool `json:"disableHTTP2" yaml:"disableHTTP2"`
	// MaxRetries makes the request use the given integer as a ceiling of
	// retrying upon receiving "Retry-After" headers and 429 status-code
	// in the response (<= 0 means no retry).
	MaxRetries int `json:"maxRetries" yaml:"maxRetries"`
	// Requests defines the different kinds of requests with weights.
	// The executor should randomly pick by weight.
	Requests []*WeightedRequest `json:"requests" yaml:"requests"`
}

// KubeGroupVersionResource identifies the resource URI.
type KubeGroupVersionResource struct {
	// Group is the name about a collection of related functionality.
	Group string `json:"group" yaml:"group"`
	// Version is a version of that group.
	Version string `json:"version" yaml:"version"`
	// Resource is a type in that versioned group APIs.
	Resource string `json:"resource" yaml:"resource"`
}

// WeightedRequest represents request with weight.
// Only one of request types may be specified.
type WeightedRequest struct {
	// Shares defines weight in the same group.
	Shares int `json:"shares" yaml:"shares"`
	// StaleList means this list request with zero resource version.
	StaleList *RequestList `json:"staleList,omitempty" yaml:"staleList,omitempty"`
	// QuorumList means this list request without kube-apiserver cache.
	QuorumList *RequestList `json:"quorumList,omitempty" yaml:"quorumList,omitempty"`
	// WatchList lists objects with the watch list feature, a.k.a streaming list.
	WatchList *RequestWatchList `json:"watchList,omitempty" yaml:"watchList,omitempty"`
	// StaleGet means this get request with zero resource version.
	StaleGet *RequestGet `json:"staleGet,omitempty" yaml:"staleGet,omitempty"`
	// QuorumGet means this get request without kube-apiserver cache.
	QuorumGet *RequestGet `json:"quorumGet,omitempty" yaml:"quorumGet,omitempty"`
	// Put means this is mutating request.
	Put *RequestPut `json:"put,omitempty" yaml:"put,omitempty"`
	// Patch means this is mutating request to update resource.
	Patch *RequestPatch `json:"patch,omitempty" yaml:"patch,omitempty"`
	// GetPodLog means this is to get log from target pod.
	GetPodLog *RequestGetPodLog `json:"getPodLog,omitempty" yaml:"getPodLog,omitempty"`
	// PostDelete means this is a post-delete operation request.
	PostDel *RequestPostDel `json:"postDel,omitempty" yaml:"postDel,omitempty"`
}

// RequestGet defines GET request for target object.
type RequestGet struct {
	// KubeGroupVersionResource identifies the resource URI.
	KubeGroupVersionResource `yaml:",inline"`
	// Namespace is object's namespace.
	Namespace string `json:"namespace" yaml:"namespace"`
	// Name is object's name.
	Name string `json:"name" yaml:"name"`
}

// RequestList defines LIST request for target objects.
type RequestList struct {
	// KubeGroupVersionResource identifies the resource URI.
	KubeGroupVersionResource `yaml:",inline"`
	// Namespace is object's namespace.
	Namespace string `json:"namespace" yaml:"namespace"`
	// Limit defines the page size.
	Limit int `json:"limit" yaml:"limit"`
	// Selector defines how to identify a set of objects.
	Selector string `json:"selector" yaml:"selector"`
	// FieldSelector defines how to identify a set of objects with field selector.
	FieldSelector string `json:"fieldSelector" yaml:"fieldSelector"`
}

type RequestWatchList struct {
	// KubeGroupVersionResource identifies the resource URI.
	KubeGroupVersionResource `yaml:",inline"`
	// Namespace is object's namespace.
	Namespace string `json:"namespace" yaml:"namespace"`
	// Selector defines how to identify a set of objects.
	Selector string `json:"selector" yaml:"selector"`
	// FieldSelector defines how to identify a set of objects with field selector.
	FieldSelector string `json:"fieldSelector" yaml:"fieldSelector"`
}

// RequestPut defines PUT request for target resource type.
type RequestPut struct {
	// KubeGroupVersionResource identifies the resource URI.
	//
	// NOTE: Currently, it should be configmap or secrets because we can
	// generate random bytes as blob for it. However, for the pod resource,
	// we need to ensure a lot of things are ready, for instance, volumes,
	// resource capacity. It's not easy to generate it randomly. Maybe we
	// can introduce pod template in the future.
	KubeGroupVersionResource `yaml:",inline"`
	// Namespace is object's namespace.
	Namespace string `json:"namespace" yaml:"namespace"`
	// Name is object's prefix name.
	Name string `json:"name" yaml:"name"`
	// KeySpaceSize is used to generate random number as name's suffix.
	KeySpaceSize int `json:"keySpaceSize" yaml:"keySpaceSize"`
	// ValueSize is the object's size in bytes.
	ValueSize int `json:"valueSize" yaml:"valueSize"`
}

// RequestPatch defines PATCH request for target resource type.
type RequestPatch struct {
	KubeGroupVersionResource `yaml:",inline"`
	// Namespace is object's namespace.
	Namespace string `json:"namespace" yaml:"namespace"`
	// Name is object's Name Pattern e.g {name}-{suffix index}.
	Name string `json:"name" yaml:"name"`
	// KeySpaceSize is used to generate random number as name's suffix.
	KeySpaceSize int `json:"keySpaceSize" yaml:"keySpaceSize"`
	// ValueSize is the object's size in bytes. how many bytes to patch data
	ValueSize int `json:"valueSize" yaml:"valueSize"`
}

// RequestGetPodLog defines GetLog request for target pod.
type RequestGetPodLog struct {
	// Namespace is pod's namespace.
	Namespace string `json:"namespace" yaml:"namespace"`
	// Name is pod's name.
	Name string `json:"name" yaml:"name"`
	// Container is target for stream logs. If empty, it's only valid
	// when there is only one container.
	Container string `json:"container" yaml:"container"`
	// TailLines is the number of lines from the end of the logs to show,
	// if set.
	TailLines *int64 `json:"tailLines" yaml:"tailLines"`
	// LimitBytes is the number of bytes to read from the server before
	// terminating the log output, if set.
	LimitBytes *int64 `json:"limitBytes" yaml:"limitBytes"`
}
type RequestPostDel struct {
	KubeGroupVersionResource `yaml:",inline"`
	Namespace                string  `json:"namespace" yaml:"namespace"`
	DeleteRatio              float64 `json:"deleteRatio" yaml:"deleteRatio"`
}

// Validate verifies fields of LoadProfile.
func (lp LoadProfile) Validate() error {
	if lp.Version != 1 {
		return fmt.Errorf("version should be 1")
	}

	// Validate that specs is provided
	if len(lp.Specs) == 0 {
		return fmt.Errorf("specs must be provided")
	}

	// Validate all specs
	for i, spec := range lp.Specs {
		if err := spec.Validate(); err != nil {
			return fmt.Errorf("specs[%d]: %w", i, err)
		}
	}

	return nil
}

// Validate verifies fields of LoadProfileSpec.
func (spec LoadProfileSpec) Validate() error {
	if spec.Conns <= 0 {
		return fmt.Errorf("conns requires > 0: %v", spec.Conns)
	}

	if spec.Rate < 0 {
		return fmt.Errorf("rate requires >= 0: %v", spec.Rate)
	}

	if spec.Total <= 0 && spec.Duration <= 0 {
		return fmt.Errorf("total requires > 0: %v or duration > 0s: %v", spec.Total, spec.Duration)
	}

	if spec.Client <= 0 {
		return fmt.Errorf("client requires > 0: %v", spec.Client)
	}

	err := spec.ContentType.Validate()
	if err != nil {
		return err
	}

	for idx, req := range spec.Requests {
		if err := req.Validate(); err != nil {
			return fmt.Errorf("idx: %v request: %v", idx, err)
		}
	}
	return nil
}

// Validate verifies fields of WeightedRequest.
func (r WeightedRequest) Validate() error {
	if r.Shares < 0 {
		return fmt.Errorf("shares(%v) requires >= 0", r.Shares)
	}

	switch {
	case r.StaleList != nil:
		return r.StaleList.Validate(true)
	case r.QuorumList != nil:
		return r.QuorumList.Validate(false)
	case r.WatchList != nil:
		return r.WatchList.Validate()
	case r.StaleGet != nil:
		return r.StaleGet.Validate()
	case r.QuorumGet != nil:
		return r.QuorumGet.Validate()
	case r.Put != nil:
		return r.Put.Validate()
	case r.Patch != nil:
		return r.Patch.Validate()
	case r.GetPodLog != nil:
		return r.GetPodLog.Validate()
	case r.PostDel != nil:
		return r.PostDel.Validate()
	default:
		return fmt.Errorf("empty request value")
	}
}

// RequestList validates RequestList type.
func (r *RequestList) Validate(stale bool) error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}

	if r.Limit < 0 {
		return fmt.Errorf("limit must >= 0")
	}

	if stale && r.Limit != 0 {
		return fmt.Errorf("stale list doesn't support pagination option: https://github.com/kubernetes/kubernetes/issues/108003")
	}
	return nil
}

func (r *RequestWatchList) Validate() error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}
	return nil
}

// Validate validates RequestGet type.
func (r *RequestGet) Validate() error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}

	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// Validate validates RequestPut type.
func (r *RequestPut) Validate() error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}

	// TODO: check resource type
	if r.Name == "" {
		return fmt.Errorf("name pattern is required")
	}
	if r.KeySpaceSize <= 0 {
		return fmt.Errorf("keySpaceSize must > 0")
	}
	if r.ValueSize <= 0 {
		return fmt.Errorf("valueSize must > 0")
	}
	return nil
}

// Validate validates RequestGetPodLog type.
func (r *RequestGetPodLog) Validate() error {
	if r.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// Validate validates KubeGroupVersionResource.
func (m *KubeGroupVersionResource) Validate() error {
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}

	if m.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	return nil
}

// GetPatchType returns the Kubernetes PatchType for a given patch type string.
// Returns the PatchType and an error if the patch type is invalid.
func GetPatchType(patchType string) (apitypes.PatchType, bool) {
	switch patchType {
	case "json":
		return apitypes.JSONPatchType, true
	case "merge":
		return apitypes.MergePatchType, true
	case "strategic-merge":
		return apitypes.StrategicMergePatchType, true
	default:
		return "", false
	}
}

// Validate validates RequestPatch type.
func (r *RequestPatch) Validate() error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}
	if r.Name == "" {
		return fmt.Errorf("name is required")
	}
	if r.Resource == "" {
		return fmt.Errorf("resource is required")
	}
	if (r.Resource == "configmaps" || r.Resource == "secrets") && r.ValueSize <= 0 {
		return fmt.Errorf("valueSize must > 0 for configmaps and secrets, to generate data to be patched")
	}

	return nil
}

func (r *RequestPostDel) Validate() error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}

	if r.DeleteRatio < 0 || r.DeleteRatio > 0.5 {
		return fmt.Errorf("delete ratio must be between 0 and 0.5: %v, create proportion should be greater than delete", r.DeleteRatio)
	}

	return nil
}
