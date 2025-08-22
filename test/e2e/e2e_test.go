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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/elenpay/dapr-trustbundle/test/utils"
)

// namespace where the project is deployed in
const namespace = "dapr-trustbundle-system"

// serviceAccountName created for the project
const serviceAccountName = "dapr-trustbundle-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "dapr-trustbundle-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "dapr-trustbundle-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
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
				"--clusterrole=dapr-trustbundle-metrics-reader",
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
						"serviceAccount": "%s"
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

		It("should copy source secret with key renaming", func() {
			testNamespace := "dapr-system"
			sourceSecretName := "dapr-trust-bundle-cert-manager"
			destSecretName := "dapr-trust-bundle"
			destConfigMapName := "dapr-trust-bundle"

			By("creating test namespace")
			cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
			_, _ = utils.Run(cmd) // Ignore error if namespace already exists

			By("creating source secret with TLS certificate data")
			cmd = exec.Command("kubectl", "create", "secret", "generic", sourceSecretName,
				"--namespace", testNamespace,
				"--from-literal=ca.crt=-----BEGIN CERTIFICATE-----\nMIICJjCCAc+gAwIBAgIBATANBgkqhkiG9w0BAQUFADCBiTELMAkGA1UEBhMCVVMx\nTest-CA-Certificate\n-----END CERTIFICATE-----",
				"--from-literal=tls.key=-----BEGIN PRIVATE KEY-----\nMIICdgIBADANBgkqhkiG9w0BAQEFAASCAmAwggJcAgEAAoGBAKtest123\nTest-Private-Key\n-----END PRIVATE KEY-----",
				"--from-literal=tls.crt=-----BEGIN CERTIFICATE-----\nMIICJjCCAc+gAwIBAgIBATANBgkqhkiG9w0BAQUFADCBiTELMAkGA1UEBhMCVVMx\nTest-TLS-Certificate\n-----END CERTIFICATE-----",
				"--from-literal=extra.data=some-extra-data",
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create source secret")

			By("waiting for destination secret to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination secret should exist")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying key renaming in destination secret")
			cmd = exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace, "-o", "jsonpath={.data}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			// Parse the JSON to verify key renaming
			var secretData map[string]string
			err = json.Unmarshal([]byte(output), &secretData)
			Expect(err).NotTo(HaveOccurred())

			// Check that keys were renamed correctly
			Expect(secretData).To(HaveKey("ca.crt"), "ca.crt should be present (unchanged)")
			Expect(secretData).To(HaveKey("issuer.key"), "tls.key should be renamed to issuer.key")
			Expect(secretData).To(HaveKey("issuer.crt"), "tls.crt should be renamed to issuer.crt")
			Expect(secretData).NotTo(HaveKey("tls.key"), "tls.key should not exist in destination")
			Expect(secretData).NotTo(HaveKey("tls.crt"), "tls.crt should not exist in destination")
			Expect(secretData).NotTo(HaveKey("extra.data"), "extra.data should be filtered out")

			By("verifying destination configmap was created with only ca.crt")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace, "-o", "jsonpath={.data}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var configMapData map[string]string
				err = json.Unmarshal([]byte(output), &configMapData)
				g.Expect(err).NotTo(HaveOccurred())

				// ConfigMap should only contain ca.crt
				g.Expect(configMapData).To(HaveKey("ca.crt"), "configmap should contain ca.crt")
				g.Expect(configMapData).To(HaveLen(1), "configmap should only contain ca.crt")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying secret type is Opaque")
			cmd = exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace, "-o", "jsonpath={.type}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Opaque"), "Destination secret should be Opaque type")

			By("verifying management labels are present")
			cmd = exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace, "-o", "jsonpath={.metadata.labels}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("dapr-trustbundle-operator"), "Should have management label")
			Expect(output).To(ContainSubstring("dapr-trust-bundle"), "Should have component label")

			// Cleanup
			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "secret", sourceSecretName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", destSecretName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", destConfigMapName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should recreate destination resources when deleted (self-healing)", func() {
			testNamespace := "dapr-system"
			sourceSecretName := "dapr-trust-bundle-cert-manager"
			destSecretName := "dapr-trust-bundle"
			destConfigMapName := "dapr-trust-bundle"

			By("creating test namespace")
			cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
			_, _ = utils.Run(cmd) // Ignore error if namespace already exists

			By("creating source secret")
			cmd = exec.Command("kubectl", "create", "secret", "generic", sourceSecretName,
				"--namespace", testNamespace,
				"--from-literal=ca.crt=-----BEGIN CERTIFICATE-----\nTest-CA-For-Self-Healing\n-----END CERTIFICATE-----",
				"--from-literal=tls.key=-----BEGIN PRIVATE KEY-----\nTest-Key-For-Self-Healing\n-----END PRIVATE KEY-----",
				"--from-literal=tls.crt=-----BEGIN CERTIFICATE-----\nTest-Cert-For-Self-Healing\n-----END CERTIFICATE-----",
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create source secret")

			By("waiting for destination resources to be created initially")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination secret should exist")

				cmd = exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace)
				_, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination configmap should exist")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("deleting destination secret to test self-healing")
			cmd = exec.Command("kubectl", "delete", "secret", destSecretName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete destination secret")

			By("verifying destination secret is recreated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination secret should be recreated")

				// Verify it has the correct keys after recreation
				cmd = exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace, "-o", "jsonpath={.data}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var secretData map[string]string
				err = json.Unmarshal([]byte(output), &secretData)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(secretData).To(HaveKey("ca.crt"), "Recreated secret should have ca.crt")
				g.Expect(secretData).To(HaveKey("issuer.key"), "Recreated secret should have issuer.key")
				g.Expect(secretData).To(HaveKey("issuer.crt"), "Recreated secret should have issuer.crt")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("deleting destination configmap to test self-healing")
			cmd = exec.Command("kubectl", "delete", "configmap", destConfigMapName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete destination configmap")

			By("verifying destination configmap is recreated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination configmap should be recreated")

				// Verify it has the correct data after recreation
				cmd = exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace, "-o", "jsonpath={.data}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())

				var configMapData map[string]string
				err = json.Unmarshal([]byte(output), &configMapData)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(configMapData).To(HaveKey("ca.crt"), "Recreated configmap should have ca.crt")
				g.Expect(configMapData).To(HaveLen(1), "Recreated configmap should only contain ca.crt")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Cleanup
			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "secret", sourceSecretName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", destSecretName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", destConfigMapName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
		})

		It("should clean up destination resources when source secret is deleted", func() {
			testNamespace := "dapr-system"
			sourceSecretName := "dapr-trust-bundle-cert-manager"
			destSecretName := "dapr-trust-bundle"
			destConfigMapName := "dapr-trust-bundle"

			By("creating test namespace")
			cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
			_, _ = utils.Run(cmd) // Ignore error if namespace already exists

			By("creating source secret")
			cmd = exec.Command("kubectl", "create", "secret", "generic", sourceSecretName,
				"--namespace", testNamespace,
				"--from-literal=ca.crt=-----BEGIN CERTIFICATE-----\nTest-CA-For-Cleanup\n-----END CERTIFICATE-----",
				"--from-literal=tls.key=-----BEGIN PRIVATE KEY-----\nTest-Key-For-Cleanup\n-----END PRIVATE KEY-----",
				"--from-literal=tls.crt=-----BEGIN CERTIFICATE-----\nTest-Cert-For-Cleanup\n-----END CERTIFICATE-----",
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create source secret")

			By("waiting for destination resources to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination secret should exist")

				cmd = exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace)
				_, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination configmap should exist")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("deleting source secret")
			cmd = exec.Command("kubectl", "delete", "secret", sourceSecretName, "-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete source secret")

			By("verifying destination resources are cleaned up")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Destination secret should be deleted")

				cmd = exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace)
				_, err = utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "Destination configmap should be deleted")
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should update destination resources when source secret is modified", func() {
			testNamespace := "dapr-system"
			sourceSecretName := "dapr-trust-bundle-cert-manager"
			destSecretName := "dapr-trust-bundle"
			destConfigMapName := "dapr-trust-bundle"

			By("creating test namespace")
			cmd := exec.Command("kubectl", "create", "namespace", testNamespace)
			_, _ = utils.Run(cmd) // Ignore error if namespace already exists

			By("creating initial source secret")
			cmd = exec.Command("kubectl", "create", "secret", "generic", sourceSecretName,
				"--namespace", testNamespace,
				"--from-literal=ca.crt=-----BEGIN CERTIFICATE-----\nInitial-CA-Certificate\n-----END CERTIFICATE-----",
				"--from-literal=tls.key=-----BEGIN PRIVATE KEY-----\nInitial-Private-Key\n-----END PRIVATE KEY-----",
				"--from-literal=tls.crt=-----BEGIN CERTIFICATE-----\nInitial-TLS-Certificate\n-----END CERTIFICATE-----",
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create source secret")

			By("waiting for destination resources to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Destination secret should exist")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("updating source secret with new certificate data")
			cmd = exec.Command("kubectl", "patch", "secret", sourceSecretName, "-n", testNamespace, "--type=merge", "-p",
				`{"data":{"ca.crt":"LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tClVwZGF0ZWQtQ0EtQ2VydGlmaWNhdGUKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQ=="}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to update source secret")

			By("verifying destination secret is updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secret", destSecretName, "-n", testNamespace, "-o", "jsonpath={.data.ca\\.crt}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// The base64 encoded value should match the updated certificate
				g.Expect(output).To(Equal("LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tClVwZGF0ZWQtQ0EtQ2VydGlmaWNhdGUKLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQ=="), "Destination secret should be updated")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying destination configmap is updated")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "configmap", destConfigMapName, "-n", testNamespace, "-o", "jsonpath={.data.ca\\.crt}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				// The decoded value should match the updated certificate
				g.Expect(output).To(ContainSubstring("Updated-CA-Certificate"), "Destination configmap should be updated")
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			// Cleanup
			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "secret", sourceSecretName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "secret", destSecretName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "configmap", destConfigMapName, "-n", testNamespace)
			_, _ = utils.Run(cmd)
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
