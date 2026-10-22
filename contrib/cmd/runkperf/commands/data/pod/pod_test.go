// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package pod

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/Azure/kperf/contrib/cmd/runkperf/commands/data"
)

func TestCheckPodParams(t *testing.T) {
	assert.NoError(t, checkPodParams(1, 1, 1))
	assert.Error(t, checkPodParams(0, 1, 1))
	assert.Error(t, checkPodParams(1, 0, 1))
	assert.Error(t, checkPodParams(1, 1, 0))
}

func TestCreatePods(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	const (
		namespace  = "default"
		podSetName = "set-a"
		total      = 7
		groupSize  = 3
		size       = 128
	)

	err := createPods(clientset, namespace, podSetName, total, groupSize, size)
	require.NoError(t, err)

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, pods.Items, total)

	for _, pod := range pods.Items {
		assert.Equal(t, data.AppLabel, pod.Labels["app"])
		assert.Equal(t, podSetName, pod.Labels["podSetName"])
		assert.Len(t, pod.Annotations[paddingAnnotation], size)
		assert.Equal(t, "runkperf-data-only", pod.Spec.SchedulerName)
		require.Len(t, pod.Spec.Containers, 1)
		assert.Equal(t, pauseImage, pod.Spec.Containers[0].Image)
	}
}

func TestListPods(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	const namespace = "default"

	require.NoError(t, createPods(clientset, namespace, "set-a", 5, 3, 64))
	require.NoError(t, createPods(clientset, namespace, "set-b", 2, 3, 32))

	podMap := make(map[string]*podSetInfo)
	selector := "app=" + data.AppLabel
	err := listPods(clientset, selector, namespace, data.DefaultPageSize, func(pod corev1.Pod) error {
		name := pod.Labels["podSetName"]
		info, ok := podMap[name]
		if !ok {
			info = &podSetInfo{size: len(pod.Annotations[paddingAnnotation])}
			podMap[name] = info
		}
		info.total++
		return nil
	})
	require.NoError(t, err)

	require.Contains(t, podMap, "set-a")
	require.Contains(t, podMap, "set-b")
	assert.Equal(t, 5, podMap["set-a"].total)
	assert.Equal(t, 64, podMap["set-a"].size)
	assert.Equal(t, 2, podMap["set-b"].total)
	assert.Equal(t, 32, podMap["set-b"].size)
}

func TestDeletePods(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	const namespace = "default"

	require.NoError(t, createPods(clientset, namespace, "set-a", 4, 2, 16))
	require.NoError(t, createPods(clientset, namespace, "set-b", 3, 2, 16))

	selectorA := "app=" + data.AppLabel + ",podSetName=set-a"
	require.NoError(t, deletePods(clientset, selectorA, namespace, 2))

	remaining, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, remaining.Items, 3)
	for _, pod := range remaining.Items {
		assert.Equal(t, "set-b", pod.Labels["podSetName"])
	}

	// Deleting a set that does not exist is a no-op and returns no error.
	require.NoError(t, deletePods(clientset, "app="+data.AppLabel+",podSetName=missing", namespace, 2))
}

func TestDeletePodsPages(t *testing.T) {
	client := fake.NewSimpleClientset()
	calls := 0
	client.PrependReactor("list", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		opts := action.(ktesting.ListActionImpl).GetListOptions()
		assert.Equal(t, "default", action.GetNamespace())
		assert.Equal(t, "app=runkperf", opts.LabelSelector)
		assert.Equal(t, data.DefaultPageSize, opts.Limit)
		list := &corev1.PodList{}
		if calls == 0 {
			assert.Empty(t, opts.Continue)
			list.Continue = "next"
			list.Items = []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "first", Labels: map[string]string{"app": "runkperf"}}}}
		} else {
			assert.Equal(t, 1, calls)
			assert.Equal(t, "next", opts.Continue)
			list.Items = []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: "second", Labels: map[string]string{"app": "runkperf"}}}}
		}
		calls++
		return true, list, nil
	})
	var deleted []string
	client.PrependReactor("delete", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		assert.Equal(t, 2, calls, "all pages must be fetched before deletion")
		name := action.(ktesting.DeleteAction).GetName()
		deleted = append(deleted, name)
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, name)
	})
	require.NoError(t, deletePods(client, "app=runkperf", "default", 1))
	assert.Equal(t, []string{"first", "second"}, deleted)
}

func TestPodOperationErrors(t *testing.T) {
	for _, verb := range []string{"list", "delete"} {
		t.Run(verb, func(t *testing.T) {
			client := fake.NewSimpleClientset(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "default"}})
			sentinel := errors.New("denied")
			client.PrependReactor(verb, "pods", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, sentinel
			})
			err := deletePods(client, "", "default", 1)
			require.Error(t, err)
			assert.ErrorIs(t, err, sentinel)
			assert.Contains(t, err.Error(), "failed to "+verb+" pod")
			if verb == "list" {
				for _, action := range client.Actions() {
					assert.NotEqual(t, "delete", action.GetVerb())
				}
			}
		})
	}
}
