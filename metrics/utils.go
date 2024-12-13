// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package metrics

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"syscall"

	"github.com/Azure/kperf/api/types"
	"golang.org/x/net/http2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// BuildPercentileLatencies builds percentile latencies.
func BuildPercentileLatencies(latencies []float64) [][2]float64 {
	if len(latencies) == 0 {
		return nil
	}

	var percentiles = []float64{0, 0.5, 0.90, 0.95, 0.99, 1}

	res := make([][2]float64, len(percentiles))

	n := len(latencies)
	sort.Float64s(latencies)
	for pi, pv := range percentiles {
		idx := int(math.Ceil(float64(n) * pv))
		if idx > 0 {
			idx--
		}
		res[pi] = [2]float64{pv, latencies[idx]}
	}
	return res
}

// BuildErrorStatsGroupByType summaries total count for each type of errors.
func BuildErrorStatsGroupByType(errors []types.ResponseError) map[string]int32 {
	res := map[string]int32{}

	for _, err := range errors {
		var key string
		switch err.Type {
		case types.ResponseErrorTypeHTTP:
			key = fmt.Sprintf("%s/%d", err.Type, err.Code)
		default:
			key = fmt.Sprintf("%s/%s", err.Type, err.Message)
		}
		res[key]++
	}
	return res
}

var (
	// errHTTP2ClientConnectionLost is used to track unexported http2 error.
	errHTTP2ClientConnectionLost = errors.New("http2: client connection lost")

	// errTLSHandshakeTimeout is used to track unexported tlsHandshakeTimeoutError from net/http.
	errTLSHandshakeTimeout = errors.New("net/http: TLS handshake timeout")
)

// codeFromHTTP parses error to get http code.
func codeFromHTTP(err error) int {
	if err == nil {
		return 0
	}

	switch {
	case apierrors.IsBadRequest(err):
		return http.StatusBadRequest // 400
	case apierrors.IsUnauthorized(err):
		return http.StatusUnauthorized // 401
	case apierrors.IsForbidden(err):
		return http.StatusForbidden // 403
	case apierrors.IsNotFound(err):
		return http.StatusNotFound // 404
	case apierrors.IsMethodNotSupported(err):
		return http.StatusMethodNotAllowed // 405
	case apierrors.IsNotAcceptable(err):
		return http.StatusNotAcceptable // 406
	case apierrors.IsAlreadyExists(err):
		return http.StatusConflict // 409
	case apierrors.IsGone(err):
		return http.StatusGone // 410
	case apierrors.IsRequestEntityTooLargeError(err):
		return http.StatusRequestEntityTooLarge // 413
	case apierrors.IsUnsupportedMediaType(err):
		return http.StatusUnsupportedMediaType // 415
	case apierrors.IsInvalid(err):
		return http.StatusUnprocessableEntity // 422
	case apierrors.IsTooManyRequests(err):
		return http.StatusTooManyRequests // 429
	case apierrors.IsInternalError(err):
		return http.StatusInternalServerError // 500
	case apierrors.IsServiceUnavailable(err):
		return http.StatusServiceUnavailable // 503
	case apierrors.IsTimeout(err):
		return http.StatusGatewayTimeout // 504
	default:
		if status, ok := err.(apierrors.APIStatus); ok || errors.As(err, &status) {
			return int(status.Status().Code)
		}
		return 0
	}
}

// isHTTP2Error returns true if it's related to http2 error.
func isHTTP2Error(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	if connErr, ok := err.(http2.ConnectionError); ok || errors.As(err, &connErr) {
		return (http2.ErrCode(connErr)).String(), true
	}

	if streamErr, ok := err.(http2.StreamError); ok || errors.As(err, &streamErr) {
		return streamErr.Code.String(), true
	}

	if connErr, ok := err.(http2.GoAwayError); ok || errors.As(err, &connErr) {
		return fmt.Sprintf("http2: server sent GOAWAY and closed the connection; ErrCode=%v, debug=%s",
			connErr.ErrCode, connErr.DebugData), true
	}

	if strings.Contains(err.Error(), errHTTP2ClientConnectionLost.Error()) {
		return errHTTP2ClientConnectionLost.Error(), true
	}
	return "", false
}

// isConnectionError returns true if it's related to connection error.
func isConnectionError(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	switch {
	case isTimeoutError(err):
		return err.Error(), true
	case isConnectionRefused(err):
		return syscall.ECONNREFUSED.Error(), true
	case isConnectionResetByPeer(err):
		return syscall.ECONNRESET.Error(), true
	case errors.Is(err, io.ErrUnexpectedEOF):
		return io.ErrUnexpectedEOF.Error(), true
	case errors.Is(err, io.EOF):
		return io.EOF.Error(), true
	case strings.Contains(err.Error(), errTLSHandshakeTimeout.Error()):
		return errTLSHandshakeTimeout.Error(), true
	default:
		return "", false
	}
}

// isTimeoutError returns true if it's related to golang standard library
// net's timeout error.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	terr, ok := err.(net.Error)
	if !ok {
		if !errors.As(err, &terr) {
			return false
		}
	}
	return terr.Timeout()
}

// isConnectionRefused returns true if the error is connection refused
func isConnectionRefused(err error) bool {
	if err == nil {
		return false
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.ECONNREFUSED
	}
	return false
}

// isConnectionResetByPeer returns true if the error is "connection reset by peer".
func isConnectionResetByPeer(err error) bool {
	if err == nil {
		return false
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.ECONNRESET
	}
	return false
}
