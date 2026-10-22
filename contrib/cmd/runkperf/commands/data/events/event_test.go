// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package events

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestCreateAndDeleteEvents(t *testing.T) {
	client := fake.NewSimpleClientset()
	opts := createOptions{
		Namespace: "default", EventSetName: "set-a", Total: 5, GroupSize: 2,
		Reason: "Benchmark", EventType: "Warning", MessageSize: 32, InvolvedObjectKind: "Pod",
	}
	require.NoError(t, createEvents(client, opts))
	other := opts
	other.EventSetName = "set-b"
	other.Total = 1
	other.InvolvedObjectName = "explicit"
	require.NoError(t, createEvents(client, other))
	events, err := client.CoreV1().Events(opts.Namespace).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, events.Items, 6)
	for _, event := range events.Items {
		assert.Equal(t, data.AppLabel, event.Labels["app"])
		assert.Equal(t, opts.Namespace, event.Namespace)
		assert.Equal(t, opts.Namespace, event.InvolvedObject.Namespace)
		assert.Equal(t, opts.InvolvedObjectKind, event.InvolvedObject.Kind)
		assert.Equal(t, opts.Reason, event.Reason)
		assert.Equal(t, opts.EventType, event.Type)
		assert.Len(t, event.Message, opts.MessageSize)
		assert.Equal(t, int32(1), event.Count)
		assert.Equal(t, "runkperf-bench", event.Source.Component)
		assert.False(t, event.FirstTimestamp.IsZero())
		assert.Equal(t, event.FirstTimestamp, event.LastTimestamp)
		if event.Labels["eventSetName"] == "set-b" {
			assert.Equal(t, "explicit", event.InvolvedObject.Name)
		} else {
			assert.Contains(t, event.InvolvedObject.Name, "set-a-obj-")
		}
	}
	require.NoError(t, deleteEvents(client, "app=runkperf,eventSetName=set-a", opts.Namespace, 2))
	events, err = client.CoreV1().Events(opts.Namespace).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, events.Items, 1)
	assert.Equal(t, "set-b", events.Items[0].Labels["eventSetName"])
}

func TestDeleteEventsPages(t *testing.T) {
	client := fake.NewSimpleClientset()
	calls := 0
	client.PrependReactor("list", "events", func(action ktesting.Action) (bool, runtime.Object, error) {
		opts := action.(ktesting.ListActionImpl).GetListOptions()
		assert.Equal(t, "default", action.GetNamespace())
		assert.Equal(t, "app=runkperf", opts.LabelSelector)
		assert.Equal(t, data.DefaultPageSize, opts.Limit)
		list := &corev1.EventList{}
		if calls == 0 {
			assert.Empty(t, opts.Continue)
			list.Continue = "next"
			list.Items = []corev1.Event{{ObjectMeta: metav1.ObjectMeta{Name: "first", Labels: map[string]string{"app": "runkperf"}}}}
		} else {
			assert.Equal(t, 1, calls)
			assert.Equal(t, "next", opts.Continue)
			list.Items = []corev1.Event{{ObjectMeta: metav1.ObjectMeta{Name: "second", Labels: map[string]string{"app": "runkperf"}}}}
		}
		calls++
		return true, list, nil
	})
	var deleted []string
	client.PrependReactor("delete", "events", func(action ktesting.Action) (bool, runtime.Object, error) {
		assert.Equal(t, 2, calls, "all pages must be fetched before deletion")
		name := action.(ktesting.DeleteAction).GetName()
		deleted = append(deleted, name)
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "events"}, name)
	})
	require.NoError(t, deleteEvents(client, "app=runkperf", "default", 1))
	assert.Equal(t, []string{"first", "second"}, deleted)
}

func TestEventOperationErrors(t *testing.T) {
	for _, verb := range []string{"list", "delete"} {
		t.Run(verb, func(t *testing.T) {
			client := fake.NewSimpleClientset(&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "event", Namespace: "default"}})
			sentinel := errors.New("denied")
			client.PrependReactor(verb, "events", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, sentinel
			})
			err := deleteEvents(client, "", "default", 1)
			require.Error(t, err)
			assert.ErrorIs(t, err, sentinel)
			assert.Contains(t, err.Error(), "failed to "+verb+" event")
			if verb == "list" {
				for _, action := range client.Actions() {
					assert.NotEqual(t, "delete", action.GetVerb())
				}
			}
		})
	}
}
