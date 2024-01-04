package types

import "fmt"

// LoadProfile defines how to create load traffic from one host to kube-apiserver.
type LoadProfile struct {
	// Version defines the version of this object.
	Version int `json:"version" yaml:"version"`
	// Description is a string value to describe this object.
	Description string `json:"description,omitempty" yaml:"description"`
	// Spec defines behavior of load profile.
	Spec LoadProfileSpec `json:"spec" yaml:"spec"`
}

// LoadProfileSpec defines the load traffic for traget resource.
type LoadProfileSpec struct {
	// Rate defines the maximum requests per second (zero is no limit).
	Rate int `json:"rate" yaml:"rate"`
	// Total defines the total number of requests.
	Total int `json:"total" yaml:"total"`
	// Conns defines total number of long connections used for traffic.
	Conns int `json:"conns" yaml:"conns"`
	// Client defines total number of HTTP clients.
	Client int `json:"client" yaml:"client"`
	// Requests defines the different kinds of requests with weights.
	// The executor should randomly pick by weight.
	Requests []*WeightedRequest
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
	StaleList *RequestList `json:"staleList" yaml:"staleList"`
	// QuorumList means this list request without kube-apiserver cache.
	QuorumList *RequestList `json:"quorumList" yaml:"quorumList"`
	// StaleGet means this get request with zero resource version.
	StaleGet *RequestGet `json:"staleGet" yaml:"staleGet"`
	// QuorumGet means this get request without kube-apiserver cache.
	QuorumGet *RequestGet `json:"quorumGet" yaml:"quorumGet"`
	// Put means this is mutating request.
	Put *RequestPut `json:"put" yaml:"put"`
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
	Selector string `json:"seletor" yaml:"seletor"`
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

// Validate verifies fields of LoadProfile.
func (lp LoadProfile) Validate() error {
	if lp.Version != 1 {
		return fmt.Errorf("version should be 1")
	}
	return lp.Spec.Validate()
}

// Validate verifies fields of LoadProfileSpec.
func (spec LoadProfileSpec) Validate() error {
	if spec.Conns <= 0 {
		return fmt.Errorf("conns requires > 0: %v", spec.Conns)
	}

	if spec.Rate < 0 {
		return fmt.Errorf("rate requires >= 0: %v", spec.Rate)
	}

	if spec.Total <= 0 {
		return fmt.Errorf("total requires > 0: %v", spec.Total)
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
		return r.StaleList.Validate()
	case r.QuorumList != nil:
		return r.QuorumList.Validate()
	case r.StaleGet != nil:
		return r.StaleGet.Validate()
	case r.QuorumGet != nil:
		return r.QuorumGet.Validate()
	case r.Put != nil:
		return r.Put.Validate()
	default:
		return fmt.Errorf("empty request value")
	}
}

// RequestList validates RequestList type.
func (r *RequestList) Validate() error {
	if err := r.KubeGroupVersionResource.Validate(); err != nil {
		return fmt.Errorf("kube metadata: %v", err)
	}

	if r.Limit < 0 {
		return fmt.Errorf("limit must >= 0")
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
