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
	"os"
	"strconv"
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
		postgresHost    = "host.k3d.internal" // Docker host from k3d (database runs in Docker Compose)
		secretName      = "postgres-credentials"
		secretNamespace = "postgres"

		// reconcileTimeout is the default timeout for waiting on reconcile-driven state changes
		// (CR phase transitions, resource creation/deletion in DB, CR deletion).
		// Adjustable globally here; individual tests override when longer waits are needed.
		reconcileTimeout = 30 * time.Second

		// pollingInterval is the frequency at which Eventually/Consistently poll.
		pollingInterval = 2 * time.Second

		// podRestartTimeout is for waiting on infrastructure recovery (pod restart, reconnection).
		podRestartTimeout = 3 * time.Minute

		// Note: drift detection tests have been moved to dedicated drift_*_test.go modules.
	)

	ctx := context.Background()

	// Database verifier for validating actual database state
	var verifier *testutil.PostgresVerifier

	// getVerifierHost returns the host for the verifier to connect to.
	// Tests connect to localhost; override with E2E_DATABASE_HOST if needed.
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

	// getInstanceHost returns the host for the DatabaseInstance CR.
	// Defaults to host.k3d.internal (Docker host from k3d).
	getInstanceHost := func() string {
		if host := os.Getenv("E2E_INSTANCE_HOST"); host != "" {
			return host
		}
		return postgresHost
	}

	// getInstancePort returns the port for the DatabaseInstance CR.
	getInstancePort := func() int64 {
		if portStr := os.Getenv("E2E_INSTANCE_PORT"); portStr != "" {
			if port, err := strconv.ParseInt(portStr, 10, 64); err == nil {
				return port
			}
		}
		return 5432
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
		verifierPort := getVerifierPort()
		GinkgoWriter.Printf("Using verifier host: %s:%d with admin user: %s\n", verifierHost, verifierPort, adminUsername)

		cfg := testutil.PostgresEngineConfig(verifierHost, verifierPort, adminUsername, adminPassword)
		verifier = testutil.NewPostgresVerifier(cfg)

		// Connect with retry since database may still be starting
		Eventually(func() error {
			return verifier.Connect(ctx)
		}, reconcileTimeout, pollingInterval).Should(Succeed(), "Should connect to PostgreSQL for verification")
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
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"), "DatabaseInstance should become Ready")

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
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Connected condition should be True")
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
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"), "Database should become Ready")

			By("verifying database actually exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, databaseName)
				if err != nil {
					GinkgoWriter.Printf("Error checking database existence: %v\n", err)
					return false
				}
				return exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Database '%s' should exist in PostgreSQL", databaseName)
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
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"), "DatabaseUser should become Ready")

			By("verifying user actually exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.UserExists(ctx, userName)
				if err != nil {
					GinkgoWriter.Printf("Error checking user existence: %v\n", err)
					return false
				}
				return exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "User '%s' should exist in PostgreSQL", userName)
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
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"), "DatabaseRole should become Ready")

			By("verifying role actually exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.RoleExists(ctx, roleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking role existence: %v\n", err)
					return false
				}
				return exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Role '%s' should exist in PostgreSQL", roleName)
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
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"), "DatabaseGrant should become Ready")

			By("verifying user has USAGE privilege on public schema in testdb")
			Eventually(func() bool {
				// Must check on the specific database where grants were applied
				hasPriv, err := verifier.HasPrivilegeOnDatabase(ctx, userName, "USAGE", "schema", "public", databaseName)
				if err != nil {
					GinkgoWriter.Printf("Error checking schema privilege: %v\n", err)
					return false
				}
				return hasPriv
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "User '%s' should have USAGE privilege on public schema in database '%s'", userName, databaseName)
		})
	})

	// NOTE: Least-privilege verification tests have been moved to integration tests
	// (internal/controller/integration_privileges_test.go) since they only need
	// database access, not a full K8s cluster. This saves ~10 minutes of E2E time.

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
			}, reconcileTimeout, pollingInterval).Should(Succeed(), "Should grant permissions on table and sequence")

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
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"))

			By("verifying user is a member of the role")
			Eventually(func() bool {
				isMember, err := verifier.HasRoleMembership(ctx, memberUser, roleName)
				if err != nil {
					GinkgoWriter.Printf("Error checking role membership: %v\n", err)
					return false
				}
				return isMember
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "User '%s' should be a member of role '%s'", memberUser, roleName)

			By("cleaning up member user")
			_ = dynamicClient.Resource(databaseUserGVR).Namespace(testNamespace).Delete(ctx, memberUser, metav1.DeleteOptions{})
		})
	})

	// ===== Phase 5: E2E Gap Coverage Tests =====

	Context("Error Recovery", func() {
		It("should recover DatabaseInstance after database pod restart", func() {
			By("verifying existing instance is Ready with Connected=True")
			obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			Expect(phase).To(Equal("Ready"))

			By("checking if PostgreSQL is deployed as a Kubernetes pod")
			pods, err := k8sClient.CoreV1().Pods("postgres").List(ctx, metav1.ListOptions{
				LabelSelector: "app=postgres",
			})
			if err != nil {
				Skip(fmt.Sprintf("Cannot list pods in postgres namespace: %v", err))
			}
			if len(pods.Items) == 0 {
				Skip("PostgreSQL is not deployed as a K8s pod (Docker Compose mode), skipping pod restart recovery test")
			}
			podName := pods.Items[0].Name
			GinkgoWriter.Printf("Deleting PostgreSQL pod: %s\n", podName)
			gracePeriod := int64(0)
			err = k8sClient.CoreV1().Pods("postgres").Delete(ctx, podName, metav1.DeleteOptions{
				GracePeriodSeconds: &gracePeriod,
			})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for instance to detect disconnection")
			Eventually(func() bool {
				obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
				for _, c := range conditions {
					condition := c.(map[string]interface{})
					if condition["type"] == "Connected" && condition["status"] == "False" {
						return true
					}
				}
				return phase == "Failed"
			}, podRestartTimeout, pollingInterval).Should(BeTrue(), "Instance should detect pod failure")

			By("waiting for PostgreSQL pod to be Running again")
			Eventually(func() bool {
				pods, err := k8sClient.CoreV1().Pods("postgres").List(ctx, metav1.ListOptions{
					LabelSelector: "app=postgres",
				})
				if err != nil || len(pods.Items) == 0 {
					return false
				}
				for _, pod := range pods.Items {
					if pod.Status.Phase == "Running" {
						GinkgoWriter.Printf("Pod %s is Running again\n", pod.Name)
						return true
					}
				}
				return false
			}, podRestartTimeout, pollingInterval).Should(BeTrue(), "PostgreSQL pod should be Running again")

			By("waiting for instance to recover to Ready with Connected=True and Healthy=True")
			Eventually(func() bool {
				obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(testNamespace).Get(ctx, instanceName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				if phase != "Ready" {
					return false
				}
				conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
				connectedTrue := false
				healthyTrue := false
				for _, c := range conditions {
					condition := c.(map[string]interface{})
					if condition["type"] == "Connected" && condition["status"] == "True" {
						connectedTrue = true
					}
					if condition["type"] == "Healthy" && condition["status"] == "True" {
						healthyTrue = true
					}
				}
				return connectedTrue && healthyTrue
			}, podRestartTimeout, pollingInterval).Should(BeTrue(), "Instance should recover to Ready with Connected=True and Healthy=True")

			By("reconnecting database verifier")
			_ = verifier.Close()
			Eventually(func() error {
				return verifier.Connect(ctx)
			}, podRestartTimeout, pollingInterval).Should(Succeed(), "Verifier should reconnect after pod restart")
		})
	})

	Context("Database Deletion Protection", func() {
		It("should block deletion when spec.deletionProtection is true", func() {
			const crName = "testdb-delprot"
			const dbName = "testdb_delprot"

			By("creating Database with deletionProtection: true")
			db := testutil.BuildDatabaseWithOptions(crName, testNamespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: true,
				DeletionPolicy:     "Delete",
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Database to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"))

			By("verifying database exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, dbName)
				return err == nil && exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue())

			By("attempting to delete the CR")
			err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, crName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("verifying resource is NOT deleted (finalizer blocks)")
			Consistently(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				return err == nil
			}, 15*time.Second, 2*time.Second).Should(BeTrue(), "Resource should still exist due to deletion protection")

			By("cleanup: adding force-delete annotation to bypass protection")
			obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			annotations := obj.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations["dbops.dbprovision.io/force-delete"] = "true"
			obj.SetAnnotations(annotations)
			_, err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				return err != nil
			}, getDeletionTimeout(), pollingInterval).Should(BeTrue(), "CR should be deleted after force-delete annotation")
		})

		It("should allow deletion with force-delete annotation", func() {
			const crName = "testdb-force"
			const dbName = "testdb_force"

			By("creating Database with deletionProtection: true")
			db := testutil.BuildDatabaseWithOptions(crName, testNamespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: true,
				DeletionPolicy:     "Delete",
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Database to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"))

			By("adding force-delete annotation before deletion")
			obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			annotations := obj.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations["dbops.dbprovision.io/force-delete"] = "true"
			obj.SetAnnotations(annotations)
			_, err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("deleting the CR")
			err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, crName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				return err != nil
			}, getDeletionTimeout(), pollingInterval).Should(BeTrue(), "CR should be deleted with force-delete annotation")

			By("verifying database was dropped from PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, dbName)
				return err == nil && !exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Database should be dropped when deletionPolicy is Delete")
		})

		It("should retain database when deletionPolicy is Retain", func() {
			const crName = "testdb-retain"
			const dbName = "testdb_retain"

			By("creating Database with deletionProtection: false, deletionPolicy: Retain")
			db := testutil.BuildDatabaseWithOptions(crName, testNamespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Retain",
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Database to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"))

			By("verifying database exists in PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, dbName)
				return err == nil && exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue())

			By("deleting the CR")
			err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, crName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				return err != nil
			}, getDeletionTimeout(), pollingInterval).Should(BeTrue(), "CR should be deleted")

			By("verifying database STILL exists in PostgreSQL (Retain policy)")
			exists, err := verifier.DatabaseExists(ctx, dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "Database should still exist after CR deletion with Retain policy")

			By("cleanup: dropping retained database")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			Expect(err).NotTo(HaveOccurred())
		})

		It("should drop database when deletionPolicy is Delete", func() {
			const crName = "testdb-delete"
			const dbName = "testdb_delete"

			By("creating Database with deletionProtection: false, deletionPolicy: Delete")
			db := testutil.BuildDatabaseWithOptions(crName, testNamespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Database to become Ready")
			Eventually(func() string {
				obj, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				if err != nil {
					return ""
				}
				phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
				return phase
			}, reconcileTimeout, pollingInterval).Should(Equal("Ready"))

			By("deleting the CR")
			err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, crName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crName, metav1.GetOptions{})
				return err != nil
			}, getDeletionTimeout(), pollingInterval).Should(BeTrue(), "CR should be deleted")

			By("verifying database was dropped from PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, dbName)
				return err == nil && !exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Database should be dropped when deletionPolicy is Delete")
		})
	})

	// NOTE: Drift Detection tests have been moved to dedicated modules:
	// - drift_role_test.go (DatabaseRole drift)
	// - drift_database_test.go (Database schema/extension drift)
	// - drift_user_test.go (DatabaseUser drift)
	// - drift_grant_test.go (DatabaseGrant drift)
	// - drift_clusterrole_test.go (ClusterDatabaseRole drift)
	// - drift_clustergrant_test.go (ClusterDatabaseGrant drift)

	Context("Multi-Resource Independence", func() {
		It("should operate multiple databases independently on the same instance", func() {
			const crNameA = "testdb-indep-a"
			const dbNameA = "testdb_indep_a"
			const crNameB = "testdb-indep-b"
			const dbNameB = "testdb_indep_b"

			By("creating two Database CRs")
			dbA := testutil.BuildDatabaseWithOptions(crNameA, testNamespace, instanceName, dbNameA, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
			})
			dbB := testutil.BuildDatabaseWithOptions(crNameB, testNamespace, instanceName, dbNameB, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
			})

			_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, dbA, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			_, err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Create(ctx, dbB, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for both databases to become Ready")
			Eventually(func() bool {
				objA, errA := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crNameA, metav1.GetOptions{})
				objB, errB := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crNameB, metav1.GetOptions{})
				if errA != nil || errB != nil {
					return false
				}
				phaseA, _, _ := unstructured.NestedString(objA.Object, "status", "phase")
				phaseB, _, _ := unstructured.NestedString(objB.Object, "status", "phase")
				return phaseA == "Ready" && phaseB == "Ready"
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Both databases should become Ready")

			By("verifying both databases exist in PostgreSQL")
			Eventually(func() bool {
				existsA, errA := verifier.DatabaseExists(ctx, dbNameA)
				existsB, errB := verifier.DatabaseExists(ctx, dbNameB)
				return errA == nil && errB == nil && existsA && existsB
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Both databases should exist in PostgreSQL")

			By("deleting only database A")
			err = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, crNameA, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for database A CR to be deleted")
			Eventually(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crNameA, metav1.GetOptions{})
				return err != nil
			}, getDeletionTimeout(), pollingInterval).Should(BeTrue(), "Database A CR should be deleted")

			By("verifying database A is dropped from PostgreSQL")
			Eventually(func() bool {
				exists, err := verifier.DatabaseExists(ctx, dbNameA)
				return err == nil && !exists
			}, reconcileTimeout, pollingInterval).Should(BeTrue(), "Database A should be dropped from PostgreSQL")

			By("verifying database B still exists and its CR is still Ready")
			existsB, err := verifier.DatabaseExists(ctx, dbNameB)
			Expect(err).NotTo(HaveOccurred())
			Expect(existsB).To(BeTrue(), "Database B should still exist in PostgreSQL")

			objB, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crNameB, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			phaseB, _, _ := unstructured.NestedString(objB.Object, "status", "phase")
			Expect(phaseB).To(Equal("Ready"), "Database B CR should still be Ready")

			By("connecting to database B to confirm full functionality")
			conn, err := verifier.ConnectAsUser(ctx, adminUsername, adminPassword, dbNameB)
			Expect(err).NotTo(HaveOccurred())

			err = conn.Query(ctx, "SELECT 1")
			Expect(err).NotTo(HaveOccurred(), "Should be able to query database B")
			conn.Close()

			By("cleanup: deleting database B")
			_ = dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Delete(ctx, crNameB, metav1.DeleteOptions{})
			Eventually(func() bool {
				_, err := dynamicClient.Resource(databaseGVR).Namespace(testNamespace).Get(ctx, crNameB, metav1.GetOptions{})
				return err != nil
			}, getDeletionTimeout(), pollingInterval).Should(BeTrue())
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
		}, getDeletionTimeout(), pollingInterval).Should(BeTrue(), "DatabaseInstance should be deleted")
	})
})
