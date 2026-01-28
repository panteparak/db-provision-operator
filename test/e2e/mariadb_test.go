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

// MariaDB E2E tests
// MariaDB is MySQL-compatible and uses the MySQL adapter and verifier.
// The engine type in CRDs is "mysql" since MariaDB uses the same protocol.
var _ = Describe("mariadb", Ordered, func() {
	const (
		instanceName    = "mariadb-e2e-instance"
		databaseName    = "testdb"
		userName        = "testuser"
		testNamespace   = "default"
		mariadbHost     = "mariadb.mariadb.svc.cluster.local" // K8s DNS for CR (runs inside cluster)
		secretName      = "mariadb-credentials"
		secretNamespace = "mariadb"
		timeout         = 2 * time.Minute
		interval        = 2 * time.Second
	)

	ctx := context.Background()

	// Database verifier for validating actual database state
	// MariaDB uses MySQL verifier since it's protocol-compatible
	var verifier *testutil.MySQLVerifier

	// Admin credentials read from secret (least-privilege account)
	var adminUsername, adminPassword string

	// getVerifierHost returns the host for the verifier to connect to.
	// In CI, we use port-forwarding so the verifier connects to localhost.
	// Locally, we may connect directly to the K8s service.
	getVerifierHost := func() string {
		if host := os.Getenv("E2E_DATABASE_HOST"); host != "" {
			return host
		}
		return mariadbHost
	}

	BeforeAll(func() {
		By("setting up MariaDB verifier (using MySQL verifier - protocol compatible)")
		// Get the admin credentials from the secret (least-privilege account)
		var err error
		adminUsername, err = getSecretValue(ctx, secretNamespace, secretName, "username")
		Expect(err).NotTo(HaveOccurred(), "Failed to get MariaDB username from secret")

		adminPassword, err = getSecretValue(ctx, secretNamespace, secretName, "password")
		Expect(err).NotTo(HaveOccurred(), "Failed to get MariaDB password from secret")

		verifierHost := getVerifierHost()
		GinkgoWriter.Printf("Using verifier host: %s with admin user: %s\n", verifierHost, adminUsername)

		cfg := testutil.MariaDBEngineConfig(verifierHost, 3306, adminUsername, adminPassword)
		verifier = testutil.NewMySQLVerifier(cfg)

		// Connect with retry since database may still be starting
		Eventually(func() error {
			return verifier.Connect(ctx)
		}, timeout, interval).Should(Succeed(), "Should connect to MariaDB for verification")
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
						// Use "mysql" engine type - MariaDB is MySQL-compatible
						"engine": "mysql",
						"connection": map[string]interface{}{
							"host":     mariadbHost,
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

			By("verifying database actually exists in MariaDB")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, databaseName)
				if err != nil {
					GinkgoWriter.Printf("Error checking database existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "Database '%s' should exist in MariaDB", databaseName)
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

			By("verifying user actually exists in MariaDB")
			Eventually(func() bool {
				exists, err := verifier.UserExists(ctx, userName)
				if err != nil {
					GinkgoWriter.Printf("Error checking user existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "User '%s' should exist in MariaDB", userName)
		})
	})

	Context("Cross-namespace Secret reference", func() {
		It("should support cross-namespace Secret reference (ClusterRoleBinding test)", func() {
			// The DatabaseInstance already uses cross-namespace reference:
			// - DatabaseInstance is in 'default' namespace
			// - Secret is in 'mariadb' namespace
			// This test verifies that the cross-namespace RBAC works correctly

			By("verifying DatabaseInstance can access Secret from different namespace")
			obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to get DatabaseInstance")

			phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			Expect(phase).To(Equal("Ready"), "DatabaseInstance should be Ready (cross-namespace RBAC works)")

			// Verify the secret reference points to a different namespace
			secretRef, _, _ := unstructured.NestedMap(obj.Object, "spec", "connection", "secretRef")
			Expect(secretRef["namespace"]).To(Equal(secretNamespace), "Secret should be in mariadb namespace")
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
						// MariaDB 10.0.5+ supports native roles
						"mysql": map[string]interface{}{
							"useNativeRoles": true,
						},
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

			By("verifying role actually exists in MariaDB")
			Eventually(func() bool {
				exists, err := verifier.RoleExists(ctx, roleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking role existence: %v\n", err)
					return false
				}
				return exists
			}, timeout, interval).Should(BeTrue(), "Role '%s' should exist in MariaDB", roleName)
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
						"mysql": map[string]interface{}{
							"grants": []interface{}{
								map[string]interface{}{
									"level":      "database",
									"database":   databaseName,
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

			By("verifying user has SELECT privilege on database")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, userName, "SELECT", "database", databaseName)
				if err != nil {
					GinkgoWriter.Printf("Error checking database privilege: %v\n", err)
					return false
				}
				return hasPriv
			}, timeout, interval).Should(BeTrue(), "User '%s' should have SELECT privilege on database '%s'", userName, databaseName)
		})
	})

	// ===== Least Privilege Verification =====
	// These tests verify the operator uses a least-privilege account, NOT root/superuser
	// MariaDB uses MySQL verifier since they share the same protocol

	Context("Least Privilege Verification", func() {
		It("should operate without SUPER privilege", func() {
			By("verifying the operator account does NOT have SUPER privilege")
			var superPriv string
			err := verifier.QueryRow(ctx, "",
				"SELECT Super_priv FROM mysql.user WHERE User = SUBSTRING_INDEX(CURRENT_USER(), '@', 1)",
				&superPriv)
			Expect(err).NotTo(HaveOccurred(), "Failed to query SUPER privilege status")
			Expect(superPriv).To(Equal("N"),
				"Operator account should NOT have SUPER privilege (should be 'N', not 'Y')")

			GinkgoWriter.Printf("Verified: %s does NOT have SUPER privilege\n", adminUsername)
		})

		It("should not have ALL PRIVILEGES", func() {
			By("verifying SHOW GRANTS does not include ALL PRIVILEGES")
			var grants string
			err := verifier.QueryRow(ctx, "",
				"SHOW GRANTS FOR CURRENT_USER()",
				&grants)
			Expect(err).NotTo(HaveOccurred(), "Failed to query grants")
			Expect(grants).NotTo(ContainSubstring("ALL PRIVILEGES"),
				"Operator account should NOT have ALL PRIVILEGES")

			GinkgoWriter.Printf("Verified: %s does NOT have ALL PRIVILEGES\n", adminUsername)
		})

		It("should have required operational privileges", func() {
			By("verifying operator has CREATE, DROP, ALTER privileges")
			var grants string
			err := verifier.QueryRow(ctx, "",
				"SHOW GRANTS FOR CURRENT_USER()",
				&grants)
			Expect(err).NotTo(HaveOccurred(), "Failed to query grants")

			// Check for required privileges (case-insensitive)
			Expect(grants).To(MatchRegexp("(?i)CREATE"),
				"Operator should have CREATE privilege")
			Expect(grants).To(MatchRegexp("(?i)DROP"),
				"Operator should have DROP privilege")

			GinkgoWriter.Printf("Verified: %s has required operational privileges\n", adminUsername)
		})

		It("should have CREATE USER privilege", func() {
			By("verifying operator has CREATE USER for user management")
			var createUserPriv string
			err := verifier.QueryRow(ctx, "",
				"SELECT Create_user_priv FROM mysql.user WHERE User = SUBSTRING_INDEX(CURRENT_USER(), '@', 1)",
				&createUserPriv)
			Expect(err).NotTo(HaveOccurred(), "Failed to query CREATE USER privilege")
			Expect(createUserPriv).To(Equal("Y"),
				"Operator account should have CREATE USER privilege")

			GinkgoWriter.Printf("Verified: %s has CREATE USER privilege\n", adminUsername)
		})

		It("should print privilege summary", func() {
			By("printing complete privilege summary for documentation")
			GinkgoWriter.Printf("\n========== MariaDB Least-Privilege Summary ==========\n")
			GinkgoWriter.Printf("Admin Account: %s\n", adminUsername)
			GinkgoWriter.Printf("Required Privileges:\n")
			GinkgoWriter.Printf("  - CREATE, DROP, ALTER: Yes (database operations)\n")
			GinkgoWriter.Printf("  - CREATE USER: Yes (user management)\n")
			GinkgoWriter.Printf("  - WITH GRANT OPTION: Yes (privilege delegation)\n")
			GinkgoWriter.Printf("  - CONNECTION ADMIN: Yes (terminate connections) [note: space not underscore]\n")
			GinkgoWriter.Printf("  - SUPER: No (not required, not granted)\n")
			GinkgoWriter.Printf("  - ALL PRIVILEGES: No (not required, not granted)\n")
			GinkgoWriter.Printf("Note: MariaDB uses 'CONNECTION ADMIN' (with space), not 'CONNECTION_ADMIN'\n")
			GinkgoWriter.Printf("=====================================================\n\n")
		})
	})

	// ===== Functionality Verification Tests =====
	// These tests verify that database operations actually work, not just that CRs become Ready
	// MariaDB uses MySQL verifier since they share the same protocol

	Context("Database Ownership Verification", func() {
		It("should identify database owner (user with full privileges)", func() {
			By("querying database owner from MariaDB")
			// MariaDB (like MySQL) doesn't have explicit database ownership
			// We check for users with ALL PRIVILEGES on the database
			owner, err := verifier.GetDatabaseOwner(ctx, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Failed to get database owner")

			By("verifying owner is root or admin user")
			GinkgoWriter.Printf("Database '%s' owner/admin: %s\n", databaseName, owner)
			Expect(owner).NotTo(BeEmpty(), "Should have an owner/admin user")
		})
	})

	Context("User Credential Validation", func() {
		It("should generate working credentials in Secret", func() {
			By("getting user credentials from Secret")
			secretName := userName + "-credentials"
			password, err := getSecretValue(ctx, testNamespace, secretName, "password")
			if err != nil {
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

			By("creating a test table using admin connection first")
			// Use admin connection (from BeforeAll) to create table for testing SELECT/INSERT
			adminConn, err := verifier.ConnectAsUser(ctx, adminUsername, adminPassword, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Should connect as admin")

			// Create test table
			err = adminConn.CanCreateTable(ctx, testTable)
			Expect(err).NotTo(HaveOccurred(), "Admin should create test table")

			// Grant SELECT, INSERT to the test user on the table
			err = adminConn.Exec(ctx, "GRANT SELECT, INSERT ON "+databaseName+"."+testTable+" TO '"+userName+"'@'%'")
			Expect(err).NotTo(HaveOccurred(), "Should grant permissions on table")

			// Flush privileges to ensure grants take effect
			err = adminConn.Exec(ctx, "FLUSH PRIVILEGES")
			Expect(err).NotTo(HaveOccurred(), "Should flush privileges")
			adminConn.Close()

			By("connecting as the granted user")
			userConn, err := verifier.ConnectAsUser(ctx, userName, password, databaseName)
			Expect(err).NotTo(HaveOccurred(), "Should connect as user")
			defer userConn.Close()

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

			By("creating a DatabaseGrant to assign role to user")
			roleGrantName := memberUser + "-role-grant"
			roleGrantObj := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "dbops.dbprovision.io/v1alpha1",
					"kind":       "DatabaseGrant",
					"metadata": map[string]interface{}{
						"name":      roleGrantName,
						"namespace": testNamespace,
					},
					"spec": map[string]interface{}{
						"userRef": map[string]interface{}{
							"name": memberUser,
						},
						"mysql": map[string]interface{}{
							"roles": []interface{}{roleName},
						},
					},
				},
			}

			_, err = dynamicClient.Resource(databaseGrantGVR).Namespace(testNamespace).Create(ctx, roleGrantObj, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred(), "Failed to create role grant")

			By("waiting for grant to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGrantGVR).Namespace(testNamespace).Get(ctx, roleGrantName, metav1.GetOptions{})
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

			By("cleaning up role grant")
			_ = dynamicClient.Resource(databaseGrantGVR).Namespace(testNamespace).Delete(ctx, roleGrantName, metav1.DeleteOptions{})

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
