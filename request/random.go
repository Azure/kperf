// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/contrib/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apitypes "k8s.io/apimachinery/pkg/types"
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
		case r.WatchList != nil:
			builder = newRequestWatchListBuilder(r.WatchList, spec.MaxRetries)
		case r.StaleGet != nil:
			builder = newRequestGetBuilder(r.StaleGet, "0", spec.MaxRetries)
		case r.QuorumGet != nil:
			builder = newRequestGetBuilder(r.QuorumGet, "", spec.MaxRetries)
		case r.GetPodLog != nil:
			builder = newRequestGetPodLogBuilder(r.GetPodLog, spec.MaxRetries)
		case r.Patch != nil:
			builder = newRequestPatchBuilder(r.Patch, "", spec.MaxRetries)
		case r.PostDel != nil:
			builder = newRequestPostDelBuilder(r.PostDel, "", spec.MaxRetries)
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
	for {
		if total > 0 && sum >= total {
			break
		}
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
	Build(cli rest.Interface) Requester
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
func (b *requestGetBuilder) Build(cli rest.Interface) Requester {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	comps := make([]string, 0, 5)
	if b.version.Group == "" {
		comps = append(comps, "api", b.version.Version)
	} else {
		comps = append(comps, "apis", b.version.Group, b.version.Version)
	}
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}
	comps = append(comps, b.resource, b.name)

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "GET",
			req: cli.Get().AbsPath(comps...).
				SpecificallyVersionedParams(
					&metav1.GetOptions{ResourceVersion: b.resourceVersion},
					scheme.ParameterCodec,
					schema.GroupVersion{Version: "v1"},
				).MaxRetries(b.maxRetries),
		},
	}
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
func (b *requestListBuilder) Build(cli rest.Interface) Requester {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	comps := make([]string, 0, 5)
	if b.version.Group == "" {
		comps = append(comps, "api", b.version.Version)
	} else {
		comps = append(comps, "apis", b.version.Group, b.version.Version)
	}
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}
	comps = append(comps, b.resource)

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "LIST",
			req: cli.Get().AbsPath(comps...).
				SpecificallyVersionedParams(
					&metav1.ListOptions{
						LabelSelector:   b.labelSelector,
						FieldSelector:   b.fieldSelector,
						ResourceVersion: b.resourceVersion,
						Limit:           b.limit,
					},
					scheme.ParameterCodec,
					schema.GroupVersion{Version: "v1"},
				).MaxRetries(b.maxRetries),
		},
	}
}

type requestWatchListBuilder struct {
	version       schema.GroupVersion
	resource      string
	namespace     string
	labelSelector string
	fieldSelector string
	maxRetries    int
}

func newRequestWatchListBuilder(src *types.RequestWatchList, maxRetries int) *requestWatchListBuilder {
	return &requestWatchListBuilder{
		version: schema.GroupVersion{
			Group:   src.Group,
			Version: src.Version,
		},
		resource:      src.Resource,
		namespace:     src.Namespace,
		labelSelector: src.Selector,
		fieldSelector: src.FieldSelector,
		maxRetries:    maxRetries,
	}
}

// Build implements RequestBuilder.Build.
func (b *requestWatchListBuilder) Build(cli rest.Interface) Requester {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	comps := make([]string, 0, 5)
	if b.version.Group == "" {
		comps = append(comps, "api", b.version.Version)
	} else {
		comps = append(comps, "apis", b.version.Group, b.version.Version)
	}
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}
	comps = append(comps, b.resource)

	return &WatchListRequester{
		BaseRequester: BaseRequester{
			method: "WATCHLIST",
			req: cli.Get().AbsPath(comps...).
				SpecificallyVersionedParams(
					&metav1.ListOptions{
						LabelSelector:        b.labelSelector,
						FieldSelector:        b.fieldSelector,
						ResourceVersion:      "",
						Watch:                true,
						SendInitialEvents:    toPtr(true),
						ResourceVersionMatch: metav1.ResourceVersionMatchNotOlderThan,
						AllowWatchBookmarks:  true,
					},
					scheme.ParameterCodec,
					schema.GroupVersion{Version: "v1"},
				).MaxRetries(b.maxRetries),
		},
	}
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
func (b *requestGetPodLogBuilder) Build(cli rest.Interface) Requester {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	apiPath, version := "api", "v1"

	comps := make([]string, 2, 7)
	comps[0], comps[1] = apiPath, version
	comps = append(comps, "namespaces", b.namespace)
	comps = append(comps, "pods", b.name, "log")

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "POD_LOG",
			req: cli.Get().AbsPath(comps...).
				SpecificallyVersionedParams(
					&corev1.PodLogOptions{
						Container:  b.container,
						TailLines:  b.tailLines,
						LimitBytes: b.limitBytes,
					},
					scheme.ParameterCodec,
					schema.GroupVersion{Version: "v1"},
				).MaxRetries(b.maxRetries),
		},
	}
}

type requestPatchBuilder struct {
	version         schema.GroupVersion
	resource        string
	resourceVersion string
	namespace       string
	name            string
	keySpaceSize    int
	patchType       apitypes.PatchType
	body            interface{}
	maxRetries      int
}

