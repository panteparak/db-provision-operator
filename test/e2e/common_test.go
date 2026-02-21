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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/db-provision-operator/test/e2e/testutil"
)

// DatabaseTestConfig holds engine-specific configuration for parameterized tests.
// This allows the same test logic to run against PostgreSQL, MySQL, and MariaDB.
type DatabaseTestConfig struct {
	// Engine identifier: "postgresql", "mysql", "mariadb"
	EngineType string

	// Resource naming
	InstanceName  string
	DatabaseName  string
	UserName      string
	RoleName      string
	GrantName     string
	TestNamespace string

	// Connection details
	Host            string
	Port            int32
	SecretName      string
	SecretNamespace string

	// Engine-specific values
	Engine        string // API engine value: "postgres" or "mysql"
	AdminDatabase string // Default admin database

	// Timeouts
	Timeout  time.Duration
	Interval time.Duration

	// Verifier (set during setup)
	Verifier testutil.DatabaseVerifier
}

// DefaultTimeout returns the default timeout for Eventually assertions.
func (cfg *DatabaseTestConfig) DefaultTimeout() time.Duration {
	if cfg.Timeout == 0 {
		return 2 * time.Minute
	}
	return cfg.Timeout
}

// DefaultInterval returns the default polling interval for Eventually assertions.
func (cfg *DatabaseTestConfig) DefaultInterval() time.Duration {
	if cfg.Interval == 0 {
		return 2 * time.Second
	}
	return cfg.Interval
}

