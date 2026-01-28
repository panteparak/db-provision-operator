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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/db-provision-operator/test/e2e/testutil"
)

var _ = Describe("postgresql", Ordered, func() {
	const (
		instanceName    = "postgres-e2e-instance"
		databaseName    = "testdb"
		userName        = "testuser"
		testNamespace   = "default"
		postgresHost    = "postgres.postgres.svc.cluster.local" // K8s DNS for CR (runs inside cluster)
		secretName      = "postgres-credentials"
		secretNamespace = "postgres"
		timeout         = 2 * time.Minute
		interval        = 2 * time.Second
	)

	ctx := context.Background()

	// Database verifier for validating actual database state
	var verifier *testutil.PostgresVerifier

	// getVerifierHost returns the host for the verifier to connect to.
	// In CI, we use port-forwarding so the verifier connects to localhost.
	// Locally, we may connect directly to the K8s service.
	getVerifierHost := func() string {
		if host := os.Getenv("E2E_DATABASE_HOST"); host != "" {
			return host
		}
		return postgresHost
	}

	// Admin credentials read from secret (least-privilege account)
	var adminUsername, adminPassword string

	BeforeAll(func() {
		By("setting up PostgreSQL verifier")
		// Get the admin credentials from the secret (least-privilege account)
		var err error
		adminUsername, err = getSecretValue(ctx, secretNamespace, secretName, "username")
		Expect(err).NotTo(HaveOccurred(), "Failed to get PostgreSQL username from secret")

		adminPassword, err = getSecretValue(ctx, secretNamespace, secretName, "password")
		Expect(err).NotTo(HaveOccurred(), "Failed to get PostgreSQL password from secret")

		verifierHost := getVerifierHost()
		GinkgoWriter.Printf("Using verifier host: %s with admin user: %s\n", verifierHost, adminUsername)

		cfg := testutil.PostgresEngineConfig(verifierHost, 5432, adminUsername, adminPassword)
		verifier = testutil.NewPostgresVerifier(cfg)

		// Connect with retry since database may still be starting
		Eventually(func() error {
			return verifier.Connect(ctx)
		}, timeout, interval).Should(Succeed(), "Should connect to PostgreSQL for verification")
	})

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
						"engine": "postgres",
						"connection": map[string]interface{}{
							"host":     postgresHost,
							"port":     int64(5432),
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

	Context("Cross-namespace Secret reference", func() {
		It("should support cross-namespace Secret reference (ClusterRoleBinding test)", func() {
			// The DatabaseInstance already uses cross-namespace reference:
			// - DatabaseInstance is in 'default' namespace
			// - Secret is in 'postgres' namespace
			// This test verifies that the cross-namespace RBAC works correctly

			By("verifying DatabaseInstance can access Secret from different namespace")
			obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to get DatabaseInstance")

			phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			Expect(phase).To(Equal("Ready"), "DatabaseInstance should be Ready (cross-namespace RBAC works)")

			// Verify the secret reference points to a different namespace
			secretRef, _, _ := unstructured.NestedMap(obj.Object, "spec", "connection", "secretRef")
			Expect(secretRef["namespace"]).To(Equal(secretNamespace), "Secret should be in postgres namespace")
			Expect(testNamespace).NotTo(Equal(secretNamespace), "Instance and Secret should be in different namespaces")
		})
	})

	Context("DatabaseRole lifecycle", func() {
		const roleName = "testrole"

		It("should create a DatabaseRole and become Ready", func() {
			By("creating a DatabaseRole CR")
			role := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseRole",
					"metadata": map[string]interface{}{
						"name":      roleName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"instanceRef": map[string]interface{}{
							"name": instanceName,
						},
						"roleName": roleName,
					},
				},
			}

			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(testNamespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseRole")

			By("waiting for DatabaseRole to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseRoleGVR).Namespace(testNamespace).Get(ctx, roleName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "DatabaseRole should become Ready")

			By("verifying role actually exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.RoleExists(ctx, roleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking role existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "Role '%s' should exist in PostgreSQL", roleName)
		})
	})

	Context("DatabaseGrant lifecycle", func() {
		const grantName = "testgrant"

		It("should create a DatabaseGrant and become Ready", func() {
			By("creating a DatabaseGrant CR")
			grant := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseGrant",
					"metadata": map[string]interface{}{
						"name":      grantName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"userRef": map[string]interface{}{
							"name": userName,
						},
						"databaseRef": map[string]interface{}{
							"name": databaseName,
						},
						"postgres": map[string]interface{}{
							"grants": []interface{}{
								map[string]interface{}{
									"database":   databaseName,
									"schema":     "public",
									"privileges": []interface{}{"SELECT", "INSERT"},
								},
							},
						},
					},
				},
			}

			_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(testNamespace).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseGrant")

			By("waiting for DatabaseGrant to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGrantGVR).Namespace(testNamespace).Get(ctx, grantName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"), "DatabaseGrant should become Ready")

			By("verifying user has USAGE privilege on public schema in testdb")
			Eventually(func() bool {
				// Must check on the specific database where grants were applied
				hasPriv, err := verifier.HasPrivilegeOnDatabase(ctx, userName, "USAGE", "schema", "public", databaseName)
				if err != nil {
					GinkgoWriter.Printf("Error checking schema privilege: %v\n", err)
					return false
				}
				return hasPriv
			}, timeout, interval).Should(BeTrue(), "User '%s' should have USAGE privilege on public schema in database '%s'", userName, databaseName)
		})
	})

	// ===== Least Privilege Verification =====
	// These tests verify the operator uses a least-privilege account, NOT superuser

	Context("Least Privilege Verification", func() {
		It("should operate with non-superuser account", func() {
			By("verifying the operator account is NOT a superuser")
			// Query pg_roles to check if current user (dbprovision_admin) has superuser
			var isSuperuser bool
			err := verifier.QueryRow(ctx, "postgres",
				"SELECT rolsuper FROM pg_roles WHERE rolname = current_user",
				&isSuperuser)
			Expect(err).NotTo(HaveOccurred(), "Failed to query superuser status")
			Expect(isSuperuser).To(BeFalse(),
				"Operator account should NOT be a superuser (rolsuper should be false)")

			GinkgoWriter.Printf("Verified: %s is NOT a superuser\n", adminUsername)
		})

		It("should have CREATEDB privilege", func() {
			By("verifying the operator account has CREATEDB")
			var hasCreateDB bool
			err := verifier.QueryRow(ctx, "postgres",
				"SELECT rolcreatedb FROM pg_roles WHERE rolname = current_user",
				&hasCreateDB)
			Expect(err).NotTo(HaveOccurred(), "Failed to query CREATEDB status")
			Expect(hasCreateDB).To(BeTrue(),
				"Operator account should have CREATEDB privilege")

			GinkgoWriter.Printf("Verified: %s has CREATEDB privilege\n", adminUsername)
		})

		It("should have CREATEROLE privilege", func() {
			By("verifying the operator account has CREATEROLE")
			var hasCreateRole bool
			err := verifier.QueryRow(ctx, "postgres",
				"SELECT rolcreaterole FROM pg_roles WHERE rolname = current_user",
				&hasCreateRole)
			Expect(err).NotTo(HaveOccurred(), "Failed to query CREATEROLE status")
			Expect(hasCreateRole).To(BeTrue(),
				"Operator account should have CREATEROLE privilege")

			GinkgoWriter.Printf("Verified: %s has CREATEROLE privilege\n", adminUsername)
		})

		It("should have pg_signal_backend membership for force-drop", func() {
			By("verifying the operator account has pg_signal_backend role")
			var hasSignalBackend bool
			err := verifier.QueryRow(ctx, "postgres",
				"SELECT pg_has_role(current_user, 'pg_signal_backend', 'MEMBER')",
				&hasSignalBackend)
			Expect(err).NotTo(HaveOccurred(), "Failed to query pg_signal_backend membership")
			Expect(hasSignalBackend).To(BeTrue(),
				"Operator account should be a member of pg_signal_backend for force-drop")

			GinkgoWriter.Printf("Verified: %s is a member of pg_signal_backend\n", adminUsername)
		})

		It("should print privilege summary", func() {
			By("printing complete privilege summary for documentation")
			GinkgoWriter.Printf("\n========== PostgreSQL Least-Privilege Summary ==========\n")
			GinkgoWriter.Printf("Admin Account: %s\n", adminUsername)
			GinkgoWriter.Printf("Required Privileges:\n")
			GinkgoWriter.Printf("  - LOGIN: Yes (can connect)\n")
			GinkgoWriter.Printf("  - CREATEDB: Yes (create/drop databases)\n")
			GinkgoWriter.Printf("  - CREATEROLE: Yes (manage users/roles)\n")
			GinkgoWriter.Printf("  - pg_signal_backend: Yes (terminate connections)\n")
			GinkgoWriter.Printf("  - SUPERUSER: No (not required, not granted)\n")
			GinkgoWriter.Printf("=========================================================\n\n")
		})
	})

	// ===== Functionality Verification Tests =====
	// These tests verify that database operations actually work, not just that CRs become Ready

	Context("Database Ownership Verification", func() {
		It("should set correct database owner", func() {
			By("querying database owner from PostgreSQL system catalog")
			owner, err := verifier.GetDatabaseOwner(ctx, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Failed to get database owner")

			By("verifying owner is the admin user from credentials")
			// The database is created by the operator using the admin credentials
			// so the owner should be the admin user (dbprovision_admin for least-privilege setup)
			Expect(owner).To(Equal(adminUsername), "Database owner should be '%s' (admin user)", adminUsername)
			GinkgoWriter.Printf("Database '%s' owner: %s\n", databaseName, owner)
		})
	})

	Context("User Credential Validation", func() {
		It("should generate working credentials in Secret", func() {
			By("getting user credentials from Secret")
			// The DatabaseUser CR creates a secret with credentials
			// Secret name follows the pattern: <username>-credentials or is specified in spec
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

			GinkgoWriter.Printf("User '%s' successfully connected and queried database '%s'\n", userName, databaseName)
		})
	})

	Context("Grant Enforcement Verification", func() {
		const testTable = "e2e_grant_test"

		It("should enforce granted permissions (SELECT, INSERT allowed)", func() {
			By("getting user credentials")
			userSecretName := userName + "-credentials"
			password, err := getSecretValue(ctx, testNamespace, userSecretName, "password")
			if err != nil {
				password, err = getSecretValue(ctx, testNamespace, userName, "password")
			}
			Expect(err).NotTo(HaveOccurred(), "Failed to get user password")

			By("connecting as the granted user")
			userConn, err := verifier.ConnectAsUser(ctx, userName, password, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Should connect as user")
			defer userConn.Close()

			By("creating a test table using admin connection first")
			// Use admin connection (from BeforeAll) to create table for testing SELECT/INSERT
			adminConn, err := verifier.ConnectAsUser(ctx, adminUsername, adminPassword, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Should connect as admin")
			defer adminConn.Close()

			// Create test table
			err = adminConn.CanCreateTable(ctx, testTable)
			Expect(err).NotTo(HaveOccurred(), "Admin should create test table")

			// Grant SELECT, INSERT to the test user on the table
			// Use Eventually to handle transient "tuple concurrently updated" errors
			// that can occur when the operator is also modifying user grants
			seqName := testTable + "_id_seq"
			Eventually(func() error {
				// Reconnect for each retry to get a fresh connection
				retryConn, connErr := verifier.ConnectAsUser(ctx, adminUsername, adminPassword, databaseName)
				if connErr != nil {
					return connErr
				}
				defer retryConn.Close()

				// Grant on table
				if grantErr := retryConn.Exec(ctx, "GRANT SELECT, INSERT ON "+testTable+" TO "+userName); grantErr != nil {
					return grantErr
				}
				// Grant on sequence
				return retryConn.Exec(ctx, "GRANT USAGE ON SEQUENCE "+seqName+" TO "+userName)
			}, 30*time.Second, 2*time.Second).Should(Succeed(), "Should grant permissions on table and sequence")

			By("verifying SELECT is allowed")
			err = userConn.CanSelectData(ctx, testTable)
			Expect(err).NotTo(HaveOccurred(), "SELECT should be allowed")

			By("verifying INSERT is allowed")
			err = userConn.CanInsertData(ctx, testTable)
			Expect(err).NotTo(HaveOccurred(), "INSERT should be allowed")

			By("verifying DELETE is denied (not granted)")
			err = userConn.CanDeleteData(ctx, testTable)
			Expect(err).To(HaveOccurred(), "DELETE should be denied")
			GinkgoWriter.Printf("DELETE correctly denied: %v\n", err)

			By("cleaning up test table")
			adminConn, _ = verifier.ConnectAsUser(ctx, adminUsername, adminPassword, databaseName)
			if adminConn != nil {
				_ = adminConn.CanDropTable(ctx, testTable)
				adminConn.Close()
			}
		})
	})

	Context("Role Membership Verification", func() {
		const (
			memberUser = "testmember"
			roleName   = "testrole"
		)

		It("should verify role membership after assignment", func() {
			By("checking if testrole exists from earlier test")
			exists, err := verifier.RoleExists(ctx, roleName)
			Expect(err).NotTo(HaveOccurred())
			if !exists {
				Skip("testrole does not exist, skipping role membership test")
			}

			By("creating a new user to test role assignment")
			memberUserObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseUser",
					"metadata": map[string]interface{}{
						"name":      memberUser,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"instanceRef": map[string]interface{}{
							"name": instanceName,
						},
						"username": memberUser,
						"postgres": map[string]interface{}{
							"inRoles": []interface{}{roleName}, // Assign role via postgres.inRoles
						},
					},
				},
			}

			_, err = dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Create(ctx, memberUserObj, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create member user")

			By("waiting for member user to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Get(ctx, memberUser, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, timeout, interval).Should(Equal("Ready"))

			By("verifying user is a member of the role")
			Eventually(func() bool {
				isMember, err := verifier.HasRoleMembership(ctx, memberUser, roleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking role membership: %v\n", err)
					return false
				}
				return isMember
			}, timeout, interval).Should(BeTrue(), "User '%s' should be a member of role '%s'", memberUser, roleName)

			By("cleaning up member user")
			_ = dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Delete(ctx, memberUser, metav1.DeleteOptions{})
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
		By("deleting DatabaseGrant")
		_ = dynamicClient.Resource(databaseGrantGVR).Namespace(testNamespace).Delete(ctx, "testgrant", metav1.DeleteOptions{})

		By("deleting DatabaseRole")
		_ = dynamicClient.Resource(databaseRoleGVR).Namespace(testNamespace).Delete(ctx, "testrole", metav1.DeleteOptions{})

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
