// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
	"context"
	"fmt"
	"time"
)

type rollingUpdateTimeoutOption struct {
	restartTimeout  time.Duration
	rolloutTimeout  time.Duration
	rolloutInterval time.Duration
}
type jobsTimeoutOption struct {
	jobInterval   time.Duration
	applyTimeout  time.Duration
	waitTimeout   time.Duration
	deleteTimeout time.Duration
}

type RollingUpdateTimeoutOpt func(*rollingUpdateTimeoutOption)

func WithRollingUpdateRestartTimeoutOpt(to time.Duration) RollingUpdateTimeoutOpt {
	return func(rto *rollingUpdateTimeoutOption) {
		rto.restartTimeout = to
	}
}

func WithRollingUpdateRolloutTimeoutOpt(to time.Duration) RollingUpdateTimeoutOpt {
	return func(rto *rollingUpdateTimeoutOption) {
		rto.rolloutTimeout = to
	}
}

func WithRollingUpdateIntervalTimeoutOpt(to time.Duration) RollingUpdateTimeoutOpt {
	return func(rto *rollingUpdateTimeoutOption) {
		rto.rolloutInterval = to
	}
}

type JobTimeoutOpt func(*jobsTimeoutOption)

func WithJobIntervalOpt(to time.Duration) JobTimeoutOpt {
	return func(jto *jobsTimeoutOption) {
		jto.jobInterval = to
	}
}
func WithJobApplyTimeoutOpt(to time.Duration) JobTimeoutOpt {
	return func(jto *jobsTimeoutOption) {
		jto.applyTimeout = to
	}
}

func WithJobWaitTimeoutOpt(to time.Duration) JobTimeoutOpt {
	return func(jto *jobsTimeoutOption) {
		jto.waitTimeout = to
	}
}

func WithJobDeleteTimeoutOpt(to time.Duration) JobTimeoutOpt {
	return func(jto *jobsTimeoutOption) {
		jto.deleteTimeout = to

	}
}

var deploymentBatchSize int = 20

type DeploymentBatchManager struct {
	KubeCfgPath           string
	DeploymentNamePattern string
	DeploymentReplica     int
	PaddingBytes          int
	cleanups              []func()
}

func (bm *DeploymentBatchManager) Add(ctx context.Context, total int) error {
	for start := 0; start < total; start += deploymentBatchSize {
		// Create a unique name for each deployment batch
		namePattern := fmt.Sprintf("%s-%d", bm.DeploymentNamePattern, start/deploymentBatchSize)

		// Calculate the current batch size, ensuring it does not exceed the total
		currentBatchSize := deploymentBatchSize
		if start+currentBatchSize > total {
			currentBatchSize = total - start
		}

		cleanup, err := DeployDeployments(ctx, bm.KubeCfgPath, namePattern, start+currentBatchSize, bm.DeploymentReplica,
			bm.PaddingBytes, start, 10*time.Minute)
		if err != nil {
			return err
		}
		// Store the cleanup function to be called later
		bm.cleanups = append(bm.cleanups, cleanup)
	}
	return nil
}

func (bm *DeploymentBatchManager) CleanAll() {
	for i := len(bm.cleanups) - 1; i >= 0; i-- {
		bm.cleanups[i]()
	}
}
