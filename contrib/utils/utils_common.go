// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package utils

import (
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
