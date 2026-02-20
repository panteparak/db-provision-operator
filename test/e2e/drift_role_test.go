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

var _ = Describe("drift/role", Ordered, func() {
	const (
		instanceName = "drift-role-instance"
		namespace    = driftTestNamespace
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	BeforeAll(func() {
		var err error
		verifier, _, _ = setupDriftVerifier(ctx)
		_ = err
		createNamespacedInstance(ctx, instanceName, namespace)
	})

	Context("detect mode", func() {
		It("should detect single attribute drift", func() {
			const crName = "drift-role-detect-single"
			const pgRoleName = "drift_role_detect_single"

			By("creating DatabaseRole with driftPolicy.mode: detect")
			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for DatabaseRole to become Ready")
			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("verifying role does NOT have CREATEDB")
			hasCreateDb, err := verifier.RoleHasCreateDb(ctx, pgRoleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasCreateDb).To(BeFalse(), "Role should not have CREATEDB initially")

			By("introducing drift: granting CREATEDB directly via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying manual change took effect")
			hasCreateDb, err = verifier.RoleHasCreateDb(ctx, pgRoleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasCreateDb).To(BeTrue(), "Manual ALTER ROLE should grant CREATEDB")

			By("waiting for operator to detect drift in CR status")
			waitForDriftDetected(databaseRoleGVR, crName, namespace, driftTimeout)

			By("verifying CREATEDB is still true (detect mode does NOT correct)")
			hasCreateDb, err = verifier.RoleHasCreateDb(ctx, pgRoleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasCreateDb).To(BeTrue(), "Detect mode should not correct drift")

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})

		It("should detect multi-attribute drift", func() {
			const crName = "drift-role-detect-multi"
			const pgRoleName = "drift_role_detect_multi"

			By("creating DatabaseRole with driftPolicy.mode: detect, no createDB/createRole")
			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB":   false,
					"createRole": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("introducing multi-attribute drift via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB CREATEROLE", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(databaseRoleGVR, crName, namespace, driftTimeout)

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})

		It("should report correct drift diffs content", func() {
			const crName = "drift-role-diffs"
			const pgRoleName = "drift_role_diffs"

			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "detect",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			waitForDriftDetected(databaseRoleGVR, crName, namespace, driftTimeout)

			By("verifying drift diffs content")
			Eventually(func() bool {
				diffs, err := getDriftDiffs(databaseRoleGVR, crName, namespace)
				if err != nil || len(diffs) == 0 {
					return false
				}
				for _, d := range diffs {
					field, _ := d["field"].(string)
					if field != "" {
						expected, _ := d["expected"].(string)
						actual, _ := d["actual"].(string)
						if expected != "" && actual != "" {
							return true
						}
					}
				}
				return false
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Drift diffs should contain field, expected, and actual values")

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})
	})

	Context("correct mode", func() {
		It("should auto-correct single attribute drift", func() {
			const crName = "drift-role-correct-single"
			const pgRoleName = "drift_role_correct_single"

			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: granting CREATEDB directly via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct drift (revoke CREATEDB)")
			Eventually(func() bool {
				hasCreateDb, err := verifier.RoleHasCreateDb(ctx, pgRoleName)
				return err == nil && !hasCreateDb
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should revoke CREATEDB (auto-correct)")

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})

		It("should auto-correct multi-attribute drift", func() {
			const crName = "drift-role-correct-multi"
			const pgRoleName = "drift_role_correct_multi"

			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "correct",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB":   false,
					"createRole": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("introducing multi-attribute drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB CREATEROLE", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for operator to auto-correct both attributes")
			Eventually(func() bool {
				hasCreateDb, err1 := verifier.RoleHasCreateDb(ctx, pgRoleName)
				hasCreateRole, err2 := verifier.RoleHasCreateRole(ctx, pgRoleName)
				return err1 == nil && err2 == nil && !hasCreateDb && !hasCreateRole
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should revoke both CREATEDB and CREATEROLE")

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})
	})

	Context("ignore mode", func() {
		It("should not detect drift in ignore mode", func() {
			const crName = "drift-role-ignore"
			const pgRoleName = "drift_role_ignore"

			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "ignore",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift: granting CREATEDB directly via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying no drift is detected (Consistently for 20s)")
			assertNoDriftDetected(databaseRoleGVR, crName, namespace, 20*time.Second)

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})
	})

	Context("mode transition", func() {
		It("should detect drift after switching from ignore to detect mode", func() {
			const crName = "drift-role-transition"
			const pgRoleName = "drift_role_transition"

			By("creating role in ignore mode")
			role := testutil.BuildDatabaseRoleWithOptions(crName, namespace, instanceName, pgRoleName, testutil.RoleBuildOptions{
				DriftMode:     "ignore",
				DriftInterval: "10s",
				PostgresConfig: map[string]interface{}{
					"createDB": false,
				},
			})
			_, err := dynamicClient.Resource(databaseRoleGVR).Namespace(namespace).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseRoleGVR, crName, namespace, driftReconcileTimeout)

			By("introducing drift while in ignore mode")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("switching drift mode from ignore to detect")
			updateCRSpec(databaseRoleGVR, crName, namespace, func(spec map[string]interface{}) {
				spec["driftPolicy"] = map[string]interface{}{
					"mode":     "detect",
					"interval": "10s",
				}
			})

			By("waiting for drift to be detected after mode change")
			waitForDriftDetected(databaseRoleGVR, crName, namespace, driftTimeout)

			By("cleanup")
			cleanupCR(databaseRoleGVR, crName, namespace)
		})
	})

	AfterAll(func() {
		By("closing verifier connection")
		if verifier != nil {
			_ = verifier.Close()
		}

		By("cleaning up drift/role instance")
		_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Delete(ctx, instanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Get(ctx, instanceName, metav1.GetOptions{})
			return err != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())
	})
})
