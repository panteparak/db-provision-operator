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
	"time"

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

var _ = Describe("DatabaseBackup Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a DatabaseBackup", func() {
		const (
			backupName   = "test-backup"
			databaseName = "test-database"
			instanceName = "test-instance"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			backupNamespacedName   types.NamespacedName
			databaseNamespacedName types.NamespacedName
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			backupNamespacedName = types.NamespacedName{Name: backupName, Namespace: namespace}
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

			// Create a Database
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
			err = k8sClient.Get(ctx, databaseNamespacedName, &dbopsv1alpha1.Database{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, database)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Clean up DatabaseBackup
			backup := &dbopsv1alpha1.DatabaseBackup{}
			err := k8sClient.Get(ctx, backupNamespacedName, backup)
			if err == nil {
				backup.Finalizers = nil
				_ = k8sClient.Update(ctx, backup)
				_ = k8sClient.Delete(ctx, backup)
			}

			// Clean up Database
			database := &dbopsv1alpha1.Database{}
			err = k8sClient.Get(ctx, databaseNamespacedName, database)
			if err == nil {
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
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
						PVC: &dbopsv1alpha1.PVCStorageConfig{
							ClaimName: "backup-pvc",
							SubPath:   "backups",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			controllerReconciler := &DatabaseBackupReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, backupNamespacedName, updatedBackup)).To(Succeed())
			Expect(updatedBackup.Finalizers).To(ContainElement(util.FinalizerDatabaseBackup))
		})

		It("should set pending phase when database is not ready", func() {
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
						PVC: &dbopsv1alpha1.PVCStorageConfig{
							ClaimName: "backup-pvc",
							SubPath:   "backups",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			controllerReconciler := &DatabaseBackupReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to check phase (database not ready)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, backupNamespacedName, updatedBackup)).To(Succeed())
			Expect(updatedBackup.Status.Phase).To(Equal(dbopsv1alpha1.PhasePending))
		})

		It("should skip reconcile when annotation is set", func() {
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
						PVC: &dbopsv1alpha1.PVCStorageConfig{
							ClaimName: "backup-pvc",
							SubPath:   "backups",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			controllerReconciler := &DatabaseBackupReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, backupNamespacedName, updatedBackup)).To(Succeed())
			Expect(updatedBackup.Finalizers).To(BeEmpty())
		})

		It("should fail when database does not exist", func() {
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "nonexistent-database",
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
						PVC: &dbopsv1alpha1.PVCStorageConfig{
							ClaimName: "backup-pvc",
							SubPath:   "backups",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			controllerReconciler := &DatabaseBackupReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})

			// Second reconcile handles the missing database
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is failed
			updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, backupNamespacedName, updatedBackup)).To(Succeed())
			Expect(updatedBackup.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseBackupReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should not allow backup when already completed", func() {
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
						PVC: &dbopsv1alpha1.PVCStorageConfig{
							ClaimName: "backup-pvc",
							SubPath:   "backups",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			// Set status to completed
			backup.Status.Phase = dbopsv1alpha1.PhaseCompleted
			now := metav1.Now()
			backup.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, backup)).To(Succeed())

			controllerReconciler := &DatabaseBackupReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: backupNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// Should not requeue for completed backups
			Expect(result.Requeue).To(BeFalse())
		})
	})
})
