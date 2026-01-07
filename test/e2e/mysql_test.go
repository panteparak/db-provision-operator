//go:build e2e

/*
Copyright 2026.

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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("mysql", Ordered, func() {
	const (
		instanceName    = "mysql-e2e-instance"
		databaseName    = "testdb"
		userName        = "testuser"
		testNamespace   = "default"
		mysqlHost       = "mysql.mysql.svc.cluster.local"
		secretName      = "mysql-credentials"
		secretNamespace = "mysql"
		timeout         = 2 * time.Minute
		interval        = 2 * time.Second
	)

	ctx := context.Background()

	Context("DatabaseInstance lifecycle", func() {
		It("should create a DatabaseInstance and become Ready", func() {
			By("creating a DatabaseInstance CR")
			instance := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseInstance",
					"metadata": map[string]interface{}{
						"name":      instanceName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"engine": "mysql",
						"connection": map[string]interface{}{
							"host":     mysqlHost,
							"port":     int64(3306),
							"database": "mysql",
							"secretRef": map[string]interface{}{
								"name":      secretName,
								"namespace": secretNamespace,
							},
						},
						"healthCheck": map[string]interface{}{
							"enabled":         true,
							"intervalSeconds": int64(30),
						},
					},
				},
			}

			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Create(ctx, instance, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseInstance")

			By("waiting for DatabaseInstance to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "DatabaseInstance should become Ready")

			By("verifying the Connected condition is True")
			Eventually(func() bool {
				obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
				for _, c := range conditions {
					condition := c.(map[string]interface{})
					if condition["type"] == "Connected" && condition["status"] == "True" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Connected condition should be True")
		})
	})

	Context("Database lifecycle", func() {
		It("should create a Database and become Ready", func() {
			By("creating a Database CR")
			database := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "Database",
					"metadata": map[string]interface{}{
						"name":      databaseName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"instanceRef": map[string]interface{}{
							"name": instanceName,
						},
						"name": databaseName,
					},
				},
			}

			_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, database, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create Database")

			By("waiting for Database to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, databaseName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "Database should become Ready")
		})
	})

	Context("DatabaseUser lifecycle", func() {
		It("should create a DatabaseUser with generated secret", func() {
			By("creating a DatabaseUser CR")
			user := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseUser",
					"metadata": map[string]interface{}{
						"name":      userName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"instanceRef": map[string]interface{}{
							"name": instanceName,
						},
						"name": userName,
						"secretTemplate": map[string]interface{}{
							"name": fmt.Sprintf("%s-credentials", userName),
						},
					},
				},
			}

			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseUser")

			By("waiting for DatabaseUser to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Get(ctx, userName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "DatabaseUser should become Ready")

			By("verifying the credentials Secret was created")
			Eventually(func() bool {
				_, err := k8sClient.CoreV1().Secrets(testNamespace).Get(ctx, fmt.Sprintf("%s-credentials", userName), metav1.GetOptions{})
				return err == nil
			}, timeout, interval).Should(BeTrue(), "Credentials secret should be created")
		})
	})

	Context("Cross-namespace Secret reference", func() {
		It("should support cross-namespace Secret reference (ClusterRoleBinding test)", func() {
			// The DatabaseInstance already uses cross-namespace reference:
			// - DatabaseInstance is in 'default' namespace
			// - Secret is in 'mysql' namespace
			// This test verifies that the cross-namespace RBAC works correctly

			By("verifying DatabaseInstance can access Secret from different namespace")
			obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to get DatabaseInstance")

			phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			Expect(phase).To(Equal("Ready"), "DatabaseInstance should be Ready (cross-namespace RBAC works)")

			// Verify the secret reference points to a different namespace
			secretRef, _, _ := unstructured.NestedMap(obj.Object, "spec", "connection", "secretRef")
			Expect(secretRef["namespace"]).To(Equal(secretNamespace), "Secret should be in mysql namespace")
			Expect(testNamespace).NotTo(Equal(secretNamespace), "Instance and Secret should be in different namespaces")
		})
	})

	// Cleanup after all tests
	AfterAll(func() {
		By("cleaning up test resources")

		// Delete in reverse order of creation
		By("deleting DatabaseUser")
		_ = dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Delete(ctx, userName, metav1.DeleteOptions{})

		By("deleting Database")
		_ = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, databaseName, metav1.DeleteOptions{})

		By("deleting DatabaseInstance")
		_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Delete(ctx, instanceName, metav1.DeleteOptions{})

		// Wait for resources to be deleted
		By("waiting for resources to be deleted")
		Eventually(func() bool {
			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
			return err != nil // Should return error (not found) when deleted
		}, timeout, interval).Should(BeTrue(), "DatabaseInstance should be deleted")
	})
})
