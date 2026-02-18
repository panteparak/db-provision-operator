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
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/db-provision-operator/test/e2e/testutil"
)

var _ = Describe("clusterdatabaseinstance", Ordered, func() {
	const (
		clusterInstanceName = "shared-postgres-cluster"
		databaseName        = "cluster-testdb"
		userName            = "cluster-testuser"
		testNamespace       = "default"
		altNamespace        = "alt-namespace" // For cross-namespace tests
		postgresHost        = "host.k3d.internal"
		secretName          = "postgres-credentials"
		secretNamespace     = "postgres"
		timeout             = 2 * time.Minute
		interval            = 2 * time.Second
	)

	ctx := context.Background()

	// Database verifier for validating actual database state
	var verifier *testutil.PostgresVerifier

	// getVerifierHost returns the host for the verifier to connect to.
	getVerifierHost := func() string {
		if host := os.Getenv("E2E_DATABASE_HOST"); host != "" {
			return host
		}
		return postgresHost
	}

	// getVerifierPort returns the port for the verifier to connect to.
	getVerifierPort := func() int32 {
		if portStr := os.Getenv("E2E_DATABASE_PORT"); portStr != "" {
			if port, err := strconv.ParseInt(portStr, 10, 32); err == nil {
				return int32(port)
			}
		}
		return 5432
	}

	// getInstanceHost returns the host for the ClusterDatabaseInstance CR.
	getInstanceHost := func() string {
		if host := os.Getenv("E2E_INSTANCE_HOST"); host != "" {
			return host
		}
		return postgresHost
	}

	// getInstancePort returns the port for the ClusterDatabaseInstance CR.
	getInstancePort := func() int64 {
		if portStr := os.Getenv("E2E_INSTANCE_PORT"); portStr != "" {
			if port, err := strconv.ParseInt(portStr, 10, 64); err == nil {
				return port
			}
		}
		return 5432
	}

	// Admin credentials read from secret
	var adminUsername, adminPassword string

	BeforeAll(func() {
		By("setting up PostgreSQL verifier")
		var err error
		adminUsername, err = getSecretValue(ctx, secretNamespace, secretName, "username")
		Expect(err).NotTo(HaveOccurred(), "Failed to get PostgreSQL username from secret")

		adminPassword, err = getSecretValue(ctx, secretNamespace, secretName, "password")
		Expect(err).NotTo(HaveOccurred(), "Failed to get PostgreSQL password from secret")

		verifierHost := getVerifierHost()
		verifierPort := getVerifierPort()
		GinkgoWriter.Printf("Using verifier host: %s:%d with admin user: %s\n", verifierHost, verifierPort, adminUsername)

		cfg := testutil.PostgresEngineConfig(verifierHost, verifierPort, adminUsername, adminPassword)
		verifier = testutil.NewPostgresVerifier(cfg)

		// Connect with retry since database may still be starting
		Eventually(func() error {
			return verifier.Connect(ctx)
		}, timeout, interval).Should(Succeed(), "Should connect to PostgreSQL for verification")

		By("ensuring alt-namespace exists for cross-namespace tests")
		altNs := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": altNamespace,
				},
			},
		}
		nsGVR := databaseInstanceGVR
		nsGVR.Group = ""
		nsGVR.Version = "v1"
		nsGVR.Resource = "namespaces"
		_, _ = dynamicClient.Resource(nsGVR).Create(ctx, altNs, metav1.CreateOptions{})
		// Ignore error if namespace already exists
	})

	Context("ClusterDatabaseInstance lifecycle", func() {
		It("should create a ClusterDatabaseInstance and become Ready", func() {
			By("creating a ClusterDatabaseInstance CR")
			// Note: ClusterDatabaseInstance is cluster-scoped, so no namespace
			clusterInstance := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseInstance",
					"metadata": map[string]interface{}{
						"name": clusterInstanceName,
						// No namespace - cluster-scoped resource
					},
					"spec": map[string]interface{}{
						"engine": "postgres",
						"connection": map[string]interface{}{
							"host":     getInstanceHost(),
							"port":     getInstancePort(),
							"database": "postgres",
							"secretRef": map[string]interface{}{
								"name":      secretName,
								"namespace": secretNamespace, // Required for cluster-scoped resources
							},
						},
						"healthCheck": map[string]interface{}{
							"enabled":         true,
							"intervalSeconds": int64(30),
						},
					},
				},
			}

			// Create cluster-scoped resource (no namespace)
			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Create(ctx, clusterInstance, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterDatabaseInstance")

			By("waiting for ClusterDatabaseInstance to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "ClusterDatabaseInstance should become Ready")

			By("verifying the Connected condition is True")
			Eventually(func() bool {
				obj, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
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

		It("should report database version", func() {
			By("verifying the version is reported in status")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				version, _, _ := unstructured.NestedString(obj.Object, "status", "version")
				return version
			}, timeout, interval).ShouldNot(BeEmpty(), "Version should be reported")
		})
	})

	Context("Database with clusterInstanceRef", func() {
		It("should create a Database referencing ClusterDatabaseInstance", func() {
			By("creating a Database CR with clusterInstanceRef")
			database := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "Database",
					"metadata": map[string]interface{}{
						"name":      databaseName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						// Use clusterInstanceRef instead of instanceRef
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
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

			By("verifying database actually exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, databaseName)
				if err != nil {
					GinkgoWriter.Printf("Error checking database existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "Database '%s' should exist in PostgreSQL", databaseName)
		})
	})

	Context("DatabaseUser with clusterInstanceRef", func() {
		It("should create a DatabaseUser referencing ClusterDatabaseInstance", func() {
			By("creating a DatabaseUser CR with clusterInstanceRef")
			user := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseUser",
					"metadata": map[string]interface{}{
						"name":      userName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						// Use clusterInstanceRef instead of instanceRef
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"username": userName,
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

			By("verifying user actually exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.UserExists(ctx, userName)
				if err != nil {
					GinkgoWriter.Printf("Error checking user existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "User '%s' should exist in PostgreSQL", userName)
		})
	})

	Context("Cross-namespace access", func() {
		const (
			crossNsDbName   = "cross-ns-db"
			crossNsUserName = "cross-ns-user"
		)

		It("should allow resources in different namespaces to use the same ClusterDatabaseInstance", func() {
			By("creating a Database in alt-namespace referencing the ClusterDatabaseInstance")
			database := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "Database",
					"metadata": map[string]interface{}{
						"name":      crossNsDbName,
						"namespace": altNamespace, // Different namespace
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"name": crossNsDbName,
					},
				},
			}

			_, err := dynamicClient.Resource(databaseGVR).Namespace(altNamespace).Create(ctx, database, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create Database in alt-namespace")

			By("waiting for Database in alt-namespace to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGVR).Namespace(altNamespace).Get(ctx, crossNsDbName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "Database should become Ready")

			By("verifying the cross-namespace database exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, crossNsDbName)
				if err != nil {
					GinkgoWriter.Printf("Error checking database existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "Cross-namespace Database '%s' should exist in PostgreSQL", crossNsDbName)

			By("creating a DatabaseUser in alt-namespace")
			user := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseUser",
					"metadata": map[string]interface{}{
						"name":      crossNsUserName,
						"namespace": altNamespace,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"username": crossNsUserName,
					},
				},
			}

			_, err = dynamicClient.Resource(databaseUserGVR).Namespace(altNamespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseUser in alt-namespace")

			By("waiting for DatabaseUser in alt-namespace to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseUserGVR).Namespace(altNamespace).Get(ctx, crossNsUserName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "DatabaseUser should become Ready")

			By("cleaning up alt-namespace resources")
			_ = dynamicClient.Resource(databaseUserGVR).Namespace(altNamespace).Delete(ctx, crossNsUserName, metav1.DeleteOptions{})
			_ = dynamicClient.Resource(databaseGVR).Namespace(altNamespace).Delete(ctx, crossNsDbName, metav1.DeleteOptions{})
		})
	})

	Context("Deletion protection", func() {
		const protectedInstanceName = "protected-cluster-instance"

		It("should block deletion when deletion protection is enabled", func() {
			By("creating a ClusterDatabaseInstance with deletion protection")
			protectedInstance := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseInstance",
					"metadata": map[string]interface{}{
						"name": protectedInstanceName,
					},
					"spec": map[string]interface{}{
						"engine":             "postgres",
						"deletionProtection": true,
						"connection": map[string]interface{}{
							"host":     getInstanceHost(),
							"port":     getInstancePort(),
							"database": "postgres",
							"secretRef": map[string]interface{}{
								"name":      secretName,
								"namespace": secretNamespace,
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Create(ctx, protectedInstance, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create protected ClusterDatabaseInstance")

			By("waiting for ClusterDatabaseInstance to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, protectedInstanceName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))

			By("attempting to delete the protected instance")
			err = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, protectedInstanceName, metav1.DeleteOptions{})
			// The delete call may succeed (it marks for deletion), but finalizer blocks actual deletion

			By("verifying the instance still exists (blocked by finalizer)")
			Consistently(func() bool {
				_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, protectedInstanceName, metav1.GetOptions{})
				return err == nil
			}, 10*time.Second, 1*time.Second).Should(BeTrue(), "Protected instance should not be deleted")

			By("cleaning up - adding force-delete annotation")
			obj, _ := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, protectedInstanceName, metav1.GetOptions{})
			if obj != nil {
				metadata := obj.Object["metadata"].(map[string]interface{})
				annotations, ok := metadata["annotations"].(map[string]interface{})
				if !ok {
					annotations = make(map[string]interface{})
					metadata["annotations"] = annotations
				}
				annotations["dbops.dbprovision.io/force-delete"] = "true"
				_, _ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Update(ctx, obj, metav1.UpdateOptions{})
			}
			_ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, protectedInstanceName, metav1.DeleteOptions{})
		})
	})

	Context("User credential validation", func() {
		It("should generate working credentials for user", func() {
			By("getting user credentials from Secret")
			secretName := userName + "-credentials"
			password, err := getSecretValue(ctx, testNamespace, secretName, "password")
			if err != nil {
				// Try alternate secret naming
				password, err = getSecretValue(ctx, testNamespace, userName, "password")
			}
			Expect(err).NotTo(HaveOccurred(), "Failed to get user password from Secret")
			Expect(password).NotTo(BeEmpty(), "Password should not be empty")

			By("connecting to database using credentials from Secret")
			userConn, err := verifier.ConnectAsUser(ctx, userName, password, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Should be able to connect as user '%s'", userName)
			defer userConn.Close()

			By("verifying connection is alive with ping")
			err = userConn.Ping(ctx)
			Expect(err).NotTo(HaveOccurred(), "Ping should succeed")

			By("running a simple query to verify database access")
			err = userConn.Query(ctx, "SELECT 1")
			Expect(err).NotTo(HaveOccurred(), "Should be able to run simple query")

			GinkgoWriter.Printf("User '%s' successfully connected and queried database '%s' via ClusterDatabaseInstance\n", userName, databaseName)
		})
	})

	// Cleanup after all tests
	AfterAll(func() {
		By("closing database verifier connection")
		if verifier != nil {
			_ = verifier.Close()
		}

		By("cleaning up test resources")

		// Delete in reverse order of creation
		By("deleting DatabaseUser")
		_ = dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Delete(ctx, userName, metav1.DeleteOptions{})

		By("deleting Database")
		_ = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, databaseName, metav1.DeleteOptions{})

		By("deleting ClusterDatabaseInstance")
		_ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, clusterInstanceName, metav1.DeleteOptions{})

		// Wait for resources to be deleted
		By("waiting for ClusterDatabaseInstance to be deleted")
		Eventually(func() bool {
			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
			return err != nil // Should return error (not found) when deleted
		}, timeout, interval).Should(BeTrue(), "ClusterDatabaseInstance should be deleted")
	})
})
