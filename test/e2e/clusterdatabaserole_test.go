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

var _ = Describe("clusterdatabaserole", Ordered, func() {
	const (
		clusterInstanceName = "shared-postgres-for-role"
		clusterRoleName     = "test-cluster-role"
		dbRoleName          = "test_cluster_role" // PostgreSQL role name (underscore format)
		testNamespace       = "default"
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

		By("creating a ClusterDatabaseInstance for role tests")
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
	})

	Context("ClusterDatabaseRole lifecycle", func() {
		It("should create a ClusterDatabaseRole and become Ready", func() {
			By("creating a ClusterDatabaseRole CR")
			clusterRole := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseRole",
					"metadata": map[string]interface{}{
						"name": clusterRoleName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleName": dbRoleName,
						"postgres": map[string]interface{}{
							"canLogin":   false,
							"inherit":    true,
							"createDb":   false,
							"createRole": false,
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, clusterRole, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterDatabaseRole")

			By("waiting for ClusterDatabaseRole to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, clusterRoleName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "ClusterDatabaseRole should become Ready")

			By("verifying the RoleReady condition is True")
			Eventually(func() bool {
				obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, clusterRoleName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
				for _, c := range conditions {
					condition := c.(map[string]interface{})
					if condition["type"] == "RoleReady" && condition["status"] == "True" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "RoleReady condition should be True")
		})

		It("should verify role actually exists in PostgreSQL", func() {
			By("checking the role exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.RoleExists(ctx, dbRoleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking role existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "Role '%s' should exist in PostgreSQL", dbRoleName)

			By("verifying role attributes in PostgreSQL")
			canLogin, err := verifier.RoleCanLogin(ctx, dbRoleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(canLogin).To(BeFalse(), "Role should not have LOGIN privilege")

			GinkgoWriter.Printf("Role '%s' verified in PostgreSQL with correct attributes\n", dbRoleName)
		})
	})

	Context("ClusterDatabaseRole update", func() {
		const updatedRoleName = "updated-cluster-role"
		const updatedDbRoleName = "updated_cluster_role"

		It("should create a role for update testing", func() {
			By("creating a ClusterDatabaseRole for update tests")
			clusterRole := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseRole",
					"metadata": map[string]interface{}{
						"name": updatedRoleName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleName": updatedDbRoleName,
						"postgres": map[string]interface{}{
							"canLogin":   false,
							"inherit":    true,
							"createDb":   false,
							"createRole": false,
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, clusterRole, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for role to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, updatedRoleName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))
		})

		It("should update role privileges", func() {
			By("getting the current ClusterDatabaseRole")
			obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, updatedRoleName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("updating the role to allow CREATEDB")
			spec := obj.Object["spec"].(map[string]interface{})
			postgres := spec["postgres"].(map[string]interface{})
			postgres["createDb"] = true

			_, err = dynamicClient.Resource(clusterDatabaseRoleGVR).Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the update to be applied")
			time.Sleep(5 * time.Second) // Allow reconciliation

			By("verifying the role has CREATEDB in PostgreSQL")
			Eventually(func() bool {
				canCreateDb, err := verifier.RoleHasCreateDb(ctx, updatedDbRoleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking CREATEDB: %v\n", err)
					return false
				}
				return canCreateDb
			}, timeout, interval).Should(BeTrue(), "Role should have CREATEDB privilege after update")
		})

		AfterAll(func() {
			By("cleaning up update test role")
			_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, updatedRoleName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseRole with login capability", func() {
		const loginRoleName = "login-cluster-role"
		const loginDbRoleName = "login_cluster_role"

		It("should create a role with login capability", func() {
			By("creating a ClusterDatabaseRole with canLogin=true")
			loginRole := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseRole",
					"metadata": map[string]interface{}{
						"name": loginRoleName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleName": loginDbRoleName,
						"postgres": map[string]interface{}{
							"canLogin":   true,
							"inherit":    true,
							"createDb":   false,
							"createRole": false,
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, loginRole, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for role to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, loginRoleName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))

			By("verifying role can login in PostgreSQL")
			Eventually(func() bool {
				canLogin, err := verifier.RoleCanLogin(ctx, loginDbRoleName)
				if err != nil {
					return false
				}
				return canLogin
			}, timeout, interval).Should(BeTrue(), "Role should have LOGIN privilege")
		})

		AfterAll(func() {
			By("cleaning up login role")
			_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, loginRoleName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseRole with missing instance", func() {
		const missingInstanceRoleName = "missing-instance-role"

		It("should fail when referencing non-existent ClusterDatabaseInstance", func() {
			By("creating a ClusterDatabaseRole referencing non-existent instance")
			role := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseRole",
					"metadata": map[string]interface{}{
						"name": missingInstanceRoleName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": "non-existent-instance",
						},
						"roleName": "missing_instance_role",
						"postgres": map[string]interface{}{
							"canLogin": false,
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "CR creation should succeed (validation at reconcile)")

			By("waiting for role to fail due to missing instance")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, missingInstanceRoleName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Or(
				Equal("Failed"),
				Equal("Pending"),
			), "ClusterDatabaseRole should fail or stay pending with missing instance reference")
		})

		AfterAll(func() {
			By("cleaning up missing instance role")
			_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, missingInstanceRoleName, metav1.DeleteOptions{})
		})
	})

	Context("ClusterDatabaseRole deletion", func() {
		const deleteRoleName = "delete-cluster-role"
		const deleteDbRoleName = "delete_cluster_role"

		It("should delete a ClusterDatabaseRole and remove it from PostgreSQL", func() {
			By("creating a ClusterDatabaseRole for deletion test")
			role := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "ClusterDatabaseRole",
					"metadata": map[string]interface{}{
						"name": deleteRoleName,
					},
					"spec": map[string]interface{}{
						"clusterInstanceRef": map[string]interface{}{
							"name": clusterInstanceName,
						},
						"roleName": deleteDbRoleName,
						"postgres": map[string]interface{}{
							"canLogin": false,
						},
					},
				},
			}

			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for role to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, deleteRoleName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))

			By("verifying role exists in PostgreSQL before deletion")
			Eventually(func() bool {
				exists, _ := verifier.RoleExists(ctx, deleteDbRoleName)
				return exists
			}, timeout, interval).Should(BeTrue())

			By("deleting the ClusterDatabaseRole")
			err = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, deleteRoleName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for ClusterDatabaseRole CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, deleteRoleName, metav1.GetOptions{})
				return err != nil
			}, timeout, interval).Should(BeTrue(), "ClusterDatabaseRole CR should be deleted")

			By("verifying role is removed from PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.RoleExists(ctx, deleteDbRoleName)
				if err != nil {
					return true // Assume not exists on error
				}
				return !exists
			}, timeout, interval).Should(BeTrue(), "Role should be removed from PostgreSQL after deletion")
		})
	})

	// Cleanup after all tests
	AfterAll(func() {
		By("closing database verifier connection")
		if verifier != nil {
			_ = verifier.Close()
		}

		By("cleaning up test resources")

		// Delete the main test role
		_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, clusterRoleName, metav1.DeleteOptions{})

		// Delete the ClusterDatabaseInstance
		_ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, clusterInstanceName, metav1.DeleteOptions{})

		By("waiting for ClusterDatabaseInstance to be deleted")
		Eventually(func() bool {
			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
			return err != nil
		}, timeout, interval).Should(BeTrue(), "ClusterDatabaseInstance should be deleted")
	})
})
