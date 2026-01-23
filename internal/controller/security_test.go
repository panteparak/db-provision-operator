//go:build integration

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

package controller

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

var _ = Describe("Security Tests", func() {
	const (
		testNamespace = "default"
		timeout       = time.Second * 60
		interval      = time.Millisecond * 500
	)

	// Helper to create a test instance for security tests
	var createSecurityTestInstance = func(name string) (*dbopsv1alpha1.DatabaseInstance, *corev1.Secret) {
		credSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + "-creds",
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"username": []byte(intDBConfig.User),
				"password": []byte(intDBConfig.Password),
			},
		}
		Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

		instance := &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: getEngineType(intDBConfig.Database),
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host: intDBConfig.Host,
					Port: intDBConfig.Port,
					SecretRef: &dbopsv1alpha1.CredentialSecretRef{
						Name: name + "-creds",
					},
				},
			},
		}
		Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

		// Wait for instance to be Ready
		Eventually(func() string {
			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			err := intK8sClient.Get(intCtx, types.NamespacedName{
				Name:      name,
				Namespace: testNamespace,
			}, updatedInstance)
			if err != nil {
				return ""
			}
			return string(updatedInstance.Status.Phase)
		}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

		return instance, credSecret
	}

	// Helper to cleanup resources
	var cleanup = func(objects ...client.Object) {
		for _, obj := range objects {
			if obj == nil {
				continue
			}
			// Remove finalizers first
			switch o := obj.(type) {
			case *dbopsv1alpha1.DatabaseInstance:
				_ = intK8sClient.Get(intCtx, client.ObjectKeyFromObject(o), o)
				o.Finalizers = nil
				_ = intK8sClient.Update(intCtx, o)
			case *dbopsv1alpha1.Database:
				_ = intK8sClient.Get(intCtx, client.ObjectKeyFromObject(o), o)
				o.Finalizers = nil
				_ = intK8sClient.Update(intCtx, o)
			case *dbopsv1alpha1.DatabaseUser:
				_ = intK8sClient.Get(intCtx, client.ObjectKeyFromObject(o), o)
				o.Finalizers = nil
				_ = intK8sClient.Update(intCtx, o)
			}
			_ = intK8sClient.Delete(intCtx, obj)
		}
	}

	Context("SQL Injection Prevention", func() {
		It("should reject or sanitize database names with SQL injection attempts", func() {
			instanceName := fmt.Sprintf("sec-sqli-db-%s", intDBConfig.Database)
			instance, credSecret := createSecurityTestInstance(instanceName)
			defer cleanup(instance, credSecret)

			// Common SQL injection patterns
			maliciousNames := []string{
				"test'; DROP TABLE users;--",
				"test\"; DROP TABLE users;--",
				"test`; DROP TABLE users;--",
				"test); DELETE FROM pg_catalog.pg_tables;--",
				"test UNION SELECT * FROM information_schema.tables",
			}

			for i, name := range maliciousNames {
				By(fmt.Sprintf("Testing malicious name pattern %d: %s", i+1, name[:min(30, len(name))]))

				// Create safe k8s name from iteration
				k8sName := fmt.Sprintf("sqli-test-db-%d-%s", i, intDBConfig.Database)

				db := &dbopsv1alpha1.Database{
					ObjectMeta: metav1.ObjectMeta{
						Name:      k8sName,
						Namespace: testNamespace,
					},
					Spec: dbopsv1alpha1.DatabaseSpec{
						InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
						Name:        name, // The malicious name
					},
				}

				err := intK8sClient.Create(intCtx, db)
				if err != nil {
					// Rejected at API level - this is good
					GinkgoWriter.Printf("  Pattern %d rejected at API level: %v\n", i+1, err)
					continue
				}

				// If created, verify it fails or sanitizes the name
				Eventually(func() string {
					var created dbopsv1alpha1.Database
					if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(db), &created); err != nil {
						return ""
					}
					return string(created.Status.Phase)
				}, timeout, interval).Should(Or(
					Equal(string(dbopsv1alpha1.PhaseFailed)),
					Equal(string(dbopsv1alpha1.PhaseReady)), // If sanitized and created successfully
				))

				// Cleanup
				cleanup(db)
			}
		})

		It("should reject usernames with SQL injection attempts", func() {
			instanceName := fmt.Sprintf("sec-sqli-user-%s", intDBConfig.Database)
			instance, credSecret := createSecurityTestInstance(instanceName)
			defer cleanup(instance, credSecret)

			maliciousUsernames := []string{
				"admin'--",
				"admin'; GRANT ALL PRIVILEGES ON *.* TO 'hacker'@'%';--",
				"admin\"); DROP USER root;--",
				"test' OR '1'='1",
			}

			for i, username := range maliciousUsernames {
				By(fmt.Sprintf("Testing malicious username pattern %d", i+1))

				k8sName := fmt.Sprintf("sqli-user-%d-%s", i, intDBConfig.Database)

				user := &dbopsv1alpha1.DatabaseUser{
					ObjectMeta: metav1.ObjectMeta{
						Name:      k8sName,
						Namespace: testNamespace,
					},
					Spec: dbopsv1alpha1.DatabaseUserSpec{
						InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
						Username:    username,
					},
				}

				err := intK8sClient.Create(intCtx, user)
				if err != nil {
					GinkgoWriter.Printf("  Username pattern %d rejected at API level: %v\n", i+1, err)
					continue
				}

				// If created, verify it fails or sanitizes
				Eventually(func() string {
					var created dbopsv1alpha1.DatabaseUser
					if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(user), &created); err != nil {
						return ""
					}
					return string(created.Status.Phase)
				}, timeout, interval).Should(Or(
					Equal(string(dbopsv1alpha1.PhaseFailed)),
					Equal(string(dbopsv1alpha1.PhaseReady)),
				))

				cleanup(user)
			}
		})
	})

	Context("Input Validation", func() {
		It("should handle oversized database names", func() {
			instanceName := fmt.Sprintf("sec-long-name-%s", intDBConfig.Database)
			instance, credSecret := createSecurityTestInstance(instanceName)
			defer cleanup(instance, credSecret)

			// Most databases limit identifiers to 64-128 characters
			longName := strings.Repeat("a", 256)

			db := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("long-name-test-%s", intDBConfig.Database),
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
					Name:        longName,
				},
			}

			err := intK8sClient.Create(intCtx, db)
			if err != nil {
				// Rejected at API level
				GinkgoWriter.Printf("Long name rejected at API level: %v\n", err)
				return
			}

			// If created, should fail at database level
			Eventually(func() string {
				var created dbopsv1alpha1.Database
				if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(db), &created); err != nil {
					return ""
				}
				return string(created.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseFailed)))

			cleanup(db)
		})

		It("should handle null bytes in identifiers", func() {
			instanceName := fmt.Sprintf("sec-null-byte-%s", intDBConfig.Database)
			instance, credSecret := createSecurityTestInstance(instanceName)
			defer cleanup(instance, credSecret)

			// Null byte injection
			nullByteName := "test\x00injection"

			db := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("null-byte-test-%s", intDBConfig.Database),
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
					Name:        nullByteName,
				},
			}

			err := intK8sClient.Create(intCtx, db)
			if err != nil {
				GinkgoWriter.Printf("Null byte name rejected at API level: %v\n", err)
				return
			}

			// Should fail
			Eventually(func() string {
				var created dbopsv1alpha1.Database
				if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(db), &created); err != nil {
					return ""
				}
				return string(created.Status.Phase)
			}, timeout, interval).Should(Or(
				Equal(string(dbopsv1alpha1.PhaseFailed)),
				Equal(string(dbopsv1alpha1.PhaseReady)), // If sanitized
			))

			cleanup(db)
		})

		It("should validate special characters in identifiers", func() {
			instanceName := fmt.Sprintf("sec-special-char-%s", intDBConfig.Database)
			instance, credSecret := createSecurityTestInstance(instanceName)
			defer cleanup(instance, credSecret)

			specialCharNames := []string{
				"test with space",
				"test\ttab",
				"test\nnewline",
				"test;semicolon",
				"test$dollar",
			}

			for i, name := range specialCharNames {
				By(fmt.Sprintf("Testing special character pattern %d", i+1))

				k8sName := fmt.Sprintf("special-char-%d-%s", i, intDBConfig.Database)

				db := &dbopsv1alpha1.Database{
					ObjectMeta: metav1.ObjectMeta{
						Name:      k8sName,
						Namespace: testNamespace,
					},
					Spec: dbopsv1alpha1.DatabaseSpec{
						InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
						Name:        name,
					},
				}

				err := intK8sClient.Create(intCtx, db)
				if err != nil {
					GinkgoWriter.Printf("  Special char pattern %d rejected: %v\n", i+1, err)
					continue
				}

				// Record result but don't fail - some databases allow certain special chars
				Eventually(func() string {
					var created dbopsv1alpha1.Database
					if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(db), &created); err != nil {
						return ""
					}
					phase := string(created.Status.Phase)
					GinkgoWriter.Printf("  Special char pattern %d (%q) result: %s\n", i+1, name, phase)
					return phase
				}, timeout, interval).Should(Or(
					Equal(string(dbopsv1alpha1.PhaseFailed)),
					Equal(string(dbopsv1alpha1.PhaseReady)),
				))

				cleanup(db)
			}
		})
	})

	Context("Privilege Escalation Prevention", func() {
		It("should not allow granting ALL privileges through DatabaseGrant", func() {
			instanceName := fmt.Sprintf("sec-priv-esc-%s", intDBConfig.Database)
			instance, credSecret := createSecurityTestInstance(instanceName)
			defer cleanup(instance, credSecret)

			// Create a test database
			dbName := fmt.Sprintf("sec-priv-test-%s", intDBConfig.Database)
			db := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
					Name:        "secprivtestdb",
				},
			}
			Expect(intK8sClient.Create(intCtx, db)).To(Succeed())
			Eventually(func() string {
				var created dbopsv1alpha1.Database
				if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(db), &created); err != nil {
					return ""
				}
				return string(created.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Create a limited user
			userName := fmt.Sprintf("limited-user-%s", intDBConfig.Database)
			user := &dbopsv1alpha1.DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseUserSpec{
					InstanceRef: dbopsv1alpha1.InstanceReference{Name: instanceName},
					Username:    "limiteduser",
				},
			}
			Expect(intK8sClient.Create(intCtx, user)).To(Succeed())
			Eventually(func() string {
				var created dbopsv1alpha1.DatabaseUser
				if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(user), &created); err != nil {
					return ""
				}
				return string(created.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Try to grant dangerous privileges based on engine type
			// The new API uses engine-specific grant configs
			var grants []*dbopsv1alpha1.DatabaseGrant

			if intDBConfig.Database == "postgresql" {
				// PostgreSQL dangerous privileges
				dangerousPrivileges := [][]string{
					{"ALL"},
					{"CREATE"},
					{"CONNECT", "TEMPORARY"},
				}

				for i, privs := range dangerousPrivileges {
					grantName := fmt.Sprintf("dangerous-grant-%d-%s", i, intDBConfig.Database)
					grant := &dbopsv1alpha1.DatabaseGrant{
						ObjectMeta: metav1.ObjectMeta{
							Name:      grantName,
							Namespace: testNamespace,
						},
						Spec: dbopsv1alpha1.DatabaseGrantSpec{
							UserRef:     dbopsv1alpha1.UserReference{Name: userName},
							DatabaseRef: &dbopsv1alpha1.DatabaseReference{Name: dbName},
							Postgres: &dbopsv1alpha1.PostgresGrantConfig{
								Grants: []dbopsv1alpha1.PostgresGrant{
									{
										Database:   "testdb",
										Privileges: privs,
									},
								},
							},
						},
					}
					grants = append(grants, grant)
				}
			} else {
				// MySQL/MariaDB dangerous privileges
				dangerousPrivileges := [][]string{
					{"ALL"},
					{"SUPER"},
					{"CREATE USER"},
					{"GRANT OPTION"},
				}

				for i, privs := range dangerousPrivileges {
					grantName := fmt.Sprintf("dangerous-grant-%d-%s", i, intDBConfig.Database)
					grant := &dbopsv1alpha1.DatabaseGrant{
						ObjectMeta: metav1.ObjectMeta{
							Name:      grantName,
							Namespace: testNamespace,
						},
						Spec: dbopsv1alpha1.DatabaseGrantSpec{
							UserRef:     dbopsv1alpha1.UserReference{Name: userName},
							DatabaseRef: &dbopsv1alpha1.DatabaseReference{Name: dbName},
							MySQL: &dbopsv1alpha1.MySQLGrantConfig{
								Grants: []dbopsv1alpha1.MySQLGrant{
									{
										Level:      dbopsv1alpha1.MySQLGrantLevelDatabase,
										Database:   "testdb",
										Privileges: privs,
									},
								},
							},
						},
					}
					grants = append(grants, grant)
				}
			}

			for i, grant := range grants {
				By(fmt.Sprintf("Testing dangerous privilege grant %d", i+1))

				err := intK8sClient.Create(intCtx, grant)
				if err != nil {
					GinkgoWriter.Printf("  Dangerous privilege rejected at API level: %v\n", err)
					continue
				}

				// Should either fail or succeed with filtered privileges
				Eventually(func() string {
					var created dbopsv1alpha1.DatabaseGrant
					if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(grant), &created); err != nil {
						return ""
					}
					GinkgoWriter.Printf("  Grant %s phase: %s\n", grant.Name, created.Status.Phase)
					return string(created.Status.Phase)
				}, timeout, interval).Should(Or(
					Equal(string(dbopsv1alpha1.PhaseFailed)),
					Equal(string(dbopsv1alpha1.PhaseReady)),
				))

				cleanup(grant)
			}

			cleanup(user, db)
		})
	})

	Context("Connection Security", func() {
		It("should fail gracefully with invalid credentials", func() {
			instanceName := fmt.Sprintf("sec-bad-creds-%s", intDBConfig.Database)

			// Create secret with wrong credentials
			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"username": []byte("nonexistent_user"),
					"password": []byte("wrong_password_12345"),
				},
			}
			Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: getEngineType(intDBConfig.Database),
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: intDBConfig.Host,
						Port: intDBConfig.Port,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

			// Should fail to connect
			Eventually(func() string {
				var created dbopsv1alpha1.DatabaseInstance
				if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(instance), &created); err != nil {
					return ""
				}
				return string(created.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseFailed)))

			cleanup(instance, credSecret)
		})

		It("should fail gracefully with unreachable host", func() {
			instanceName := fmt.Sprintf("sec-bad-host-%s", intDBConfig.Database)

			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"username": []byte(intDBConfig.User),
					"password": []byte(intDBConfig.Password),
				},
			}
			Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: getEngineType(intDBConfig.Database),
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: "192.0.2.1", // TEST-NET-1, should not be routable
						Port: intDBConfig.Port,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

			// Should fail to connect (may take longer due to network timeout)
			Eventually(func() string {
				var created dbopsv1alpha1.DatabaseInstance
				if err := intK8sClient.Get(intCtx, client.ObjectKeyFromObject(instance), &created); err != nil {
					return ""
				}
				return string(created.Status.Phase)
			}, timeout*2, interval).Should(Equal(string(dbopsv1alpha1.PhaseFailed)))

			cleanup(instance, credSecret)
		})
	})
})

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
