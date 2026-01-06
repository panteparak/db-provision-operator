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

var _ = Describe("DatabaseGrant Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating a DatabaseGrant", func() {
		const (
			grantName    = "test-grant"
			userName     = "test-user"
			instanceName = "test-instance"
			databaseName = "test-database"
			namespace    = "default"
		)

		var (
			ctx                    context.Context
			grantNamespacedName    types.NamespacedName
			userNamespacedName     types.NamespacedName
			instanceNamespacedName types.NamespacedName
			databaseNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			grantNamespacedName = types.NamespacedName{Name: grantName, Namespace: namespace}
			userNamespacedName = types.NamespacedName{Name: userName, Namespace: namespace}
			instanceNamespacedName = types.NamespacedName{Name: instanceName, Namespace: namespace}
			databaseNamespacedName = types.NamespacedName{Name: databaseName, Namespace: namespace}

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

			// Create a DatabaseUser
			user := &dbopsv1alpha1.DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseUserSpec{
					Username: "testuser",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			err = k8sClient.Get(ctx, userNamespacedName, &dbopsv1alpha1.DatabaseUser{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, user)).To(Succeed())
			}
		})

		AfterEach(func() {
			// Clean up DatabaseGrant
			grant := &dbopsv1alpha1.DatabaseGrant{}
			err := k8sClient.Get(ctx, grantNamespacedName, grant)
			if err == nil {
				grant.Finalizers = nil
				_ = k8sClient.Update(ctx, grant)
				_ = k8sClient.Delete(ctx, grant)
			}

			// Clean up DatabaseUser
			user := &dbopsv1alpha1.DatabaseUser{}
			err = k8sClient.Get(ctx, userNamespacedName, user)
			if err == nil {
				user.Finalizers = nil
				_ = k8sClient.Update(ctx, user)
				_ = k8sClient.Delete(ctx, user)
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
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: userName,
					},
					DatabaseRef: &dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			controllerReconciler := &DatabaseGrantReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: grantNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, updatedGrant)).To(Succeed())
			Expect(updatedGrant.Finalizers).To(ContainElement(util.FinalizerDatabaseGrant))
		})

		It("should set pending phase when user is not ready", func() {
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: userName,
					},
					DatabaseRef: &dbopsv1alpha1.DatabaseReference{
						Name: databaseName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			controllerReconciler := &DatabaseGrantReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile to add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: grantNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile to check phase (user not ready)
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: grantNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status
			updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, updatedGrant)).To(Succeed())
			Expect(updatedGrant.Status.Phase).To(Equal(dbopsv1alpha1.PhasePending))
		})

		It("should skip reconcile when annotation is set", func() {
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
					Annotations: map[string]string{
						util.AnnotationSkipReconcile: "true",
					},
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: userName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			controllerReconciler := &DatabaseGrantReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: grantNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())

			// Verify no finalizer was added (skipped)
			updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, updatedGrant)).To(Succeed())
			Expect(updatedGrant.Finalizers).To(BeEmpty())
		})

		It("should fail when user does not exist", func() {
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: "nonexistent-user",
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			controllerReconciler := &DatabaseGrantReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, _ = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: grantNamespacedName,
			})

			// Second reconcile handles the missing user
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: grantNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify status is failed
			updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, updatedGrant)).To(Succeed())
			Expect(updatedGrant.Status.Phase).To(Equal(dbopsv1alpha1.PhaseFailed))
		})

		It("should return not found error when resource does not exist", func() {
			controllerReconciler := &DatabaseGrantReconciler{
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
