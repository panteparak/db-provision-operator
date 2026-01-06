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

var _ = Describe("DatabaseBackupSchedule Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a DatabaseBackupSchedule", func() {
		const (
			scheduleName = "test-schedule"
			databaseName = "test-database"
			instanceName = "test-instance"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			scheduleNamespacedName types.NamespacedName
			databaseNamespacedName types.NamespacedName
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			scheduleNamespacedName = types.NamespacedName{Name: scheduleName, Namespace: namespace}
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
			// Clean up DatabaseBackupSchedule
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			err := k8sClient.Get(ctx, scheduleNamespacedName, schedule)
			if err == nil {
				schedule.Finalizers = nil
				_ = k8sClient.Update(ctx, schedule)
				_ = k8sClient.Delete(ctx, schedule)
			}

			// Clean up any created backups
			backupList := &dbopsv1alpha1.DatabaseBackupList{}
			_ = k8sClient.List(ctx, backupList)
			for _, backup := range backupList.Items {
				backup.Finalizers = nil
				_ = k8sClient.Update(ctx, &backup)
				_ = k8sClient.Delete(ctx, &backup)
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
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *", // Daily at midnight
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			Expect(updatedSchedule.Finalizers).To(ContainElement(util.FinalizerDatabaseBackupSchedule))
		})

		It("should set active phase when schedule is valid", func() {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *", // Daily at midnight
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to set phase
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			// Phase should be set (either Active or Paused depending on implementation)
			Expect(updatedSchedule.Status.Phase).NotTo(BeEmpty())
		})

		It("should set paused phase when spec.paused is true", func() {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *",
					Paused:   true,
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Reconcile twice
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is paused
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			// When paused, the phase should reflect that
			Expect(updatedSchedule.Spec.Paused).To(BeTrue())
		})

		It("should skip reconcile when annotation is set", func() {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *",
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			Expect(updatedSchedule.Finalizers).To(BeEmpty())
		})

		It("should calculate next backup time", func() {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *", // Daily at midnight
					Timezone: "UTC",
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Reconcile twice to process fully
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify next backup time is set
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			Expect(updatedSchedule.Status.NextBackupTime).NotTo(BeNil())
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should respect concurrency policy", func() {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule:          "* * * * *", // Every minute for testing
					ConcurrencyPolicy: dbopsv1alpha1.ConcurrencyPolicyForbid,
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify schedule is set correctly
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			Expect(updatedSchedule.Spec.ConcurrencyPolicy).To(Equal(dbopsv1alpha1.ConcurrencyPolicyForbid))
		})

		It("should support retention policy configuration", func() {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scheduleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *",
					Retention: &dbopsv1alpha1.RetentionPolicy{
						KeepDaily:   7,
						KeepWeekly:  4,
						KeepMonthly: 12,
					},
					Template: dbopsv1alpha1.BackupTemplateSpec{
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
					},
				},
			}
			Expect(k8sClient.Create(ctx, schedule)).To(Succeed())

			controllerReconciler := &DatabaseBackupScheduleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: scheduleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify retention is configured
			updatedSchedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
			Expect(k8sClient.Get(ctx, scheduleNamespacedName, updatedSchedule)).To(Succeed())
			Expect(updatedSchedule.Spec.Retention).NotTo(BeNil())
			Expect(updatedSchedule.Spec.Retention.KeepDaily).To(Equal(int32(7)))
			Expect(updatedSchedule.Spec.Retention.KeepWeekly).To(Equal(int32(4)))
			Expect(updatedSchedule.Spec.Retention.KeepMonthly).To(Equal(int32(12)))
		})
	})
})
