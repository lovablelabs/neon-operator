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

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	neonv1alpha1 "oltp.molnett.org/neon-operator/api/v1alpha1"
	"oltp.molnett.org/neon-operator/test/fixtures"
	"oltp.molnett.org/neon-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "neon"

// serviceAccountName created for the project
const serviceAccountName = "neon-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "neon-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "neon-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the baseline security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace, "--dry-run=client", "-o", "yaml")
		namespaceManifest, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to generate namespace manifest")

		cmd = exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(namespaceManifest)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to apply namespace")

		By("labeling the namespace to enforce the baseline security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=baseline")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with baseline policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command(
			"make",
			"deploy",
			fmt.Sprintf("IMG_OPERATOR=%s", projectImage),
		)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmdCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		cmd = exec.CommandContext(cmdCtx, "make", "undeploy", "ignore-not-found=true")
		_, _ = utils.Run(cmd)
		cancel()

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall", "ignore-not-found=true")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=neon-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})

	Context("API validation", func() {
		It("rejects unsupported values on create", func() {
			ctx := context.Background()
			cli := newE2EClient()
			next := 0
			name := func(prefix string) string {
				next++
				return fmt.Sprintf("validation-%s-%d", prefix, next)
			}
			cluster := func(mutate func(*neonv1alpha1.Cluster)) client.Object {
				obj := fixtures.NewCluster(name("cluster"), namespace)
				mutate(obj)
				return obj
			}
			project := func(mutate func(*neonv1alpha1.Project)) client.Object {
				obj := fixtures.NewProject(name("project"), namespace, "cluster-a")
				mutate(obj)
				return obj
			}
			branch := func(mutate func(*neonv1alpha1.Branch)) client.Object {
				obj := fixtures.NewBranch(name("branch"), namespace, "project-a")
				mutate(obj)
				return obj
			}
			pageserver := func(mutate func(*neonv1alpha1.Pageserver)) client.Object {
				obj := fixtures.NewPageserver(name("pageserver"), namespace, "cluster-a", 0)
				mutate(obj)
				return obj
			}
			safekeeper := func(mutate func(*neonv1alpha1.Safekeeper)) client.Object {
				obj := fixtures.NewSafekeeper(name("safekeeper"), namespace, "cluster-a", 0)
				mutate(obj)
				return obj
			}

			for _, tc := range []struct {
				name string
				obj  client.Object
			}{
				{name: "Cluster numSafekeepers", obj: cluster(func(c *neonv1alpha1.Cluster) { c.Spec.NumSafekeepers = 4 })},
				{name: "Cluster defaultPGVersion", obj: cluster(func(c *neonv1alpha1.Cluster) { c.Spec.DefaultPGVersion = 13 })},
				{name: "Cluster neonImage", obj: cluster(func(c *neonv1alpha1.Cluster) { c.Spec.NeonImage = "" })},
				{name: "Cluster bucketCredentialsSecret.name", obj: cluster(func(c *neonv1alpha1.Cluster) { c.Spec.BucketCredentialsSecret.Name = "" })},
				{name: "Cluster storageControllerDatabaseSecret.name", obj: cluster(func(c *neonv1alpha1.Cluster) { c.Spec.StorageControllerDatabaseSecret.Name = "" })},
				{name: "Cluster storageControllerDatabaseSecret.key", obj: cluster(func(c *neonv1alpha1.Cluster) { c.Spec.StorageControllerDatabaseSecret.Key = "" })},
				{name: "Project cluster", obj: project(func(p *neonv1alpha1.Project) { p.Spec.ClusterName = "" })},
				{name: "Project tenantId", obj: project(func(p *neonv1alpha1.Project) { p.Spec.TenantID = "not-a-neon-id" })},
				{name: "Project pgVersion", obj: project(func(p *neonv1alpha1.Project) { p.Spec.PGVersion = 13 })},
				{name: "Branch projectID", obj: branch(func(b *neonv1alpha1.Branch) { b.Spec.ProjectID = "" })},
				{name: "Branch timelineID", obj: branch(func(b *neonv1alpha1.Branch) { b.Spec.TimelineID = "not-a-neon-id" })},
				{name: "Branch pgVersion", obj: branch(func(b *neonv1alpha1.Branch) { b.Spec.PGVersion = 13 })},
				{name: "Pageserver cluster", obj: pageserver(func(p *neonv1alpha1.Pageserver) { p.Spec.Cluster = "" })},
				{name: "Pageserver bucketCredentialsSecret.name", obj: pageserver(func(p *neonv1alpha1.Pageserver) { p.Spec.BucketCredentialsSecret.Name = "" })},
				{name: "Pageserver storageConfig.size", obj: pageserver(func(p *neonv1alpha1.Pageserver) { p.Spec.StorageConfig.Size = "tenGi" })},
				{name: "Safekeeper cluster", obj: safekeeper(func(s *neonv1alpha1.Safekeeper) { s.Spec.Cluster = "" })},
				{name: "Safekeeper storageConfig.size", obj: safekeeper(func(s *neonv1alpha1.Safekeeper) { s.Spec.StorageConfig.Size = "tenGi" })},
			} {
				By("expecting invalid create for " + tc.name)
				err := cli.Create(ctx, tc.obj)
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected invalid error for %s, got %v", tc.name, err)
			}
		})

		It("rejects unsafe updates", func() {
			ctx := context.Background()
			cli := newE2EClient()
			expectInvalidUpdate := func(rule string, update func() error) {
				invalid := false
				err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
					err := update()
					if apierrors.IsInvalid(err) {
						invalid = true
						return nil
					}
					return err
				})
				Expect(err).NotTo(HaveOccurred(), "expected invalid update for %s, got %v", rule, err)
				Expect(invalid).To(BeTrue(), "expected invalid update for %s", rule)
			}
			updateProject := func(key types.NamespacedName, mutate func(*neonv1alpha1.Project)) func() error {
				return func() error {
					obj := &neonv1alpha1.Project{}
					if err := cli.Get(ctx, key, obj); err != nil {
						return err
					}
					mutate(obj)
					return cli.Update(ctx, obj)
				}
			}
			updateBranch := func(key types.NamespacedName, mutate func(*neonv1alpha1.Branch)) func() error {
				return func() error {
					obj := &neonv1alpha1.Branch{}
					if err := cli.Get(ctx, key, obj); err != nil {
						return err
					}
					mutate(obj)
					return cli.Update(ctx, obj)
				}
			}
			updatePageserver := func(key types.NamespacedName, mutate func(*neonv1alpha1.Pageserver)) func() error {
				return func() error {
					obj := &neonv1alpha1.Pageserver{}
					if err := cli.Get(ctx, key, obj); err != nil {
						return err
					}
					mutate(obj)
					return cli.Update(ctx, obj)
				}
			}
			updateSafekeeper := func(key types.NamespacedName, mutate func(*neonv1alpha1.Safekeeper)) func() error {
				return func() error {
					obj := &neonv1alpha1.Safekeeper{}
					if err := cli.Get(ctx, key, obj); err != nil {
						return err
					}
					mutate(obj)
					return cli.Update(ctx, obj)
				}
			}

			project := fixtures.NewProject("validation-project-update", namespace, "cluster-a")
			project.Spec.TenantID = "00000000000000000000000000000001"
			Expect(cli.Create(ctx, project)).To(Succeed())

			projectKey := types.NamespacedName{Name: project.Name, Namespace: namespace}
			expectInvalidUpdate("Project cluster immutability", updateProject(projectKey, func(p *neonv1alpha1.Project) { p.Spec.ClusterName = "cluster-b" }))
			expectInvalidUpdate("Project tenantId immutability", updateProject(projectKey, func(p *neonv1alpha1.Project) { p.Spec.TenantID = "00000000000000000000000000000002" }))

			branch := fixtures.NewBranch("validation-branch-update", namespace, "project-a")
			branch.Spec.TimelineID = "00000000000000000000000000000001"
			Expect(cli.Create(ctx, branch)).To(Succeed())

			branchKey := types.NamespacedName{Name: branch.Name, Namespace: namespace}
			expectInvalidUpdate("Branch projectID immutability", updateBranch(branchKey, func(b *neonv1alpha1.Branch) { b.Spec.ProjectID = "project-b" }))
			expectInvalidUpdate("Branch timelineID immutability", updateBranch(branchKey, func(b *neonv1alpha1.Branch) { b.Spec.TimelineID = "00000000000000000000000000000002" }))

			pageserver := fixtures.NewPageserver("validation-pageserver-update", namespace, "cluster-a", 0)
			pageserver.Spec.StorageConfig.StorageClass = ptr.To("fast")
			Expect(cli.Create(ctx, pageserver)).To(Succeed())

			pageserverKey := types.NamespacedName{Name: pageserver.Name, Namespace: namespace}
			expectInvalidUpdate("Pageserver id immutability", updatePageserver(pageserverKey, func(p *neonv1alpha1.Pageserver) { p.Spec.ID = 1 }))
			expectInvalidUpdate("Pageserver cluster immutability", updatePageserver(pageserverKey, func(p *neonv1alpha1.Pageserver) { p.Spec.Cluster = "cluster-b" }))
			expectInvalidUpdate("Pageserver storage size immutability", updatePageserver(pageserverKey, func(p *neonv1alpha1.Pageserver) { p.Spec.StorageConfig.Size = "2Gi" }))
			expectInvalidUpdate("Pageserver storage class immutability", updatePageserver(pageserverKey, func(p *neonv1alpha1.Pageserver) { p.Spec.StorageConfig.StorageClass = ptr.To("slow") }))

			safekeeper := fixtures.NewSafekeeper("validation-safekeeper-update", namespace, "cluster-a", 0)
			safekeeper.Spec.StorageConfig.StorageClass = ptr.To("fast")
			Expect(cli.Create(ctx, safekeeper)).To(Succeed())

			safekeeperKey := types.NamespacedName{Name: safekeeper.Name, Namespace: namespace}
			expectInvalidUpdate("Safekeeper id immutability", updateSafekeeper(safekeeperKey, func(s *neonv1alpha1.Safekeeper) { s.Spec.ID = 1 }))
			expectInvalidUpdate("Safekeeper cluster immutability", updateSafekeeper(safekeeperKey, func(s *neonv1alpha1.Safekeeper) { s.Spec.Cluster = "cluster-b" }))
			expectInvalidUpdate("Safekeeper storage size immutability", updateSafekeeper(safekeeperKey, func(s *neonv1alpha1.Safekeeper) { s.Spec.StorageConfig.Size = "2Gi" }))
			expectInvalidUpdate("Safekeeper storage class immutability", updateSafekeeper(safekeeperKey, func(s *neonv1alpha1.Safekeeper) { s.Spec.StorageConfig.StorageClass = ptr.To("slow") }))
		})
	})

	Context("Neon Lifecycle", func() {
		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				dumpLifecycleDiagnostics(namespace)
			}
		})

		It("reconciles Cluster, Project, and Branch and accepts SELECT 1", func() {
			ctx := context.Background()
			cli := newE2EClient()

			By("creating MinIO and storage-DB infrastructure")
			createInfra(ctx, cli, namespace)
			waitForDeploymentAvailable(ctx, cli, namespace, lifecycleStorageDBName, lifecycleAvailableTimeout)
			waitForDeploymentAvailable(ctx, cli, namespace, lifecycleMinIOName, lifecycleAvailableTimeout)

			By("creating the MinIO bucket")
			ensureMinIOBucket(namespace)

			By("creating Cluster")
			cluster := fixtures.NewCluster(lifecycleClusterName, namespace)
			Expect(cli.Create(ctx, cluster)).To(Succeed())
			waitForCRAvailable(ctx, cli, cluster)

			By("creating Pageserver and Safekeepers")
			pageserver := fixtures.NewPageserver(lifecyclePageserverName, namespace, lifecycleClusterName, lifecyclePageserverID)
			Expect(cli.Create(ctx, pageserver)).To(Succeed())
			safekeepers := make([]*neonv1alpha1.Safekeeper, 0, len(lifecycleSafekeeperIDs))
			for _, id := range lifecycleSafekeeperIDs {
				sk := fixtures.NewSafekeeper(fmt.Sprintf("safekeeper-e2e-%d", id), namespace, lifecycleClusterName, id)
				Expect(cli.Create(ctx, sk)).To(Succeed())
				safekeepers = append(safekeepers, sk)
			}
			waitForCRAvailable(ctx, cli, pageserver)
			for _, sk := range safekeepers {
				waitForCRAvailable(ctx, cli, sk)
			}

			By("creating Project")
			project := fixtures.NewProject(lifecycleProjectName, namespace, lifecycleClusterName)
			Expect(cli.Create(ctx, project)).To(Succeed())
			waitForCRAvailable(ctx, cli, project)

			By("creating Branch")
			branch := fixtures.NewBranch(lifecycleBranchName, namespace, lifecycleProjectName)
			Expect(cli.Create(ctx, branch)).To(Succeed())
			waitForCRAvailable(ctx, cli, branch)

			By("waiting for compute pod readiness")
			podName := waitForComputePodReady(ctx, cli, namespace, lifecycleBranchName)

			By("running SELECT 1 against the compute pod")
			execSelectOne(namespace, podName)
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