func newRequestPatchBuilder(src *types.RequestPatch, resourceVersion string, maxRetries int) *requestPatchBuilder {
	patchType, _ := types.GetPatchType(src.PatchType)

	return &requestPatchBuilder{
		version: schema.GroupVersion{
			Group:   src.Group,
			Version: src.Version,
		},
		resource:        src.Resource,
		resourceVersion: resourceVersion,
		namespace:       src.Namespace,
		name:            src.Name,
		keySpaceSize:    src.KeySpaceSize,
		patchType:       patchType,
		body:            []byte(src.Body),
		maxRetries:      maxRetries,
	}
}

// Build implements RequestBuilder.Build.
func (b *requestPatchBuilder) Build(cli rest.Interface) Requester {
	// https://kubernetes.io/docs/reference/using-api/#api-groups
	comps := make([]string, 0, 5)
	if b.version.Group == "" {
		comps = append(comps, "api", b.version.Version)
	} else {
		comps = append(comps, "apis", b.version.Group, b.version.Version)
	}
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}
	// Generate random suffix based on keySpaceSize
	randomInt, _ := rand.Int(rand.Reader, big.NewInt(int64(b.keySpaceSize)))
	suffix := randomInt.Int64()

	// Create final resource name: name-{suffix}
	finalName := fmt.Sprintf("%s-%d", b.name, suffix)
	comps = append(comps, b.resource, finalName)

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "PATCH",
			req: cli.Patch(b.patchType).AbsPath(comps...).
				Body(b.body).
				MaxRetries(b.maxRetries),
		},
	}
}

type requestPostDelBuilder struct {
	version         schema.GroupVersion
	resource        string
	resourceVersion string
	namespace       string
	deleteRatio     float64
	maxRetries      int

	// Per-builder cache for created resources
	cache *Cache

	// Per-builder atomic counter for unique ID generation
	resourceCounter int64
}

func newRequestPostDelBuilder(src *types.RequestPostDel, resourceVersion string, maxRetries int) *requestPostDelBuilder {
	return &requestPostDelBuilder{
		version:         schema.GroupVersion{Group: src.Group, Version: src.Version},
		resource:        src.Resource,
		resourceVersion: resourceVersion,
		namespace:       src.Namespace,
		deleteRatio:     src.DeleteRatio,
		maxRetries:      maxRetries,
		cache:           InitCache(), // Initialize the cache
	}
}

// Build implements RequestBuilder.Build.
func (b *requestPostDelBuilder) Build(cli rest.Interface) Requester {
	comps := make([]string, 0, 5)
	if b.version.Group == "" {
		comps = append(comps, "api", b.version.Version)
	} else {
		comps = append(comps, "apis", b.version.Group, b.version.Version)
	}
	if b.namespace != "" {
		comps = append(comps, "namespaces", b.namespace)
	}

	// Random pick operation DELETE or CREATE based on deleteRatio weight probability
	randomInt, _ := rand.Int(rand.Reader, big.NewInt(1000))
	shouldDelete := float64(randomInt.Int64())/1000.0 < b.deleteRatio

	if shouldDelete {
		// Try to get a name from cache
		if name, ok := b.cache.Pop(); ok {
			comps = append(comps, b.resource, name)

			return &PostDelDiscardRequester{
				builder:   b,
				name:      name,
				operation: "DELETE",
				DiscardRequester: DiscardRequester{
					BaseRequester: BaseRequester{
						method: "DELETE",
						req: cli.Delete().AbsPath(comps...).
							MaxRetries(b.maxRetries),
					},
				},
			}
		}
		// If cache is empty, fall through to POST
	}

	// POST logic - create resource and add to cache if successful
	comps = append(comps, b.resource)

	// Use builder's atomic counter for synchronized unique ID generation
	counter := atomic.AddInt64(&b.resourceCounter, 1)
	timestamp := time.Now().UnixNano()
	name := fmt.Sprintf("%d-%d", timestamp, counter)

	body, _ := utils.RenderTemplate(b.resource, map[string]interface{}{
		"namePattern": name,
		"namespace":   b.namespace,
	})

	return &PostDelDiscardRequester{
		builder:   b,
		name:      name,
		operation: "POST",
		DiscardRequester: DiscardRequester{
			BaseRequester: BaseRequester{
				method: "POST",
				req:    cli.Post().AbsPath(comps...).Body(body).MaxRetries(b.maxRetries),
			},
		},
	}
}

// PostDelDiscardRequester handles both POST and DELETE requests with cache management
type PostDelDiscardRequester struct {
	builder   *requestPostDelBuilder
	name      string
	operation string // "POST" or "DELETE"
	DiscardRequester
}

func (reqr *PostDelDiscardRequester) Do(ctx context.Context) (bytes int64, err error) {
	// Use DiscardRequester's Do method to discard response body
	bytes, err = reqr.DiscardRequester.Do(ctx)

	switch reqr.operation {
	case "POST":
		// Only add to cache if POST request was successful
		if err == nil {
			reqr.builder.cache.Push(reqr.name)
		}
	case "DELETE":
		// If DELETE request failed, restore the item back to cache
		// since the resource still exists in Kubernetes
		if err != nil {
			reqr.builder.cache.Push(reqr.name)
		}
	}

	return bytes, err
}

func toPtr[T any](v T) *T {
	return &v
}
