package controller

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/specs/compute"
	"oltp.molnett.org/neon-operator/utils"
)

var _ = Describe("Branch Credentials Reconcile", func() {
	It("creates and heals branch credentials secret with annotation", func() {
		ctx := context.Background()
		clusterName := "branch-test-cluster"
		projectName := "branch-test-project"
		branchName := "branch-test-branch"
		ns := "default"

		_, privateKey, err := ed25519.GenerateKey(nil)
		Expect(err).NotTo(HaveOccurred())
		privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
		Expect(err).NotTo(HaveOccurred())
		privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
		publicDER, err := x509.MarshalPKIXPublicKey(privateKey.Public())
		Expect(err).NotTo(HaveOccurred())
		publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

		cluster := &neonv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: ns},
			Spec: neonv1alpha1.ClusterSpec{
				NumSafekeepers:          3,
				DefaultPGVersion:        17,
				BucketCredentialsSecret: &corev1.SecretReference{Name: "bucket", Namespace: ns},
				StorageControllerDatabaseSecret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "db"},
					Key:                  "uri",
				},
			},
		}
		project := &neonv1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: projectName, Namespace: ns},
			Spec: neonv1alpha1.ProjectSpec{
				ClusterName: clusterName,
				TenantID:    utils.GenerateNeonID(),
				PGVersion:   17,
			},
		}
		branch := &neonv1alpha1.Branch{
			ObjectMeta: metav1.ObjectMeta{Name: branchName, Namespace: ns},
			Spec: neonv1alpha1.BranchSpec{
				ProjectID:  projectName,
				TimelineID: utils.GenerateNeonID(),
				PGVersion:  17,
			},
		}
		jwtSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("cluster-%s-jwt", clusterName), Namespace: ns},
			Data: map[string][]byte{
				"private.pem": privatePEM,
				"public.pem":  publicPEM,
			},
		}

		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		Expect(k8sClient.Create(ctx, project)).To(Succeed())
		Expect(k8sClient.Create(ctx, branch)).To(Succeed())
		Expect(k8sClient.Create(ctx, jwtSecret)).To(Succeed())

		reconciler := &BranchReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		Expect(reconciler.reconcileCredentialsSecret(ctx, branch, project)).To(Succeed())
		Expect(reconciler.ensureBranchCredentialAnnotation(ctx, branch)).To(Succeed())

		secretName := compute.CredentialsSecretName(branchName)
		credentialsSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: ns}, credentialsSecret)).To(Succeed())
		Expect(compute.CredentialsDataIsValid(credentialsSecret.Data)).To(BeTrue())

		storedPassword := string(credentialsSecret.Data[compute.CredentialsPasswordKey])

		// Drift one field and ensure reconcile heals it.
		credentialsSecret.Data[compute.CredentialsPasswordMD5] = []byte("broken")
		Expect(k8sClient.Update(ctx, credentialsSecret)).To(Succeed())

		Expect(reconciler.reconcileCredentialsSecret(ctx, branch, project)).To(Succeed())

		healedSecret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: ns}, healedSecret)).To(Succeed())
		Expect(compute.CredentialsDataIsValid(healedSecret.Data)).To(BeTrue())
		Expect(string(healedSecret.Data[compute.CredentialsPasswordKey])).To(Equal(storedPassword))

		reloadedBranch := &neonv1alpha1.Branch{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: branchName, Namespace: ns}, reloadedBranch)).To(Succeed())
		Expect(reloadedBranch.Annotations).To(HaveKeyWithValue(compute.BranchCredentialsSecretAnnotation, secretName))

		deployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-compute-node", branchName), Namespace: ns}, deployment)).NotTo(Succeed())
	})
})
