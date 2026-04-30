package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/test/utils"
)

const (
	lifecycleClusterName         = "cluster-e2e"
	lifecycleProjectName         = "project-e2e"
	lifecycleBranchName          = "branch-e2e"
	lifecyclePageserverName      = "pageserver-e2e"
	lifecycleDatabaseServiceName = "storage-db-e2e"
	lifecycleDatabaseAppName     = "storage-db-e2e"
	lifecycleMinIOName           = "minio-e2e"
	lifecycleBucketSecretName    = "bucket-e2e"
	lifecycleDatabaseSecretName  = "storage-db-e2e"
	lifecycleMinIOClientPodName  = "minio-mc-e2e"
	lifecycleExistsTimeout       = 90 * time.Second
	lifecycleReadyTimeout        = 4 * time.Minute
	lifecycleComputeReadyTimeout = 3 * time.Minute
	lifecyclePollInterval        = time.Second
)

var e2eK8sClient client.Client

func e2eClient() client.Client {
	if e2eK8sClient != nil {
		return e2eK8sClient
	}

	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "failed to build kube config for e2e client")

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme)).To(Succeed())
	Expect(neonv1alpha1.AddToScheme(scheme)).To(Succeed())

	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred(), "failed to create e2e client")

	e2eK8sClient = cli
	return e2eK8sClient
}

func buildRequiredSecrets(testNamespace string) []*corev1.Secret {
	return []*corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      lifecycleBucketSecretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("e2e-access-key"),
				"AWS_SECRET_ACCESS_KEY": []byte("e2e-secret-key"),
				"AWS_REGION":            []byte("us-east-1"),
				"BUCKET_NAME":           []byte("neon-e2e-bucket"),
				"AWS_ENDPOINT_URL":      []byte(fmt.Sprintf("http://%s:9000", lifecycleMinIOName)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      lifecycleDatabaseSecretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"uri": []byte(fmt.Sprintf("postgresql://postgres:postgres@%s:5432/postgres?sslmode=disable", lifecycleDatabaseServiceName)),
			},
		},
	}
}

func buildStorageDatabaseDeployment(testNamespace string) *appsv1.Deployment {
	labels := map[string]string{"app.kubernetes.io/name": lifecycleDatabaseAppName}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleDatabaseAppName,
			Namespace: testNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgres",
							Image: "postgres:16-alpine",
							Env: []corev1.EnvVar{
								{Name: "POSTGRES_PASSWORD", Value: "postgres"},
								{Name: "POSTGRES_USER", Value: "postgres"},
								{Name: "POSTGRES_DB", Value: "postgres"},
							},
							Ports: []corev1.ContainerPort{{ContainerPort: 5432}},
						},
					},
				},
			},
		},
	}
}

func buildStorageDatabaseService(testNamespace string) *corev1.Service {
	labels := map[string]string{"app.kubernetes.io/name": lifecycleDatabaseAppName}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleDatabaseServiceName,
			Namespace: testNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "postgres",
					Port: 5432,
				},
			},
		},
	}
}

func buildMinIODeployment(testNamespace string) *appsv1.Deployment {
	labels := map[string]string{"app.kubernetes.io/name": lifecycleMinIOName}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleMinIOName,
			Namespace: testNamespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "minio",
							Image: "minio/minio:latest",
							Args:  []string{"server", "/data"},
							Env: []corev1.EnvVar{
								{Name: "MINIO_ROOT_USER", Value: "e2e-access-key"},
								{Name: "MINIO_ROOT_PASSWORD", Value: "e2e-secret-key"},
							},
							Ports: []corev1.ContainerPort{{ContainerPort: 9000}},
						},
					},
				},
			},
		},
	}
}

func buildMinIOService(testNamespace string) *corev1.Service {
	labels := map[string]string{"app.kubernetes.io/name": lifecycleMinIOName}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleMinIOName,
			Namespace: testNamespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name: "s3",
					Port: 9000,
				},
			},
		},
	}
}

func buildClusterFixture(testNamespace string) *neonv1alpha1.Cluster {
	return &neonv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleClusterName,
			Namespace: testNamespace,
		},
		Spec: neonv1alpha1.ClusterSpec{
			NumSafekeepers:   3,
			DefaultPGVersion: 17,
			NeonImage:        "neondatabase/neon:8463",
			BucketCredentialsSecret: &corev1.SecretReference{
				Name:      lifecycleBucketSecretName,
				Namespace: testNamespace,
			},
			StorageControllerDatabaseSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: lifecycleDatabaseSecretName},
				Key:                  "uri",
			},
		},
	}
}

