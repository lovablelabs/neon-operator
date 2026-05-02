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
	"oltp.molnett.org/neon-operator/specs/safekeeper"
	"oltp.molnett.org/neon-operator/utils"
)

func (r *SafekeeperReconciler) createSafekeeperResources(ctx context.Context, sk *neonv1alpha1.Safekeeper) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling safekeeper Service")
	svc := safekeeper.Service(sk)
	if err := utils.ReconcileSSA(ctx, r.Client, r.Scheme, sk, svc, func(cur *corev1.Service) bool {
		return !equality.Semantic.DeepDerivative(svc.Spec, cur.Spec)
	}); err != nil {
		return err
	}

	log.Info("Reconciling safekeeper headless Service")
	headless := safekeeper.HeadlessService(sk)
	if err := utils.ReconcileSSA(ctx, r.Client, r.Scheme, sk, headless, func(cur *corev1.Service) bool {
		return !equality.Semantic.DeepDerivative(headless.Spec, cur.Spec)
	}); err != nil {
		return err
	}

	log.Info("Reconciling safekeeper StatefulSet")
	var cluster neonv1alpha1.Cluster
	if err := r.Get(ctx, types.NamespacedName{Name: sk.Spec.Cluster, Namespace: sk.Namespace}, &cluster); err != nil {
		return fmt.Errorf("failed to get parent cluster: %w", err)
	}
	sts := safekeeper.StatefulSet(sk, cluster.Spec.NeonImage)
	return utils.ReconcileSSA(ctx, r.Client, r.Scheme, sk, sts, func(cur *appsv1.StatefulSet) bool {
		return !equality.Semantic.DeepDerivative(sts.Spec, cur.Spec)
	})
}
