//go:build envtest && !integration

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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/util"
)

var _ = Describe("Database Controller", func() {
	Context("When creating a Database", func() {
		const (
			databaseName = "test-database"
			instanceName = "test-instance"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			databaseNamespacedName types.NamespacedName
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			databaseNamespacedName = types.NamespacedName{Name: databaseName, Namespace: namespace}
			instanceNamespacedName = types.NamespacedName{Name: instanceName, Namespace: namespace}

			// Create a secret for the instance
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("password123"),
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: namespace}, &corev1.Secret{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}

			// Create a DatabaseInstance
			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: dbopsv1alpha1.EngineTypePostgres,
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: "localhost",
						Port: 5432,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			err = k8sClient.Get(ctx, instanceNamespacedName, &dbopsv1alpha1.DatabaseInstance{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, instance)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Clean up Database
			database := &dbopsv1alpha1.Database{}
			err := k8sClient.Get(ctx, databaseNamespacedName, database)
			if err == nil {
				// Remove finalizer to allow deletion
				database.Finalizers = nil
				_ = k8sClient.Update(ctx, database)
				_ = k8sClient.Delete(ctx, database)
			}

			// Clean up DatabaseInstance
			instance := &dbopsv1alpha1.DatabaseInstance{}
			err = k8sClient.Get(ctx, instanceNamespacedName, instance)
			if err == nil {
				instance.Finalizers = nil
				_ = k8sClient.Update(ctx, instance)
				_ = k8sClient.Delete(ctx, instance)
			}
		})

		It("should add finalizer when created", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "testdb",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Finalizers).To(ContainElement(util.FinalizerDatabase))
		})

		It("should set pending phase when instance is not ready", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "testdb",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to check phase (instance not ready)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			updatedDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Status.Phase).To(Equal(dbopsv1alpha1.PhasePending))
		})

		It("should skip reconcile when annotation is set", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "testdb",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Finalizers).To(BeEmpty())
		})

		It("should fail when instance does not exist", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "testdb",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: "nonexistent-instance",
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})

			// Second reconcile handles the missing instance
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is failed
			updatedDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should block deletion when DeletionProtection is true", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:       databaseName,
					Namespace:  namespace,
					Finalizers: []string{util.FinalizerDatabase},
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "protected_db",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
					DeletionProtection: true,
					DeletionPolicy:     dbopsv1alpha1.DeletionPolicyDelete,
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			// Mark for deletion
			Expect(k8sClient.Delete(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Reconcile should fail due to deletion protection
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("deletion protection"))

			// Verify status shows deletion blocked
			updatedDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
			Expect(updatedDatabase.Status.Message).To(ContainSubstring("Deletion blocked"))

			// Verify the Ready condition shows DeletionProtected reason
			readyCond := util.GetCondition(updatedDatabase.Status.Conditions, util.ConditionTypeReady)
			if readyCond != nil {
				Expect(readyCond.Reason).To(Equal(util.ReasonDeletionProtected))
			}
		})

		It("should allow deletion when force-delete annotation is set", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:       databaseName,
					Namespace:  namespace,
					Finalizers: []string{util.FinalizerDatabase},
					Annotations: map[string]string{
						util.AnnotationForceDelete: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "force_delete_db",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
					DeletionProtection: true,
					DeletionPolicy:     dbopsv1alpha1.DeletionPolicyRetain,
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			// Mark for deletion
			Expect(k8sClient.Delete(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Reconcile should succeed (force-delete bypasses deletion protection)
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was removed (database should be deleted)
			deletedDatabase := &dbopsv1alpha1.Database{}
			err = k8sClient.Get(ctx, databaseNamespacedName, deletedDatabase)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should set owner reference to DatabaseInstance", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "owned_db",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			// Verify the database was created
			createdDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, createdDatabase)).To(Succeed())

			// Verify instanceRef is set correctly
			Expect(createdDatabase.Spec.InstanceRef.Name).To(Equal(instanceName))
			Expect(createdDatabase.Spec.InstanceRef.Namespace).To(BeEmpty()) // Same namespace
		})

		It("should update status message on error", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "error_db",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: "nonexistent-instance",
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})

			// Second reconcile to check error handling
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: databaseNamespacedName,
			})

			// Verify status message contains error information
			updatedDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, updatedDatabase)).To(Succeed())
			Expect(updatedDatabase.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
			Expect(updatedDatabase.Status.Message).NotTo(BeEmpty())
			Expect(updatedDatabase.Status.Message).To(ContainSubstring("not found"))
		})

		It("should read PostgreSQL extensions spec correctly", func() {
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					Name: "extensions_db",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
					Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
						Encoding:   "UTF8",
						Tablespace: "pg_default",
						Template:   "template0",
						Extensions: []dbopsv1alpha1.PostgresExtension{
							{
								Name:    "uuid-ossp",
								Schema:  "public",
								Version: "1.1",
							},
							{
								Name:   "pg_stat_statements",
								Schema: "public",
							},
							{
								Name: "hstore",
							},
						},
						Schemas: []dbopsv1alpha1.PostgresSchema{
							{
								Name:  "app",
								Owner: "app_user",
							},
							{
								Name: "analytics",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			// Verify the database was created with PostgreSQL config
			createdDatabase := &dbopsv1alpha1.Database{}
			Expect(k8sClient.Get(ctx, databaseNamespacedName, createdDatabase)).To(Succeed())
			Expect(createdDatabase.Spec.Postgres).NotTo(BeNil())
			Expect(createdDatabase.Spec.Postgres.Encoding).To(Equal("UTF8"))
			Expect(createdDatabase.Spec.Postgres.Tablespace).To(Equal("pg_default"))
			Expect(createdDatabase.Spec.Postgres.Template).To(Equal("template0"))

			// Verify extensions
			Expect(createdDatabase.Spec.Postgres.Extensions).To(HaveLen(3))
			Expect(createdDatabase.Spec.Postgres.Extensions[0].Name).To(Equal("uuid-ossp"))
			Expect(createdDatabase.Spec.Postgres.Extensions[0].Schema).To(Equal("public"))
			Expect(createdDatabase.Spec.Postgres.Extensions[0].Version).To(Equal("1.1"))
			Expect(createdDatabase.Spec.Postgres.Extensions[1].Name).To(Equal("pg_stat_statements"))
			Expect(createdDatabase.Spec.Postgres.Extensions[2].Name).To(Equal("hstore"))

			// Verify schemas
			Expect(createdDatabase.Spec.Postgres.Schemas).To(HaveLen(2))
			Expect(createdDatabase.Spec.Postgres.Schemas[0].Name).To(Equal("app"))
			Expect(createdDatabase.Spec.Postgres.Schemas[0].Owner).To(Equal("app_user"))
			Expect(createdDatabase.Spec.Postgres.Schemas[1].Name).To(Equal("analytics"))
		})
	})
})
