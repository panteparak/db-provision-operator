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

var _ = Describe("DatabaseRestore Controller", func() {
	Context("When creating a DatabaseRestore", func() {
		const (
			restoreName  = "test-restore"
			backupName   = "test-backup"
			databaseName = "test-database"
			instanceName = "test-instance"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			restoreNamespacedName  types.NamespacedName
			backupNamespacedName   types.NamespacedName
			databaseNamespacedName types.NamespacedName
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			restoreNamespacedName = types.NamespacedName{Name: restoreName, Namespace: namespace}
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

			// Create a DatabaseBackup
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
			err = k8sClient.Get(ctx, backupNamespacedName, &dbopsv1alpha1.DatabaseBackup{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, backup)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Clean up DatabaseRestore
			restore := &dbopsv1alpha1.DatabaseRestore{}
			err := k8sClient.Get(ctx, restoreNamespacedName, restore)
			if err == nil {
				restore.Finalizers = nil
				_ = k8sClient.Update(ctx, restore)
				_ = k8sClient.Delete(ctx, restore)
			}

			// Clean up DatabaseBackup
			backup := &dbopsv1alpha1.DatabaseBackup{}
			err = k8sClient.Get(ctx, backupNamespacedName, backup)
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
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: instanceName,
						},
						DatabaseName: "restored_db",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, restoreNamespacedName, updatedRestore)).To(Succeed())
			Expect(updatedRestore.Finalizers).To(ContainElement(util.FinalizerDatabaseRestore))
		})

		It("should set pending phase when backup is not ready", func() {
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: instanceName,
						},
						DatabaseName: "restored_db",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to check phase (backup not ready)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			updatedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, restoreNamespacedName, updatedRestore)).To(Succeed())
			Expect(updatedRestore.Status.Phase).To(Equal(dbopsv1alpha1.PhasePending))
		})

		It("should skip reconcile when annotation is set", func() {
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: instanceName,
						},
						DatabaseName: "restored_db",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, restoreNamespacedName, updatedRestore)).To(Succeed())
			Expect(updatedRestore.Finalizers).To(BeEmpty())
		})

		It("should fail when backup does not exist", func() {
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: "nonexistent-backup",
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: instanceName,
						},
						DatabaseName: "restored_db",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})

			// Second reconcile handles the missing backup
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is failed
			updatedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, restoreNamespacedName, updatedRestore)).To(Succeed())
			Expect(updatedRestore.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should not re-run restore when already completed", func() {
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: instanceName,
						},
						DatabaseName: "restored_db",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			// Set status to completed
			restore.Status.Phase = dbopsv1alpha1.PhaseCompleted
			now := metav1.Now()
			restore.Status.CompletedAt = &now
			Expect(k8sClient.Status().Update(ctx, restore)).To(Succeed())

			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// Should not requeue for completed restores
			Expect(result.Requeue).To(BeFalse())
		})

		It("should require confirmation for in-place restore", func() {
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InPlace: true,
						DatabaseRef: &dbopsv1alpha1.DatabaseReference{
							Name: databaseName,
						},
					},
					// No confirmation provided
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			controllerReconciler := &DatabaseRestoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})

			// Second reconcile should fail without confirmation
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: restoreNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is failed due to missing confirmation
			updatedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, restoreNamespacedName, updatedRestore)).To(Succeed())
			Expect(updatedRestore.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
		})
	})

	Context("When verifying restore spec parsing", func() {
		const (
			restoreName  = "test-restore-spec"
			backupName   = "test-backup-spec"
			databaseName = "test-database-spec"
			instanceName = "test-instance-spec"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			restoreNamespacedName  types.NamespacedName
			backupNamespacedName   types.NamespacedName
			databaseNamespacedName types.NamespacedName
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			restoreNamespacedName = types.NamespacedName{Name: restoreName, Namespace: namespace}
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

			// Create a completed DatabaseBackup with storage info
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
			err = k8sClient.Get(ctx, backupNamespacedName, &dbopsv1alpha1.DatabaseBackup{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, backup)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Clean up DatabaseRestore
			restore := &dbopsv1alpha1.DatabaseRestore{}
			err := k8sClient.Get(ctx, restoreNamespacedName, restore)
			if err == nil {
				restore.Finalizers = nil
				_ = k8sClient.Update(ctx, restore)
				_ = k8sClient.Delete(ctx, restore)
			}

			// Clean up DatabaseBackup
			backup := &dbopsv1alpha1.DatabaseBackup{}
			err = k8sClient.Get(ctx, backupNamespacedName, backup)
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

		It("should correctly parse backup reference with namespace", func() {
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name:      backupName,
						Namespace: "other-namespace",
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name:      instanceName,
							Namespace: namespace,
						},
						DatabaseName: "restored_db",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			fetchedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, restoreNamespacedName, fetchedRestore)).To(Succeed())

			// Verify backup reference is correctly parsed
			Expect(fetchedRestore.Spec.BackupRef).NotTo(BeNil())
			Expect(fetchedRestore.Spec.BackupRef.Name).To(Equal(backupName))
			Expect(fetchedRestore.Spec.BackupRef.Namespace).To(Equal("other-namespace"))

			// Verify target instance reference
			Expect(fetchedRestore.Spec.Target.InstanceRef).NotTo(BeNil())
			Expect(fetchedRestore.Spec.Target.InstanceRef.Name).To(Equal(instanceName))
			Expect(fetchedRestore.Spec.Target.InstanceRef.Namespace).To(Equal(namespace))
		})

		It("should correctly parse FromPath spec with S3 storage configuration", func() {
			directPathRestoreName := "test-restore-direct-path"
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      directPathRestoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					// Using FromPath instead of BackupRef for direct path restore
					FromPath: &dbopsv1alpha1.RestoreFromPath{
						BackupPath: "/backups/manual/production-2024-01-15.dump",
						Storage: dbopsv1alpha1.StorageConfig{
							Type: dbopsv1alpha1.StorageTypeS3,
							S3: &dbopsv1alpha1.S3StorageConfig{
								Bucket:   "backup-bucket",
								Region:   "us-east-1",
								Prefix:   "postgres",
								Endpoint: "https://s3.amazonaws.com",
								SecretRef: dbopsv1alpha1.S3SecretRef{
									Name: "s3-creds",
								},
							},
						},
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: instanceName,
						},
						DatabaseName: "restored_from_path",
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			fetchedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: directPathRestoreName, Namespace: namespace}, fetchedRestore)).To(Succeed())

			// Verify FromPath is correctly stored
			Expect(fetchedRestore.Spec.FromPath).NotTo(BeNil())
			Expect(fetchedRestore.Spec.FromPath.BackupPath).To(Equal("/backups/manual/production-2024-01-15.dump"))

			// Verify storage configuration for direct path restore
			Expect(fetchedRestore.Spec.FromPath.Storage.Type).To(Equal(dbopsv1alpha1.StorageTypeS3))
			Expect(fetchedRestore.Spec.FromPath.Storage.S3.Bucket).To(Equal("backup-bucket"))
			Expect(fetchedRestore.Spec.FromPath.Storage.S3.Region).To(Equal("us-east-1"))

			// Clean up
			restoreToDelete := &dbopsv1alpha1.DatabaseRestore{}
			_ = k8sClient.Get(ctx, types.NamespacedName{Name: directPathRestoreName, Namespace: namespace}, restoreToDelete)
			if restoreToDelete.Name != "" {
				restoreToDelete.Finalizers = nil
				_ = k8sClient.Update(ctx, restoreToDelete)
				_ = k8sClient.Delete(ctx, restoreToDelete)
			}
		})

		It("should correctly parse target database spec with PostgreSQL restore options", func() {
			targetRestoreName := "test-restore-target"
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetRestoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name:      instanceName,
							Namespace: namespace,
						},
						DatabaseName: "new_production_db",
						InPlace:      false,
					},
					// PostgreSQL-specific restore options
					Postgres: &dbopsv1alpha1.PostgresRestoreConfig{
						DropExisting:    false,
						CreateDatabase:  true,
						DataOnly:        false,
						SchemaOnly:      false,
						NoOwner:         true,
						NoPrivileges:    false,
						DisableTriggers: true,
						Analyze:         true,
						Jobs:            4,
						RoleMapping: map[string]string{
							"old_owner": "new_owner",
							"old_role":  "new_role",
						},
						Schemas: []string{"public", "app"},
						Tables:  []string{"users", "orders"},
					},
					Confirmation: &dbopsv1alpha1.RestoreConfirmation{
						AcknowledgeDataLoss: dbopsv1alpha1.RestoreConfirmDataLoss,
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			fetchedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: targetRestoreName, Namespace: namespace}, fetchedRestore)).To(Succeed())

			// Verify target configuration
			Expect(fetchedRestore.Spec.Target.DatabaseName).To(Equal("new_production_db"))
			Expect(fetchedRestore.Spec.Target.InPlace).To(BeFalse())
			Expect(fetchedRestore.Spec.Target.InstanceRef.Name).To(Equal(instanceName))

			// Verify PostgreSQL-specific options
			Expect(fetchedRestore.Spec.Postgres).NotTo(BeNil())
			Expect(fetchedRestore.Spec.Postgres.CreateDatabase).To(BeTrue())
			Expect(fetchedRestore.Spec.Postgres.NoOwner).To(BeTrue())
			Expect(fetchedRestore.Spec.Postgres.DisableTriggers).To(BeTrue())
			Expect(fetchedRestore.Spec.Postgres.Analyze).To(BeTrue())
			Expect(fetchedRestore.Spec.Postgres.Jobs).To(Equal(int32(4)))

			// Verify role mapping
			Expect(fetchedRestore.Spec.Postgres.RoleMapping).To(HaveLen(2))
			Expect(fetchedRestore.Spec.Postgres.RoleMapping["old_owner"]).To(Equal("new_owner"))

			// Verify schema/table filtering
			Expect(fetchedRestore.Spec.Postgres.Schemas).To(ContainElements("public", "app"))
			Expect(fetchedRestore.Spec.Postgres.Tables).To(ContainElements("users", "orders"))

			// Verify confirmation
			Expect(fetchedRestore.Spec.Confirmation).NotTo(BeNil())
			Expect(fetchedRestore.Spec.Confirmation.AcknowledgeDataLoss).To(Equal(dbopsv1alpha1.RestoreConfirmDataLoss))

			// Clean up
			_ = k8sClient.Delete(ctx, restore)
		})

		It("should correctly parse in-place restore with database reference", func() {
			inPlaceRestoreName := "test-restore-inplace-spec"
			restore := &dbopsv1alpha1.DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      inPlaceRestoreName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: backupName,
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InPlace: true,
						DatabaseRef: &dbopsv1alpha1.DatabaseReference{
							Name:      databaseName,
							Namespace: namespace,
						},
					},
					Confirmation: &dbopsv1alpha1.RestoreConfirmation{
						AcknowledgeDataLoss: dbopsv1alpha1.RestoreConfirmDataLoss,
					},
				},
			}
			Expect(k8sClient.Create(ctx, restore)).To(Succeed())

			fetchedRestore := &dbopsv1alpha1.DatabaseRestore{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: inPlaceRestoreName, Namespace: namespace}, fetchedRestore)).To(Succeed())

			// Verify in-place restore settings
			Expect(fetchedRestore.Spec.Target.InPlace).To(BeTrue())
			Expect(fetchedRestore.Spec.Target.DatabaseRef).NotTo(BeNil())
			Expect(fetchedRestore.Spec.Target.DatabaseRef.Name).To(Equal(databaseName))
			Expect(fetchedRestore.Spec.Target.DatabaseRef.Namespace).To(Equal(namespace))

			// Verify confirmation is stored for destructive in-place restore
			Expect(fetchedRestore.Spec.Confirmation).NotTo(BeNil())
			Expect(fetchedRestore.Spec.Confirmation.AcknowledgeDataLoss).To(Equal(dbopsv1alpha1.RestoreConfirmDataLoss))

			// Clean up
			_ = k8sClient.Delete(ctx, restore)
		})
	})
})
