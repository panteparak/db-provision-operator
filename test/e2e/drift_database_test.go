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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/db-provision-operator/test/e2e/testutil"
)

var _ = Describe("drift/database", Ordered, func() {
	const (
		instanceName = "drift-db-instance"
		namespace    = driftTestNamespace
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	BeforeAll(func() {
		verifier, _, _ = setupDriftVerifier(ctx)
		createNamespacedInstance(ctx, instanceName, namespace)
	})

	Context("detect mode", func() {
		It("should detect missing schema drift", func() {
			const crName = "drift-db-schema-detect"
			const dbName = "drift_db_schema_detect"

			By("creating Database with schemas and driftPolicy.mode: detect")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				DriftMode:          "detect",
				DriftInterval:      "10s",
				Schemas: []map[string]interface{}{
					{"name": "app_schema"},
					{"name": "data_schema"},
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, driftReconcileTimeout)

			By("verifying both schemas exist")
			Eventually(func() bool {
				exists1, err1 := verifier.SchemaExistsInDatabase(ctx, "app_schema", dbName)
				exists2, err2 := verifier.SchemaExistsInDatabase(ctx, "data_schema", dbName)
				return err1 == nil && err2 == nil && exists1 && exists2
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue(), "Both schemas should exist")

			By("introducing drift: dropping data_schema")
			err = verifier.ExecOnDatabase(ctx, dbName, "DROP SCHEMA data_schema CASCADE")
			Expect(err).NotTo(HaveOccurred())

			By("verifying schema is gone")
			exists, err := verifier.SchemaExistsInDatabase(ctx, "data_schema", dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "data_schema should be dropped")

			By("waiting for operator to detect schema drift in status")
			waitForDriftDetected(databaseGVR, crName, namespace, driftTimeout)

			By("verifying schema is still missing (detect mode does NOT correct)")
			exists, err = verifier.SchemaExistsInDatabase(ctx, "data_schema", dbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeFalse(), "Detect mode should not recreate schema")

			By("cleanup")
			cleanupCR(databaseGVR, crName, namespace)
		})

		It("should detect extension drift", func() {
			const crName = "drift-db-ext-detect"
			const dbName = "drift_db_ext_detect"

			By("creating Database with pg_trgm extension and detect mode")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				DriftMode:          "detect",
				DriftInterval:      "10s",
			})
			// Add extension to the postgres config
			spec := db.Object["spec"].(map[string]interface{})
			spec["postgres"] = map[string]interface{}{
				"extensions": []interface{}{
					map[string]interface{}{
						"name": "pg_trgm",
					},
				},
			}
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, driftReconcileTimeout)

			By("verifying extension exists")
			Eventually(func() bool {
				exists, err := verifier.ExtensionExistsInDatabase(ctx, "pg_trgm", dbName)
				return err == nil && exists
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue(), "pg_trgm should be installed")

			By("introducing drift: dropping extension")
			err = verifier.ExecOnDatabase(ctx, dbName, "DROP EXTENSION IF EXISTS pg_trgm")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(databaseGVR, crName, namespace, driftTimeout)

			By("cleanup")
			cleanupCR(databaseGVR, crName, namespace)
		})
	})

	Context("correct mode", func() {
		It("should auto-correct missing schema", func() {
			const crName = "drift-db-schema-correct"
			const dbName = "drift_db_schema_correct"

			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				DriftMode:          "correct",
				DriftInterval:      "10s",
				Schemas: []map[string]interface{}{
					{"name": "app_schema"},
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, driftReconcileTimeout)

			By("verifying schema exists")
			Eventually(func() bool {
				exists, err := verifier.SchemaExistsInDatabase(ctx, "app_schema", dbName)
				return err == nil && exists
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: dropping app_schema")
			err = verifier.ExecOnDatabase(ctx, dbName, "DROP SCHEMA app_schema CASCADE")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct schema drift (recreate schema)")
			Eventually(func() bool {
				exists, err := verifier.SchemaExistsInDatabase(ctx, "app_schema", dbName)
				return err == nil && exists
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should recreate app_schema (auto-correct)")

			By("cleanup")
			cleanupCR(databaseGVR, crName, namespace)
		})

		It("should auto-correct missing extension", func() {
			const crName = "drift-db-ext-correct"
			const dbName = "drift_db_ext_correct"

			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				DriftMode:          "correct",
				DriftInterval:      "10s",
			})
			spec := db.Object["spec"].(map[string]interface{})
			spec["postgres"] = map[string]interface{}{
				"extensions": []interface{}{
					map[string]interface{}{
						"name": "pg_trgm",
					},
				},
			}
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, driftReconcileTimeout)

			By("verifying extension is installed")
			Eventually(func() bool {
				exists, err := verifier.ExtensionExistsInDatabase(ctx, "pg_trgm", dbName)
				return err == nil && exists
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: dropping extension")
			err = verifier.ExecOnDatabase(ctx, dbName, "DROP EXTENSION IF EXISTS pg_trgm")
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct extension drift")
			Eventually(func() bool {
				exists, err := verifier.ExtensionExistsInDatabase(ctx, "pg_trgm", dbName)
				return err == nil && exists
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should reinstall pg_trgm (auto-correct)")

			By("cleanup")
			cleanupCR(databaseGVR, crName, namespace)
		})
	})

	Context("ignore mode", func() {
		It("should not detect schema drift in ignore mode", func() {
			const crName = "drift-db-schema-ignore"
			const dbName = "drift_db_schema_ignore"

			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				DriftMode:          "ignore",
				DriftInterval:      "10s",
				Schemas: []map[string]interface{}{
					{"name": "app_schema"},
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: dropping schema")
			Eventually(func() bool {
				exists, err := verifier.SchemaExistsInDatabase(ctx, "app_schema", dbName)
				return err == nil && exists
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			err = verifier.ExecOnDatabase(ctx, dbName, "DROP SCHEMA app_schema CASCADE")
			Expect(err).NotTo(HaveOccurred())

			By("verifying no drift is detected (Consistently for 20s)")
			assertNoDriftDetected(databaseGVR, crName, namespace, 20*time.Second)

			By("cleanup")
			cleanupCR(databaseGVR, crName, namespace)
		})
	})

	AfterAll(func() {
		if verifier != nil {
			_ = verifier.Close()
		}

		deletionTimeout := getDeletionTimeout()

		// Sweep any leftover child resources (handles test-failure scenarios)
		// Database CRs default to deletionProtection=true, so add force-delete annotation first.
		By("sweeping leftover child resources")
		addForceDeleteToAll(ctx, databaseGVR, namespace)
		_ = dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		_ = dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		_ = dynamicClient.Resource(databaseUserGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		_ = dynamicClient.Resource(databaseGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})

		By("waiting for child resources to be removed")
		Eventually(func() bool {
			grants, _ := dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			roles, _ := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			users, _ := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			dbs, _ := dynamicClient.Resource(databaseGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			return len(grants.Items) == 0 && len(roles.Items) == 0 && len(users.Items) == 0 && len(dbs.Items) == 0
		}, deletionTimeout, driftPollingInterval).Should(BeTrue(), "child resources should be deleted")

		By("deleting DatabaseInstance and waiting")
		_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Delete(ctx, instanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Get(ctx, instanceName, metav1.GetOptions{})
			return err != nil
		}, deletionTimeout, driftPollingInterval).Should(BeTrue(), "DatabaseInstance should be deleted")
	})
})
