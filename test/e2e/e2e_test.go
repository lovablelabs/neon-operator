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
	"k8s.io/apimachinery/pkg/types"

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
		By("cleaning lifecycle fixtures before undeploy")
		ctx := context.Background()
		cli := e2eClient()
		cleanupLifecycleFixtures(ctx, cli, namespace)
		assertLifecycleFixturesDeleted(ctx, cli, namespace)

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

			By("Fetching lifecycle custom resources")
			cmd = exec.Command("kubectl", "get", "cluster,project,branch,pageserver,safekeeper", "-n", namespace, "-o", "wide")
			resourcesOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Lifecycle resources:\n%s", resourcesOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get lifecycle resources: %s", err)
			}

			By("Fetching pageserver logs")
			cmd = exec.Command("kubectl", "logs", "cluster-e2e-pageserver-0", "-n", namespace, "--tail=200")
			pageserverLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Pageserver logs:\n%s", pageserverLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get pageserver logs: %s", err)
			}

			By("Fetching safekeeper logs")
			for _, name := range []string{"cluster-e2e-safekeeper-0", "cluster-e2e-safekeeper-1", "cluster-e2e-safekeeper-2"} {
				cmd = exec.Command("kubectl", "logs", name, "-n", namespace, "--tail=100")
				safekeeperLogs, safekeeperErr := utils.Run(cmd)
				if safekeeperErr == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "Safekeeper logs (%s):\n%s", name, safekeeperLogs)
				} else {
					_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get safekeeper logs for %s: %s", name, safekeeperErr)
				}
			}

			By("Fetching storage-controller logs")
			cmd = exec.Command("kubectl", "logs", "deploy/cluster-e2e-storage-controller", "-n", namespace, "--tail=200")
			storageControllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Storage-controller logs:\n%s", storageControllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get storage-controller logs: %s", err)
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
			cmd := exec.Command("kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: neon-metrics-reader
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, metricsRoleBindingName, serviceAccountName, namespace))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply ClusterRoleBinding")

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

		// TODO: Customize the e2e test suite with scenarios specific to your project.
		// Consider applying sample/CR(s) and check their status and/or verifying
		// the reconciliation by using the metrics, i.e.:
		// metricsOutput := getMetricsOutput()
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})

	Context("Neon Lifecycle", func() {
		It("reconciles cluster, project, and branch to Ready", func() {
			ctx := context.Background()
			cli := e2eClient()

			By("creating required lifecycle secrets")
			createLifecycleSecrets(ctx, cli, namespace)

			By("creating minio fixture for pageserver remote storage")
			createMinIOFixture(ctx, cli, namespace)

			By("waiting for minio readiness")
			waitForMinIOReady(ctx, cli, namespace)

			By("ensuring remote storage bucket exists")
			ensureMinIOBucket(namespace)

			By("creating storage database fixture for storage-controller")
			createStorageDatabaseFixture(ctx, cli, namespace)

			By("waiting for storage database readiness")
			waitForStorageDatabaseReady(ctx, cli, namespace)

			DeferCleanup(func() {
				cleanupLifecycleFixtures(ctx, cli, namespace)
				assertLifecycleFixturesDeleted(ctx, cli, namespace)
			})

			By("creating Cluster via typed Go API object")
			createLifecycleCluster(ctx, cli, namespace)

			By("waiting for Cluster Ready=True")
			waitForClusterReady(ctx, cli, types.NamespacedName{Name: lifecycleClusterName, Namespace: namespace})

			By("waiting for storage-controller deployment availability")
			waitForStorageControllerReady(ctx, cli, namespace, lifecycleClusterName)

			By("creating pageserver and safekeeper resources")
			createDataPlaneFixtures(ctx, cli, namespace)

			By("waiting for pageserver readiness")
			waitForPageserverReady(ctx, cli, types.NamespacedName{Name: lifecyclePageserverName, Namespace: namespace})

			By("waiting for safekeeper readiness")
			waitForSafekeeperReady(ctx, cli, types.NamespacedName{Name: "safekeeper-e2e-0", Namespace: namespace})
			waitForSafekeeperReady(ctx, cli, types.NamespacedName{Name: "safekeeper-e2e-1", Namespace: namespace})
			waitForSafekeeperReady(ctx, cli, types.NamespacedName{Name: "safekeeper-e2e-2", Namespace: namespace})

			By("creating Project via typed Go API object")
			createLifecycleProject(ctx, cli, namespace)

			By("waiting for Project Ready=True")
			waitForProjectReady(ctx, cli, types.NamespacedName{Name: lifecycleProjectName, Namespace: namespace})

			By("creating Branch via typed Go API object")
			createLifecycleBranch(ctx, cli, namespace)

			By("waiting for Branch Ready=True")
			waitForBranchReady(ctx, cli, types.NamespacedName{Name: lifecycleBranchName, Namespace: namespace})

			By("waiting for branch compute pod readiness")
			waitForComputePodReady(ctx, cli, namespace, lifecycleBranchName)
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
