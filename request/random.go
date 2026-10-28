// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package request

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"

	"github.com/Azure/kperf/api/types"
)

// WeightedRandomRequests is used to generate requests based on LoadProfileSpec.
type WeightedRandomRequests struct {
	once         sync.Once
	wg           sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	reqBuilderCh chan RESTRequestBuilder

	shares      []int
	reqBuilders []RESTRequestBuilder
}

// NewWeightedRandomRequests creates new instance of WeightedRandomRequests.
func NewWeightedRandomRequests(spec *types.LoadProfileSpec) (*WeightedRandomRequests, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid load profile spec: %v", err)
	}

	shares := make([]int, 0, len(spec.Requests))
	reqBuilders := make([]RESTRequestBuilder, 0, len(spec.Requests))
	for _, r := range spec.Requests {
		shares = append(shares, r.Shares)

		var builder RESTRequestBuilder
		switch {
		case r.StaleList != nil:
			builder = newRequestListBuilder(r.StaleList, "0", spec.MaxRetries)
		case r.QuorumList != nil:
			builder = newRequestListBuilder(r.QuorumList, "", spec.MaxRetries)
		case r.WatchList != nil:
			builder = newRequestWatchListBuilder(r.WatchList, spec.MaxRetries)
		case r.StaleGet != nil:
			builder = newRequestGetBuilder(r.StaleGet, "0", spec.MaxRetries)
		case r.QuorumGet != nil:
			builder = newRequestGetBuilder(r.QuorumGet, "", spec.MaxRetries)
		case r.GetPodLog != nil:
			builder = newRequestGetPodLogBuilder(r.GetPodLog, spec.MaxRetries)
		case r.Patch != nil:
			builder = newRequestPatchBuilder(r.Patch, "", spec.MaxRetries)
		case r.PostDel != nil:
			builder = newRequestPostDelBuilder(r.PostDel, "", spec.MaxRetries)
		case r.Put != nil:
			builder = newRequestPutBuilder(r.Put, spec.MaxRetries)
		default:
			return nil, fmt.Errorf("unknown request type: %+v", r)
		}
		reqBuilders = append(reqBuilders, builder)
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &WeightedRandomRequests{
		ctx:          ctx,
		cancel:       cancel,
		reqBuilderCh: make(chan RESTRequestBuilder),
		shares:       shares,
		reqBuilders:  reqBuilders,
	}, nil
}

// Run starts to random pick request.
func (r *WeightedRandomRequests) Run(ctx context.Context, total int) {
	defer r.wg.Done()
	r.wg.Add(1)

	sum := 0
	for total <= 0 || sum < total {
		builder := r.randomPick()
		select {
		case r.reqBuilderCh <- builder:
			sum++
		case <-r.ctx.Done():
			return
		case <-ctx.Done():
			return
		}
	}
}

// Chan returns channel to get random request.
func (r *WeightedRandomRequests) Chan() chan RESTRequestBuilder {
	return r.reqBuilderCh
}

func (r *WeightedRandomRequests) randomPick() RESTRequestBuilder {
	sum := 0
	for _, s := range r.shares {
		sum += s
	}

	rndInt, err := rand.Int(rand.Reader, big.NewInt(int64(sum)))
	if err != nil {
		panic(err)
	}

	rnd := rndInt.Int64()
	for i := range r.shares {
		s := int64(r.shares[i])
		if rnd < s {
			return r.reqBuilders[i]
		}
		rnd -= s
	}
	panic("unreachable")
}

// Stop stops request generator.
func (r *WeightedRandomRequests) Stop() {
	r.once.Do(func() {
		r.cancel()
		r.wg.Wait()
		close(r.reqBuilderCh)
	})
}

// RandomPayload returns a string of exactly n bytes, uniformly sampled from
// [a-zA-Z0-9] via crypto/rand with rejection sampling to avoid modulo bias.
// It returns "" when n <= 0 and panics if reading random bytes fails.
func RandomPayload(n int) string {
	if n <= 0 {
		return ""
	}
	b, err := RandBytes(n)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// RandBytes returns n uniformly sampled alphanumeric bytes using crypto/rand.
// It returns an error for non-positive lengths or if reading random bytes fails.
func RandBytes(n int) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("length must be positive")
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// Only use byte values in [0, threshold). Since len(alphabet)=62 and
	// 248 is the largest multiple of 62 below 256, mapping b%62 is uniform
	// for accepted bytes. Bytes 248..255 are skipped to avoid modulo bias.
	const threshold = byte(256 - 256%len(alphabet))

	out := make([]byte, n)

	bufSize := n
	if bufSize > 4096 {
		bufSize = 4096
	}
	buf := make([]byte, bufSize)

	for i := 0; i < n; {
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("failed to read random bytes: %w", err)
		}

		for _, b := range buf {
			if b >= threshold {
				continue
			}
			out[i] = alphabet[int(b)%len(alphabet)]
			i++
			if i == n {
				return out, nil
			}
		}
	}

	return out, nil
}
