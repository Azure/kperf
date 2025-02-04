// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package log

import (
	"fmt"

	"k8s.io/klog/v2"
)

type klogger struct {
	level klog.Level
	kvs   []any
}

// Logf implements Logger.Logf.
func (kl klogger) Logf(msg string, args ...any) {
	if len(kl.kvs) > 0 {
		klog.V(kl.level).InfoS(fmt.Sprintf(msg, args...), kl.kvs...)
		return
	}
	klog.V(kl.level).Infof(msg, args...)
}

// LogKV implements Logger.LogKV.
func (kl klogger) LogKV(kvs ...any) {
	klog.V(kl.level).InfoS("", append(copySlice(kl.kvs), kvs...)...)
}

// WithKeyValues implements Logger.WithKeyValues.
func (kl klogger) WithKeyValues(kvs ...any) Logger {
	return klogger{
		level: kl.level,
		kvs:   append(copySlice(kl.kvs), kvs...),
	}
}

// NewLogger returns builtin Logger implementation.
func NewLogger(level int32) Logger {
	return klogger{level: klog.Level(level)}
}

func copySlice(src []any) []any {
	if len(src) == 0 {
		return []any{}
	}

	dst := make([]any, len(src))
	copy(dst, src)
	return dst
}
