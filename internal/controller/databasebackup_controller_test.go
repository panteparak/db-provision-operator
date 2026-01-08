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

var _ = Describe("DatabaseBackup Controller", func() {
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

		It("should correctly parse S3 storage configuration", func() {
			s3BackupName := "test-backup-s3"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3BackupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypeS3,
						S3: &dbopsv1alpha1.S3StorageConfig{
							Bucket:         "my-backup-bucket",
							Region:         "us-west-2",
							Prefix:         "backups/postgres",
							Endpoint:       "https://s3.us-west-2.amazonaws.com",
							ForcePathStyle: false,
							SecretRef: dbopsv1alpha1.S3SecretRef{
								Name: "s3-credentials",
								Keys: &dbopsv1alpha1.S3SecretKeys{
									AccessKey: "AWS_ACCESS_KEY_ID",
									SecretKey: "AWS_SECRET_ACCESS_KEY",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			// Verify the S3 storage spec was stored correctly
			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: s3BackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.Storage.Type).To(Equal(dbopsv1alpha1.StorageTypeS3))
			Expect(fetchedBackup.Spec.Storage.S3).NotTo(BeNil())
			Expect(fetchedBackup.Spec.Storage.S3.Bucket).To(Equal("my-backup-bucket"))
			Expect(fetchedBackup.Spec.Storage.S3.Region).To(Equal("us-west-2"))
			Expect(fetchedBackup.Spec.Storage.S3.Prefix).To(Equal("backups/postgres"))
			Expect(fetchedBackup.Spec.Storage.S3.Endpoint).To(Equal("https://s3.us-west-2.amazonaws.com"))
			Expect(fetchedBackup.Spec.Storage.S3.ForcePathStyle).To(BeFalse())
			Expect(fetchedBackup.Spec.Storage.S3.SecretRef.Name).To(Equal("s3-credentials"))

			// Clean up
			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse GCS storage configuration", func() {
			gcsBackupName := "test-backup-gcs"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gcsBackupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypeGCS,
						GCS: &dbopsv1alpha1.GCSStorageConfig{
							Bucket: "gcs-backup-bucket",
							Prefix: "db-backups/daily",
							SecretRef: dbopsv1alpha1.SecretKeySelector{
								Name: "gcs-service-account",
								Key:  "service-account.json",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: gcsBackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.Storage.Type).To(Equal(dbopsv1alpha1.StorageTypeGCS))
			Expect(fetchedBackup.Spec.Storage.GCS).NotTo(BeNil())
			Expect(fetchedBackup.Spec.Storage.GCS.Bucket).To(Equal("gcs-backup-bucket"))
			Expect(fetchedBackup.Spec.Storage.GCS.Prefix).To(Equal("db-backups/daily"))
			Expect(fetchedBackup.Spec.Storage.GCS.SecretRef.Name).To(Equal("gcs-service-account"))
			Expect(fetchedBackup.Spec.Storage.GCS.SecretRef.Key).To(Equal("service-account.json"))

			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse Azure storage configuration", func() {
			azureBackupName := "test-backup-azure"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      azureBackupName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypeAzure,
						Azure: &dbopsv1alpha1.AzureStorageConfig{
							Container:      "backup-container",
							StorageAccount: "mystorageaccount",
							Prefix:         "postgres/production",
							SecretRef: dbopsv1alpha1.SecretReference{
								Name: "azure-storage-credentials",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: azureBackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.Storage.Type).To(Equal(dbopsv1alpha1.StorageTypeAzure))
			Expect(fetchedBackup.Spec.Storage.Azure).NotTo(BeNil())
			Expect(fetchedBackup.Spec.Storage.Azure.Container).To(Equal("backup-container"))
			Expect(fetchedBackup.Spec.Storage.Azure.StorageAccount).To(Equal("mystorageaccount"))
			Expect(fetchedBackup.Spec.Storage.Azure.Prefix).To(Equal("postgres/production"))
			Expect(fetchedBackup.Spec.Storage.Azure.SecretRef.Name).To(Equal("azure-storage-credentials"))

			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse compression configuration", func() {
			compBackupName := "test-backup-compression"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compBackupName,
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
							SubPath:   "compressed",
						},
					},
					Compression: &dbopsv1alpha1.CompressionConfig{
						Enabled:   true,
						Algorithm: dbopsv1alpha1.CompressionZstd,
						Level:     9,
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: compBackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.Compression).NotTo(BeNil())
			Expect(fetchedBackup.Spec.Compression.Enabled).To(BeTrue())
			Expect(fetchedBackup.Spec.Compression.Algorithm).To(Equal(dbopsv1alpha1.CompressionZstd))
			Expect(fetchedBackup.Spec.Compression.Level).To(Equal(int32(9)))

			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse different compression algorithms", func() {
			// Test gzip compression
			gzipBackupName := "test-backup-gzip"
			gzipBackup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gzipBackupName,
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
						},
					},
					Compression: &dbopsv1alpha1.CompressionConfig{
						Enabled:   true,
						Algorithm: dbopsv1alpha1.CompressionGzip,
						Level:     6,
					},
				},
			}
			Expect(k8sClient.Create(ctx, gzipBackup)).To(Succeed())

			fetchedGzip := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: gzipBackupName, Namespace: namespace}, fetchedGzip)).To(Succeed())
			Expect(fetchedGzip.Spec.Compression.Algorithm).To(Equal(dbopsv1alpha1.CompressionGzip))

			// Test LZ4 compression
			lz4BackupName := "test-backup-lz4"
			lz4Backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      lz4BackupName,
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
						},
					},
					Compression: &dbopsv1alpha1.CompressionConfig{
						Enabled:   true,
						Algorithm: dbopsv1alpha1.CompressionLZ4,
						Level:     3,
					},
				},
			}
			Expect(k8sClient.Create(ctx, lz4Backup)).To(Succeed())

			fetchedLz4 := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: lz4BackupName, Namespace: namespace}, fetchedLz4)).To(Succeed())
			Expect(fetchedLz4.Spec.Compression.Algorithm).To(Equal(dbopsv1alpha1.CompressionLZ4))

			_ = k8sClient.Delete(ctx, gzipBackup)
			_ = k8sClient.Delete(ctx, lz4Backup)
		})

		It("should correctly parse encryption configuration", func() {
			encBackupName := "test-backup-encryption"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      encBackupName,
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
							SubPath:   "encrypted",
						},
					},
					Encryption: &dbopsv1alpha1.EncryptionConfig{
						Enabled:   true,
						Algorithm: dbopsv1alpha1.EncryptionAES256GCM,
						SecretRef: &dbopsv1alpha1.SecretKeySelector{
							Name: "encryption-key-secret",
							Key:  "encryption-key",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: encBackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.Encryption).NotTo(BeNil())
			Expect(fetchedBackup.Spec.Encryption.Enabled).To(BeTrue())
			Expect(fetchedBackup.Spec.Encryption.Algorithm).To(Equal(dbopsv1alpha1.EncryptionAES256GCM))
			Expect(fetchedBackup.Spec.Encryption.SecretRef).NotTo(BeNil())
			Expect(fetchedBackup.Spec.Encryption.SecretRef.Name).To(Equal("encryption-key-secret"))
			Expect(fetchedBackup.Spec.Encryption.SecretRef.Key).To(Equal("encryption-key"))

			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse AES-256-CBC encryption algorithm", func() {
			cbcBackupName := "test-backup-aes-cbc"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cbcBackupName,
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
						},
					},
					Encryption: &dbopsv1alpha1.EncryptionConfig{
						Enabled:   true,
						Algorithm: dbopsv1alpha1.EncryptionAES256CBC,
						SecretRef: &dbopsv1alpha1.SecretKeySelector{
							Name: "cbc-key-secret",
							Key:  "key",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cbcBackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.Encryption.Algorithm).To(Equal(dbopsv1alpha1.EncryptionAES256CBC))

			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse TTL/expiration spec", func() {
			ttlBackupName := "test-backup-ttl"
			backup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ttlBackupName,
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
							SubPath:   "ttl-backups",
						},
					},
					TTL:                   "168h", // 7 days
					ActiveDeadlineSeconds: 7200,
				},
			}
			Expect(k8sClient.Create(ctx, backup)).To(Succeed())

			fetchedBackup := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ttlBackupName, Namespace: namespace}, fetchedBackup)).To(Succeed())

			Expect(fetchedBackup.Spec.TTL).To(Equal("168h"))
			Expect(fetchedBackup.Spec.ActiveDeadlineSeconds).To(Equal(int64(7200)))

			_ = k8sClient.Delete(ctx, backup)
		})

		It("should correctly parse various TTL durations", func() {
			// Test short TTL (24 hours)
			shortTtlName := "test-backup-short-ttl"
			shortTtlBackup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shortTtlName,
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
						},
					},
					TTL: "24h",
				},
			}
			Expect(k8sClient.Create(ctx, shortTtlBackup)).To(Succeed())

			fetchedShort := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: shortTtlName, Namespace: namespace}, fetchedShort)).To(Succeed())
			Expect(fetchedShort.Spec.TTL).To(Equal("24h"))

			// Test long TTL (30 days = 720 hours)
			longTtlName := "test-backup-long-ttl"
			longTtlBackup := &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      longTtlName,
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
						},
					},
					TTL: "720h",
				},
			}
			Expect(k8sClient.Create(ctx, longTtlBackup)).To(Succeed())

			fetchedLong := &dbopsv1alpha1.DatabaseBackup{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: longTtlName, Namespace: namespace}, fetchedLong)).To(Succeed())
			Expect(fetchedLong.Spec.TTL).To(Equal("720h"))

			_ = k8sClient.Delete(ctx, shortTtlBackup)
			_ = k8sClient.Delete(ctx, longTtlBackup)
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
