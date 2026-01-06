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

var _ = Describe("DatabaseRole Controller", func() {
	Context("When creating a DatabaseRole", func() {
		const (
			roleName     = "test-role"
			instanceName = "test-instance"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			roleNamespacedName     types.NamespacedName
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			roleNamespacedName = types.NamespacedName{Name: roleName, Namespace: namespace}
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
			// Clean up DatabaseRole
			role := &dbopsv1alpha1.DatabaseRole{}
			err := k8sClient.Get(ctx, roleNamespacedName, role)
			if err == nil {
				// Remove finalizer to allow deletion
				role.Finalizers = nil
				_ = k8sClient.Update(ctx, role)
				_ = k8sClient.Delete(ctx, role)
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
			role := &dbopsv1alpha1.DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRoleSpec{
					RoleName: "testrole",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			controllerReconciler := &DatabaseRoleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: roleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedRole := &dbopsv1alpha1.DatabaseRole{}
			Expect(k8sClient.Get(ctx, roleNamespacedName, updatedRole)).To(Succeed())
			Expect(updatedRole.Finalizers).To(ContainElement(util.FinalizerDatabaseRole))
		})

		It("should set pending phase when instance is not ready", func() {
			role := &dbopsv1alpha1.DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRoleSpec{
					RoleName: "testrole",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			controllerReconciler := &DatabaseRoleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: roleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to check phase (instance not ready)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: roleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			updatedRole := &dbopsv1alpha1.DatabaseRole{}
			Expect(k8sClient.Get(ctx, roleNamespacedName, updatedRole)).To(Succeed())
			Expect(updatedRole.Status.Phase).To(Equal(dbopsv1alpha1.PhasePending))
		})

		It("should skip reconcile when annotation is set", func() {
			role := &dbopsv1alpha1.DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseRoleSpec{
					RoleName: "testrole",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			controllerReconciler := &DatabaseRoleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: roleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedRole := &dbopsv1alpha1.DatabaseRole{}
			Expect(k8sClient.Get(ctx, roleNamespacedName, updatedRole)).To(Succeed())
			Expect(updatedRole.Finalizers).To(BeEmpty())
		})

		It("should fail when instance does not exist", func() {
			role := &dbopsv1alpha1.DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseRoleSpec{
					RoleName: "testrole",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: "nonexistent-instance",
					},
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			controllerReconciler := &DatabaseRoleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: roleNamespacedName,
			})

			// Second reconcile handles the missing instance
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: roleNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is failed
			updatedRole := &dbopsv1alpha1.DatabaseRole{}
			Expect(k8sClient.Get(ctx, roleNamespacedName, updatedRole)).To(Succeed())
			Expect(updatedRole.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseRoleReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})
})
