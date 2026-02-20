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

var _ = Describe("drift/grant", Ordered, func() {
	const (
		instanceName     = "drift-grant-instance"
		namespace        = driftTestNamespace
		grantDbCRName    = "drift-grant-db"
		grantDbName      = "drift_grant_db"
		grantRoleCRName  = "drift-grant-role"
		grantPgRoleName  = "drift_grant_role"
		memberRoleCRName = "drift-grant-member"
		memberPgRoleName = "drift_grant_member"
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	BeforeAll(func() {
		verifier, _, _ = setupDriftVerifier(ctx)
		createNamespacedInstance(ctx, instanceName, namespace)

		By("creating a Database for grant tests")
		db := testutil.BuildDatabaseWithOptions(grantDbCRName, namespace, instanceName, grantDbName, testutil.DatabaseBuildOptions{
			DeletionProtection: false,
			DeletionPolicy:     "Delete",
		})
		_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		waitForReady(databaseGVR, grantDbCRName, namespace, driftReconcileTimeout)

		By("creating the grantee role")
		granteeRole := testutil.BuildDatabaseRoleWithOptions(grantRoleCRName, namespace, instanceName, grantPgRoleName, testutil.RoleBuildOptions{})
		_, err = dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, granteeRole, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		waitForReady(databaseRoleGVR, grantRoleCRName, namespace, driftReconcileTimeout)

		By("creating a member role for role membership tests")
		memberRole := testutil.BuildDatabaseRoleWithOptions(memberRoleCRName, namespace, instanceName, memberPgRoleName, testutil.RoleBuildOptions{})
		_, err = dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, memberRole, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		waitForReady(databaseRoleGVR, memberRoleCRName, namespace, driftReconcileTimeout)
	})

	Context("detect mode", func() {
		It("should detect revoked privilege", func() {
			const crName = "drift-grant-detect-priv"

			By("creating DatabaseGrant with CONNECT+TEMPORARY on database, detect mode")
			grant := testutil.BuildDatabaseGrantWithOptions(crName, namespace, "role", grantRoleCRName, testutil.GrantBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				Postgres: map[string]interface{}{
					"grants": []interface{}{
						map[string]interface{}{
							"database":   grantDbName,
							"privileges": []interface{}{"CONNECT", "TEMPORARY"},
						},
					},
				},
			})
			_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGrantGVR, crName, namespace, driftReconcileTimeout)

			By("verifying TEMPORARY privilege exists")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, grantPgRoleName, "TEMPORARY", "database", grantDbName)
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: revoking TEMPORARY privilege via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE %s FROM %s", grantDbName, grantPgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(databaseGrantGVR, crName, namespace, driftTimeout)

			By("verifying TEMPORARY is still revoked (detect mode)")
			hasPriv, err := verifier.HasPrivilege(ctx, grantPgRoleName, "TEMPORARY", "database", grantDbName)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasPriv).To(BeFalse(), "Detect mode should not re-grant privilege")

			By("cleanup")
			cleanupCR(databaseGrantGVR, crName, namespace)
		})

		It("should detect role membership drift", func() {
			const crName = "drift-grant-detect-member"

			By("creating DatabaseGrant with role membership, detect mode")
			grant := testutil.BuildDatabaseGrantWithOptions(crName, namespace, "role", grantRoleCRName, testutil.GrantBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				Postgres: map[string]interface{}{
					"roles": []interface{}{memberPgRoleName},
				},
			})
			_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGrantGVR, crName, namespace, driftReconcileTimeout)

			By("verifying role membership exists")
			Eventually(func() bool {
				hasMembership, err := verifier.HasRoleMembership(ctx, grantPgRoleName, memberPgRoleName)
				return err == nil && hasMembership
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue(), "Grantee should be member of member role")

			By("introducing drift: revoking role membership via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE %s FROM %s", memberPgRoleName, grantPgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(databaseGrantGVR, crName, namespace, driftTimeout)

			By("cleanup")
			cleanupCR(databaseGrantGVR, crName, namespace)
		})
	})

	Context("correct mode", func() {
		It("should auto-correct revoked privilege", func() {
			const crName = "drift-grant-correct-priv"

			grant := testutil.BuildDatabaseGrantWithOptions(crName, namespace, "role", grantRoleCRName, testutil.GrantBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				Postgres: map[string]interface{}{
					"grants": []interface{}{
						map[string]interface{}{
							"database":   grantDbName,
							"privileges": []interface{}{"CONNECT", "TEMPORARY"},
						},
					},
				},
			})
			_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGrantGVR, crName, namespace, driftReconcileTimeout)

			By("verifying TEMPORARY privilege exists")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, grantPgRoleName, "TEMPORARY", "database", grantDbName)
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: revoking TEMPORARY via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE %s FROM %s", grantDbName, grantPgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct (re-grant TEMPORARY)")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, grantPgRoleName, "TEMPORARY", "database", grantDbName)
				return err == nil && hasPriv
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should re-grant TEMPORARY (auto-correct)")

			By("cleanup")
			cleanupCR(databaseGrantGVR, crName, namespace)
		})

		It("should auto-correct revoked role membership", func() {
			const crName = "drift-grant-correct-member"

			grant := testutil.BuildDatabaseGrantWithOptions(crName, namespace, "role", grantRoleCRName, testutil.GrantBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				Postgres: map[string]interface{}{
					"roles": []interface{}{memberPgRoleName},
				},
			})
			_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGrantGVR, crName, namespace, driftReconcileTimeout)

			By("verifying membership exists")
			Eventually(func() bool {
				hasMembership, err := verifier.HasRoleMembership(ctx, grantPgRoleName, memberPgRoleName)
				return err == nil && hasMembership
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: revoking membership")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE %s FROM %s", memberPgRoleName, grantPgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct (re-grant membership)")
			Eventually(func() bool {
				hasMembership, err := verifier.HasRoleMembership(ctx, grantPgRoleName, memberPgRoleName)
				return err == nil && hasMembership
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should restore role membership (auto-correct)")

			By("cleanup")
			cleanupCR(databaseGrantGVR, crName, namespace)
		})
	})

	Context("ignore mode", func() {
		It("should not detect privilege drift in ignore mode", func() {
			const crName = "drift-grant-ignore"

			grant := testutil.BuildDatabaseGrantWithOptions(crName, namespace, "role", grantRoleCRName, testutil.GrantBuildOptions{
				DriftMode:     "ignore",
				DriftInterval: "10s",
				Postgres: map[string]interface{}{
					"grants": []interface{}{
						map[string]interface{}{
							"database":   grantDbName,
							"privileges": []interface{}{"CONNECT", "TEMPORARY"},
						},
					},
				},
			})
			_, err := dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGrantGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: revoking TEMPORARY")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, grantPgRoleName, "TEMPORARY", "database", grantDbName)
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE %s FROM %s", grantDbName, grantPgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying no drift is detected (Consistently for 20s)")
			assertNoDriftDetected(databaseGrantGVR, crName, namespace, 20*time.Second)

			By("cleanup")
			cleanupCR(databaseGrantGVR, crName, namespace)
		})
	})

	AfterAll(func() {
		if verifier != nil {
			_ = verifier.Close()
		}

		By("cleaning up grant test resources")
		_ = dynamicClient.Resource(databaseGrantGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{})
		_ = dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Delete(ctx, memberRoleCRName, metav1.DeleteOptions{})
		_ = dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Delete(ctx, grantRoleCRName, metav1.DeleteOptions{})
		_ = dynamicClient.Resource(databaseGVR).Namespace(namespace).Delete(ctx, grantDbCRName, metav1.DeleteOptions{})
		_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Delete(ctx, instanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Get(ctx, instanceName, metav1.GetOptions{})
			return err != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())
	})
})
