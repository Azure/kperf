// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"

	"github.com/Azure/kperf/api/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// WeightedRandomRequests is used to generate requests based on LoadProfileSpec.
type WeightedRandomRequests struct {
	once         sync.Once
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	reqBuilderCh chan RESTRequestBuilder

	shares      []int
	reqBuilders []RESTRequestBuilder
}

// NewWeightedRandomRequests creates new instance of WeightedRandomRequests.
func NewWeightedRandomRequests(spec *types.LoadProfileSpec) (*WeightedRandomRequests, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid load profile spec: %v", err)
	}

	shares := make([]int, 0, len(spec.Requests))
	reqBuilders := make([]RESTRequestBuilder, 0, len(spec.Requests))
	for _, r := range spec.Requests {
		shares = append(shares, r.Shares)

		var builder RESTRequestBuilder
		switch {
		case r.StaleList != nil:
			builder = newRequestListBuilder(r.StaleList, "0", spec.MaxRetries)
		case r.QuorumList != nil:
			builder = newRequestListBuilder(r.QuorumList, "", spec.MaxRetries)
		case r.StaleGet != nil:
			builder = newRequestGetBuilder(r.StaleGet, "0", spec.MaxRetries)
		case r.QuorumGet != nil:
			builder = newRequestGetBuilder(r.QuorumGet, "", spec.MaxRetries)
		case r.GetPodLog != nil:
			builder = newRequestGetPodLogBuilder(r.GetPodLog, spec.MaxRetries)
		default:
			return nil, fmt.Errorf("not implement for PUT yet")
		}
		reqBuilders = append(reqBuilders, builder)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &WeightedRandomRequests{
		ctx:          ctx,
		cancel:       cancel,
		reqBuilderCh: make(chan RESTRequestBuilder),
		shares:       shares,
		reqBuilders:  reqBuilders,
	}, nil
}

// Run starts to random pick request.
func (r *WeightedRandomRequests) Run(ctx context.Context, total int) {
	defer r.wg.Done()
	r.wg.Add(1)

	sum := 0
	for sum < total {
		builder := r.randomPick()
		select {
		case r.reqBuilderCh <- builder:
			sum++
		case <-r.ctx.Done():
			return
		case <-ctx.Done():
			return
		}
	}
}

// Chan returns channel to get random request.
func (r *WeightedRandomRequests) Chan() chan RESTRequestBuilder {
	return r.reqBuilderCh
}

func (r *WeightedRandomRequests) randomPick() RESTRequestBuilder {
	sum := 0
	for _, s := range r.shares {
		sum += s
	}

	rndInt, err := rand.Int(rand.Reader, big.NewInt(int64(sum)))
	if err != nil {
		panic(err)
	}

	rnd := rndInt.Int64()
	for i := range r.shares {
		s := int64(r.shares[i])
		if rnd < s {
			return r.reqBuilders[i]
		}
		rnd -= s
	}
	panic("unreachable")
}

// Stop stops request generator.
func (r *WeightedRandomRequests) Stop() {
	r.once.Do(func() {
		r.cancel()
		r.wg.Wait()
		close(r.reqBuilderCh)
	})
}

// RESTRequestBuilder is used to build rest.Request.
type RESTRequestBuilder interface {
	Build(cli rest.Interface) (method string, _ *rest.Request)
}

type requestGetBuilder struct {
	version         schema.GroupVersion
	resource        string
	namespace       string
	name            string
	resourceVersion string
	maxRetries      int
}

func newRequestGetBuilder(src *types.RequestGet, resourceVersion string, maxRetries int) *requestGetBuilder {
	return &requestGetBuilder{
		version: schema.GroupVersion{
			Group:   src.Group,
			Version: src.Version,
		},
		resource:        src.Resource,
		namespace:       src.Namespace,
		name:            src.Name,
		resourceVersion: resourceVersion,
		maxRetries:      maxRetries,
	}
}

// Build implements RequestBuilder.Build.
func (b *requestGetBuilder) Build(cli rest.Interface) (string, *rest.Request) {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	apiPath := "apis"
	if b.version.Group == "" {
		apiPath = "api"
	}

	comps := make([]string, 2, 5)
	comps[0], comps[1] = apiPath, b.version.Version
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}
	comps = append(comps, b.resource, b.name)

	return "GET", cli.Get().AbsPath(comps...).
		SpecificallyVersionedParams(
			&metav1.GetOptions{ResourceVersion: b.resourceVersion},
			scheme.ParameterCodec,
			schema.GroupVersion{Version: "v1"},
		).MaxRetries(b.maxRetries)
}

type requestListBuilder struct {
	version         schema.GroupVersion
	resource        string
	namespace       string
	limit           int64
	labelSelector   string
	fieldSelector   string
	resourceVersion string
	maxRetries      int
}

func newRequestListBuilder(src *types.RequestList, resourceVersion string, maxRetries int) *requestListBuilder {
	return &requestListBuilder{
		version: schema.GroupVersion{
			Group:   src.Group,
			Version: src.Version,
		},
		resource:        src.Resource,
		namespace:       src.Namespace,
		limit:           int64(src.Limit),
		labelSelector:   src.Selector,
		fieldSelector:   src.FieldSelector,
		resourceVersion: resourceVersion,
		maxRetries:      maxRetries,
	}
}

// Build implements RequestBuilder.Build.
func (b *requestListBuilder) Build(cli rest.Interface) (string, *rest.Request) {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	apiPath := "apis"
	if b.version.Group == "" {
		apiPath = "api"
	}

	comps := make([]string, 2, 5)
	comps[0], comps[1] = apiPath, b.version.Version
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}
	comps = append(comps, b.resource)

	return "LIST", cli.Get().AbsPath(comps...).
		SpecificallyVersionedParams(
			&metav1.ListOptions{
				LabelSelector:   b.labelSelector,
				FieldSelector:   b.fieldSelector,
				ResourceVersion: b.resourceVersion,
				Limit:           b.limit,
			},
			scheme.ParameterCodec,
			schema.GroupVersion{Version: "v1"},
		).MaxRetries(b.maxRetries)
}

type requestGetPodLogBuilder struct {
	namespace  string
	name       string
	container  string
	tailLines  *int64
	limitBytes *int64
	maxRetries int
}

func newRequestGetPodLogBuilder(src *types.RequestGetPodLog, maxRetries int) *requestGetPodLogBuilder {
	b := &requestGetPodLogBuilder{
		namespace:  src.Namespace,
		name:       src.Name,
		container:  src.Container,
		maxRetries: maxRetries,
	}
	if src.TailLines != nil {
		b.tailLines = toPtr(*src.TailLines)
	}
	if src.LimitBytes != nil {
		b.limitBytes = toPtr(*src.LimitBytes)
	}
	return b
}

// Build implements RequestBuilder.Build.
func (b *requestGetPodLogBuilder) Build(cli rest.Interface) (string, *rest.Request) {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	apiPath, version := "api", "v1"

	comps := make([]string, 2, 7)
	comps[0], comps[1] = apiPath, version
	comps = append(comps, "namespaces", b.namespace)
	comps = append(comps, "pods", b.name, "log")

	return "POD_LOG", cli.Get().AbsPath(comps...).
		SpecificallyVersionedParams(
			&corev1.PodLogOptions{
				Container:  b.container,
				TailLines:  b.tailLines,
				LimitBytes: b.limitBytes,
			},
			scheme.ParameterCodec,
			schema.GroupVersion{Version: "v1"},
		).MaxRetries(b.maxRetries)
}

func toPtr[T any](v T) *T {
	return &v
}