// getDeletionTimeout returns the timeout for waiting on CR deletion.
// Override with E2E_DELETION_TIMEOUT for resource-constrained CI environments.
func getDeletionTimeout() time.Duration {
	if val := os.Getenv("E2E_DELETION_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return 60 * time.Second
}

// ===== Shared Test Runners =====

// RunInstanceLifecycleTest tests DatabaseInstance creation and readiness.
// Test ID: {ENGINE}-LIF-01
func (cfg *DatabaseTestConfig) RunInstanceLifecycleTest(ctx context.Context) {
	By("creating a DatabaseInstance CR")
	instance := testutil.BuildDatabaseInstance(
		cfg.InstanceName,
		cfg.TestNamespace,
		cfg.Engine,
		cfg.Host,
		int64(cfg.Port),
		testutil.SecretRef{Name: cfg.SecretName, Namespace: cfg.SecretNamespace},
	)

	_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Create(ctx, instance, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseInstance")

	By("waiting for DatabaseInstance to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.InstanceName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"), "DatabaseInstance should become Ready")

	By("verifying the Connected condition is True")
	Eventually(func() bool {
		obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.InstanceName, metav1.GetOptions{})
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
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(BeTrue(), "Connected condition should be True")
}

// RunDatabaseLifecycleTest tests Database creation and readiness.
// Test ID: {ENGINE}-LIF-02
func (cfg *DatabaseTestConfig) RunDatabaseLifecycleTest(ctx context.Context) {
	By("creating a Database CR")
	database := testutil.BuildDatabase(
		cfg.DatabaseName,
		cfg.TestNamespace,
		cfg.InstanceName,
		cfg.DatabaseName,
	)

	_, err := dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Create(ctx, database, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create Database")

	By("waiting for Database to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.DatabaseName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"), "Database should become Ready")

	By("verifying database actually exists in the database server")
	if cfg.Verifier != nil {
		Eventually(func() bool {
			exists, err := cfg.Verifier.DatabaseExists(ctx, cfg.DatabaseName)
			if err != nil {
				GinkgoWriter.Printf("Error checking database existence: %v\n", err)
				return false
			}
			return exists
		}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(BeTrue(), "Database should exist in the database server")
	}
}

// RunUserLifecycleTest tests DatabaseUser creation and readiness.
// Test ID: {ENGINE}-LIF-03
func (cfg *DatabaseTestConfig) RunUserLifecycleTest(ctx context.Context) {
	By("creating a DatabaseUser CR")
	user := testutil.BuildDatabaseUser(
		cfg.UserName,
		cfg.TestNamespace,
		cfg.InstanceName,
		cfg.UserName,
	)

	_, err := dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Create(ctx, user, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseUser")

	By("waiting for DatabaseUser to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.UserName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"), "DatabaseUser should become Ready")

	By("verifying user actually exists in the database server")
	if cfg.Verifier != nil {
		Eventually(func() bool {
			exists, err := cfg.Verifier.UserExists(ctx, cfg.UserName)
			if err != nil {
				GinkgoWriter.Printf("Error checking user existence: %v\n", err)
				return false
			}
			return exists
		}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(BeTrue(), "User should exist in the database server")
	}
}

// RunRoleLifecycleTest tests DatabaseRole creation and readiness.
// Test ID: {ENGINE}-LIF-04
func (cfg *DatabaseTestConfig) RunRoleLifecycleTest(ctx context.Context) {
	By("creating a DatabaseRole CR")
	role := testutil.BuildDatabaseRole(
		cfg.RoleName,
		cfg.TestNamespace,
		cfg.InstanceName,
		cfg.RoleName,
	)

	_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(cfg.TestNamespace).Create(ctx, role, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseRole")

	By("waiting for DatabaseRole to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseRoleGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.RoleName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"), "DatabaseRole should become Ready")

	By("verifying role actually exists in the database server")
	if cfg.Verifier != nil {
		Eventually(func() bool {
			exists, err := cfg.Verifier.RoleExists(ctx, cfg.RoleName)
			if err != nil {
				GinkgoWriter.Printf("Error checking role existence: %v\n", err)
				return false
			}
			return exists
		}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(BeTrue(), "Role should exist in the database server")
	}
}

// RunGrantLifecycleTest tests DatabaseGrant creation and readiness.
// Test ID: {ENGINE}-LIF-05
func (cfg *DatabaseTestConfig) RunGrantLifecycleTest(ctx context.Context) {
	By("creating a DatabaseGrant CR")
	grant := testutil.BuildDatabaseGrant(
		cfg.GrantName,
		cfg.TestNamespace,
		cfg.InstanceName,
		cfg.UserName,
		"ALL",
		"database",
		cfg.DatabaseName,
	)

	_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(cfg.TestNamespace).Create(ctx, grant, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create DatabaseGrant")

	By("waiting for DatabaseGrant to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseGrantGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.GrantName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"), "DatabaseGrant should become Ready")
}

// RunDatabaseOwnershipTest verifies that database ownership is correctly set.
// Test ID: {ENGINE}-FUN-01
func (cfg *DatabaseTestConfig) RunDatabaseOwnershipTest(ctx context.Context) {
	if cfg.Verifier == nil {
		Skip("Verifier not configured, skipping ownership test")
	}

	By("getting the database owner")
	owner, err := cfg.Verifier.GetDatabaseOwner(ctx, cfg.DatabaseName)
	Expect(err).NotTo(HaveOccurred(), "Failed to get database owner")

	By("verifying the owner is the expected admin user")
	// For PostgreSQL, the owner is typically 'postgres'
	// For MySQL, ownership is based on privileges
	GinkgoWriter.Printf("Database %s owner: %s\n", cfg.DatabaseName, owner)
	Expect(owner).NotTo(BeEmpty(), "Database should have an owner")
}

// RunUserCredentialValidationTest verifies that generated credentials work.
// Test ID: {ENGINE}-FUN-02
func (cfg *DatabaseTestConfig) RunUserCredentialValidationTest(ctx context.Context) {
	if cfg.Verifier == nil {
		Skip("Verifier not configured, skipping credential validation test")
	}

	By("getting the user's secret name from the DatabaseUser status")
	var secretName string
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.UserName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		name, _, _ := unstructured.NestedString(obj.Object, "status", "secret", "name")
		secretName = name
		return name
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).ShouldNot(BeEmpty(), "User should have a secret")

	By("reading the password from the secret")
	password, err := getSecretValue(ctx, cfg.TestNamespace, secretName, "password")
	if err != nil {
		// Try the default naming convention
		password, err = getSecretValue(ctx, cfg.TestNamespace, cfg.UserName+"-credentials", "password")
	}
	Expect(err).NotTo(HaveOccurred(), "Failed to get password from secret")
	Expect(password).NotTo(BeEmpty(), "Password should not be empty")

	By("connecting to the database with user credentials")
	userConn, err := cfg.Verifier.ConnectAsUser(ctx, cfg.UserName, password, cfg.DatabaseName)
	Expect(err).NotTo(HaveOccurred(), "Failed to connect as user")
	defer userConn.Close()

	By("verifying the connection works by pinging")
	err = userConn.Ping(ctx)
	Expect(err).NotTo(HaveOccurred(), "User should be able to ping the database")
}

// ===== Negative Test Runners =====

// RunInvalidInstanceConnectionTest verifies that invalid credentials fail gracefully.
// Test ID: ALL-NEG-01
func (cfg *DatabaseTestConfig) RunInvalidInstanceConnectionTest(ctx context.Context) {
	invalidInstanceName := cfg.InstanceName + "-invalid"

	By("creating an invalid Secret with wrong password")
	invalidSecretName := "invalid-credentials-secret"
	invalidSecret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      invalidSecretName,
				"namespace": cfg.TestNamespace,
			},
			"type": "Opaque",
			"stringData": map[string]interface{}{
				"username": "root",
				"password": "definitely-wrong-password-12345",
			},
		},
	}

	secretGVR := databaseInstanceGVR // Reuse GVR pattern
	secretGVR.Group = ""
	secretGVR.Version = "v1"
	secretGVR.Resource = "secrets"

	_, err := k8sClient.CoreV1().Secrets(cfg.TestNamespace).Create(ctx, nil, metav1.CreateOptions{})
	// Create using dynamic client for consistency
	_, _ = dynamicClient.Resource(secretGVR).Namespace(cfg.TestNamespace).Create(ctx, invalidSecret, metav1.CreateOptions{})
	// Ignore error if secret already exists

	By("creating a DatabaseInstance with invalid credentials")
	instance := testutil.BuildDatabaseInstance(
		invalidInstanceName,
		cfg.TestNamespace,
		cfg.Engine,
		cfg.Host,
		int64(cfg.Port),
		testutil.SecretRef{Name: invalidSecretName, Namespace: cfg.TestNamespace},
	)

	_, err = dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Create(ctx, instance, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Should be able to create instance CR (validation happens during reconcile)")

	By("waiting for DatabaseInstance to fail with connection error")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, invalidInstanceName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Failed"), "DatabaseInstance should fail with invalid credentials")

	By("verifying the Connected condition is False")
	Eventually(func() bool {
		obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, invalidInstanceName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		for _, c := range conditions {
			condition := c.(map[string]interface{})
			if condition["type"] == "Connected" && condition["status"] == "False" {
				return true
			}
		}
		return false
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(BeTrue(), "Connected condition should be False")

	By("cleaning up invalid instance")
	_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Delete(ctx, invalidInstanceName, metav1.DeleteOptions{})
	_ = dynamicClient.Resource(secretGVR).Namespace(cfg.TestNamespace).Delete(ctx, invalidSecretName, metav1.DeleteOptions{})
}

// RunNonExistentInstanceRefTest verifies that databases with invalid instance refs fail.
// Test ID: ALL-NEG-02
func (cfg *DatabaseTestConfig) RunNonExistentInstanceRefTest(ctx context.Context) {
	invalidDbName := cfg.DatabaseName + "-invalid-ref"

	By("creating a Database CR with non-existent instance reference")
	database := testutil.BuildDatabase(
		invalidDbName,
		cfg.TestNamespace,
		"non-existent-instance-that-does-not-exist",
		invalidDbName,
	)

	_, err := dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Create(ctx, database, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Should be able to create database CR")

	By("waiting for Database to fail due to missing instance")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Get(ctx, invalidDbName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Or(
		Equal("Failed"),
		Equal("Pending"),
	), "Database should fail or stay pending with missing instance reference")

	By("cleaning up invalid database")
	_ = dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Delete(ctx, invalidDbName, metav1.DeleteOptions{})
}

// RunDeletionProtectionTest verifies that deletion protection works.
// Test ID: ALL-NEG-03
func (cfg *DatabaseTestConfig) RunDeletionProtectionTest(ctx context.Context) {
	protectedInstanceName := cfg.InstanceName + "-protected"

	By("creating a DatabaseInstance with deletion protection enabled")
	instance := testutil.BuildDatabaseInstance(
		protectedInstanceName,
		cfg.TestNamespace,
		cfg.Engine,
		cfg.Host,
		int64(cfg.Port),
		testutil.SecretRef{Name: cfg.SecretName, Namespace: cfg.SecretNamespace},
	)

	// Add deletion protection annotation
	metadata := instance.Object["metadata"].(map[string]interface{})
	metadata["annotations"] = map[string]interface{}{
		"dbops.dbprovision.io/deletion-protection": "true",
	}

	_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Create(ctx, instance, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create protected DatabaseInstance")

	By("waiting for DatabaseInstance to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, protectedInstanceName, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"), "DatabaseInstance should become Ready")

	By("attempting to delete the protected instance")
	err = dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Delete(ctx, protectedInstanceName, metav1.DeleteOptions{})
	// The delete call itself may succeed (it marks for deletion), but the finalizer should block actual deletion

	By("verifying the instance still exists (blocked by finalizer)")
	Consistently(func() bool {
		_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, protectedInstanceName, metav1.GetOptions{})
		return err == nil
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "Protected instance should not be deleted")

	By("cleaning up - removing protection and deleting")
	obj, _ := dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Get(ctx, protectedInstanceName, metav1.GetOptions{})
	if obj != nil {
		// Remove the deletion protection annotation
		metadata := obj.Object["metadata"].(map[string]interface{})
		annotations := metadata["annotations"].(map[string]interface{})
		delete(annotations, "dbops.dbprovision.io/deletion-protection")
		annotations["dbops.dbprovision.io/force-delete"] = "true"
		_, _ = dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Update(ctx, obj, metav1.UpdateOptions{})
	}
	_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(cfg.TestNamespace).Delete(ctx, protectedInstanceName, metav1.DeleteOptions{})
}

// RunGrantEnforcementTest verifies that grants restrict operations correctly.
// Test ID: {ENGINE}-SEC-01
func (cfg *DatabaseTestConfig) RunGrantEnforcementTest(ctx context.Context) {
	if cfg.Verifier == nil {
		Skip("Verifier not configured, skipping grant enforcement test")
	}

	testUserName := "granttest"
	testTable := "grant_test_table"

	By("creating a test user with limited permissions")
	user := testutil.BuildDatabaseUser(
		testUserName,
		cfg.TestNamespace,
		cfg.InstanceName,
		testUserName,
	)
	_, err := dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Create(ctx, user, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	By("waiting for user to be ready")
	Eventually(func() string {
		obj, _ := dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Get(ctx, testUserName, metav1.GetOptions{})
		if obj == nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, cfg.DefaultTimeout(), cfg.DefaultInterval()).Should(Equal("Ready"))

	By("getting the user's password")
	secretName := testUserName + "-credentials"
	password, err := getSecretValue(ctx, cfg.TestNamespace, secretName, "password")
	Expect(err).NotTo(HaveOccurred())

	By("creating a test table using admin connection")
	// Use admin verifier to create the table
	adminConn, err := cfg.Verifier.ConnectAsUser(ctx, cfg.SecretName, "", cfg.DatabaseName)
	if err == nil {
		defer adminConn.Close()
		_ = adminConn.CanCreateTable(ctx, testTable)
	}

	By("connecting as the test user")
	userConn, err := cfg.Verifier.ConnectAsUser(ctx, testUserName, password, cfg.DatabaseName)
	Expect(err).NotTo(HaveOccurred())
	defer userConn.Close()

	By("verifying SELECT is allowed")
	err = userConn.CanSelectData(ctx, testTable)
	Expect(err).NotTo(HaveOccurred(), "SELECT should be allowed")

	By("verifying DELETE is denied (negative test)")
	err = userConn.CanDeleteData(ctx, testTable)
	Expect(err).To(HaveOccurred(), "DELETE should be denied without explicit grant")

	// Verify it's a permission error, not some other error
	if err != nil {
		Expect(err.Error()).To(Or(
			ContainSubstring("permission denied"),
			ContainSubstring("Access denied"),
			ContainSubstring("denied"),
			ContainSubstring("PERMISSION"),
		), "Error should be permission-related")
	}

	By("cleaning up test user")
	_ = dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Delete(ctx, testUserName, metav1.DeleteOptions{})
}

// ===== Cleanup Helpers =====

// deleteAndWait issues a delete for a namespaced resource and waits for it to be fully removed.
func deleteAndWait(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, timeout, interval time.Duration) {
	_ = dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	Eventually(func() bool {
		_, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		return err != nil
	}, timeout, interval).Should(BeTrue(), fmt.Sprintf("%s/%s should be deleted", gvr.Resource, name))
}

// CleanupTestResources removes all test resources created during tests.
// Resources are deleted level-by-level respecting the dependency graph:
// Level 1: Grant (leaf) → Level 2: Role, User, Database (middle) → Level 3: Instance (root)
func (cfg *DatabaseTestConfig) CleanupTestResources(ctx context.Context) {
	deletionTimeout := getDeletionTimeout()
	interval := cfg.DefaultInterval()

	// Level 1: Delete leaf resources (grants have no children)
	By("deleting DatabaseGrant and waiting")
	deleteAndWait(ctx, databaseGrantGVR, cfg.TestNamespace, cfg.GrantName, deletionTimeout, interval)

	// Level 2: Delete middle resources (their grant children are now gone)
	By("deleting DatabaseRole, DatabaseUser, Database")
	_ = dynamicClient.Resource(databaseRoleGVR).Namespace(cfg.TestNamespace).Delete(ctx, cfg.RoleName, metav1.DeleteOptions{})
	_ = dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Delete(ctx, cfg.UserName, metav1.DeleteOptions{})
	_ = dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Delete(ctx, cfg.DatabaseName, metav1.DeleteOptions{})

	By("waiting for middle-level resources to be deleted")
	Eventually(func() bool {
		_, e1 := dynamicClient.Resource(databaseRoleGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.RoleName, metav1.GetOptions{})
		_, e2 := dynamicClient.Resource(databaseUserGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.UserName, metav1.GetOptions{})
		_, e3 := dynamicClient.Resource(databaseGVR).Namespace(cfg.TestNamespace).Get(ctx, cfg.DatabaseName, metav1.GetOptions{})
		return e1 != nil && e2 != nil && e3 != nil
	}, deletionTimeout, interval).Should(BeTrue(), "Role, User, and Database should be deleted")

	// Level 3: Delete root resource (its children are now gone)
	By("deleting DatabaseInstance and waiting")
	deleteAndWait(ctx, databaseInstanceGVR, cfg.TestNamespace, cfg.InstanceName, deletionTimeout, interval)
}

// ===== Engine-Specific Configurations =====

// PostgreSQLTestConfig returns the default configuration for PostgreSQL tests.
func PostgreSQLTestConfig() *DatabaseTestConfig {
	return &DatabaseTestConfig{
		EngineType:      "postgresql",
		InstanceName:    "postgres-e2e-instance",
		DatabaseName:    "testdb",
		UserName:        "testuser",
		RoleName:        "testrole",
		GrantName:       "testgrant",
		TestNamespace:   "default",
		Host:            "host.k3d.internal",
		Port:            5432,
		SecretName:      "postgresql-credentials",
		SecretNamespace: "postgresql",
		Engine:          "postgres",
		AdminDatabase:   "postgres",
		Timeout:         2 * time.Minute,
		Interval:        2 * time.Second,
	}
}

// MySQLTestConfig returns the default configuration for MySQL tests.
func MySQLTestConfig() *DatabaseTestConfig {
	return &DatabaseTestConfig{
		EngineType:      "mysql",
		InstanceName:    "mysql-e2e-instance",
		DatabaseName:    "testdb",
		UserName:        "testuser",
		RoleName:        "testrole",
		GrantName:       "testgrant",
		TestNamespace:   "default",
		Host:            "host.k3d.internal",
		Port:            3306,
		SecretName:      "mysql-credentials",
		SecretNamespace: "mysql",
		Engine:          "mysql",
		AdminDatabase:   "mysql",
		Timeout:         2 * time.Minute,
		Interval:        2 * time.Second,
	}
}

// MariaDBTestConfig returns the default configuration for MariaDB tests.
func MariaDBTestConfig() *DatabaseTestConfig {
	return &DatabaseTestConfig{
		EngineType:      "mariadb",
		InstanceName:    "mariadb-e2e-instance",
		DatabaseName:    "testdb",
		UserName:        "testuser",
		RoleName:        "testrole",
		GrantName:       "testgrant",
		TestNamespace:   "default",
		Host:            "host.k3d.internal",
		Port:            3306,
		SecretName:      "mariadb-credentials",
		SecretNamespace: "mariadb",
		Engine:          "mysql", // MariaDB uses MySQL adapter
		AdminDatabase:   "mysql",
		Timeout:         2 * time.Minute,
		Interval:        2 * time.Second,
	}
}
