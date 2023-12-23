package request

import (
	"fmt"
	"math"

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
// 3. Support Protobuf as accepted content
func NewClients(kubeCfgPath string, ConnsNum int, userAgent string, qps int, contentType string) ([]rest.Interface, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeCfgPath)

	if err != nil {
		return nil, err
	}

	if qps == 0 {
		qps = math.MaxInt32
	}

	restCfg.QPS = float32(qps)
	restCfg.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	restCfg.UserAgent = userAgent

	if restCfg.UserAgent == "" {
		restCfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	// Set the content type
	switch contentType {
	case "json":
		restCfg.ContentType = "application/json"
	case "protobuf":
		restCfg.ContentType = "application/vnd.kubernetes.protobuf"
	default:
		return nil, fmt.Errorf("invalid content type: %s", contentType)
	}

	restClients := make([]rest.Interface, 0, ConnsNum)
	for i := 0; i < ConnsNum; i++ {
		cfgShallowCopy := *restCfg

		restCli, err := rest.UnversionedRESTClientFor(&cfgShallowCopy)
		if err != nil {
			fmt.Printf("Failed to create rest client: %v\n", err)
			return nil, err
		}
		restClients = append(restClients, restCli)
	}
	return restClients, nil
}
