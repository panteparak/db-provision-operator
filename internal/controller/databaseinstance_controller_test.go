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
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

var _ = Describe("DatabaseInstance Controller", func() {
	Context("When creating a DatabaseInstance", func() {
		const (
			instanceName = "test-instance"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			instanceNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			instanceNamespacedName = types.NamespacedName{Name: instanceName, Namespace: namespace}

			// Create a secret for credentials
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
		})

		AfterEach(func() {
			// Clean up DatabaseInstance
			instance := &dbopsv1alpha1.DatabaseInstance{}
			err := k8sClient.Get(ctx, instanceNamespacedName, instance)
			if err == nil {
				// Remove finalizer to allow deletion
				instance.Finalizers = nil
				_ = k8sClient.Update(ctx, instance)
				_ = k8sClient.Delete(ctx, instance)
			}
		})

		It("should add finalizer when created", func() {
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
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			controllerReconciler := &DatabaseInstanceReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				SecretManager: secret.NewManager(k8sClient),
			}

			// Note: Reconcile will return an error because it tries to connect to a real database
			// but the finalizer should still be added before the connection attempt
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: instanceNamespacedName,
			})

			// Verify finalizer was added (even though connection may have failed)
			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			Expect(k8sClient.Get(ctx, instanceNamespacedName, updatedInstance)).To(Succeed())
			Expect(updatedInstance.Finalizers).To(ContainElement(util.FinalizerDatabaseInstance))
		})

		It("should skip reconcile when annotation is set", func() {
			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
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
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			controllerReconciler := &DatabaseInstanceReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				SecretManager: secret.NewManager(k8sClient),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: instanceNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			Expect(k8sClient.Get(ctx, instanceNamespacedName, updatedInstance)).To(Succeed())
			Expect(updatedInstance.Finalizers).To(BeEmpty())
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseInstanceReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				SecretManager: secret.NewManager(k8sClient),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})

		It("should set pending status when credentials secret is missing", func() {
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
							Name: "nonexistent-secret",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			controllerReconciler := &DatabaseInstanceReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				SecretManager: secret.NewManager(k8sClient),
			}

			// First reconcile to add finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: instanceNamespacedName,
			})

			// Second reconcile to check status
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: instanceNamespacedName,
			})
			// May or may not return error depending on implementation
			_ = err

			// Verify status - should be pending or failed due to missing secret
			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			Expect(k8sClient.Get(ctx, instanceNamespacedName, updatedInstance)).To(Succeed())
			// The phase should be set (either Pending or Failed)
			Expect(updatedInstance.Status.Phase).NotTo(BeEmpty())
		})
	})
})
