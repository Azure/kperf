package request

import (
	"context"
	"io"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
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

func (reqr *WatchListRequester) Do(ctx context.Context) (bytes int64, err error) {
	result := &unstructured.UnstructuredList{}
	err = reqr.req.WatchList(ctx).Into(result)
	return 0, err
}
