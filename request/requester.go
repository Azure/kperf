package request

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"time"
	_ "unsafe" // unsafe to use internal function from client-go

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"
)

type Requester interface {
	Method() string
	URL() *url.URL
	Timeout(time.Duration)
	Do(context.Context) (bytes int64, err error)
}

type BaseRequester struct {
	method string
	req    *rest.Request
}

func (reqr *BaseRequester) Method() string {
	return reqr.method
}

func (reqr *BaseRequester) URL() *url.URL {
	return reqr.req.URL()
}

func (reqr *BaseRequester) Timeout(timeout time.Duration) {
	reqr.req.Timeout(timeout)
}

type DiscardRequester struct {
	BaseRequester
}

func (reqr *DiscardRequester) Do(ctx context.Context) (bytes int64, err error) {
	respBody, err := reqr.req.Stream(ctx)
	if err != nil {
		return 0, err
	}
	defer respBody.Close()

	return io.Copy(io.Discard, respBody)
}

type WatchListRequester struct {
	BaseRequester
}

func (reqr *WatchListRequester) Do(ctx context.Context) (zero int64, _ error) {
	cl := clock.RealClock{}
	temporaryStore := cache.NewStore(cache.DeletionHandlingMetaNamespaceKeyFunc)

	start := time.Now()

	w, err := reqr.req.Watch(ctx)
	if err != nil {
		return zero, err
	}
	watchListBookmarkReceived, err := handleAnyWatch(start, w, temporaryStore, nil, nil, "", "", func(_ string) {}, true, cl, make(chan error), ctx.Done())
	w.Stop()
	if err != nil {
		return zero, err
	}

	if watchListBookmarkReceived {
		return zero, nil
	}
	return zero, fmt.Errorf("don't receive bookmark")
}

//go:linkname handleAnyWatch k8s.io/client-go/tools/cache.handleAnyWatch
func handleAnyWatch(start time.Time,
	w watch.Interface,
	store cache.Store,
	expectedType reflect.Type,
	expectedGVK *schema.GroupVersionKind,
	name string,
	expectedTypeName string,
	setLastSyncResourceVersion func(string),
	exitOnWatchListBookmarkReceived bool,
	clock clock.Clock,
	errCh chan error,
	stopCh <-chan struct{},
) (bool, error)
