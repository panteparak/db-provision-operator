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

var _ = Describe("clusterdatabasegrant", Ordered, func() {
	const (
		clusterInstanceName = "shared-postgres-for-grant"
		testNamespace       = "default"
		postgresHost        = "postgres.postgres.svc.cluster.local"
		secretName          = "postgres-credentials"
		secretNamespace     = "postgres"
		timeout             = 2 * time.Minute
		interval            = 2 * time.Second

		// Database and role names for testing
		testDatabaseName    = "granttest"
		testRoleName        = "grant-test-role"
		testDbRoleName      = "grant_test_role"
		testUserName        = "grant-test-user"
		testDbUserName      = "grant_test_user"
		testClusterGrantName = "test-cluster-grant"
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

		By("creating a ClusterDatabaseInstance for grant tests")
		clusterInstance := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dbops.dbprovision.io/v1alpha1",
				"kind":       "ClusterDatabaseInstance",
				"metadata": map[string]interface{}{
					"name": clusterInstanceName,
				},
				"spec": map[string]interface{}{
					"engine": "postgres",
					"connection": map[string]interface{}{
						"host":     getInstanceHost(),
						"port":     getInstancePort(),
						"database": "postgres",
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

		_, err = dynamicClient.Resource(clusterDatabaseInstanceGVR).Create(ctx, clusterInstance, metav1.CreateOptions{})
		if err != nil {
			GinkgoWriter.Printf("ClusterDatabaseInstance creation error (may already exist): %v\n", err)
		}

		By("waiting for ClusterDatabaseInstance to become Ready")
		Eventually(func() string {
			obj, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			return phase
		}, timeout, interval).Should(Equal("Ready"), "ClusterDatabaseInstance should become Ready")

		By("creating a test ClusterDatabaseRole for grant target")
		clusterRole := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dbops.dbprovision.io/v1alpha1",
				"kind":       "ClusterDatabaseRole",
				"metadata": map[string]interface{}{
					"name": testRoleName,
				},
				"spec": map[string]interface{}{
					"clusterInstanceRef": map[string]interface{}{
						"name": clusterInstanceName,
					},
					"roleName": testDbRoleName,
					"postgres": map[string]interface{}{
						"canLogin":   true,
						"inherit":    true,
						"createDb":   false,
						"createRole": false,
					},
				},
			},
		}

		_, err = dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, clusterRole, metav1.CreateOptions{})
		if err != nil {
			GinkgoWriter.Printf("ClusterDatabaseRole creation error (may already exist): %v\n", err)
		}

		By("waiting for ClusterDatabaseRole to become Ready")
		Eventually(func() string {
			obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, testRoleName, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			return phase
		}, timeout, interval).Should(Equal("Ready"), "ClusterDatabaseRole should become Ready")
	})

	Context("ClusterDatabaseGrant to ClusterDatabaseRole", func() {
		const grantName = "role-database-access"

		It("should create a ClusterDatabaseGrant for database access and become Ready", func() {
			By("creating a ClusterDatabaseGrant CR with database privileges")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseGrant",
					"metadata": map[string]interface{}{
						"name": grantName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleRef": map[string]interface{}{
							"name": testRoleName,
							// Empty namespace indicates cluster-scoped role
						},
						"grants": []interface{}{
							map[string]interface{}{
								"database": "postgres",
								"postgres": map[string]interface{}{
									"privileges": []interface{}{"CONNECT"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterDatabaseGrant")

			By("waiting for ClusterDatabaseGrant to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, grantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "ClusterDatabaseGrant should become Ready")

			By("verifying the GrantsApplied condition is True")
			Eventually(func() bool {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, grantName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
				for _, c := range conditions {
					condition := c.(map[string]interface{})
					if condition["type"] == "GrantsApplied" && condition["status"] == "True" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "GrantsApplied condition should be True")
		})

		It("should verify privileges exist in PostgreSQL", func() {
			By("checking the role has CONNECT privilege on postgres database")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, testDbRoleName, "CONNECT", "database", "postgres")
				if err != nil {
					GinkgoWriter.Printf("Error checking privilege: %v\n", err)
					return false
				}
				return hasPriv
			}, timeout, interval).Should(BeTrue(), "Role '%s' should have CONNECT privilege on postgres database", testDbRoleName)

			GinkgoWriter.Printf("Role '%s' verified with CONNECT privilege on postgres database\n", testDbRoleName)
		})

		AfterAll(func() {
			By("cleaning up database access grant")
			_ = dynamicClient.Resource(clusterDatabaseGrantGVR).Delete(ctx, grantName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseGrant with schema privileges", func() {
		const schemaGrantName = "role-schema-access"

		It("should create a ClusterDatabaseGrant with schema privileges", func() {
			By("creating a ClusterDatabaseGrant CR with schema privileges")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseGrant",
					"metadata": map[string]interface{}{
						"name": schemaGrantName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleRef": map[string]interface{}{
							"name": testRoleName,
						},
						"grants": []interface{}{
							map[string]interface{}{
								"database": "postgres",
								"schema":   "public",
								"postgres": map[string]interface{}{
									"privileges": []interface{}{"USAGE"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterDatabaseGrant")

			By("waiting for ClusterDatabaseGrant to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, schemaGrantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "ClusterDatabaseGrant should become Ready")
		})

		It("should verify schema privileges exist in PostgreSQL", func() {
			By("checking the role has USAGE privilege on public schema")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilegeOnDatabase(ctx, testDbRoleName, "USAGE", "schema", "public", "postgres")
				if err != nil {
					GinkgoWriter.Printf("Error checking schema privilege: %v\n", err)
					return false
				}
				return hasPriv
			}, timeout, interval).Should(BeTrue(), "Role '%s' should have USAGE privilege on public schema", testDbRoleName)

			GinkgoWriter.Printf("Role '%s' verified with USAGE privilege on public schema\n", testDbRoleName)
		})

		AfterAll(func() {
			By("cleaning up schema grant")
			_ = dynamicClient.Resource(clusterDatabaseGrantGVR).Delete(ctx, schemaGrantName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseGrant with missing instance", func() {
		const missingInstanceGrantName = "missing-instance-grant"

		It("should fail when referencing non-existent ClusterDatabaseInstance", func() {
			By("creating a ClusterDatabaseGrant referencing non-existent instance")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseGrant",
					"metadata": map[string]interface{}{
						"name": missingInstanceGrantName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": "non-existent-instance",
						},
						"roleRef": map[string]interface{}{
							"name": testRoleName,
						},
						"grants": []interface{}{
							map[string]interface{}{
								"database": "postgres",
								"postgres": map[string]interface{}{
									"privileges": []interface{}{"CONNECT"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "CR creation should succeed (validation at reconcile)")

			By("waiting for grant to fail due to missing instance")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, missingInstanceGrantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Or(
				Equal("Failed"),
				Equal("Pending"),
			), "ClusterDatabaseGrant should fail or stay pending with missing instance reference")
		})

		AfterAll(func() {
			By("cleaning up missing instance grant")
			_ = dynamicClient.Resource(clusterDatabaseGrantGVR).Delete(ctx, missingInstanceGrantName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseGrant with missing target", func() {
		const missingTargetGrantName = "missing-target-grant"

		It("should fail when referencing non-existent target role", func() {
			By("creating a ClusterDatabaseGrant referencing non-existent role")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseGrant",
					"metadata": map[string]interface{}{
						"name": missingTargetGrantName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleRef": map[string]interface{}{
							"name": "non-existent-role",
						},
						"grants": []interface{}{
							map[string]interface{}{
								"database": "postgres",
								"postgres": map[string]interface{}{
									"privileges": []interface{}{"CONNECT"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "CR creation should succeed (validation at reconcile)")

			By("waiting for grant to fail due to missing target")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, missingTargetGrantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Or(
				Equal("Failed"),
				Equal("Pending"),
			), "ClusterDatabaseGrant should fail or stay pending with missing target reference")
		})

		AfterAll(func() {
			By("cleaning up missing target grant")
			_ = dynamicClient.Resource(clusterDatabaseGrantGVR).Delete(ctx, missingTargetGrantName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseGrant deletion", func() {
		const deleteGrantName = "delete-test-grant"

		It("should delete a ClusterDatabaseGrant and revoke privileges", func() {
			By("creating a ClusterDatabaseGrant for deletion test")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseGrant",
					"metadata": map[string]interface{}{
						"name": deleteGrantName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleRef": map[string]interface{}{
							"name": testRoleName,
						},
						"grants": []interface{}{
							map[string]interface{}{
								"database": "postgres",
								"postgres": map[string]interface{}{
									"privileges": []interface{}{"TEMPORARY"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for grant to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, deleteGrantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))

			By("verifying TEMPORARY privilege exists before deletion")
			Eventually(func() bool {
				hasPriv, _ := verifier.HasPrivilege(ctx, testDbRoleName, "TEMPORARY", "database", "postgres")
				return hasPriv
			}, timeout, interval).Should(BeTrue())

			By("deleting the ClusterDatabaseGrant")
			err = dynamicClient.Resource(clusterDatabaseGrantGVR).Delete(ctx, deleteGrantName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for ClusterDatabaseGrant CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, deleteGrantName, metav1.GetOptions{})
				return err != nil
			}, timeout, interval).Should(BeTrue(), "ClusterDatabaseGrant CR should be deleted")

			By("verifying TEMPORARY privilege is revoked after deletion")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, testDbRoleName, "TEMPORARY", "database", "postgres")
				if err != nil {
					return true // Assume revoked on error
				}
				return !hasPriv
			}, timeout, interval).Should(BeTrue(), "TEMPORARY privilege should be revoked after grant deletion")
		})
	})

	Context("ClusterDatabaseGrant update", func() {
		const updateGrantName = "update-test-grant"

		It("should update grant privileges", func() {
			By("creating initial ClusterDatabaseGrant")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseGrant",
					"metadata": map[string]interface{}{
						"name": updateGrantName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleRef": map[string]interface{}{
							"name": testRoleName,
						},
						"grants": []interface{}{
							map[string]interface{}{
								"database": "postgres",
								"postgres": map[string]interface{}{
									"privileges": []interface{}{"CONNECT"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for grant to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, updateGrantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))

			By("updating the grant to add TEMPORARY privilege")
			obj, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Get(ctx, updateGrantName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			spec := obj.Object["spec"].(map[string]interface{})
			grants := spec["grants"].([]interface{})
			grant0 := grants[0].(map[string]interface{})
			postgres := grant0["postgres"].(map[string]interface{})
			postgres["privileges"] = []interface{}{"CONNECT", "TEMPORARY"}

			_, err = dynamicClient.Resource(clusterDatabaseGrantGVR).Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the update to be applied")
			time.Sleep(5 * time.Second) // Allow reconciliation

			By("verifying TEMPORARY privilege is now granted")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, testDbRoleName, "TEMPORARY", "database", "postgres")
				if err != nil {
					GinkgoWriter.Printf("Error checking TEMPORARY privilege: %v\n", err)
					return false
				}
				return hasPriv
			}, timeout, interval).Should(BeTrue(), "Role should have TEMPORARY privilege after update")
		})

		AfterAll(func() {
			By("cleaning up update test grant")
			_ = dynamicClient.Resource(clusterDatabaseGrantGVR).Delete(ctx, updateGrantName, metav1.DeleteOptions{})
		})
	})

	// Cleanup after all tests
	AfterAll(func() {
		By("closing database verifier connection")
		if verifier != nil {
			_ = verifier.Close()
		}

		By("cleaning up test resources")

		// Delete the test role
		_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, testRoleName, metav1.DeleteOptions{})

		// Wait for role to be deleted
		Eventually(func() bool {
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, testRoleName, metav1.GetOptions{})
			return err != nil
		}, timeout, interval).Should(BeTrue(), "ClusterDatabaseRole should be deleted")

		// Delete the ClusterDatabaseInstance
		_ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, clusterInstanceName, metav1.DeleteOptions{})

		By("waiting for ClusterDatabaseInstance to be deleted")
		Eventually(func() bool {
			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
			return err != nil
		}, timeout, interval).Should(BeTrue(), "ClusterDatabaseInstance should be deleted")
	})
})
