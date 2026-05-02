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

package controller

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/storagebroker"
	"oltp.molnett.org/neon-operator/specs/storagecontroller"
	"oltp.molnett.org/neon-operator/utils"
)

// This is not a proper error. It indicated we should return a empty requeue after an object has been changed.
var ErrRequeueAfterChange = errors.New("requeue after change")

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=clusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=clusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=branches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=branches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=neon.oltp.molnett.org,resources=branches/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	log.Info("Reconcile loop start", "request", req)
	defer func() {
		log.Info("Reconcile loop end", "request", req)
	}()

	cluster, err := r.getCluster(ctx, req)
	if err != nil {
		return ctrl.Result{}, err
	}

	ctx = context.WithValue(ctx, utils.ClusterNameKey, cluster.Name)

	result, err := r.reconcile(ctx, cluster)
	if errors.Is(err, ErrRequeueAfterChange) {
		return result, nil
	} else if err != nil {
		log.Error(err, "Reconcile failed")
		return ctrl.Result{}, err
	}

	return result, nil
}

//nolint:unparam
func (r *ClusterReconciler) reconcile(ctx context.Context, cluster *neonv1alpha1.Cluster) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	createErr := r.createClusterResources(ctx, cluster)
	if createErr != nil {
		log.Error(createErr, "error while creating cluster resources")
	}

	if err := r.updateStatus(ctx, cluster, createErr); err != nil {
		log.Error(err, "failed to update cluster status")
		return ctrl.Result{}, err
	}

	if createErr != nil {
		return ctrl.Result{}, fmt.Errorf("not able to create cluster resources: %w", createErr)
	}
	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) updateStatus(ctx context.Context, cluster *neonv1alpha1.Cluster, reconcileErr error) error {
	scAvailable, scReason, scMessage := r.deploymentState(ctx, cluster, storagecontroller.Name(cluster.Name), "Storage controller")
	sbAvailable, sbReason, sbMessage := r.deploymentState(ctx, cluster, storagebroker.Name(cluster.Name), "Storage broker")

	return utils.PatchStatus(ctx, r.Client, cluster, func(c *neonv1alpha1.Cluster) {
		c.Status.ObservedGeneration = c.Generation
		conds := &c.Status.Conditions

		if reconcileErr != nil {
			utils.SetCondition(c, conds, utils.ConditionAvailable, metav1.ConditionFalse, utils.ReasonResourceCreateFailed, reconcileErr.Error())
			utils.SetCondition(c, conds, utils.ConditionProgressing, metav1.ConditionTrue, utils.ReasonReconciling, "Retrying after resource creation failure")
			utils.SetCondition(c, conds, utils.ConditionStorageControllerAvailable, scAvailable, scReason, scMessage)
			utils.SetCondition(c, conds, utils.ConditionStorageBrokerAvailable, sbAvailable, sbReason, sbMessage)
			return
		}

		utils.SetCondition(c, conds, utils.ConditionStorageControllerAvailable, scAvailable, scReason, scMessage)
		utils.SetCondition(c, conds, utils.ConditionStorageBrokerAvailable, sbAvailable, sbReason, sbMessage)

		switch {
		case scAvailable == metav1.ConditionTrue && sbAvailable == metav1.ConditionTrue:
			utils.SetCondition(c, conds, utils.ConditionAvailable, metav1.ConditionTrue, utils.ReasonAsExpected, "Cluster components are Available")
			utils.SetCondition(c, conds, utils.ConditionProgressing, metav1.ConditionFalse, utils.ReasonAsExpected, "Cluster is at desired state")
		default:
			utils.SetCondition(c, conds, utils.ConditionAvailable, metav1.ConditionFalse, utils.ReasonChildDeploymentNotAvailable, "One or more cluster components are not Available")
			utils.SetCondition(c, conds, utils.ConditionProgressing, metav1.ConditionTrue, utils.ReasonReconciling, "Waiting for child Deployments to become Available")
		}
	})
}

func (r *ClusterReconciler) deploymentState(ctx context.Context, cluster *neonv1alpha1.Cluster, name, label string) (metav1.ConditionStatus, string, string) {
	dep := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: cluster.Namespace}, dep)
	switch {
	case apierrors.IsNotFound(err):
		return metav1.ConditionFalse, utils.ReasonChildResourceMissing, fmt.Sprintf("%s Deployment has not been observed yet", label)
	case err != nil:
		return metav1.ConditionUnknown, utils.ReasonChildResourceMissing, err.Error()
	case utils.IsDeploymentAvailable(dep):
		return metav1.ConditionTrue, utils.ReasonAsExpected, fmt.Sprintf("%s Deployment is Available", label)
	default:
		return metav1.ConditionFalse, utils.ReasonChildDeploymentNotAvailable, fmt.Sprintf("%s Deployment is not yet Available", label)
	}
}

func (r *ClusterReconciler) getCluster(ctx context.Context, req ctrl.Request) (*neonv1alpha1.Cluster, error) {
	log := logf.FromContext(ctx)
	cluster := &neonv1alpha1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Cluster has been deleted")
			return nil, nil
		}

		return nil, fmt.Errorf("cannot get the resource: %w", err)
	}
	return cluster, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&neonv1alpha1.Cluster{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("cluster").
		Complete(r)
}
