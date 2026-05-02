package controller

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrl "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/storagebroker"
	"oltp.molnett.org/neon-operator/specs/storagecontroller"
	"oltp.molnett.org/neon-operator/utils"
)

func (r *ClusterReconciler) createClusterResources(ctx context.Context, cluster *neonv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)

	log.Info("Reconciling JWT keys")
	if err := r.reconcileJWTKeys(ctx, cluster); err != nil {
		return err
	}

	log.Info("Reconciling storage controller")
	if err := r.reconcileStorageController(ctx, cluster); err != nil {
		return err
	}

	log.Info("Reconciling storage broker")
	return r.reconcileStorageBroker(ctx, cluster)
}

func (r *ClusterReconciler) reconcileJWTKeys(ctx context.Context, cluster *neonv1alpha1.Cluster) error {
	log := logf.FromContext(ctx)

	var secret corev1.Secret
	err := r.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("cluster-%s-jwt", cluster.Name), Namespace: cluster.Namespace}, &secret)
	if err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	log.Info("Creating new JWT keys secret for cluster", "cluster", cluster.Name)

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}

	privKeyVBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyVBytes})

	pubKeyVBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	pubKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubKeyVBytes})

	jwtSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cluster-%s-jwt", cluster.Name),
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"private.pem": privKeyPEM,
			"public.pem":  pubKeyPEM,
		},
	}

	if err := ctrl.SetControllerReference(cluster, jwtSecret, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	return r.Create(ctx, jwtSecret)
}

func (r *ClusterReconciler) reconcileStorageController(ctx context.Context, cluster *neonv1alpha1.Cluster) error {
	dep := storagecontroller.Deployment(cluster)
	if err := utils.ReconcileSSA(ctx, r.Client, r.Scheme, cluster, dep, func(cur *appsv1.Deployment) bool {
		return !equality.Semantic.DeepDerivative(dep.Spec, cur.Spec)
	}); err != nil {
		return err
	}

	svc := storagecontroller.Service(cluster)
	return utils.ReconcileSSA(ctx, r.Client, r.Scheme, cluster, svc, func(cur *corev1.Service) bool {
		return !equality.Semantic.DeepDerivative(svc.Spec, cur.Spec)
	})
}

func (r *ClusterReconciler) reconcileStorageBroker(ctx context.Context, cluster *neonv1alpha1.Cluster) error {
	dep := storagebroker.Deployment(cluster)
	if err := utils.ReconcileSSA(ctx, r.Client, r.Scheme, cluster, dep, func(cur *appsv1.Deployment) bool {
		return !equality.Semantic.DeepDerivative(dep.Spec, cur.Spec)
	}); err != nil {
		return err
	}

	svc := storagebroker.Service(cluster)
	return utils.ReconcileSSA(ctx, r.Client, r.Scheme, cluster, svc, func(cur *corev1.Service) bool {
		return !equality.Semantic.DeepDerivative(svc.Spec, cur.Spec)
	})
}