func buildProjectFixture(testNamespace string) *neonv1alpha1.Project {
	return &neonv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleProjectName,
			Namespace: testNamespace,
		},
		Spec: neonv1alpha1.ProjectSpec{
			ClusterName: lifecycleClusterName,
			PGVersion:   17,
		},
	}
}

func buildBranchFixture(testNamespace string) *neonv1alpha1.Branch {
	return &neonv1alpha1.Branch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecycleBranchName,
			Namespace: testNamespace,
		},
		Spec: neonv1alpha1.BranchSpec{
			PGVersion: 17,
			ProjectID: lifecycleProjectName,
		},
	}
}

func buildPageserverFixture(testNamespace string) *neonv1alpha1.Pageserver {
	return &neonv1alpha1.Pageserver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lifecyclePageserverName,
			Namespace: testNamespace,
		},
		Spec: neonv1alpha1.PageserverSpec{
			ID:      0,
			Cluster: lifecycleClusterName,
			BucketCredentialsSecret: &corev1.SecretReference{
				Name:      lifecycleBucketSecretName,
				Namespace: testNamespace,
			},
			StorageConfig: neonv1alpha1.StorageConfig{Size: "10Gi"},
		},
	}
}

func buildSafekeeperFixture(testNamespace string, id uint32) *neonv1alpha1.Safekeeper {
	return &neonv1alpha1.Safekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("safekeeper-e2e-%d", id),
			Namespace: testNamespace,
		},
		Spec: neonv1alpha1.SafekeeperSpec{
			ID:      id,
			Cluster: lifecycleClusterName,
			StorageConfig: neonv1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}
}

func createLifecycleSecrets(ctx context.Context, cli client.Client, testNamespace string) {
	for _, secret := range buildRequiredSecrets(testNamespace) {
		Expect(cli.Create(ctx, secret)).To(Succeed(), "failed to create secret %s/%s", secret.Namespace, secret.Name)
	}
}

func createStorageDatabaseFixture(ctx context.Context, cli client.Client, testNamespace string) {
	deployment := buildStorageDatabaseDeployment(testNamespace)
	service := buildStorageDatabaseService(testNamespace)

	Expect(cli.Create(ctx, deployment)).To(Succeed(), "failed to create storage database deployment %s/%s", deployment.Namespace, deployment.Name)
	Expect(cli.Create(ctx, service)).To(Succeed(), "failed to create storage database service %s/%s", service.Namespace, service.Name)
}

func createMinIOFixture(ctx context.Context, cli client.Client, testNamespace string) {
	deployment := buildMinIODeployment(testNamespace)
	service := buildMinIOService(testNamespace)

	Expect(cli.Create(ctx, deployment)).To(Succeed(), "failed to create minio deployment %s/%s", deployment.Namespace, deployment.Name)
	Expect(cli.Create(ctx, service)).To(Succeed(), "failed to create minio service %s/%s", service.Namespace, service.Name)
}

func createLifecycleCluster(ctx context.Context, cli client.Client, testNamespace string) {
	cluster := buildClusterFixture(testNamespace)
	Expect(cli.Create(ctx, cluster)).To(Succeed(), "failed to create cluster %s/%s", cluster.Namespace, cluster.Name)
}

func createLifecycleProject(ctx context.Context, cli client.Client, testNamespace string) {
	project := buildProjectFixture(testNamespace)
	Expect(cli.Create(ctx, project)).To(Succeed(), "failed to create project %s/%s", project.Namespace, project.Name)
}

func createLifecycleBranch(ctx context.Context, cli client.Client, testNamespace string) {
	branch := buildBranchFixture(testNamespace)
	Expect(cli.Create(ctx, branch)).To(Succeed(), "failed to create branch %s/%s", branch.Namespace, branch.Name)
}

func createDataPlaneFixtures(ctx context.Context, cli client.Client, testNamespace string) {
	pageserver := buildPageserverFixture(testNamespace)
	Expect(cli.Create(ctx, pageserver)).To(Succeed(), "failed to create pageserver %s/%s", pageserver.Namespace, pageserver.Name)

	for _, id := range []uint32{0, 1, 2} {
		safekeeper := buildSafekeeperFixture(testNamespace, id)
		Expect(cli.Create(ctx, safekeeper)).To(Succeed(), "failed to create safekeeper %s/%s", safekeeper.Namespace, safekeeper.Name)
	}
}

