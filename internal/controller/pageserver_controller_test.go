package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/test/fixtures"
	"oltp.molnett.org/neon-operator/utils"
)

var _ = Describe("Pageserver Controller", func() {
	const (
		clusterName    = "ps-suite"
		pageserverName = "ps-suite-ps0"
		pageserverID   = uint64(0)
	)
	var (
		namespace string
		stsName   = clusterName + "-pageserver-0"
	)

	BeforeEach(func() {
		namespace = newTestNamespace()
		Expect(k8sClient.Create(ctx, fixtures.NewBucketCredsSecret(clusterName, namespace))).To(Succeed())
		Expect(k8sClient.Create(ctx, fixtures.NewCluster(clusterName, namespace))).To(Succeed())
		Expect(k8sClient.Create(ctx, fixtures.NewPageserver(pageserverName, namespace, clusterName, pageserverID))).To(Succeed())
	})

	It("creates StatefulSet, services and ConfigMap owned by the Pageserver CR", func() {
		Eventually(func(g Gomega) {
			sts := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: namespace}, sts)).To(Succeed())
			g.Expect(sts.OwnerReferences).To(HaveLen(1))
			g.Expect(sts.OwnerReferences[0].Kind).To(Equal("Pageserver"))
			g.Expect(sts.Spec.ServiceName).To(Equal(stsName + "-headless"))
			g.Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))

			svc := &corev1.Service{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: namespace}, svc)).To(Succeed())
			g.Expect(svc.Spec.ClusterIP).NotTo(Equal(corev1.ClusterIPNone))

			headless := &corev1.Service{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName + "-headless", Namespace: namespace}, headless)).To(Succeed())
			g.Expect(headless.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))

			cm := &corev1.ConfigMap{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: namespace}, cm)).To(Succeed())
			g.Expect(cm.Data).To(HaveKey("pageserver.toml"))
		}, 10*time.Second, 200*time.Millisecond).Should(Succeed())
	})

	It("flips Available to True once the StatefulSet reports ready replicas", func() {
		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: namespace}, sts)
		}, 10*time.Second, 200*time.Millisecond).Should(Succeed())

		Eventually(func(g Gomega) {
			ps := &neonv1alpha1.Pageserver{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pageserverName, Namespace: namespace}, ps)).To(Succeed())
			cond := meta.FindStatusCondition(ps.Status.Conditions, utils.ConditionAvailable)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		}, 10*time.Second, 200*time.Millisecond).Should(Succeed())

		patch := sts.DeepCopy()
		patch.Status.ObservedGeneration = sts.Generation
		patch.Status.Replicas = 1
		patch.Status.ReadyReplicas = 1
		patch.Status.AvailableReplicas = 1
		Expect(k8sClient.Status().Patch(ctx, patch, client.MergeFrom(sts))).To(Succeed())

		Eventually(func(g Gomega) {
			ps := &neonv1alpha1.Pageserver{}
			g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pageserverName, Namespace: namespace}, ps)).To(Succeed())
			cond := meta.FindStatusCondition(ps.Status.Conditions, utils.ConditionAvailable)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		}, 10*time.Second, 200*time.Millisecond).Should(Succeed())
	})
})
