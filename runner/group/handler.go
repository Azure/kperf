// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.

package group

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Azure/kperf/api/types"

	"gopkg.in/yaml.v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	apitypes "k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
)

var (
	// errRetryable marks error is retryable
	errRetryable = errors.New("retry")
)

// Handler is to run a set of runners with same load profile.
type Handler struct {
	name      string
	namespace string

	spec     *types.RunnerGroupSpec
	ownerRef *metav1.OwnerReference

	// FIXME(weifu): should we migrate this field into RunnerGroupSpec?
	imageRef string

	clientset kubernetes.Interface
}

// NewHandler returns new instance of Handler.
func NewHandler(
	clientset kubernetes.Interface,
	namespace, name string,
	spec *types.RunnerGroupSpec,
	imageRef string,
) (*Handler, error) {
	ownRef, err := buildOwnerReference(spec.OwnerReference)
	if err != nil {
		return nil, err
	}

	return &Handler{
		name:      name,
		namespace: namespace,
		spec:      spec,
		ownerRef:  ownRef,
		imageRef:  imageRef,
		clientset: clientset,
	}, nil
}

// Name returns RunnerGroup's name
func (h *Handler) Name() string {
	return h.name
}

// Info returns RunnerGroup information with status.
func (h *Handler) Info(ctx context.Context) *types.RunnerGroup {
	rg := &types.RunnerGroup{
		Name: h.name,
		Spec: h.spec,
		Status: &types.RunnerGroupStatus{
			State: types.RunnerGroupStatusStateUnknown,
		},
	}

	cli := h.clientset.BatchV1().Jobs(h.namespace)
	job, err := cli.Get(ctx, h.name, metav1.GetOptions{})
	if err != nil {
		klog.V(2).ErrorS(err, "failed to job for runner group", "job", h.name)
		return rg
	}

	state := types.RunnerGroupStatusStateRunning
	if jobFinished(job) {
		state = types.RunnerGroupStatusStateFinished
	} else if job.Status.StartTime == nil {
		state = types.RunnerGroupStatusStateUnknown
	}

	rg.Status.State = state
	rg.Status.StartTime = job.Status.StartTime
	rg.Status.Succeeded = job.Status.Succeeded
	rg.Status.Failed = job.Status.Failed
	return rg
}

// Deploy deploys a group of runners.
func (h *Handler) Deploy(ctx context.Context, uploadURL string) error {
	if err := h.uploadLoadProfileAsConfigMap(ctx); err != nil {
		return fmt.Errorf("failed to ensure if load profile has been uploaded: %w", err)
	}
	return h.deployRunners(ctx, uploadURL)
}

// configMapDataKeyLoadProfile is load profile's name in configmap.
var configMapDataKeyLoadProfile = "load_profile.yaml"

// uploadLoadProfileAsConfigMap stores load profile as configmap for runner.
func (h *Handler) uploadLoadProfileAsConfigMap(ctx context.Context) error {
	cli := h.clientset.CoreV1().ConfigMaps(h.namespace)

	cm, err := cli.Get(ctx, h.name, metav1.GetOptions{})
	if err == nil {
		// FIXME: should we check the content?
		if _, ok := cm.Data[configMapDataKeyLoadProfile]; !ok {
			return fmt.Errorf("configmap %s doesn't have load profile", h.name)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	raw, err := yaml.Marshal(h.spec.Profile)
	if err != nil {
		return fmt.Errorf("failed to marshal load profile into yaml: %w", err)
	}

	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.name,
			Namespace: h.namespace,
		},
		Immutable: toPtr(true),
		Data: map[string]string{
			configMapDataKeyLoadProfile: string(raw),
		},
	}
	if h.ownerRef != nil {
		cm.OwnerReferences = append(cm.OwnerReferences, *h.ownerRef)
	}
	_, err = cli.Create(ctx, cm, metav1.CreateOptions{})
	return err
}

