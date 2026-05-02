package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/pageserver"
	"oltp.molnett.org/neon-operator/utils"
)

func (r *PageserverReconciler) createPageserverResources(ctx context.Context, ps *neonv1alpha1.Pageserver) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling pageserver ConfigMap")
	if err := r.reconcileConfigMap(ctx, ps); err != nil {
		return err
	}

	log.Info("Reconciling pageserver Service")
	svc := pageserver.Service(ps)
	if err := utils.ReconcileSSA(ctx, r.Client, r.Scheme, ps, svc, func(cur *corev1.Service) bool {
		return !equality.Semantic.DeepDerivative(svc.Spec, cur.Spec)
	}); err != nil {
		return err
	}

	log.Info("Reconciling pageserver headless Service")
	headless := pageserver.HeadlessService(ps)
	if err := utils.ReconcileSSA(ctx, r.Client, r.Scheme, ps, headless, func(cur *corev1.Service) bool {
		return !equality.Semantic.DeepDerivative(headless.Spec, cur.Spec)
	}); err != nil {
		return err
	}

	log.Info("Reconciling pageserver StatefulSet")
	var cluster neonv1alpha1.Cluster
	if err := r.Get(ctx, types.NamespacedName{Name: ps.Spec.Cluster, Namespace: ps.Namespace}, &cluster); err != nil {
		return fmt.Errorf("failed to get parent cluster: %w", err)
	}
	sts := pageserver.StatefulSet(ps, cluster.Spec.NeonImage)
	return utils.ReconcileSSA(ctx, r.Client, r.Scheme, ps, sts, func(cur *appsv1.StatefulSet) bool {
		return !equality.Semantic.DeepDerivative(sts.Spec, cur.Spec)
	})
}

func (r *PageserverReconciler) reconcileConfigMap(ctx context.Context, ps *neonv1alpha1.Pageserver) error {
	var bucketSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: ps.Spec.BucketCredentialsSecret.Name, Namespace: ps.Namespace}, &bucketSecret); err != nil {
		return fmt.Errorf("failed to get bucket credentials secret: %w", err)
	}

	cm := pageserver.ConfigMap(ps, &bucketSecret)
	return utils.ReconcileSSA(ctx, r.Client, r.Scheme, ps, cm, func(cur *corev1.ConfigMap) bool {
		return !equality.Semantic.DeepDerivative(cm.Data, cur.Data)
	})
}
