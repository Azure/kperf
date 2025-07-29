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
			builder = newRequestPostDelBuilder(r.PostDel, "", spec.Total, spec.MaxRetries) // Pass spec.Total here
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
	comps = append(comps, b.resource, b.name)

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
	total           int // Added total field to store the total number of requests
	maxRetries      int
}

func newRequestPostDelBuilder(src *types.RequestPostDel, resourceVersion string, total int, maxRetries int) *requestPostDelBuilder {
	return &requestPostDelBuilder{
		version:         schema.GroupVersion{Group: src.Group, Version: src.Version},
		resource:        src.Resource,
		resourceVersion: resourceVersion,
		namespace:       src.Namespace,
		deleteRatio:     src.DeleteRatio,
		total:           total, // Initialize total field
		maxRetries:      maxRetries,
	}
}

// var (
// 	postCache = struct {
// 		sync.Mutex
// 		items []string
// 	}{}
// )

// Build implements RequestBuilder.Build.
func (b *requestPostDelBuilder) Build(cli rest.Interface) Requester {
	// Calculate POST and DELETE request distribution using b.total
	// postCount, deleteCount := int(float64(b.total)*(1-b.deleteRatio)), int(float64(b.total)*b.deleteRatio)

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

	// POST request - generate random name
	randomNum, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	name := fmt.Sprintf("%s-%d", b.namespace, randomNum.Int64())
	// postCache.Lock()
	// postCache.items = append(postCache.items, name)
	// postCache.Unlock()

	body, err := utils.RenderTemplate(b.resource, name, b.namespace)
	if err != nil {
		panic(err)
	}

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "POST",
			req:    cli.Post().AbsPath(comps...).Body(body).MaxRetries(b.maxRetries),
		},
	}

	// postCache.Lock()
	// cacheSize := len(postCache.items)
	// postCache.Unlock()

	// if cacheSize > 0 && deleteCount > 0 {
	// 	// DELETE request
	// 	postCache.Lock()
	// 	if len(postCache.items) == 0 {
	// 		postCache.Unlock()
	// 		return nil // No items to delete
	// 	}
	// 	nameIndex := mrand.Intn(len(postCache.items))
	// 	name := postCache.items[nameIndex]
	// 	postCache.items = append(postCache.items[:nameIndex], postCache.items[nameIndex+1:]...)
	// 	postCache.Unlock()

	// 	if b.namespace != "" {
	// 		comps = append(comps, "namespaces", b.namespace)
	// 	}
	// 	comps = append(comps, name)

	// 	deleteCount-- // Decrement DELETE request count
	// 	return &DiscardRequester{
	// 		BaseRequester: BaseRequester{
	// 			method: "DELETE",
	// 			req:    cli.Delete().AbsPath(comps...).MaxRetries(b.maxRetries),
	// 		},
	// 	}
	// } else if postCount > 0 {
	// 	// POST request
	// 	name := b.namespace + "-" + time.Now().Format("20060102150405") // Use timestamp for unique name
	// 	postCache.Lock()
	// 	postCache.items = append(postCache.items, name)
	// 	postCache.Unlock()

	// 	if b.namespace != "" {
	// 		comps = append(comps, "namespaces", b.namespace)
	// 	}
	// 	comps = append(comps, b.resource)

	// 	body, err := utils.RenderTemplate(b.resource, name, b.namespace)
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	postCount-- // Decrement POST request count
	// 	return &DiscardRequester{
	// 		BaseRequester: BaseRequester{
	// 			method: "POST",
	// 			req:    cli.Post().AbsPath(comps...).Body(body).MaxRetries(b.maxRetries),
	// 		},
	// 	}
	// }

}

func toPtr[T any](v T) *T {
	return &v
}