// deployRunners deploys a group of runners as batch job.
func (h *Handler) deployRunners(ctx context.Context, uploadURL string) error {
	cli := h.clientset.BatchV1().Jobs(h.namespace)

	_, err := cli.Get(ctx, h.name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			_, err = cli.Create(ctx, h.buildBatchJobObject(uploadURL), metav1.CreateOptions{})
		}
		return err
	}
	// FIXME: should we check the content?
	return nil
}

// Pods returns all the pods controlled by the job.
func (h *Handler) Pods(ctx context.Context) ([]*corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchLabels: map[string]string{
				"batch.kubernetes.io/job-name": h.name,
				"job-name":                     h.name,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create label selector: %w", err)
	}

	opts := metav1.ListOptions{
		LabelSelector: selector.String(),
		// NOTE:
		//
		// List pods from cache to prevent apiserver from list all
		// items from ETCD cluster.
		ResourceVersion: "0",
	}

	pods, err := h.clientset.CoreV1().Pods(h.namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods created by jobs %s: %w",
			h.name, err)
	}

	res := make([]*corev1.Pod, 0, len(pods.Items))
	for idx := range pods.Items {
		pod := pods.Items[idx]
		res = append(res, &pod)
	}
	return res, nil
}

// IsControlled returns true if the pod is controlled by the group.
func (h *Handler) IsControlled(ctx context.Context, podName string) (bool, error) {
	// Fast path: job's name will be the prefix of pod's name.
	if !strings.HasPrefix(podName, h.name) {
		return false, nil
	}

	pods, err := h.Pods(ctx)
	if err != nil {
		return false, err
	}

	for _, pod := range pods {
		if pod.Name == podName {
			return true, nil
		}
	}
	return false, nil
}

// Wait waits runners until they finish.
func (h *Handler) Wait(ctx context.Context) error {
	cli := h.clientset.BatchV1().Jobs(h.namespace)

	job, err := cli.Get(ctx, h.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get job %s from namespace %s: %w",
			h.name, h.namespace, err)
	}

	if jobFinished(job) {
		return nil
	}

	// NOTE: It's to align with client-go package. Please check out the
	// following reference for detail.
	//
	// https://github.com/kubernetes/client-go/blob/v0.28.4/tools/cache/reflector.go#L219
	//
	// TODO(weifu): fix staticcheck check
	//
	//nolint:staticcheck
	backoff := wait.NewExponentialBackoffManager(
		800*time.Millisecond, 30*time.Second, 2*time.Minute,
		2.0, 1.0, &clock.RealClock{})

	lastRv := job.ResourceVersion
	fieldSelector := fields.OneTermEqualSelector(metav1.ObjectNameField, h.name).String()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		opts := metav1.ListOptions{
			FieldSelector:       fieldSelector,
			ResourceVersion:     lastRv,
			AllowWatchBookmarks: true,
		}

		w, err := cli.Watch(ctx, opts)
		if err != nil {
			// should retry if apiserver is down or unavailable.
			if utilnet.IsConnectionRefused(err) ||
				apierrors.IsTooManyRequests(err) ||
				apierrors.IsInternalError(err) {

				<-backoff.Backoff().C()

				continue
			}

			return fmt.Errorf("failed to initialize watch for job %s: %w", h.name, err)
		}

		err = h.waitForJob(ctx, w, &lastRv)
		if err != nil {
			switch {
			case apierrors.IsResourceExpired(err) || apierrors.IsGone(err):
				klog.V(2).Infof("reset last seen revision and continue, since receive: %v", err)
				lastRv = ""
				continue
			// should retry if apiserver is down or unavailable.
			case apierrors.IsTooManyRequests(err) || apierrors.IsInternalError(err):
				<-backoff.Backoff().C()
				continue
			case errors.Is(err, errRetryable):
				<-backoff.Backoff().C()
				continue
			default:
				return err
			}
		}
		return nil
	}
}

