// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/kperf/api/types"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestResponseMetric_ObserveFailure(t *testing.T) {
	observedAt := time.Now()
	dur := 10 * time.Second

	expectedErrors := []types.ResponseError{
		{
			URL:       "0",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP,
			Code:      429,
		},
		{
			URL:       "1",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP,
			Code:      500,
		},
		{
			URL:       "2",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP,
			Code:      504,
		},
		{
			URL:       "3",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP2Protocol,
			Message:   "http2: server sent GOAWAY and closed the connection; ErrCode=NO_ERROR, debug=",
		},
		{
			URL:       "4",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP2Protocol,
			Message:   "http2: server sent GOAWAY and closed the connection; ErrCode=PROTOCOL_ERROR, debug=",
		},
		{
			URL:       "5",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP2Protocol,
			Message:   "http2: client connection lost",
		},
		{
			URL:       "6",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP2Protocol,
			Message:   "http2: client connection lost",
		},
		{
			URL:       "7",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeHTTP2Protocol,
			Message:   http2.ErrCode(10).String(),
		},
		{
			URL:       "8",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeConnection,
			Message:   "net/http: TLS handshake timeout",
		},
		{
			URL:       "9",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeConnection,
			Message:   "net/http: TLS handshake timeout",
		},
		{
			URL:       "10",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeConnection,
			Message:   "context deadline exceeded",
		},
		{
			URL:       "11",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeConnection,
			Message:   syscall.ECONNRESET.Error(),
		},
		{
			URL:       "12",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeConnection,
			Message:   syscall.ECONNREFUSED.Error(),
		},
		{
			URL:       "13",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeConnection,
			Message:   io.ErrUnexpectedEOF.Error(),
		},
		{
			URL:       "14",
			Timestamp: observedAt,
			Duration:  dur.Seconds(),
			Type:      types.ResponseErrorTypeUnknown,
			Message:   "unknown",
		},
	}

	errs := []error{
		// http code
		apierrors.NewTooManyRequestsError("retry it later"),
		apierrors.NewInternalError(errors.New("oops")),
		apierrors.NewTimeoutError("timeout in test", 100),
		// http2
		http2.GoAwayError{
			LastStreamID: 1000,
			ErrCode:      0,
		},
		fmt.Errorf("oops: %w",
			http2.GoAwayError{
				LastStreamID: 1000,
				ErrCode:      1,
			},
		),
		errHTTP2ClientConnectionLost,
		fmt.Errorf("oops: %w", errHTTP2ClientConnectionLost),
		http2.StreamError{
			StreamID: 100,
			Code:     10,
		},
		// net
		errTLSHandshakeTimeout,
		fmt.Errorf("oops: %w", errTLSHandshakeTimeout),
		context.DeadlineExceeded, // i/o timeout
		fmt.Errorf("oops: %w", syscall.ECONNRESET),
		fmt.Errorf("oops: %w", syscall.ECONNREFUSED),
		fmt.Errorf("oops: %w", io.ErrUnexpectedEOF),
		// unknown
		fmt.Errorf("unknown"),
	}

	m := NewResponseMetric()
	for idx, err := range errs {
		m.ObserveFailure(fmt.Sprintf("%d", idx), observedAt, dur.Seconds(), err)
	}
	errors := m.Gather().Errors
	assert.Equal(t, expectedErrors, errors)
}
