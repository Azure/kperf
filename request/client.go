package request

import (
	"math"
	"net/http"

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
func NewClients(kubeCfgPath string, num int, userAgent string, qps int) ([]rest.Interface, error) {
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
	//set number of connections per host
	restCfg.Transport = &http.Transport{
		MaxConnsPerHost: num,
	}

	//set Protobuf as accepted content
	restCfg.ContentType = "application/vnd.kubernetes.protobuf"

	restClients := make([]rest.Interface, 0, num)
	for i := 0; i < num; i++ {
		cfgShallowCopy := *restCfg

		restCli, err := rest.UnversionedRESTClientFor(&cfgShallowCopy)
		if err != nil {
			return nil, err
		}
		restClients = append(restClients, restCli)
	}
	return restClients, nil
}
