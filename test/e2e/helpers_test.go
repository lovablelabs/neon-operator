/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	testutils "oltp.molnett.org/neon-operator/test/utils"
	"oltp.molnett.org/neon-operator/utils"
)

const (
	lifecycleClusterName    = "cluster-e2e"
	lifecycleProjectName    = "project-e2e"
	lifecycleBranchName     = "branch-e2e"
	lifecyclePageserverName = "pageserver-e2e"
	lifecyclePageserverID   = uint64(1)

	lifecycleMinIOName          = "minio-e2e"
	lifecycleMinIOClientPodName = "minio-mc-e2e"
	lifecycleMinIOBucket        = "neon-e2e-bucket"
	lifecycleMinIOAccessKey     = "e2e-access-key"
	lifecycleMinIOSecretKey     = "e2e-secret-key"
	lifecycleStorageDBName      = "storage-db-e2e"

	lifecycleExistsTimeout       = 90 * time.Second
	lifecycleAvailableTimeout    = 4 * time.Minute
	lifecycleComputeReadyTimeout = 5 * time.Minute
	lifecycleSQLTimeout          = 3 * time.Minute
	lifecyclePollInterval        = 2 * time.Second
	lifecycleSQLRetryInterval    = 5 * time.Second
)

var lifecycleSafekeeperIDs = []uint32{1, 2, 3}

func newE2EClient() client.Client {
	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "failed to build kubeconfig for e2e client")

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	Expect(neonv1alpha1.AddToScheme(scheme)).To(Succeed())

	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), "failed to create e2e client")
	return cli
}

func bucketCredsSecret(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleClusterName + "-bucket-creds",
			Namespace: ns,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"AWS_ACCESS_KEY_ID":     lifecycleMinIOAccessKey,
			"AWS_SECRET_ACCESS_KEY": lifecycleMinIOSecretKey,
			"AWS_REGION":            "us-east-1",
			"BUCKET_NAME":           lifecycleMinIOBucket,
			"AWS_ENDPOINT_URL":      fmt.Sprintf("http://%s:9000", lifecycleMinIOName),
		},
	}
}

func storconDBSecret(ns string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleClusterName + "-storcon-db",
			Namespace: ns,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"uri": fmt.Sprintf("postgresql://postgres:postgres@%s:5432/postgres?sslmode=disable", lifecycleStorageDBName),
		},
	}
}

func storageDBDeployment(ns string) *appsv1.Deployment {
	l := map[string]string{"app.kubernetes.io/name": lifecycleStorageDBName}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: lifecycleStorageDBName, Namespace: ns, Labels: l},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: l},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: l},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "postgres",
						Image: "postgres:16-alpine",
						Env: []corev1.EnvVar{
							{Name: "POSTGRES_USER", Value: "postgres"},
							{Name: "POSTGRES_PASSWORD", Value: "postgres"},
							{Name: "POSTGRES_DB", Value: "postgres"},
						},
						Ports: []corev1.ContainerPort{{ContainerPort: 5432}},
					}},
				},
			},
		},
	}
}

func storageDBService(ns string) *corev1.Service {
	l := map[string]string{"app.kubernetes.io/name": lifecycleStorageDBName}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: lifecycleStorageDBName, Namespace: ns, Labels: l},
		Spec: corev1.ServiceSpec{
			Selector: l,
			Ports:    []corev1.ServicePort{{Name: "postgres", Port: 5432}},
		},
	}
}

func minioDeployment(ns string) *appsv1.Deployment {
	l := map[string]string{"app.kubernetes.io/name": lifecycleMinIOName}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: lifecycleMinIOName, Namespace: ns, Labels: l},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{MatchLabels: l},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: l},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "minio",
						Image: "minio/minio:latest",
						Args:  []string{"server", "/data"},
						Env: []corev1.EnvVar{
							{Name: "MINIO_ROOT_USER", Value: lifecycleMinIOAccessKey},
							{Name: "MINIO_ROOT_PASSWORD", Value: lifecycleMinIOSecretKey},
						},
						Ports: []corev1.ContainerPort{{ContainerPort: 9000}},
					}},
				},
			},
		},
	}
}

func minioService(ns string) *corev1.Service {
	l := map[string]string{"app.kubernetes.io/name": lifecycleMinIOName}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: lifecycleMinIOName, Namespace: ns, Labels: l},
		Spec: corev1.ServiceSpec{
			Selector: l,
			Ports:    []corev1.ServicePort{{Name: "s3", Port: 9000}},
		},
	}
}

func createInfra(ctx context.Context, cli client.Client, ns string) {
	for _, obj := range []client.Object{
		bucketCredsSecret(ns),
		storconDBSecret(ns),
		storageDBDeployment(ns),
		storageDBService(ns),
		minioDeployment(ns),
		minioService(ns),
	} {
		Expect(cli.Create(ctx, obj)).To(Succeed(), "failed to create %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
	}
}

func waitForDeploymentAvailable(ctx context.Context, cli client.Client, ns, name string, timeout time.Duration) {
	Eventually(func(g Gomega) {
		dep := &appsv1.Deployment{}
		err := cli.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, dep)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get deployment %s/%s", ns, name)
		g.Expect(dep.Status.AvailableReplicas).To(BeNumerically(">=", 1),
			"deployment %s/%s not available: ready=%d available=%d updated=%d",
			ns, name, dep.Status.ReadyReplicas, dep.Status.AvailableReplicas, dep.Status.UpdatedReplicas)
	}, timeout, lifecyclePollInterval).Should(Succeed())
}