func waitForClusterReady(ctx context.Context, cli client.Client, namespacedName types.NamespacedName) {
	Eventually(func(g Gomega) {
		cluster := &neonv1alpha1.Cluster{}
		err := cli.Get(ctx, namespacedName, cluster)
		g.Expect(err).NotTo(HaveOccurred(), "Cluster %s/%s was not created", namespacedName.Namespace, namespacedName.Name)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		cluster := &neonv1alpha1.Cluster{}
		err := cli.Get(ctx, namespacedName, cluster)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get Cluster %s/%s", namespacedName.Namespace, namespacedName.Name)
		g.Expect(isReadyConditionTrue(cluster.Status.Conditions)).To(
			BeTrue(),
			"cluster %s/%s not ready yet, phase=%q conditions=%s",
			namespacedName.Namespace,
			namespacedName.Name,
			cluster.Status.Phase,
			formatConditions(cluster.Status.Conditions),
		)
	}, lifecycleReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForProjectReady(ctx context.Context, cli client.Client, namespacedName types.NamespacedName) {
	Eventually(func(g Gomega) {
		project := &neonv1alpha1.Project{}
		err := cli.Get(ctx, namespacedName, project)
		g.Expect(err).NotTo(HaveOccurred(), "Project %s/%s was not created", namespacedName.Namespace, namespacedName.Name)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		project := &neonv1alpha1.Project{}
		err := cli.Get(ctx, namespacedName, project)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get Project %s/%s", namespacedName.Namespace, namespacedName.Name)
		g.Expect(isReadyConditionTrue(project.Status.Conditions)).To(
			BeTrue(),
			"project %s/%s not ready yet, phase=%q conditions=%s",
			namespacedName.Namespace,
			namespacedName.Name,
			project.Status.Phase,
			formatConditions(project.Status.Conditions),
		)
	}, lifecycleReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForBranchReady(ctx context.Context, cli client.Client, namespacedName types.NamespacedName) {
	Eventually(func(g Gomega) {
		branch := &neonv1alpha1.Branch{}
		err := cli.Get(ctx, namespacedName, branch)
		g.Expect(err).NotTo(HaveOccurred(), "Branch %s/%s was not created", namespacedName.Namespace, namespacedName.Name)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		branch := &neonv1alpha1.Branch{}
		err := cli.Get(ctx, namespacedName, branch)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get Branch %s/%s", namespacedName.Namespace, namespacedName.Name)
		g.Expect(isReadyConditionTrue(branch.Status.Conditions)).To(
			BeTrue(),
			"branch %s/%s not ready yet, phase=%q conditions=%s",
			namespacedName.Namespace,
			namespacedName.Name,
			branch.Status.Phase,
			formatConditions(branch.Status.Conditions),
		)
	}, lifecycleReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForComputePodReady(ctx context.Context, cli client.Client, testNamespace, branchName string) {
	Eventually(func(g Gomega) {
		computeLabels := labels.SelectorFromSet(map[string]string{
			"molnett.org/component": "compute",
			"molnett.org/branch":    branchName,
		})

		pods := &corev1.PodList{}
		err := cli.List(ctx, pods, &client.ListOptions{
			Namespace:     testNamespace,
			LabelSelector: computeLabels,
		})
		g.Expect(err).NotTo(HaveOccurred(), "failed to list compute pods")
		g.Expect(pods.Items).NotTo(BeEmpty(), "no compute pods found for branch %q", branchName)

		sort.Slice(pods.Items, func(i, j int) bool {
			return pods.Items[i].Name < pods.Items[j].Name
		})

		pod := pods.Items[0]
		g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning), "compute pod %s not running", pod.Name)
		g.Expect(isPodReady(pod.Status.Conditions)).To(
			BeTrue(),
			"compute pod %s not ready, phase=%q conditions=%s",
			pod.Name,
			pod.Status.Phase,
			formatPodConditions(pod.Status.Conditions),
		)
	}, lifecycleComputeReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForStorageControllerReady(ctx context.Context, cli client.Client, testNamespace, clusterName string) {
	deploymentName := fmt.Sprintf("%s-storage-controller", clusterName)

	Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		err := cli.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: testNamespace}, deployment)
		g.Expect(err).NotTo(HaveOccurred(), "storage-controller deployment %s/%s was not created", testNamespace, deploymentName)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		err := cli.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: testNamespace}, deployment)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get storage-controller deployment %s/%s", testNamespace, deploymentName)
		g.Expect(deployment.Status.AvailableReplicas).To(BeNumerically(">=", 1),
			"storage-controller deployment %s/%s not available yet, ready=%d available=%d updated=%d",
			testNamespace,
			deploymentName,
			deployment.Status.ReadyReplicas,
			deployment.Status.AvailableReplicas,
			deployment.Status.UpdatedReplicas,
		)
	}, lifecycleComputeReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForStorageDatabaseReady(ctx context.Context, cli client.Client, testNamespace string) {
	Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		err := cli.Get(ctx, types.NamespacedName{Name: lifecycleDatabaseAppName, Namespace: testNamespace}, deployment)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get storage database deployment %s/%s", testNamespace, lifecycleDatabaseAppName)
		g.Expect(deployment.Status.AvailableReplicas).To(BeNumerically(">=", 1),
			"storage database deployment %s/%s not available yet, ready=%d available=%d updated=%d",
			testNamespace,
			lifecycleDatabaseAppName,
			deployment.Status.ReadyReplicas,
			deployment.Status.AvailableReplicas,
			deployment.Status.UpdatedReplicas,
		)
	}, lifecycleComputeReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForMinIOReady(ctx context.Context, cli client.Client, testNamespace string) {
	Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		err := cli.Get(ctx, types.NamespacedName{Name: lifecycleMinIOName, Namespace: testNamespace}, deployment)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get minio deployment %s/%s", testNamespace, lifecycleMinIOName)
		g.Expect(deployment.Status.AvailableReplicas).To(BeNumerically(">=", 1),
			"minio deployment %s/%s not available yet, ready=%d available=%d updated=%d",
			testNamespace,
			lifecycleMinIOName,
			deployment.Status.ReadyReplicas,
			deployment.Status.AvailableReplicas,
			deployment.Status.UpdatedReplicas,
		)
	}, lifecycleComputeReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func ensureMinIOBucket(testNamespace string) {
	cleanupCmd := exec.Command("kubectl", "delete", "pod", lifecycleMinIOClientPodName, "-n", testNamespace, "--ignore-not-found")
	_, _ = utils.Run(cleanupCmd)

	runCmd := exec.Command(
		"kubectl", "run", lifecycleMinIOClientPodName,
		"--restart=Never",
		"--namespace", testNamespace,
		"--image=minio/mc:latest",
		"--command",
		"--",
		"/bin/sh",
		"-c",
		"mc alias set local http://minio-e2e:9000 e2e-access-key e2e-secret-key && mc mb -p local/neon-e2e-bucket || true",
	)
	_, err := utils.Run(runCmd)
	Expect(err).NotTo(HaveOccurred(), "failed to start minio bucket bootstrap pod")

	Eventually(func(g Gomega) {
		waitCmd := exec.Command("kubectl", "get", "pod", lifecycleMinIOClientPodName, "-n", testNamespace, "-o", "jsonpath={.status.phase}")
		output, waitErr := utils.Run(waitCmd)
		g.Expect(waitErr).NotTo(HaveOccurred(), "failed to read minio bootstrap pod phase")
		g.Expect(output).To(Equal("Succeeded"), "minio bucket bootstrap pod did not succeed")
	}, lifecycleComputeReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForPageserverReady(ctx context.Context, cli client.Client, namespacedName types.NamespacedName) {
	Eventually(func(g Gomega) {
		pageserver := &neonv1alpha1.Pageserver{}
		err := cli.Get(ctx, namespacedName, pageserver)
		g.Expect(err).NotTo(HaveOccurred(), "Pageserver %s/%s was not created", namespacedName.Namespace, namespacedName.Name)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		pageserver := &neonv1alpha1.Pageserver{}
		err := cli.Get(ctx, namespacedName, pageserver)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get Pageserver %s/%s", namespacedName.Namespace, namespacedName.Name)
		g.Expect(isReadyConditionTrue(pageserver.Status.Conditions)).To(
			BeTrue(),
			"pageserver %s/%s not ready yet, phase=%q conditions=%s",
			namespacedName.Namespace,
			namespacedName.Name,
			pageserver.Status.Phase,
			formatConditions(pageserver.Status.Conditions),
		)
	}, lifecycleReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func waitForSafekeeperReady(ctx context.Context, cli client.Client, namespacedName types.NamespacedName) {
	Eventually(func(g Gomega) {
		safekeeper := &neonv1alpha1.Safekeeper{}
		err := cli.Get(ctx, namespacedName, safekeeper)
		g.Expect(err).NotTo(HaveOccurred(), "Safekeeper %s/%s was not created", namespacedName.Namespace, namespacedName.Name)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())

	Eventually(func(g Gomega) {
		safekeeper := &neonv1alpha1.Safekeeper{}
		err := cli.Get(ctx, namespacedName, safekeeper)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get Safekeeper %s/%s", namespacedName.Namespace, namespacedName.Name)
		g.Expect(isReadyConditionTrue(safekeeper.Status.Conditions)).To(
			BeTrue(),
			"safekeeper %s/%s not ready yet, phase=%q conditions=%s",
			namespacedName.Namespace,
			namespacedName.Name,
			safekeeper.Status.Phase,
			formatConditions(safekeeper.Status.Conditions),
		)
	}, lifecycleReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func cleanupLifecycleFixtures(ctx context.Context, cli client.Client, testNamespace string) {
	deleteDataPlaneControllers(ctx, cli, testNamespace)
	cleanupPageserverResources(ctx, cli, testNamespace)

	for _, obj := range []client.Object{
		buildBranchFixture(testNamespace),
		buildProjectFixture(testNamespace),
		buildPageserverFixture(testNamespace),
		buildSafekeeperFixture(testNamespace, 0),
		buildSafekeeperFixture(testNamespace, 1),
		buildSafekeeperFixture(testNamespace, 2),
		buildClusterFixture(testNamespace),
		buildMinIOService(testNamespace),
		buildMinIODeployment(testNamespace),
		buildStorageDatabaseService(testNamespace),
		buildStorageDatabaseDeployment(testNamespace),
	} {
		err := cli.Delete(ctx, obj)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "failed to delete %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
		}
	}

	for _, secret := range buildRequiredSecrets(testNamespace) {
		err := cli.Delete(ctx, secret)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "failed to delete Secret %s/%s", secret.Namespace, secret.Name)
		}
	}

	minioClientCleanup := exec.Command("kubectl", "delete", "pod", lifecycleMinIOClientPodName, "-n", testNamespace, "--ignore-not-found")
	_, _ = utils.Run(minioClientCleanup)
}

func deleteDataPlaneControllers(ctx context.Context, cli client.Client, testNamespace string) {
	for _, obj := range []client.Object{
		buildPageserverFixture(testNamespace),
		buildSafekeeperFixture(testNamespace, 0),
		buildSafekeeperFixture(testNamespace, 1),
		buildSafekeeperFixture(testNamespace, 2),
	} {
		err := cli.Delete(ctx, obj)
		if err != nil && !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "failed to pre-delete data-plane controller %T %s/%s", obj, obj.GetNamespace(), obj.GetName())
		}
	}
}

func cleanupPageserverResources(ctx context.Context, cli client.Client, testNamespace string) {
	pageserver := buildPageserverFixture(testNamespace)
	if err := cli.Get(ctx, types.NamespacedName{Name: pageserver.Name, Namespace: pageserver.Namespace}, pageserver); err == nil {
		if len(pageserver.Finalizers) > 0 {
			err := clearObjectFinalizers(ctx, cli, pageserver)
			Expect(err).NotTo(HaveOccurred(), "failed to clear pageserver finalizers for %s/%s", pageserver.Namespace, pageserver.Name)
		}
	} else if !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred(), "failed to get pageserver %s/%s for finalizer cleanup", pageserver.Namespace, pageserver.Name)
	}

	Eventually(func(g Gomega) {
		pageserverPods, err := listPageserverPods(ctx, cli, testNamespace)
		g.Expect(err).NotTo(HaveOccurred(), "failed to list pageserver pods for cleanup")

		for i := range pageserverPods.Items {
			pod := &pageserverPods.Items[i]

			if len(pod.Finalizers) > 0 {
				patchErr := clearObjectFinalizers(ctx, cli, pod)
				if patchErr != nil && !apierrors.IsNotFound(patchErr) {
					g.Expect(patchErr).NotTo(HaveOccurred(), "failed to clear pageserver pod finalizers for %s/%s", pod.Namespace, pod.Name)
				}
			}

			zeroGracePeriod := int64(0)
			backgroundPropagation := metav1.DeletePropagationBackground
			deleteErr := cli.Delete(ctx, pod, &client.DeleteOptions{
				GracePeriodSeconds: &zeroGracePeriod,
				PropagationPolicy:  &backgroundPropagation,
			})
			if deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
				g.Expect(deleteErr).NotTo(HaveOccurred(), "failed to delete pageserver pod %s/%s", pod.Namespace, pod.Name)
			}
		}

		remainingPods, err := listPageserverPods(ctx, cli, testNamespace)
		g.Expect(err).NotTo(HaveOccurred(), "failed to re-list pageserver pods for cleanup assertion")

		g.Expect(remainingPods.Items).To(BeEmpty(), "expected pageserver pods to be deleted, still found %d", len(remainingPods.Items))
	}, lifecycleReadyTimeout, lifecyclePollInterval).Should(Succeed())
}

func listPageserverPods(ctx context.Context, cli client.Client, testNamespace string) (*corev1.PodList, error) {
	pageserverPods := &corev1.PodList{}
	err := cli.List(ctx, pageserverPods, &client.ListOptions{
		Namespace: testNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"molnett.org/component": "pageserver",
		}),
	})
	return pageserverPods, err
}

