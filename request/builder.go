// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"strings"
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

// When this placeholder is present in a patch body, it is
// replaced on every request with a random string. This guarantees each patch
// actually mutates the target object.
const patchRandomPlaceholder = "{{.Random}}"

type requestPatchBuilder struct {
	version         schema.GroupVersion
	resource        string
	resourceVersion string
	namespace       string
	name            string
	keySpaceSize    int
	patchType       apitypes.PatchType
	body            string
	maxRetries      int
}

// renderPatchBody replaces every occurrence of patchRandomPlaceholder in raw
// with a random string.
func renderPatchBody(raw string) []byte {
	if !strings.Contains(raw, patchRandomPlaceholder) {
		return []byte(raw)
	}
	return []byte(strings.ReplaceAll(raw, patchRandomPlaceholder, RandomPayload(8)))
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
		body:            src.Body,
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

	// Render a unique body per request if patchRandomPlaceholder exists. Bodies without the
	// placeholder pass through unchanged.
	reqBody := renderPatchBody(b.body)

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "PATCH",
			req: cli.Patch(b.patchType).AbsPath(comps...).
				Body(reqBody).
				MaxRetries(b.maxRetries),
		},
	}
}

type requestPostDelBuilder struct {
	version            schema.GroupVersion
	resource           string
	resourceVersion    string
	namespace          string
	deleteRatio        float64
	payloadSize        int
	nodeSelector       map[string]string
	labels             map[string]string
	serviceAccountName string
	schedulerName      string
	maxRetries         int

	// Per-builder cache for created resources
	cache *Cache

	// Per-builder atomic counter for unique ID generation
	resourceCounter int64
}

func newRequestPostDelBuilder(src *types.RequestPostDel, resourceVersion string, maxRetries int) *requestPostDelBuilder {
	b := &requestPostDelBuilder{
		version:         schema.GroupVersion{Group: src.Group, Version: src.Version},
		resource:        src.Resource,
		resourceVersion: resourceVersion,
		namespace:       src.Namespace,
		deleteRatio:     src.DeleteRatio,
		payloadSize:     src.PayloadSize,
		maxRetries:      maxRetries,
		cache:           InitCache(), // Initialize the cache
	}
	if p := src.Pod; p != nil {
		b.nodeSelector = maps.Clone(p.NodeSelector)
		b.labels = maps.Clone(p.Labels)
		b.serviceAccountName = p.ServiceAccountName
		b.schedulerName = p.SchedulerName
	}
	return b
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

	body, err := utils.RenderTemplate(b.resource, map[string]interface{}{
		"namePattern":        name,
		"namespace":          b.namespace,
		"payload":            RandomPayload(b.payloadSize),
		"nodeSelector":       b.nodeSelector,
		"labels":             b.labels,
		"serviceAccountName": b.serviceAccountName,
		"schedulerName":      b.schedulerName,
	})
	if err != nil {
		panic(fmt.Errorf("failed to render %s template: %w", b.resource, err))
	}

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
		// Create-only runs retain objects, but do not need a deletion-name cache.
		if err == nil && reqr.builder.deleteRatio > 0 {
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

// requestPutBuilder builds PUT requests that overwrite an existing ConfigMap
// named `<name>-<rand[0, keySpaceSize)>` with a body whose `data.payload` field
// carries `payloadSize` random bytes. The target ConfigMap must already exist
// (e.g. created via `runkperf data cm add`); otherwise the apiserver returns
// 404 and the request is counted as a failure. Only `configmaps` is supported
// today — see RequestPut.Validate.
type requestPutBuilder struct {
	version      schema.GroupVersion
	resource     string
	namespace    string
	name         string
	keySpaceSize int
	payloadSize  int
	maxRetries   int
}

func newRequestPutBuilder(src *types.RequestPut, maxRetries int) *requestPutBuilder {
	return &requestPutBuilder{
		version: schema.GroupVersion{
			Group:   src.Group,
			Version: src.Version,
		},
		resource:     src.Resource,
		namespace:    src.Namespace,
		name:         src.Name,
		keySpaceSize: src.KeySpaceSize,
		payloadSize:  src.PayloadSize,
		maxRetries:   maxRetries,
	}
}

// Build implements RESTRequestBuilder.Build.
func (b *requestPutBuilder) Build(cli rest.Interface) Requester {
	comps := []string{"api", b.version.Version, "namespaces", b.namespace, b.resource}

	randomInt, _ := rand.Int(rand.Reader, big.NewInt(int64(b.keySpaceSize)))
	finalName := fmt.Sprintf("%s-%d", b.name, randomInt.Int64())
	comps = append(comps, finalName)

	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      finalName,
			Namespace: b.namespace,
		},
		Data: map[string]string{"payload": RandomPayload(b.payloadSize)},
	}
	body, _ := json.Marshal(cm)

	return &DiscardRequester{
		BaseRequester: BaseRequester{
			method: "PUT",
			req:    cli.Put().AbsPath(comps...).Body(body).MaxRetries(b.maxRetries),
		},
	}
}
