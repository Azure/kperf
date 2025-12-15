// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"fmt"

	"github.com/Azure/kperf/api/types"
	"github.com/Azure/kperf/request/executor"
)

func init() {
	// Register the request builder factories with the executor package
	executor.SetRequestBuilderFactory(CreateRequestBuilder)
	executor.SetExactRequestBuilderFactory(CreateRequestBuilderFromExact)
}

// CreateRequestBuilder creates a RESTRequestBuilder from a WeightedRequest.
// This function is used by weighted-random mode executors.
func CreateRequestBuilder(r *types.WeightedRequest, maxRetries int) (executor.RESTRequestBuilder, error) {
	var builder executor.RESTRequestBuilder
	switch {
	case r.StaleList != nil:
		builder = newRequestListBuilder(r.StaleList, "0", maxRetries)
	case r.QuorumList != nil:
		builder = newRequestListBuilder(r.QuorumList, "", maxRetries)
	case r.WatchList != nil:
		builder = newRequestWatchListBuilder(r.WatchList, maxRetries)
	case r.StaleGet != nil:
		builder = newRequestGetBuilder(r.StaleGet, "0", maxRetries)
	case r.QuorumGet != nil:
		builder = newRequestGetBuilder(r.QuorumGet, "", maxRetries)
	case r.GetPodLog != nil:
		builder = newRequestGetPodLogBuilder(r.GetPodLog, maxRetries)
	case r.Patch != nil:
		builder = newRequestPatchBuilder(r.Patch, "", maxRetries)
	case r.PostDel != nil:
		builder = newRequestPostDelBuilder(r.PostDel, "", maxRetries)
	default:
		return nil, fmt.Errorf("unsupported request type")
	}
	return builder, nil
}

// CreateRequestBuilderFromExact creates a RESTRequestBuilder from an ExactRequest.
// This function is used by time-series and other exact-replay mode executors.
func CreateRequestBuilderFromExact(req *types.ExactRequest, maxRetries int) (executor.RESTRequestBuilder, error) {
	resourceVersion := req.ResourceVersion

	switch req.Method {
	case "GET":
		return newRequestGetBuilder(&types.RequestGet{
			KubeGroupVersionResource: types.KubeGroupVersionResource{
				Group:    req.Group,
				Version:  req.Version,
				Resource: req.Resource,
			},
			Namespace: req.Namespace,
			Name:      req.Name,
		}, resourceVersion, maxRetries), nil

	case "LIST":
		return newRequestListBuilder(&types.RequestList{
			KubeGroupVersionResource: types.KubeGroupVersionResource{
				Group:    req.Group,
				Version:  req.Version,
				Resource: req.Resource,
			},
			Namespace:     req.Namespace,
			Limit:         req.Limit,
			Selector:      req.LabelSelector,
			FieldSelector: req.FieldSelector,
		}, resourceVersion, maxRetries), nil

	case "PATCH":
		patchType, ok := types.GetPatchType(req.PatchType)
		if !ok {
			return nil, fmt.Errorf("invalid patch type: %s", req.PatchType)
		}
		return newRequestPatchBuilder(&types.RequestPatch{
			KubeGroupVersionResource: types.KubeGroupVersionResource{
				Group:    req.Group,
				Version:  req.Version,
				Resource: req.Resource,
			},
			Namespace: req.Namespace,
			Name:      req.Name,
			Body:      req.Body,
			PatchType: string(patchType),
		}, resourceVersion, maxRetries), nil

	case "POST":
		return newRequestPostDelBuilder(&types.RequestPostDel{
			KubeGroupVersionResource: types.KubeGroupVersionResource{
				Group:    req.Group,
				Version:  req.Version,
				Resource: req.Resource,
			},
			Namespace: req.Namespace,
		}, resourceVersion, maxRetries), nil

	case "DELETE":
		return newRequestPostDelBuilder(&types.RequestPostDel{
			KubeGroupVersionResource: types.KubeGroupVersionResource{
				Group:    req.Group,
				Version:  req.Version,
				Resource: req.Resource,
			},
			Namespace:   req.Namespace,
			DeleteRatio: 1.0,
		}, resourceVersion, maxRetries), nil

	default:
		return nil, fmt.Errorf("unsupported method: %s", req.Method)
	}
}