func ensureMinIOBucket(ns string) {
	_, _ = testutils.Run(exec.Command("kubectl", "delete", "pod", lifecycleMinIOClientPodName,
		"-n", ns, "--ignore-not-found"))

	cmd := exec.Command(
		"kubectl", "run", lifecycleMinIOClientPodName,
		"--restart=Never",
		"--namespace", ns,
		"--image=minio/mc:latest",
		"--command",
		"--",
		"/bin/sh", "-c",
		fmt.Sprintf(
			"mc alias set local http://%s:9000 %s %s && mc mb -p local/%s",
			lifecycleMinIOName, lifecycleMinIOAccessKey, lifecycleMinIOSecretKey, lifecycleMinIOBucket,
		),
	)
	_, err := testutils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "failed to start minio bucket bootstrap pod")

	Eventually(func(g Gomega) {
		out, err := testutils.Run(exec.Command("kubectl", "get", "pod", lifecycleMinIOClientPodName,
			"-n", ns, "-o", "jsonpath={.status.phase}"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(out).To(Equal("Succeeded"), "minio bucket bootstrap pod did not succeed")
	}, lifecycleAvailableTimeout, lifecyclePollInterval).Should(Succeed())
}

// crStatusConditions is implemented by every neon CRD via api/v1alpha1/status_funcs.go.
type crStatusConditions interface {
	client.Object
	StatusConditions() *[]metav1.Condition
}

func waitForCRAvailable(ctx context.Context, cli client.Client, obj crStatusConditions) {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}

	Eventually(func(g Gomega) {
		g.Expect(cli.Get(ctx, key, obj)).To(Succeed(),
			"%T %s/%s was not created", obj, key.Namespace, key.Name)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		g.Expect(cli.Get(ctx, key, obj)).To(Succeed(), "failed to get %T %s/%s", obj, key.Namespace, key.Name)
		conds := *obj.StatusConditions()
		g.Expect(meta.IsStatusConditionTrue(conds, utils.ConditionAvailable)).To(
			BeTrue(),
			"%T %s/%s not Available yet, conditions=%s",
			obj, key.Namespace, key.Name, formatConditions(conds),
		)
	}, lifecycleAvailableTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForComputePodReady(ctx context.Context, cli client.Client, ns, branchName string) string {
	var podName string
	Eventually(func(g Gomega) {
		pods := &corev1.PodList{}
		sel := labels.SelectorFromSet(map[string]string{
			"molnett.org/component": "compute",
			"molnett.org/branch":    branchName,
		})
		g.Expect(cli.List(ctx, pods, &client.ListOptions{Namespace: ns, LabelSelector: sel})).To(Succeed())
		g.Expect(pods.Items).NotTo(BeEmpty(), "no compute pods for branch %q", branchName)

		pod := pods.Items[0]
		g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning),
			"compute pod %s phase=%q", pod.Name, pod.Status.Phase)
		g.Expect(isPodReady(pod.Status.Conditions)).To(
			BeTrue(),
			"compute pod %s not Ready, conditions=%s", pod.Name, formatPodConditions(pod.Status.Conditions),
		)
		podName = pod.Name
	}, lifecycleComputeReadyTimeout, lifecyclePollInterval).Should(Succeed())
	return podName
}

func execSelectOne(ns, podName string) {
	Eventually(func(g Gomega) {
		out, err := testutils.Run(exec.Command(
			"kubectl", "exec", "-n", ns, podName, "--",
			"psql", "-p", "55433", "-U", "cloud_admin", "-d", "postgres", "-tAc", "SELECT 1",
		))
		g.Expect(err).NotTo(HaveOccurred(), "psql exec failed: %s", out)
		g.Expect(strings.TrimSpace(out)).To(Equal("1"), "unexpected SELECT 1 output: %q", out)
	}, lifecycleSQLTimeout, lifecycleSQLRetryInterval).Should(Succeed())
}

func dumpLifecycleDiagnostics(ns string) {
	dump := func(label string, args ...string) {
		out, err := testutils.Run(exec.Command("kubectl", append([]string{"-n", ns}, args...)...))
		if err == nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "=== %s ===\n%s\n", label, out)
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "=== %s (failed: %v) ===\n", label, err)
		}
	}

	storconDeploy := fmt.Sprintf("deploy/%s-storage-controller", lifecycleClusterName)
	storbrokerDeploy := fmt.Sprintf("deploy/%s-storage-broker", lifecycleClusterName)
	computeSelector := fmt.Sprintf("molnett.org/component=compute,molnett.org/branch=%s", lifecycleBranchName)

	dump("lifecycle CRs", "get", "cluster,project,branch,pageserver,safekeeper", "-o", "wide")
	dump("pods", "get", "pods", "-o", "wide")
	dump("events (last)", "get", "events", "--sort-by=.lastTimestamp")
	dump("storage-controller logs", "logs", storconDeploy, "--tail=200")
	dump("storage-broker logs", "logs", storbrokerDeploy, "--tail=100")
	dump("pageserver logs", "logs", "-l", "molnett.org/component=pageserver", "--tail=200")
	dump("safekeeper logs", "logs", "-l", "molnett.org/component=safekeeper", "--tail=100", "--prefix")
	dump("compute logs", "logs", "-l", computeSelector, "--tail=200")
}

func isPodReady(conds []corev1.PodCondition) bool {
	for _, c := range conds {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func formatConditions(conds []metav1.Condition) string {
	if len(conds) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		parts = append(parts, fmt.Sprintf("{%s=%s reason=%s msg=%q}", c.Type, c.Status, c.Reason, c.Message))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatPodConditions(conds []corev1.PodCondition) string {
	if len(conds) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(conds))
	for _, c := range conds {
		parts = append(parts, fmt.Sprintf("{%s=%s reason=%s}", c.Type, c.Status, c.Reason))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
