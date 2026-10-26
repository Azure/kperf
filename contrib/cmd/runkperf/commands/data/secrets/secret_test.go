// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package secrets

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

func TestCreateAndDeleteSecrets(t *testing.T) {
	client := fake.NewClientset()
	require.NoError(t, createSecrets(client, "default", "set-a", 32, 2, 5))
	require.NoError(t, createSecrets(client, "default", "set-b", 16, 10, 1))
	secrets, err := client.CoreV1().Secrets("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, secrets.Items, 6)
	groups := map[string]int{}
	for _, secret := range secrets.Items {
		assert.Equal(t, data.AppLabel, secret.Labels["app"])
		assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
		if secret.Labels["secName"] == "set-a" {
			assert.Len(t, secret.Data["data"], 32)
			groups[secret.Labels["ownerID"]]++
		} else {
			assert.Equal(t, "set-b", secret.Labels["secName"])
			assert.Len(t, secret.Data["data"], 16)
		}
	}
	assert.Equal(t, map[string]int{"0": 2, "2": 2, "4": 1}, groups)
	require.NoError(t, deleteSecrets(client, "app=runkperf,secName=set-a", "default", 2))
	secrets, err = client.CoreV1().Secrets("default").List(context.TODO(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Len(t, secrets.Items, 1)
	assert.Equal(t, "set-b", secrets.Items[0].Labels["secName"])
}

func TestDeleteSecretsPages(t *testing.T) {
	client := fake.NewClientset()
	calls := 0
	client.PrependReactor("list", "secrets", func(action ktesting.Action) (bool, runtime.Object, error) {
		opts := action.(ktesting.ListActionImpl).GetListOptions()
		assert.Equal(t, "default", action.GetNamespace())
		assert.Equal(t, "app=runkperf", opts.LabelSelector)
		assert.Equal(t, data.DefaultPageSize, opts.Limit)
		list := &corev1.SecretList{}
		if calls == 0 {
			assert.Empty(t, opts.Continue)
			list.Continue = "next"
			list.Items = []corev1.Secret{{ObjectMeta: metav1.ObjectMeta{Name: "first", Labels: map[string]string{"app": "runkperf"}}}}
		} else {
			assert.Equal(t, 1, calls)
			assert.Equal(t, "next", opts.Continue)
			list.Items = []corev1.Secret{{ObjectMeta: metav1.ObjectMeta{Name: "second", Labels: map[string]string{"app": "runkperf"}}}}
		}
		calls++
		return true, list, nil
	})
	var deleted []string
	client.PrependReactor("delete", "secrets", func(action ktesting.Action) (bool, runtime.Object, error) {
		assert.Equal(t, 2, calls, "all pages must be fetched before deletion")
		name := action.(ktesting.DeleteAction).GetName()
		deleted = append(deleted, name)
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secrets"}, name)
	})
	require.NoError(t, deleteSecrets(client, "app=runkperf", "default", 1))
	assert.Equal(t, []string{"first", "second"}, deleted)
}

func TestSecretOperationErrors(t *testing.T) {
	for _, verb := range []string{"list", "delete"} {
		t.Run(verb, func(t *testing.T) {
			client := fake.NewClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret", Namespace: "default"}})
			sentinel := errors.New("denied")
			client.PrependReactor(verb, "secrets", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, sentinel
			})
			err := deleteSecrets(client, "", "default", 1)
			require.Error(t, err)
			assert.ErrorIs(t, err, sentinel)
			assert.Contains(t, err.Error(), "failed to "+verb+" secret")
			if verb == "list" {
				for _, action := range client.Actions() {
					assert.NotEqual(t, "delete", action.GetVerb())
				}
			}
		})
	}
}
