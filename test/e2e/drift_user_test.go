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

	"github.com/db-provision-operator/test/e2e/testutil"
)

var _ = Describe("drift/user", Ordered, func() {
	const (
		instanceName = "drift-user-instance"
		namespace    = driftTestNamespace
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	BeforeAll(func() {
		verifier, _, _ = setupDriftVerifier(ctx)
		createNamespacedInstance(ctx, instanceName, namespace)
	})

	Context("detect mode", func() {
		It("should detect connectionLimit drift", func() {
			const crName = "drift-user-connlimit-detect"
			const pgUserName = "drift_user_connlimit_detect"

			By("creating DatabaseUser with connectionLimit: 10 and detect mode")
			user := testutil.BuildDatabaseUserWithOptions(crName, namespace, instanceName, pgUserName, testutil.UserBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"connectionLimit": int64(10),
				},
			})
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("verifying connectionLimit is 10")
			Eventually(func() bool {
				limit, err := verifier.GetConnectionLimit(ctx, pgUserName)
				return err == nil && limit == 10
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue(), "connectionLimit should be 10")

			By("introducing drift: changing connectionLimit via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER USER %s CONNECTION LIMIT 999", pgUserName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying manual change took effect")
			limit, err := verifier.GetConnectionLimit(ctx, pgUserName)
			Expect(err).NotTo(HaveOccurred())
			Expect(limit).To(Equal(int32(999)), "Connection limit should be 999 after manual change")

			By("waiting for drift detection")
			waitForDriftDetected(databaseUserGVR, crName, namespace, driftTimeout)

			By("verifying connectionLimit is still 999 (detect mode does NOT correct)")
			limit, err = verifier.GetConnectionLimit(ctx, pgUserName)
			Expect(err).NotTo(HaveOccurred())
			Expect(limit).To(Equal(int32(999)), "Detect mode should not correct connectionLimit")

			By("cleanup")
			cleanupCR(databaseUserGVR, crName, namespace)
		})

		It("should detect createDB drift", func() {
			const crName = "drift-user-createdb-detect"
			const pgUserName = "drift_user_createdb_detect"

			user := testutil.BuildDatabaseUserWithOptions(crName, namespace, instanceName, pgUserName, testutil.UserBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: granting CREATEDB")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER USER %s CREATEDB", pgUserName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(databaseUserGVR, crName, namespace, driftTimeout)

			By("cleanup")
			cleanupCR(databaseUserGVR, crName, namespace)
		})
	})

	Context("correct mode", func() {
		It("should auto-correct connectionLimit drift", func() {
			const crName = "drift-user-connlimit-correct"
			const pgUserName = "drift_user_connlimit_correct"

			user := testutil.BuildDatabaseUserWithOptions(crName, namespace, instanceName, pgUserName, testutil.UserBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"connectionLimit": int64(10),
				},
			})
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: changing connectionLimit")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER USER %s CONNECTION LIMIT 999", pgUserName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct connectionLimit back to 10")
			Eventually(func() bool {
				limit, err := verifier.GetConnectionLimit(ctx, pgUserName)
				return err == nil && limit == 10
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should restore connectionLimit to 10")

			By("cleanup")
			cleanupCR(databaseUserGVR, crName, namespace)
		})

		It("should auto-correct createDB drift", func() {
			const crName = "drift-user-createdb-correct"
			const pgUserName = "drift_user_createdb_correct"

			user := testutil.BuildDatabaseUserWithOptions(crName, namespace, instanceName, pgUserName, testutil.UserBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: granting CREATEDB")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER USER %s CREATEDB", pgUserName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct (revoke CREATEDB)")
			Eventually(func() bool {
				hasCreateDb, err := verifier.RoleHasCreateDb(ctx, pgUserName)
				return err == nil && !hasCreateDb
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should revoke CREATEDB")

			By("cleanup")
			cleanupCR(databaseUserGVR, crName, namespace)
		})

		It("should auto-correct multi-attribute drift", func() {
			const crName = "drift-user-multi-correct"
			const pgUserName = "drift_user_multi_correct"

			user := testutil.BuildDatabaseUserWithOptions(crName, namespace, instanceName, pgUserName, testutil.UserBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB":        false,
					"connectionLimit": int64(10),
				},
			})
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("introducing multi-attribute drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER USER %s CREATEDB CONNECTION LIMIT 999", pgUserName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct both attributes")
			Eventually(func() bool {
				hasCreateDb, err1 := verifier.RoleHasCreateDb(ctx, pgUserName)
				limit, err2 := verifier.GetConnectionLimit(ctx, pgUserName)
				return err1 == nil && err2 == nil && !hasCreateDb && limit == 10
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should correct both CREATEDB and connectionLimit")

			By("cleanup")
			cleanupCR(databaseUserGVR, crName, namespace)
		})
	})

	Context("ignore mode", func() {
		It("should not detect drift in ignore mode", func() {
			const crName = "drift-user-ignore"
			const pgUserName = "drift_user_ignore"

			user := testutil.BuildDatabaseUserWithOptions(crName, namespace, instanceName, pgUserName, testutil.UserBuildOptions{
				DriftMode:     "ignore",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"connectionLimit": int64(10),
				},
			})
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: changing connectionLimit")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER USER %s CONNECTION LIMIT 999", pgUserName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying no drift is detected (Consistently for 20s)")
			assertNoDriftDetected(databaseUserGVR, crName, namespace, 20*time.Second)

			By("cleanup")
			cleanupCR(databaseUserGVR, crName, namespace)
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
