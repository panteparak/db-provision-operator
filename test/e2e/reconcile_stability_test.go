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

var _ = Describe("reconcile/stability", Ordered, func() {
	const (
		instanceName = "stability-instance"
		namespace    = driftTestNamespace
	)

	ctx := context.Background()

	BeforeAll(func() {
		_, _, _ = setupDriftVerifier(ctx)
		createNamespacedInstance(ctx, instanceName, namespace)
	})

	Context("steady-state resources should not re-reconcile", func() {
		It("DatabaseUser should not enter reconcile loop", func() {
			const crName = "stability-user"
			const pgUserName = "stability_user"

			By("creating DatabaseUser and waiting for Ready")
			user := testutil.BuildDatabaseUser(crName, namespace, instanceName, pgUserName)
			_, err := dynamicClient.Resource(databaseUserGVR).Namespace(namespace).Create(ctx, user, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			waitForReady(databaseUserGVR, crName, namespace, driftReconcileTimeout)

			By("asserting reconcileID remains stable for 30s (no reconcile loop)")
			assertNoReconcileLoop(databaseUserGVR, crName, namespace, 30*time.Second)

			By("cleanup")
			addForceDeleteAnnotation(ctx, databaseUserGVR, namespace, crName)
			cleanupCR(databaseUserGVR, crName, namespace)
		})

		It("Database should not enter reconcile loop", func() {
			const crName = "stability-db"
			const dbName = "stability_db"

			By("creating Database and waiting for Ready")
			db := testutil.BuildDatabase(crName, namespace, instanceName, dbName)
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			waitForReady(databaseGVR, crName, namespace, driftReconcileTimeout)

			By("asserting reconcileID remains stable for 30s (no reconcile loop)")
			assertNoReconcileLoop(databaseGVR, crName, namespace, 30*time.Second)

			By("cleanup")
			addForceDeleteAnnotation(ctx, databaseGVR, namespace, crName)
			cleanupCR(databaseGVR, crName, namespace)
		})

		It("DatabaseRole should not enter reconcile loop", func() {
			const crName = "stability-role"
			const roleName = "stability_role"

			By("creating DatabaseRole and waiting for Ready")
			role := testutil.BuildDatabaseRole(crName, namespace, instanceName, roleName)
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("asserting reconcileID remains stable for 30s (no reconcile loop)")
			assertNoReconcileLoop(databaseRoleGVR, crName, namespace, 30*time.Second)

			By("cleanup")
			addForceDeleteAnnotation(ctx, databaseRoleGVR, namespace, crName)
			cleanupCR(databaseRoleGVR, crName, namespace)
		})

		It("DatabaseInstance should not enter reconcile loop", func() {
			// The instance created in BeforeAll is already Ready — verify it's stable
			By("asserting reconcileID remains stable for 30s (no reconcile loop)")
			assertNoReconcileLoop(databaseInstanceGVR, instanceName, namespace, 30*time.Second)
		})
	})

	AfterAll(func() {
		deletionTimeout := getDeletionTimeout()

		By("sweeping leftover child resources")
		addForceDeleteToAll(ctx, databaseGrantGVR, namespace)
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
		addForceDeleteAnnotation(ctx, databaseInstanceGVR, namespace, instanceName)
		_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Delete(ctx, instanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Get(ctx, instanceName, metav1.GetOptions{})
			return err != nil
		}, deletionTimeout, driftPollingInterval).Should(BeTrue(), "DatabaseInstance should be deleted")
	})
})
