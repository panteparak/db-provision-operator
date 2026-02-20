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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/db-provision-operator/test/e2e/testutil"
)

var _ = Describe("drift/clusterrole", Ordered, func() {
	const (
		clusterInstanceName = "drift-clusterrole-instance"
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	BeforeAll(func() {
		verifier, _, _ = setupDriftVerifier(ctx)
		createClusterInstance(ctx, clusterInstanceName)
	})

	// buildClusterRole creates a ClusterDatabaseRole unstructured object with drift policy.
	buildClusterRole := func(name, pgRoleName, driftMode, driftInterval string, postgresConfig map[string]interface{}) *unstructured.Unstructured {
		spec := map[string]interface{}{
			"clusterInstanceRef": map[string]interface{}{
				"name": clusterInstanceName,
			},
			"roleName": pgRoleName,
		}

		if driftMode != "" {
			driftPolicy := map[string]interface{}{
				"mode": driftMode,
			}
			if driftInterval != "" {
				driftPolicy["interval"] = driftInterval
			}
			spec["driftPolicy"] = driftPolicy
		}

		if len(postgresConfig) > 0 {
			spec["postgres"] = postgresConfig
		}

		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dbops.dbprovision.io/v1alpha1",
				"kind":       "ClusterDatabaseRole",
				"metadata": map[string]interface{}{
					"name": name,
				},
				"spec": spec,
			},
		}
	}

	Context("detect mode", func() {
		It("should detect attribute drift", func() {
			const crName = "drift-crole-detect"
			const pgRoleName = "drift_crole_detect"

			By("creating ClusterDatabaseRole with detect mode")
			role := buildClusterRole(crName, pgRoleName, "detect", "10s", map[string]interface{}{
				"createDB": false,
			})
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseRoleGVR, crName, "", driftReconcileTimeout)

			By("introducing drift: granting CREATEDB via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(clusterDatabaseRoleGVR, crName, "", driftTimeout)

			By("verifying CREATEDB is still true (detect mode)")
			hasCreateDb, err := verifier.RoleHasCreateDb(ctx, pgRoleName)
			Expect(err).NotTo(HaveOccurred())
			Expect(hasCreateDb).To(BeTrue(), "Detect mode should not correct drift")

			By("cleanup")
			cleanupCR(clusterDatabaseRoleGVR, crName, "")
		})

		It("should report correct drift diffs content", func() {
			const crName = "drift-crole-diffs"
			const pgRoleName = "drift_crole_diffs"

			role := buildClusterRole(crName, pgRoleName, "detect", "10s", map[string]interface{}{
				"createDB": false,
			})
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseRoleGVR, crName, "", driftReconcileTimeout)

			By("introducing drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			waitForDriftDetected(clusterDatabaseRoleGVR, crName, "", driftTimeout)

			By("verifying diffs contain field/expected/actual")
			Eventually(func() bool {
				diffs, err := getDriftDiffs(clusterDatabaseRoleGVR, crName, "")
				if err != nil || len(diffs) == 0 {
					return false
				}
				for _, d := range diffs {
					field, _ := d["field"].(string)
					expected, _ := d["expected"].(string)
					actual, _ := d["actual"].(string)
					if field != "" && expected != "" && actual != "" {
						return true
					}
				}
				return false
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Diffs should contain field, expected, and actual")

			By("cleanup")
			cleanupCR(clusterDatabaseRoleGVR, crName, "")
		})
	})

	Context("correct mode", func() {
		It("should auto-correct attribute drift", func() {
			const crName = "drift-crole-correct"
			const pgRoleName = "drift_crole_correct"

			role := buildClusterRole(crName, pgRoleName, "correct", "10s", map[string]interface{}{
				"createDB": false,
			})
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseRoleGVR, crName, "", driftReconcileTimeout)

			By("introducing drift: granting CREATEDB")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for auto-correction")
			Eventually(func() bool {
				hasCreateDb, err := verifier.RoleHasCreateDb(ctx, pgRoleName)
				return err == nil && !hasCreateDb
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should revoke CREATEDB")

			By("cleanup")
			cleanupCR(clusterDatabaseRoleGVR, crName, "")
		})

		It("should auto-correct multi-attribute drift", func() {
			const crName = "drift-crole-correct-multi"
			const pgRoleName = "drift_crole_correct_multi"

			role := buildClusterRole(crName, pgRoleName, "correct", "10s", map[string]interface{}{
				"createDB":   false,
				"createRole": false,
			})
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseRoleGVR, crName, "", driftReconcileTimeout)

			By("introducing multi-attribute drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB CREATEROLE", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for auto-correction of both attributes")
			Eventually(func() bool {
				hasCreateDb, err1 := verifier.RoleHasCreateDb(ctx, pgRoleName)
				hasCreateRole, err2 := verifier.RoleHasCreateRole(ctx, pgRoleName)
				return err1 == nil && err2 == nil && !hasCreateDb && !hasCreateRole
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Both CREATEDB and CREATEROLE should be revoked")

			By("cleanup")
			cleanupCR(clusterDatabaseRoleGVR, crName, "")
		})
	})

	Context("ignore mode", func() {
		It("should not detect drift in ignore mode", func() {
			const crName = "drift-crole-ignore"
			const pgRoleName = "drift_crole_ignore"

			role := buildClusterRole(crName, pgRoleName, "ignore", "10s", map[string]interface{}{
				"createDB": false,
			})
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseRoleGVR, crName, "", driftReconcileTimeout)

			By("introducing drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying no drift detected (Consistently for 20s)")
			assertNoDriftDetected(clusterDatabaseRoleGVR, crName, "", 20*time.Second)

			By("cleanup")
			cleanupCR(clusterDatabaseRoleGVR, crName, "")
		})
	})

	Context("mode transition", func() {
		It("should correct drift after switching from detect to correct mode", func() {
			const crName = "drift-crole-transition"
			const pgRoleName = "drift_crole_transition"

			By("creating role in detect mode")
			role := buildClusterRole(crName, pgRoleName, "detect", "10s", map[string]interface{}{
				"createDB": false,
			})
			_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, role, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseRoleGVR, crName, "", driftReconcileTimeout)

			By("introducing drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("ALTER ROLE %s CREATEDB", pgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(clusterDatabaseRoleGVR, crName, "", driftTimeout)

			By("switching from detect to correct mode")
			updateCRSpec(clusterDatabaseRoleGVR, crName, "", func(spec map[string]interface{}) {
				spec["driftPolicy"] = map[string]interface{}{
					"mode":     "correct",
					"interval": "10s",
				}
			})

			By("waiting for auto-correction after mode switch")
			Eventually(func() bool {
				hasCreateDb, err := verifier.RoleHasCreateDb(ctx, pgRoleName)
				return err == nil && !hasCreateDb
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Drift should be corrected after mode switch to correct")

			By("cleanup")
			cleanupCR(clusterDatabaseRoleGVR, crName, "")
		})
	})

	AfterAll(func() {
		if verifier != nil {
			_ = verifier.Close()
		}
		_ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, clusterInstanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
			return err != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())
	})
})
