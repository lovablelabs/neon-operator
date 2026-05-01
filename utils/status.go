/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ConditionAvailable                  = "Available"
	ConditionProgressing                = "Progressing"
	ConditionStorageControllerAvailable = "StorageControllerAvailable"
	ConditionStorageBrokerAvailable     = "StorageBrokerAvailable"
	ConditionTenantIDAssigned           = "TenantIDAssigned"
	ConditionAttached                   = "Attached"
	ConditionTimelineIDAssigned         = "TimelineIDAssigned"
	ConditionTimelineCreated            = "TimelineCreated"
	ConditionComputeReady               = "ComputeReady"
	ConditionResourcesReady             = "ResourcesReady"
)

const (
	ReasonAsExpected                   = "AsExpected"
	ReasonReconciling                  = "Reconciling"
	ReasonChildPodNotReady             = "ChildPodNotReady"
	ReasonChildDeploymentNotAvailable  = "ChildDeploymentNotAvailable"
	ReasonChildResourceMissing         = "ChildResourceMissing"
	ReasonResourceCreateFailed         = "ResourceCreateFailed"
	ReasonStorageControllerUnreachable = "StorageControllerUnreachable"
	ReasonAttachFailed                 = "AttachFailed"
	ReasonTenantIDPending              = "TenantIDPending"
	ReasonTimelineIDPending            = "TimelineIDPending"
	ReasonTimelineCreationFailed       = "TimelineCreationFailed"
)

// StatusObject is implemented by every CRD with a Status subresource so PatchStatus
// can compare and copy status without a central type switch.
type StatusObject interface {
	client.Object
	StatusValue() any
	AssignStatusFrom(client.Object)
}

// PodBackedStatus is implemented by CRDs whose readiness derives from a single
// owned Pod (Pageserver, Safekeeper).
type PodBackedStatus interface {
	StatusObject
	StatusConditions() *[]metav1.Condition
	SetObservedGeneration(int64)
}

// SetCondition wraps meta.SetStatusCondition; it preserves LastTransitionTime
// when only ObservedGeneration / Reason / Message change.
func SetCondition(
	obj client.Object,
	conds *[]metav1.Condition,
	t string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(conds, metav1.Condition{
		Type:               t,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: obj.GetGeneration(),
	})
}

// PatchStatus runs mutate on a fresh copy of obj and patches the status
// subresource if the resulting status differs.
func PatchStatus[T StatusObject](ctx context.Context, c client.Client, obj T, mutate func(T)) error {
	log := logf.FromContext(ctx)

	current := obj.DeepCopyObject().(T)
	if err := c.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, current); err != nil {
		return err
	}

	updated := current.DeepCopyObject().(T)
	mutate(updated)

	if equality.Semantic.DeepEqual(current.StatusValue(), updated.StatusValue()) {
		return nil
	}

	patchOptions := client.MergeFromWithOptions(current, client.MergeFromWithOptimisticLock{})
	if err := c.Status().Patch(ctx, updated, patchOptions); err != nil {
		log.Error(err, "failed to patch status")
		return err
	}

	obj.AssignStatusFrom(updated)
	return nil
}

// UpdatePodBackedStatus applies the standard pod-backed condition pattern:
// ResourcesReady reflects reconcileErr; Available + Progressing roll up the
// child Pod's Ready condition. Used by Pageserver and Safekeeper.
func UpdatePodBackedStatus[T PodBackedStatus](
	ctx context.Context,
	c client.Client,
	obj T,
	podName, label string,
	reconcileErr error,
) error {
	pod := &corev1.Pod{}
	podErr := c.Get(ctx, types.NamespacedName{Name: podName, Namespace: obj.GetNamespace()}, pod)

	return PatchStatus(ctx, c, obj, func(o T) {
		o.SetObservedGeneration(o.GetGeneration())
		conds := o.StatusConditions()

		if reconcileErr != nil {
			msg := reconcileErr.Error()
			SetCondition(o, conds, ConditionResourcesReady, metav1.ConditionFalse, ReasonResourceCreateFailed, msg)
			SetCondition(o, conds, ConditionAvailable, metav1.ConditionFalse, ReasonResourceCreateFailed, msg)
			SetCondition(o, conds, ConditionProgressing, metav1.ConditionTrue, ReasonReconciling,
				"Retrying after resource creation failure")
			return
		}

		SetCondition(o, conds, ConditionResourcesReady, metav1.ConditionTrue, ReasonAsExpected,
			"Child resources reconciled")

		switch {
		case apierrors.IsNotFound(podErr):
			SetCondition(o, conds, ConditionAvailable, metav1.ConditionFalse, ReasonChildResourceMissing,
				fmt.Sprintf("%s Pod has not been observed yet", label))
			SetCondition(o, conds, ConditionProgressing, metav1.ConditionTrue, ReasonReconciling,
				"Waiting for child Pod to appear")
		case podErr != nil:
			SetCondition(o, conds, ConditionAvailable, metav1.ConditionUnknown, ReasonChildResourceMissing, podErr.Error())
			SetCondition(o, conds, ConditionProgressing, metav1.ConditionTrue, ReasonReconciling,
				"Could not read child Pod")
		case IsPodReady(pod):
			SetCondition(o, conds, ConditionAvailable, metav1.ConditionTrue, ReasonAsExpected,
				fmt.Sprintf("%s Pod is Ready", label))
			SetCondition(o, conds, ConditionProgressing, metav1.ConditionFalse, ReasonAsExpected,
				fmt.Sprintf("%s is at desired state", label))
		default:
			reason, message := PodNotReadyDetail(pod)
			if reason == "" {
				reason = ReasonChildPodNotReady
				message = fmt.Sprintf("%s Pod is not Ready", label)
			}
			SetCondition(o, conds, ConditionAvailable, metav1.ConditionFalse, reason, message)
			SetCondition(o, conds, ConditionProgressing, metav1.ConditionTrue, ReasonReconciling,
				fmt.Sprintf("Waiting for %s Pod readiness", label))
		}
	})
}

func IsPodReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// PodNotReadyDetail returns a kubelet-supplied (reason, message) describing why
// a Pod isn't Ready: the first informative Waiting or non-zero Terminated state
// across init and main containers. Returns ("", "") when nothing actionable is
// surfaced — callers should fall back to a generic message.
func PodNotReadyDetail(pod *corev1.Pod) (reason, message string) {
	if pod == nil {
		return "", ""
	}
	for _, statuses := range [][]corev1.ContainerStatus{pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses} {
		for _, cs := range statuses {
			if w := cs.State.Waiting; w != nil && w.Reason != "" {
				msg := w.Message
				if msg == "" {
					msg = fmt.Sprintf("container %q is %s", cs.Name, w.Reason)
				}
				return w.Reason, msg
			}
			if t := cs.State.Terminated; t != nil && t.ExitCode != 0 {
				reason := t.Reason
				if reason == "" {
					reason = "ContainerTerminated"
				}
				msg := t.Message
				if msg == "" {
					msg = fmt.Sprintf("container %q exited with code %d", cs.Name, t.ExitCode)
				}
				return reason, msg
			}
		}
	}
	return "", ""
}

func IsDeploymentAvailable(dep *appsv1.Deployment) bool {
	if dep == nil {
		return false
	}
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
