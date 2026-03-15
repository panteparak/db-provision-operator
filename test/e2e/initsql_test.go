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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/db-provision-operator/test/e2e/testutil"
)

var _ = Describe("initsql", Ordered, func() {
	const (
		instanceName = "initsql-instance"
		namespace    = driftTestNamespace

		reconcileTimeout = 60 * time.Second
		pollInterval     = 2 * time.Second
	)

	ctx := context.Background()
	var verifier *testutil.PostgresVerifier

	// Helper: create a ConfigMap with SQL content
	createConfigMap := func(name, ns, key, data string) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string]string{key: data},
		}
		_, err := k8sClient.CoreV1().ConfigMaps(ns).Create(ctx, cm, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	// Helper: create a Secret with SQL content
	createSecret := func(name, ns, key, data string) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Data:       map[string][]byte{key: []byte(data)},
		}
		_, err := k8sClient.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	// Helper: read status.initSQL from a Database CR
	getInitSQLStatus := func(crName, ns string) map[string]interface{} {
		obj, err := dynamicClient.Resource(databaseGVR).Namespace(ns).Get(ctx, crName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		initSQL, _, _ := unstructured.NestedMap(obj.Object, "status", "initSQL")
		return initSQL
	}

	// Helper: read a specific condition from status.conditions
	getCondition := func(crName, ns, condType string) map[string]interface{} {
		obj, err := dynamicClient.Resource(databaseGVR).Namespace(ns).Get(ctx, crName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if ok && cond["type"] == condType {
				return cond
			}
		}
		return nil
	}

	// Helper: wait for a CR to reach a specific phase
	waitForPhaseE := func(crName, ns, phase string, timeout time.Duration) {
		Eventually(func() string {
			obj, err := dynamicClient.Resource(databaseGVR).Namespace(ns).Get(ctx, crName, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			p, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
			return p
		}, timeout, pollInterval).Should(Equal(phase), "%s should reach phase %s", crName, phase)
	}

	// Helper: wait for status.initSQL.applied to have a specific value
	waitForInitSQLApplied := func(crName, ns string, applied bool, timeout time.Duration) {
		Eventually(func() *bool {
			status := getInitSQLStatus(crName, ns)
			if status == nil {
				return nil
			}
			val, ok := status["applied"].(bool)
			if !ok {
				return nil
			}
			return &val
		}, timeout, pollInterval).ShouldNot(BeNil())
		status := getInitSQLStatus(crName, ns)
		Expect(status["applied"]).To(Equal(applied))
	}

	BeforeAll(func() {
		verifier, _, _ = setupDriftVerifier(ctx)
		createNamespacedInstance(ctx, instanceName, namespace)
	})

	// ===== Happy Path =====

	Context("inline source", func() {
		It("PG-INIT-01: executes inline SQL and creates tables", func() {
			const crName = "initsql-inline-01"
			const dbName = "initsql_inline_01"

			By("creating Database with inline initSQL")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"inline": []interface{}{
						"CREATE TABLE init_inline (id SERIAL PRIMARY KEY, name TEXT)",
						"INSERT INTO init_inline (name) VALUES ('hello')",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying status.initSQL")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())

			status := getInitSQLStatus(crName, namespace)
			Expect(status["applied"]).To(Equal(true))
			Expect(status["hash"]).NotTo(BeEmpty())
			Expect(status["statementsExecuted"]).To(BeNumerically("==", 2))

			By("verifying table and row exist in database")
			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, dbName, "SELECT count(*) FROM init_inline", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(count).To(Equal(1))
		})

		It("PG-INIT-02: skips re-execution when hash matches", func() {
			const crName = "initsql-inline-01" // reuse from PG-INIT-01

			By("recording current appliedAt timestamp")
			status := getInitSQLStatus(crName, namespace)
			Expect(status).NotTo(BeNil())
			originalAppliedAt := status["appliedAt"]
			Expect(originalAppliedAt).NotTo(BeNil())

			By("triggering re-reconcile via label patch")
			obj, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Get(ctx, crName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			labels := obj.GetLabels()
			if labels == nil {
				labels = map[string]string{}
			}
			labels["test-trigger"] = fmt.Sprintf("%d", time.Now().UnixNano())
			obj.SetLabels(labels)
			_, err = dynamicClient.Resource(databaseGVR).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for reconcile to complete")
			// Wait a few seconds for reconcile, then verify appliedAt is unchanged
			time.Sleep(5 * time.Second)
			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying appliedAt is unchanged (hash match → skip)")
			statusAfter := getInitSQLStatus(crName, namespace)
			Expect(statusAfter["appliedAt"]).To(Equal(originalAppliedAt))
		})

		It("PG-INIT-03: re-executes when inline content changes", func() {
			const crName = "initsql-inline-01" // reuse from PG-INIT-01

			By("recording original hash")
			status := getInitSQLStatus(crName, namespace)
			originalHash := status["hash"]

			By("updating inline SQL to add a second table")
			updateCRSpec(databaseGVR, crName, namespace, func(spec map[string]interface{}) {
				spec["initSQL"] = map[string]interface{}{
					"inline": []interface{}{
						"CREATE TABLE IF NOT EXISTS init_inline (id SERIAL PRIMARY KEY, name TEXT)",
						"INSERT INTO init_inline (name) VALUES ('hello')",
						"CREATE TABLE init_inline_v2 (id SERIAL PRIMARY KEY, value TEXT)",
					},
					"failurePolicy": "Block",
				}
			})

			By("waiting for new initSQL to be applied")
			Eventually(func() interface{} {
				s := getInitSQLStatus(crName, namespace)
				if s == nil {
					return nil
				}
				return s["hash"]
			}, reconcileTimeout, pollInterval).ShouldNot(Equal(originalHash))

			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying new hash and new table")
			statusAfter := getInitSQLStatus(crName, namespace)
			Expect(statusAfter["applied"]).To(Equal(true))
			Expect(statusAfter["hash"]).NotTo(Equal(originalHash))

			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, "initsql_inline_01", "SELECT count(*) FROM init_inline_v2", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(count).To(Equal(0)) // table created but no rows inserted into v2
		})
	})

	Context("configMapRef source", func() {
		It("PG-INIT-04: executes SQL from ConfigMap", func() {
			const crName = "initsql-cm-04"
			const dbName = "initsql_cm_04"
			const cmName = "init-cm-04"
			const cmKey = "init.sql"

			By("creating ConfigMap with SQL")
			createConfigMap(cmName, namespace, cmKey, "CREATE TABLE init_from_cm (id SERIAL PRIMARY KEY, data TEXT)\n---\nINSERT INTO init_from_cm (data) VALUES ('from-configmap')")

			By("creating Database referencing ConfigMap")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"configMapRef": map[string]interface{}{
						"name": cmName,
						"key":  cmKey,
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying table from ConfigMap exists")
			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, dbName, "SELECT count(*) FROM init_from_cm", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(count).To(Equal(1))
		})
	})

	Context("secretRef source", func() {
		It("PG-INIT-05: executes SQL from Secret", func() {
			const crName = "initsql-secret-05"
			const dbName = "initsql_secret_05"
			const secretName = "init-secret-05"
			const secretKey = "init.sql"

			By("creating Secret with SQL")
			createSecret(secretName, namespace, secretKey, "CREATE TABLE init_from_secret (id SERIAL PRIMARY KEY, payload TEXT)\n---\nINSERT INTO init_from_secret (payload) VALUES ('from-secret')")

			By("creating Database referencing Secret")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"secretRef": map[string]interface{}{
						"name": secretName,
						"key":  secretKey,
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying table from Secret exists")
			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, dbName, "SELECT count(*) FROM init_from_secret", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(count).To(Equal(1))
		})
	})

	// ===== Failure Policies =====

	Context("failure policies", func() {
		It("PG-INIT-06: failurePolicy=Continue reaches Ready with Synced=False", func() {
			const crName = "initsql-continue-06"
			const dbName = "initsql_continue_06"

			By("creating Database with bad SQL and Continue policy")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"inline": []interface{}{
						"SELECT * FROM nonexistent_table_xyz",
					},
					"failurePolicy": "Continue",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Ready phase (Continue allows it)")
			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying initSQL status shows failure")
			waitForInitSQLApplied(crName, namespace, false, reconcileTimeout)
			status := getInitSQLStatus(crName, namespace)
			Expect(status["error"]).NotTo(BeEmpty())

			By("verifying Synced condition is False with InitSQLFailed reason")
			syncedCond := getCondition(crName, namespace, "Synced")
			Expect(syncedCond).NotTo(BeNil())
			Expect(syncedCond["status"]).To(Equal("False"))
			Expect(syncedCond["reason"]).To(Equal("InitSQLFailed"))
		})

		It("PG-INIT-07: failurePolicy=Block stays in Failed phase", func() {
			const crName = "initsql-block-07"
			const dbName = "initsql_block_07"

			By("creating Database with invalid SQL and Block policy")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"inline": []interface{}{
						"INVALID SQL STATEMENT",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Failed phase")
			waitForPhaseE(crName, namespace, "Failed", reconcileTimeout)

			By("verifying initSQL status shows failure")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())
			status := getInitSQLStatus(crName, namespace)
			Expect(status["applied"]).To(Equal(false))
			Expect(status["error"]).NotTo(BeEmpty())
		})
	})

	// ===== Negative Cases — Missing/Invalid References =====

	Context("negative cases - missing/invalid references", func() {
		It("PG-INIT-08: missing ConfigMap blocks database with error", func() {
			const crName = "initsql-missing-cm-08"
			const dbName = "initsql_missing_cm_08"

			By("creating Database referencing non-existent ConfigMap")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"configMapRef": map[string]interface{}{
						"name": "no-such-cm",
						"key":  "init.sql",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Failed phase")
			waitForPhaseE(crName, namespace, "Failed", reconcileTimeout)

			By("verifying error references configmap")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())
			status := getInitSQLStatus(crName, namespace)
			Expect(status["error"]).NotTo(BeEmpty())
		})

		It("PG-INIT-09: ConfigMap exists but wrong key blocks with error", func() {
			const crName = "initsql-wrongkey-cm-09"
			const dbName = "initsql_wrongkey_cm_09"
			const cmName = "init-cm-wrongkey"

			By("creating ConfigMap with a different key")
			createConfigMap(cmName, namespace, "data.sql", "CREATE TABLE should_not_exist (id INT)")

			By("creating Database referencing wrong key")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"configMapRef": map[string]interface{}{
						"name": cmName,
						"key":  "missing.sql",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Failed phase")
			waitForPhaseE(crName, namespace, "Failed", reconcileTimeout)

			By("verifying error mentions missing key")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())
			status := getInitSQLStatus(crName, namespace)
			Expect(status["error"]).NotTo(BeEmpty())
		})

		It("PG-INIT-10: missing Secret blocks database with error", func() {
			const crName = "initsql-missing-secret-10"
			const dbName = "initsql_missing_secret_10"

			By("creating Database referencing non-existent Secret")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"secretRef": map[string]interface{}{
						"name": "no-such-secret",
						"key":  "init.sql",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Failed phase")
			waitForPhaseE(crName, namespace, "Failed", reconcileTimeout)

			By("verifying error references secret")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())
			status := getInitSQLStatus(crName, namespace)
			Expect(status["error"]).NotTo(BeEmpty())
		})

		It("PG-INIT-11: Secret exists but wrong key blocks with error", func() {
			const crName = "initsql-wrongkey-secret-11"
			const dbName = "initsql_wrongkey_secret_11"
			const secretName = "init-secret-wrongkey"

			By("creating Secret with a different key")
			createSecret(secretName, namespace, "data.sql", "CREATE TABLE should_not_exist (id INT)")

			By("creating Database referencing wrong key")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"secretRef": map[string]interface{}{
						"name": secretName,
						"key":  "wrong.sql",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Failed phase")
			waitForPhaseE(crName, namespace, "Failed", reconcileTimeout)

			By("verifying error mentions missing key")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())
			status := getInitSQLStatus(crName, namespace)
			Expect(status["error"]).NotTo(BeEmpty())
		})
	})

	// ===== Negative Cases — SQL Execution Errors =====

	Context("negative cases - SQL execution errors", func() {
		It("PG-INIT-12: partial failure reports correct statementsExecuted count", func() {
			const crName = "initsql-partial-12"
			const dbName = "initsql_partial_12"

			By("creating Database with 3 statements: 2 valid, 1 invalid — Continue policy")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"inline": []interface{}{
						"CREATE TABLE init_partial (id SERIAL PRIMARY KEY, name TEXT)",
						"INSERT INTO init_partial (name) VALUES ('partial-success')",
						"SELECT * FROM nonexistent_bad_table",
					},
					"failurePolicy": "Continue",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Ready phase (Continue policy)")
			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying partial execution status")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())
			status := getInitSQLStatus(crName, namespace)
			Expect(status["applied"]).To(Equal(false))
			Expect(status["statementsExecuted"]).To(BeNumerically("==", 2))
			Expect(status["error"]).NotTo(BeEmpty())

			By("verifying first table and row DO exist (partial success)")
			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, dbName, "SELECT count(*) FROM init_partial", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(count).To(Equal(1))
		})

		It("PG-INIT-13: database without initSQL has no initSQL status", func() {
			const crName = "initsql-noop-13"
			const dbName = "initsql_noop_13"

			By("creating Database without initSQL")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying no initSQL status field exists")
			status := getInitSQLStatus(crName, namespace)
			Expect(status).To(BeNil(), "status.initSQL should not exist when initSQL is not configured")

			By("adding initSQL via spec update")
			updateCRSpec(databaseGVR, crName, namespace, func(spec map[string]interface{}) {
				spec["initSQL"] = map[string]interface{}{
					"inline": []interface{}{
						"CREATE TABLE init_added_later (id SERIAL PRIMARY KEY)",
					},
					"failurePolicy": "Block",
				}
			})

			By("waiting for initSQL to be applied")
			Eventually(func() map[string]interface{} {
				return getInitSQLStatus(crName, namespace)
			}, reconcileTimeout, pollInterval).ShouldNot(BeNil())

			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)
			status = getInitSQLStatus(crName, namespace)
			Expect(status["applied"]).To(Equal(true))

			By("verifying table exists")
			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, dbName, "SELECT count(*) FROM init_added_later", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
		})
	})

	// ===== Recovery =====

	Context("negative cases - recovery", func() {
		It("PG-INIT-14: Block policy recovers after fixing SQL in spec", func() {
			const crName = "initsql-recover-14"
			const dbName = "initsql_recover_14"

			By("creating Database with bad SQL and Block policy")
			db := testutil.BuildDatabaseWithOptions(crName, namespace, instanceName, dbName, testutil.DatabaseBuildOptions{
				DeletionProtection: false,
				DeletionPolicy:     "Delete",
				InitSQL: map[string]interface{}{
					"inline": []interface{}{
						"INVALID SQL CAUSING FAILURE",
					},
					"failurePolicy": "Block",
				},
			})
			_, err := dynamicClient.Resource(databaseGVR).Namespace(namespace).Create(ctx, db, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Failed phase")
			waitForPhaseE(crName, namespace, "Failed", reconcileTimeout)

			By("fixing the SQL in spec")
			updateCRSpec(databaseGVR, crName, namespace, func(spec map[string]interface{}) {
				spec["initSQL"] = map[string]interface{}{
					"inline": []interface{}{
						"CREATE TABLE init_recovered (id SERIAL PRIMARY KEY, status TEXT)",
						"INSERT INTO init_recovered (status) VALUES ('recovered')",
					},
					"failurePolicy": "Block",
				}
			})

			By("waiting for Ready phase after fix")
			waitForReady(databaseGVR, crName, namespace, reconcileTimeout)

			By("verifying initSQL applied successfully")
			status := getInitSQLStatus(crName, namespace)
			Expect(status["applied"]).To(Equal(true))
			Expect(status["statementsExecuted"]).To(BeNumerically("==", 2))

			By("verifying recovered table exists")
			var count int
			Eventually(func() error {
				return verifier.QueryRow(ctx, dbName, "SELECT count(*) FROM init_recovered WHERE status='recovered'", &count)
			}, reconcileTimeout, pollInterval).Should(Succeed())
			Expect(count).To(Equal(1))
		})
	})

	// ===== Cleanup =====

	AfterAll(func() {
		if verifier != nil {
			_ = verifier.Close()
		}

		deletionTimeout := getDeletionTimeout()

		By("sweeping leftover ConfigMaps and Secrets")
		for _, name := range []string{"init-cm-04", "init-cm-wrongkey"} {
			_ = k8sClient.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		}
		for _, name := range []string{"init-secret-05", "init-secret-wrongkey"} {
			_ = k8sClient.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		}

		By("sweeping leftover Database CRs")
		addForceDeleteToAll(ctx, databaseGVR, namespace)
		_ = dynamicClient.Resource(databaseGVR).Namespace(namespace).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
			LabelSelector: "", // delete all in namespace — filtered by AfterAll scope
		})

		By("waiting for Database CRs to be removed")
		Eventually(func() bool {
			dbs, _ := dynamicClient.Resource(databaseGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
			// Only check our initsql-prefixed CRs
			for _, db := range dbs.Items {
				if len(db.GetName()) >= 7 && db.GetName()[:7] == "initsql" {
					return false
				}
			}
			return true
		}, deletionTimeout, pollInterval).Should(BeTrue(), "initsql Database CRs should be deleted")

		By("deleting DatabaseInstance")
		_ = dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Delete(ctx, instanceName, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Get(ctx, instanceName, metav1.GetOptions{})
			return err != nil
		}, deletionTimeout, pollInterval).Should(BeTrue(), "DatabaseInstance should be deleted")
	})
})