func clearObjectFinalizers(ctx context.Context, cli client.Client, obj client.Object) error {
	return cli.Patch(ctx, obj, client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"finalizers":[]}}`)))
}

func assertLifecycleFixturesDeleted(ctx context.Context, cli client.Client, testNamespace string) {
	for _, obj := range []client.Object{
		buildBranchFixture(testNamespace),
		buildProjectFixture(testNamespace),
		buildPageserverFixture(testNamespace),
		buildSafekeeperFixture(testNamespace, 0),
		buildSafekeeperFixture(testNamespace, 1),
		buildSafekeeperFixture(testNamespace, 2),
		buildClusterFixture(testNamespace),
		buildMinIOService(testNamespace),
		buildMinIODeployment(testNamespace),
		buildStorageDatabaseService(testNamespace),
		buildStorageDatabaseDeployment(testNamespace),
	} {
		assertObjectDeleted(ctx, cli, obj)
	}

	for _, secret := range buildRequiredSecrets(testNamespace) {
		assertObjectDeleted(ctx, cli, secret)
	}

	assertNoPageserverPodsRemain(ctx, cli, testNamespace)

	assertObjectDeleted(ctx, cli, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: lifecycleMinIOClientPodName, Namespace: testNamespace}})
}

func assertNoPageserverPodsRemain(ctx context.Context, cli client.Client, testNamespace string) {
	Eventually(func(g Gomega) {
		pageserverPods := &corev1.PodList{}
		err := cli.List(ctx, pageserverPods, &client.ListOptions{
			Namespace: testNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"molnett.org/component": "pageserver",
			}),
		})
		g.Expect(err).NotTo(HaveOccurred(), "failed to list pageserver pods for deletion assertion")

		remaining := make([]string, 0, len(pageserverPods.Items))
		for _, pod := range pageserverPods.Items {
			remaining = append(remaining, fmt.Sprintf("%s phase=%s finalizers=%v", pod.Name, pod.Status.Phase, pod.Finalizers))
		}

		g.Expect(remaining).To(BeEmpty(), "expected no pageserver pods to remain, got: %s", strings.Join(remaining, "; "))
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())
}

func assertObjectDeleted(ctx context.Context, cli client.Client, obj client.Object) {
	Eventually(func(g Gomega) {
		target := obj.DeepCopyObject().(client.Object)
		err := cli.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, target)
		g.Expect(apierrors.IsNotFound(err)).To(
			BeTrue(),
			"expected %T %s/%s to be deleted, got err=%v",
			target,
			obj.GetNamespace(),
			obj.GetName(),
			err,
		)
	}, lifecycleExistsTimeout, lifecyclePollInterval).Should(Succeed())
}

func isReadyConditionTrue(conditions []metav1.Condition) bool {
	for _, condition := range conditions {
		if condition.Type == "Ready" {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
}

func isPodReady(conditions []corev1.PodCondition) bool {
	for _, condition := range conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func formatConditions(conditions []metav1.Condition) string {
	if len(conditions) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(conditions))
	for _, c := range conditions {
		parts = append(parts, fmt.Sprintf("{type=%s status=%s reason=%s message=%q}", c.Type, c.Status, c.Reason, c.Message))
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

func formatPodConditions(conditions []corev1.PodCondition) string {
	if len(conditions) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(conditions))
	for _, c := range conditions {
		parts = append(parts, fmt.Sprintf("{type=%s status=%s reason=%s message=%q}", c.Type, c.Status, c.Reason, c.Message))
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

func int32Ptr(value int32) *int32 {
	return &value
}
