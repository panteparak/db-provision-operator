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

package util

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Finalizers", func() {
	var (
		obj *corev1.ConfigMap
	)

	BeforeEach(func() {
		obj = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-object",
				Namespace: "default",
			},
		}
	})

	Describe("AddFinalizer", func() {
		Context("when finalizer does not exist", func() {
			It("should add the finalizer", func() {
				result := AddFinalizer(obj, FinalizerDatabase)

				Expect(result).To(BeTrue())
				Expect(obj.GetFinalizers()).To(ContainElement(FinalizerDatabase))
			})

			It("should add multiple different finalizers", func() {
				AddFinalizer(obj, FinalizerDatabase)
				AddFinalizer(obj, FinalizerDatabaseUser)

				Expect(obj.GetFinalizers()).To(HaveLen(2))
				Expect(obj.GetFinalizers()).To(ContainElement(FinalizerDatabase))
				Expect(obj.GetFinalizers()).To(ContainElement(FinalizerDatabaseUser))
			})
		})

		Context("when finalizer already exists", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{FinalizerDatabase})
			})

			It("should return false", func() {
				result := AddFinalizer(obj, FinalizerDatabase)

				Expect(result).To(BeFalse())
			})

			It("should not duplicate the finalizer", func() {
				AddFinalizer(obj, FinalizerDatabase)

				Expect(obj.GetFinalizers()).To(HaveLen(1))
			})
		})

		Context("when object has nil finalizers", func() {
			BeforeEach(func() {
				obj.SetFinalizers(nil)
			})

			It("should add the finalizer", func() {
				result := AddFinalizer(obj, FinalizerDatabase)

				Expect(result).To(BeTrue())
				Expect(obj.GetFinalizers()).To(ContainElement(FinalizerDatabase))
			})
		})
	})

	Describe("RemoveFinalizer", func() {
		Context("when finalizer exists", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{FinalizerDatabase, FinalizerDatabaseUser})
			})

			It("should remove the finalizer", func() {
				result := RemoveFinalizer(obj, FinalizerDatabase)

				Expect(result).To(BeTrue())
				Expect(obj.GetFinalizers()).NotTo(ContainElement(FinalizerDatabase))
			})

			It("should preserve other finalizers", func() {
				RemoveFinalizer(obj, FinalizerDatabase)

				Expect(obj.GetFinalizers()).To(ContainElement(FinalizerDatabaseUser))
			})
		})

		Context("when finalizer does not exist", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{FinalizerDatabaseUser})
			})

			It("should return false", func() {
				result := RemoveFinalizer(obj, FinalizerDatabase)

				Expect(result).To(BeFalse())
			})

			It("should not modify the list", func() {
				RemoveFinalizer(obj, FinalizerDatabase)

				Expect(obj.GetFinalizers()).To(HaveLen(1))
				Expect(obj.GetFinalizers()).To(ContainElement(FinalizerDatabaseUser))
			})
		})

		Context("when removing the only finalizer", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{FinalizerDatabase})
			})

			It("should result in empty finalizers", func() {
				RemoveFinalizer(obj, FinalizerDatabase)

				Expect(obj.GetFinalizers()).To(BeEmpty())
			})
		})

		Context("when object has nil finalizers", func() {
			BeforeEach(func() {
				obj.SetFinalizers(nil)
			})

			It("should return false", func() {
				result := RemoveFinalizer(obj, FinalizerDatabase)

				Expect(result).To(BeFalse())
			})
		})
	})

	Describe("HasFinalizer", func() {
		Context("when finalizer exists", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{FinalizerDatabase, FinalizerDatabaseUser})
			})

			It("should return true", func() {
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeTrue())
			})

			It("should find any finalizer in the list", func() {
				Expect(HasFinalizer(obj, FinalizerDatabaseUser)).To(BeTrue())
			})
		})

		Context("when finalizer does not exist", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{FinalizerDatabaseUser})
			})

			It("should return false", func() {
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeFalse())
			})
		})

		Context("when finalizers is empty", func() {
			BeforeEach(func() {
				obj.SetFinalizers([]string{})
			})

			It("should return false", func() {
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeFalse())
			})
		})

		Context("when finalizers is nil", func() {
			BeforeEach(func() {
				obj.SetFinalizers(nil)
			})

			It("should return false", func() {
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeFalse())
			})
		})
	})

	Describe("IsMarkedForDeletion", func() {
		Context("when object is marked for deletion", func() {
			BeforeEach(func() {
				now := metav1.NewTime(time.Now())
				obj.SetDeletionTimestamp(&now)
			})

			It("should return true", func() {
				Expect(IsMarkedForDeletion(obj)).To(BeTrue())
			})
		})

		Context("when object is not marked for deletion", func() {
			It("should return false", func() {
				Expect(IsMarkedForDeletion(obj)).To(BeFalse())
			})
		})

		Context("when deletion timestamp is zero", func() {
			BeforeEach(func() {
				obj.SetDeletionTimestamp(nil)
			})

			It("should return false", func() {
				Expect(IsMarkedForDeletion(obj)).To(BeFalse())
			})
		})
	})

	Describe("HasForceDeleteAnnotation", func() {
		Context("when force-delete annotation is set to true", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationForceDelete: "true",
				})
			})

			It("should return true", func() {
				Expect(HasForceDeleteAnnotation(obj)).To(BeTrue())
			})
		})

		Context("when force-delete annotation is set to false", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationForceDelete: "false",
				})
			})

			It("should return false", func() {
				Expect(HasForceDeleteAnnotation(obj)).To(BeFalse())
			})
		})

		Context("when force-delete annotation is set to other value", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationForceDelete: "yes",
				})
			})

			It("should return false", func() {
				Expect(HasForceDeleteAnnotation(obj)).To(BeFalse())
			})
		})

		Context("when force-delete annotation is empty string", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationForceDelete: "",
				})
			})

			It("should return false", func() {
				Expect(HasForceDeleteAnnotation(obj)).To(BeFalse())
			})
		})

		Context("when force-delete annotation is not set", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					"other-annotation": "value",
				})
			})

			It("should return false", func() {
				Expect(HasForceDeleteAnnotation(obj)).To(BeFalse())
			})
		})

		Context("when annotations is nil", func() {
			BeforeEach(func() {
				obj.SetAnnotations(nil)
			})

			It("should return false", func() {
				Expect(HasForceDeleteAnnotation(obj)).To(BeFalse())
			})
		})
	})

	Describe("ShouldSkipReconcile", func() {
		Context("when skip-reconcile annotation is set to true", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile: "true",
				})
			})

			It("should return true", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())
			})
		})

		Context("when pause-reconcile annotation is set to true", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationPauseReconcile: "true",
				})
			})

			It("should return true", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())
			})
		})

		Context("when both skip and pause annotations are set to true", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile:  "true",
					AnnotationPauseReconcile: "true",
				})
			})

			It("should return true", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())
			})
		})

		Context("when skip-reconcile is false and pause-reconcile is true", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile:  "false",
					AnnotationPauseReconcile: "true",
				})
			})

			It("should return true", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())
			})
		})

		Context("when skip-reconcile is true and pause-reconcile is false", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile:  "true",
					AnnotationPauseReconcile: "false",
				})
			})

			It("should return true", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())
			})
		})

		Context("when skip-reconcile annotation is set to false", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile: "false",
				})
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})

		Context("when pause-reconcile annotation is set to false", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationPauseReconcile: "false",
				})
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})

		Context("when both annotations are set to false", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile:  "false",
					AnnotationPauseReconcile: "false",
				})
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})

		Context("when annotations are not set", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					"other-annotation": "value",
				})
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})

		Context("when annotations is nil", func() {
			BeforeEach(func() {
				obj.SetAnnotations(nil)
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})

		Context("when skip-reconcile has non-boolean value", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationSkipReconcile: "yes",
				})
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})

		Context("when pause-reconcile has non-boolean value", func() {
			BeforeEach(func() {
				obj.SetAnnotations(map[string]string{
					AnnotationPauseReconcile: "1",
				})
			})

			It("should return false", func() {
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())
			})
		})
	})

	Describe("Finalizer Constants", func() {
		It("should have expected finalizer values", func() {
			Expect(FinalizerDatabaseInstance).To(Equal("dbops.dbprovision.io/instance-protection"))
			Expect(FinalizerDatabase).To(Equal("dbops.dbprovision.io/database-protection"))
			Expect(FinalizerDatabaseUser).To(Equal("dbops.dbprovision.io/user-protection"))
			Expect(FinalizerDatabaseRole).To(Equal("dbops.dbprovision.io/role-protection"))
			Expect(FinalizerDatabaseGrant).To(Equal("dbops.dbprovision.io/grant-protection"))
			Expect(FinalizerDatabaseBackup).To(Equal("dbops.dbprovision.io/backup-protection"))
			Expect(FinalizerDatabaseRestore).To(Equal("dbops.dbprovision.io/restore-protection"))
			Expect(FinalizerDatabaseBackupSchedule).To(Equal("dbops.dbprovision.io/schedule-protection"))
		})
	})

	Describe("Annotation Constants", func() {
		It("should have expected annotation values", func() {
			Expect(AnnotationForceDelete).To(Equal("dbops.dbprovision.io/force-delete"))
			Expect(AnnotationSkipReconcile).To(Equal("dbops.dbprovision.io/skip-reconcile"))
			Expect(AnnotationPauseReconcile).To(Equal("dbops.dbprovision.io/pause-reconcile"))
		})
	})

	Describe("Integration scenarios", func() {
		Context("typical deletion workflow", func() {
			It("should handle finalizer lifecycle correctly", func() {
				// Initially no finalizer
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeFalse())

				// Add finalizer during create/update
				result := AddFinalizer(obj, FinalizerDatabase)
				Expect(result).To(BeTrue())
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeTrue())

				// Mark for deletion
				now := metav1.NewTime(time.Now())
				obj.SetDeletionTimestamp(&now)
				Expect(IsMarkedForDeletion(obj)).To(BeTrue())

				// Remove finalizer after cleanup
				result = RemoveFinalizer(obj, FinalizerDatabase)
				Expect(result).To(BeTrue())
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeFalse())
			})
		})

		Context("force delete workflow", func() {
			It("should handle force delete with finalizer", func() {
				// Add finalizer
				AddFinalizer(obj, FinalizerDatabase)

				// Mark for deletion
				now := metav1.NewTime(time.Now())
				obj.SetDeletionTimestamp(&now)

				// Without force-delete annotation
				Expect(HasForceDeleteAnnotation(obj)).To(BeFalse())

				// Set force-delete annotation
				obj.SetAnnotations(map[string]string{
					AnnotationForceDelete: "true",
				})
				Expect(HasForceDeleteAnnotation(obj)).To(BeTrue())

				// Remove finalizer to allow deletion
				RemoveFinalizer(obj, FinalizerDatabase)
				Expect(HasFinalizer(obj, FinalizerDatabase)).To(BeFalse())
			})
		})

		Context("pause and resume reconciliation", func() {
			It("should handle pause/resume correctly", func() {
				// Initially not skipped
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())

				// Pause reconciliation
				obj.SetAnnotations(map[string]string{
					AnnotationPauseReconcile: "true",
				})
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())

				// Resume reconciliation
				annotations := obj.GetAnnotations()
				annotations[AnnotationPauseReconcile] = "false"
				obj.SetAnnotations(annotations)
				Expect(ShouldSkipReconcile(obj)).To(BeFalse())

				// Skip using skip-reconcile
				annotations[AnnotationSkipReconcile] = "true"
				obj.SetAnnotations(annotations)
				Expect(ShouldSkipReconcile(obj)).To(BeTrue())
			})
		})
	})
})
