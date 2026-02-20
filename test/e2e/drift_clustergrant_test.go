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

var _ = Describe("drift/clustergrant", Ordered, func() {
	const (
		clusterInstanceName = "drift-clustergrant-instance"
		granteeRoleCRName   = "drift-cgrant-grantee"
		granteePgRoleName   = "drift_cgrant_grantee"
		memberRoleCRName    = "drift-cgrant-member"
		memberPgRoleName    = "drift_cgrant_member"
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	// buildClusterGrant creates a ClusterDatabaseGrant unstructured object.
	buildClusterGrant := func(name, driftMode, driftInterval string, postgres map[string]interface{}) *unstructured.Unstructured {
		spec := map[string]interface{}{
			"clusterInstanceRef": map[string]interface{}{
				"name": clusterInstanceName,
			},
			"roleRef": map[string]interface{}{
				"name": granteeRoleCRName,
			},
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

		if len(postgres) > 0 {
			spec["postgres"] = postgres
		}

		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dbops.dbprovision.io/v1alpha1",
				"kind":       "ClusterDatabaseGrant",
				"metadata": map[string]interface{}{
					"name": name,
				},
				"spec": spec,
			},
		}
	}

	BeforeAll(func() {
		verifier, _, _ = setupDriftVerifier(ctx)
		createClusterInstance(ctx, clusterInstanceName)

		By("creating grantee ClusterDatabaseRole")
		granteeRole := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dbops.dbprovision.io/v1alpha1",
				"kind":       "ClusterDatabaseRole",
				"metadata": map[string]interface{}{
					"name": granteeRoleCRName,
				},
				"spec": map[string]interface{}{
					"clusterInstanceRef": map[string]interface{}{
						"name": clusterInstanceName,
					},
					"roleName": granteePgRoleName,
					"postgres": map[string]interface{}{
						"login":   true,
						"inherit": true,
					},
				},
			},
		}
		_, err := dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, granteeRole, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		waitForReady(clusterDatabaseRoleGVR, granteeRoleCRName, "", driftReconcileTimeout)

		By("creating member ClusterDatabaseRole for membership tests")
		memberRole := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dbops.dbprovision.io/v1alpha1",
				"kind":       "ClusterDatabaseRole",
				"metadata": map[string]interface{}{
					"name": memberRoleCRName,
				},
				"spec": map[string]interface{}{
					"clusterInstanceRef": map[string]interface{}{
						"name": clusterInstanceName,
					},
					"roleName": memberPgRoleName,
					"postgres": map[string]interface{}{
						"inherit": true,
					},
				},
			},
		}
		_, err = dynamicClient.Resource(clusterDatabaseRoleGVR).Create(ctx, memberRole, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		waitForReady(clusterDatabaseRoleGVR, memberRoleCRName, "", driftReconcileTimeout)
	})

	Context("detect mode", func() {
		It("should detect revoked privilege", func() {
			const crName = "drift-cgrant-detect-priv"

			By("creating ClusterDatabaseGrant with CONNECT+TEMPORARY, detect mode")
			grant := buildClusterGrant(crName, "detect", "10s", map[string]interface{}{
				"grants": []interface{}{
					map[string]interface{}{
						"database":   "postgres",
						"privileges": []interface{}{"CONNECT", "TEMPORARY"},
					},
				},
			})
			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseGrantGVR, crName, "", driftReconcileTimeout)

			By("verifying TEMPORARY privilege exists")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, granteePgRoleName, "TEMPORARY", "database", "postgres")
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: revoking TEMPORARY via SQL")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE postgres FROM %s", granteePgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for drift detection")
			waitForDriftDetected(clusterDatabaseGrantGVR, crName, "", driftTimeout)

			By("cleanup")
			cleanupCR(clusterDatabaseGrantGVR, crName, "")
		})

		It("should report correct drift diffs", func() {
			const crName = "drift-cgrant-diffs"

			grant := buildClusterGrant(crName, "detect", "10s", map[string]interface{}{
				"grants": []interface{}{
					map[string]interface{}{
						"database":   "postgres",
						"privileges": []interface{}{"CONNECT", "TEMPORARY"},
					},
				},
			})
			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseGrantGVR, crName, "", driftReconcileTimeout)

			By("verifying privilege exists")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, granteePgRoleName, "TEMPORARY", "database", "postgres")
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE postgres FROM %s", granteePgRoleName))
			Expect(err).NotTo(HaveOccurred())

			waitForDriftDetected(clusterDatabaseGrantGVR, crName, "", driftTimeout)

			By("verifying diffs contain meaningful content")
			Eventually(func() bool {
				diffs, err := getDriftDiffs(clusterDatabaseGrantGVR, crName, "")
				if err != nil || len(diffs) == 0 {
					return false
				}
				for _, d := range diffs {
					field, _ := d["field"].(string)
					if field != "" {
						return true
					}
				}
				return false
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Diffs should contain field information")

			By("cleanup")
			cleanupCR(clusterDatabaseGrantGVR, crName, "")
		})
	})

	Context("correct mode", func() {
		It("should auto-correct revoked privilege", func() {
			const crName = "drift-cgrant-correct-priv"

			grant := buildClusterGrant(crName, "correct", "10s", map[string]interface{}{
				"grants": []interface{}{
					map[string]interface{}{
						"database":   "postgres",
						"privileges": []interface{}{"CONNECT", "TEMPORARY"},
					},
				},
			})
			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseGrantGVR, crName, "", driftReconcileTimeout)

			By("verifying TEMPORARY privilege exists")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, granteePgRoleName, "TEMPORARY", "database", "postgres")
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: revoking TEMPORARY")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE postgres FROM %s", granteePgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for auto-correction (re-grant TEMPORARY)")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, granteePgRoleName, "TEMPORARY", "database", "postgres")
				return err == nil && hasPriv
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should re-grant TEMPORARY")

			By("cleanup")
			cleanupCR(clusterDatabaseGrantGVR, crName, "")
		})

		It("should auto-correct revoked role membership", func() {
			const crName = "drift-cgrant-correct-member"

			grant := buildClusterGrant(crName, "correct", "10s", map[string]interface{}{
				"roles": []interface{}{memberPgRoleName},
			})
			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseGrantGVR, crName, "", driftReconcileTimeout)

			By("verifying membership exists")
			Eventually(func() bool {
				hasMembership, err := verifier.HasRoleMembership(ctx, granteePgRoleName, memberPgRoleName)
				return err == nil && hasMembership
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			By("introducing drift: revoking membership")
			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE %s FROM %s", memberPgRoleName, granteePgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("waiting for auto-correction (re-grant membership)")
			Eventually(func() bool {
				hasMembership, err := verifier.HasRoleMembership(ctx, granteePgRoleName, memberPgRoleName)
				return err == nil && hasMembership
			}, driftTimeout, driftPollingInterval).Should(BeTrue(), "Operator should restore role membership")

			By("cleanup")
			cleanupCR(clusterDatabaseGrantGVR, crName, "")
		})
	})

	Context("ignore mode", func() {
		It("should not detect drift in ignore mode", func() {
			const crName = "drift-cgrant-ignore"

			grant := buildClusterGrant(crName, "ignore", "10s", map[string]interface{}{
				"grants": []interface{}{
					map[string]interface{}{
						"database":   "postgres",
						"privileges": []interface{}{"CONNECT", "TEMPORARY"},
					},
				},
			})
			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseGrantGVR, crName, "", driftReconcileTimeout)

			By("introducing drift")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, granteePgRoleName, "TEMPORARY", "database", "postgres")
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE postgres FROM %s", granteePgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("verifying no drift detected (Consistently for 20s)")
			assertNoDriftDetected(clusterDatabaseGrantGVR, crName, "", 20*time.Second)

			By("cleanup")
			cleanupCR(clusterDatabaseGrantGVR, crName, "")
		})
	})

	Context("mode transition", func() {
		It("should detect drift after switching from ignore to detect mode", func() {
			const crName = "drift-cgrant-transition"

			By("creating grant in ignore mode")
			grant := buildClusterGrant(crName, "ignore", "10s", map[string]interface{}{
				"grants": []interface{}{
					map[string]interface{}{
						"database":   "postgres",
						"privileges": []interface{}{"CONNECT", "TEMPORARY"},
					},
				},
			})
			_, err := dynamicClient.Resource(clusterDatabaseGrantGVR).Create(ctx, grant, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(clusterDatabaseGrantGVR, crName, "", driftReconcileTimeout)

			By("introducing drift while in ignore mode")
			Eventually(func() bool {
				hasPriv, err := verifier.HasPrivilege(ctx, granteePgRoleName, "TEMPORARY", "database", "postgres")
				return err == nil && hasPriv
			}, driftReconcileTimeout, driftPollingInterval).Should(BeTrue())

			err = verifier.ExecOnDatabase(ctx, "postgres", fmt.Sprintf("REVOKE TEMPORARY ON DATABASE postgres FROM %s", granteePgRoleName))
			Expect(err).NotTo(HaveOccurred())

			By("switching from ignore to detect mode")
			updateCRSpec(clusterDatabaseGrantGVR, crName, "", func(spec map[string]interface{}) {
				spec["driftPolicy"] = map[string]interface{}{
					"mode":     "detect",
					"interval": "10s",
				}
			})

			By("waiting for drift to be detected after mode change")
			waitForDriftDetected(clusterDatabaseGrantGVR, crName, "", driftTimeout)

			By("cleanup")
			cleanupCR(clusterDatabaseGrantGVR, crName, "")
		})
	})

	AfterAll(func() {
		if verifier != nil {
			_ = verifier.Close()
		}

		By("cleaning up cluster grant test resources")
		_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, memberRoleCRName, metav1.DeleteOptions{})
		_ = dynamicClient.Resource(clusterDatabaseRoleGVR).Delete(ctx, granteeRoleCRName, metav1.DeleteOptions{})

		// Wait for roles to be deleted before deleting instance
		Eventually(func() bool {
			_, err1 := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, granteeRoleCRName, metav1.GetOptions{})
			_, err2 := dynamicClient.Resource(clusterDatabaseRoleGVR).Get(ctx, memberRoleCRName, metav1.GetOptions{})
			return err1 != nil && err2 != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())

		_ = dynamicClient.Resource(clusterDatabaseInstanceGVR).Delete(ctx, clusterInstanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, clusterInstanceName, metav1.GetOptions{})
			return err != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())
	})
})