// waitForJob will return if job finish.
func (h *Handler) waitForJob(ctx context.Context, w watch.Interface, rv *string) error {
	defer w.Stop()

	expectedType := reflect.TypeOf(&batchv1.Job{})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.ResultChan():
			if !ok {
				return fmt.Errorf("unexpected closed watch channel: %w", errRetryable)
			}

			if event.Type == watch.Error {
				return apierrors.FromObject(event.Object)
			}

			obj := event.Object

			if typ := reflect.TypeOf(obj); typ != expectedType {
				klog.V(2).Infof("unexpected type: %v", typ)
				continue
			}

			job := obj.(*batchv1.Job)

			switch event.Type {
			case watch.Modified:
				klog.V(5).Infof("Job %s Expected %v, Failed %v, Successed: %v",
					job.Name, *job.Spec.Completions, job.Status.Failed, job.Status.Succeeded)

				if jobFinished(job) {
					return nil
				}
			default:
				klog.V(2).Infof("receive event type %s", event.Type)
			}
			*rv = job.ResourceVersion
		}
	}
}

// buildBatchJobObject builds job object to run runners.
func (h *Handler) buildBatchJobObject(uploadURL string) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.name,
			Namespace: h.namespace,
		},
		Spec: batchv1.JobSpec{
			Parallelism:  toPtr(h.spec.Count),
			Completions:  toPtr(h.spec.Count),
			BackoffLimit: toPtr(int32(0)),
			// FIXME: Should not re-create pod
			CompletionMode: toPtr(batchv1.IndexedCompletion),
			Template:       corev1.PodTemplateSpec{},
		},
	}

	if h.ownerRef != nil {
		job.OwnerReferences = append(job.OwnerReferences, *h.ownerRef)
	}

	job.Spec.Template.Spec = corev1.PodSpec{
		Affinity: &corev1.Affinity{},
		Containers: []corev1.Container{
			{
				Name:  "runner",
				Image: h.imageRef,
				Env: []corev1.EnvVar{
					{
						Name: "POD_NAME",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: metav1.ObjectNameField,
							},
						},
					},
					{
						Name: "POD_NAMESPACE",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "metadata.namespace",
							},
						},
					},
					{
						Name: "POD_UID",
						ValueFrom: &corev1.EnvVarSource{
							FieldRef: &corev1.ObjectFieldSelector{
								FieldPath: "metadata.uid",
							},
						},
					},
					{
						Name:  "TARGET_URL",
						Value: uploadURL,
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "config",
						MountPath: "/config",
					},
					{
						Name:      "host-root-tmp",
						MountPath: "/data",
					},
				},
				Command: []string{
					"/run_runner.sh",
				},
			},
		},
		RestartPolicy: corev1.RestartPolicyNever,
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: h.name,
						},
					},
				},
			},
			{
				Name: "host-root-tmp",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp",
					},
				},
			},
		},
	}

	if len(h.spec.NodeAffinity) > 0 {
		matchExpressions := make([]corev1.NodeSelectorRequirement, 0, len(h.spec.NodeAffinity))
		for key, values := range h.spec.NodeAffinity {
			matchExpressions = append(matchExpressions, corev1.NodeSelectorRequirement{
				Key:      key,
				Operator: corev1.NodeSelectorOpIn,
				Values:   values,
			})
		}

		job.Spec.Template.Spec.Affinity.NodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: matchExpressions,
					},
				},
			},
		}
	}

	if sa := h.spec.ServiceAccount; sa != nil {
		job.Spec.Template.Spec.ServiceAccountName = *sa
	}

	return job
}

func buildOwnerReference(ref *string) (*metav1.OwnerReference, error) {
	if ref == nil {
		return nil, nil
	}

	tokens := strings.SplitN(*ref, ":", 4)
	if len(tokens) != 4 {
		return nil, fmt.Errorf("%s own reference is not apiVersion:kind:name:uid format", *ref)
	}

	return &metav1.OwnerReference{
		APIVersion: tokens[0],
		Kind:       tokens[1],
		Name:       tokens[2],
		UID:        apitypes.UID(tokens[3]),
		Controller: toPtr(true),
	}, nil
}

func jobFinished(job *batchv1.Job) bool {
	return job.Status.Failed+job.Status.Succeeded == *job.Spec.Completions
}

func toPtr[T any](v T) *T {
	return &v
}
