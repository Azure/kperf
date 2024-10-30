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

	"github.com/Azure/kperf/api/types"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestResponseMetric_ObserveFailure(t *testing.T) {
	expectedStats := types.ResponseErrorStats{
		UnknownErrors: []string{
			"unknown",
		},
		ResponseCodes: map[int]int32{
			429: 1,
			500: 1,
			504: 1,
		},
		NetErrors: map[string]int32{
			"net/http: TLS handshake timeout": 2,
			"connection reset by peer":        1,
			"connection refused":              1,
			"unexpected EOF":                  1,
			"context deadline exceeded":       1,
		},
		HTTP2Errors: types.HTTP2ErrorStats{
			ConnectionErrors: map[string]int32{
				"http2: client connection lost": 2,
				"http2: server sent GOAWAY and closed the connection; ErrCode=NO_ERROR, debug=\"\"":       1,
				"http2: server sent GOAWAY and closed the connection; ErrCode=PROTOCOL_ERROR, debug=\"\"": 1,
			},
			StreamErrors: map[string]int32{
				"CONNECT_ERROR": 1,
			},
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
	for _, err := range errs {
		m.ObserveFailure(err)
	}
	stats := m.Gather().ErrorStats
	assert.Equal(t, expectedStats, stats)
}
