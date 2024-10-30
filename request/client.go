// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"fmt"
	"math"
	"net/http"

	"github.com/Azure/kperf/api/types"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/scheme"
)

// NewClients creates N rest.Interface.
//
// FIXME(weifu):
//
// 1. Is it possible to build one http2 client with multiple connections?
// 2. How to monitor HTTP2 GOAWAY frame?
func NewClients(kubeCfgPath string, connsNum int, opts ...ClientCfgOpt) ([]rest.Interface, error) {
	var cfg = defaultClientCfg
	for _, opt := range opts {
		opt(&cfg)
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)
	if err != nil {
		return nil, err
	}
	restCfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	// NOTE:
	//
	// Make transport uncacheable. With default proxy function, client-go
	// will create new transport even if multiple clients use the same TLS
	// configuration. If not, all the clients will share one transport.
	// If protocol is HTTP2, there will be only one connection.
	//
	// REF: https://github.com/kubernetes/client-go/blob/c5938c6876a62f53c1f4ee55b879ca5c74253ae8/transport/cache.go#L154
	restCfg.Proxy = http.ProxyFromEnvironment

	err = cfg.apply(restCfg)
	if err != nil {
		return nil, err
	}

	restClients := make([]rest.Interface, 0, connsNum)
	for i := 0; i < connsNum; i++ {
		cfgShallowCopy := *restCfg

		restCli, err := rest.UnversionedRESTClientFor(&cfgShallowCopy)
		if err != nil {
			return nil, err
		}
		restClients = append(restClients, restCli)
	}
	return restClients, nil
}

// defaultClientCfg is default setting for http client.
var defaultClientCfg = clientCfg{
	qps:         float64(math.MaxInt32),
	contentType: types.ContentTypeJSON,
}

type clientCfg struct {
	userAgent    string
	qps          float64
	contentType  types.ContentType
	disableHTTP2 bool
}

// apply sets value to k8s.io/client-go/rest.Config.
func (cfg *clientCfg) apply(restCfg *rest.Config) error {
	// set qps
	restCfg.QPS = float32(cfg.qps)

	// set user agent
	restCfg.UserAgent = cfg.userAgent
	if restCfg.UserAgent == "" {
		restCfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	// set the content type
	switch cfg.contentType {
	case types.ContentTypeJSON:
		restCfg.ContentType = "application/json"
	case types.ContentTypeProtobuffer:
		restCfg.ContentType = "application/vnd.kubernetes.protobuf"
	default:
		return fmt.Errorf("invalid content type: %s", cfg.contentType)
	}

	// disable HTTP2
	if cfg.disableHTTP2 {
		restCfg.NextProtos = []string{"http/1.1"}
	}
	return nil
}

// ClientCfgOpt is used to update default client setting.
type ClientCfgOpt func(*clientCfg)

// WithClientQPSOpt updates QPS value.
func WithClientQPSOpt(qps float64) ClientCfgOpt {
	return func(cfg *clientCfg) {
		if qps > 0 {
			cfg.qps = qps
		}
	}
}

// WithClientUserAgentOpt updates user agent.
func WithClientUserAgentOpt(ua string) ClientCfgOpt {
	return func(cfg *clientCfg) {
		cfg.userAgent = ua
	}
}

// WithClientContentTypeOpt updates content type of response.
func WithClientContentTypeOpt(ct types.ContentType) ClientCfgOpt {
	return func(cfg *clientCfg) {
		cfg.contentType = ct
	}
}

// WithClientDisableHTTP2Opt disables HTTP2 protocol.
func WithClientDisableHTTP2Opt(b bool) ClientCfgOpt {
	return func(cfg *clientCfg) {
		cfg.disableHTTP2 = b
	}
}
