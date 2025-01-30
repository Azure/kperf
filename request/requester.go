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
	respBody, err := reqr.req.Stream(context.Background())
	bytes = 0
	if err == nil {
		defer respBody.Close()
		bytes, err = io.Copy(io.Discard, respBody)
		// Based on HTTP2 Spec Section 8.1 [1],
		//
		// A server can send a complete response prior to the client
		// sending an entire request if the response does not depend
		// on any portion of the request that has not been sent and
		// received. When this is true, a server MAY request that the
		// client abort transmission of a request without error by
		// sending a RST_STREAM with an error code of NO_ERROR after
		// sending a complete response (i.e., a frame with the END_STREAM
		// flag). Clients MUST NOT discard responses as a result of receiving
		// such a RST_STREAM, though clients can always discard responses
		// at their discretion for other reasons.
		//
		// We should mark NO_ERROR as nil here.
		//
		// [1]: https://httpwg.org/specs/rfc7540.html#HttpSequence
		if err != nil && isHTTP2StreamNoError(err) {
			err = nil
		}
	}
	return bytes, err
}

type WatchListRequester struct {
	BaseRequester
}

func (reqr *WatchListRequester) Do(ctx context.Context) (bytes int64, err error) {
	result := &unstructured.UnstructuredList{}
	err = reqr.req.WatchList(ctx).Into(result)
	return 0, err
}
