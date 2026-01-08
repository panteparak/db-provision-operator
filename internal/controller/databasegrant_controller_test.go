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

var _ = Describe("DatabaseGrant Controller", func() {
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

	Context("When verifying grant spec parsing", func() {
		const (
			grantName    = "test-grant-spec"
			userName     = "test-user-spec"
			instanceName = "test-instance-spec"
			databaseName = "test-database-spec"
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

		It("should correctly read PostgreSQL grant type with multiple privileges", func() {
			// Test that the spec correctly holds multiple privileges at different grant levels
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
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:        "testdb",
								Schema:          "public",
								Tables:          []string{"users", "orders", "products"},
								Privileges:      []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
								WithGrantOption: true,
							},
							{
								Database:   "testdb",
								Schema:     "analytics",
								Sequences:  []string{"*"},
								Privileges: []string{"USAGE", "SELECT"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			// Verify the spec was stored correctly
			fetchedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, fetchedGrant)).To(Succeed())

			// Verify PostgreSQL grants are correctly parsed
			Expect(fetchedGrant.Spec.Postgres).NotTo(BeNil())
			Expect(fetchedGrant.Spec.Postgres.Grants).To(HaveLen(2))

			// Verify first grant (table-level with multiple privileges)
			firstGrant := fetchedGrant.Spec.Postgres.Grants[0]
			Expect(firstGrant.Database).To(Equal("testdb"))
			Expect(firstGrant.Schema).To(Equal("public"))
			Expect(firstGrant.Tables).To(ContainElements("users", "orders", "products"))
			Expect(firstGrant.Privileges).To(ContainElements("SELECT", "INSERT", "UPDATE", "DELETE"))
			Expect(firstGrant.WithGrantOption).To(BeTrue())

			// Verify second grant (sequence-level)
			secondGrant := fetchedGrant.Spec.Postgres.Grants[1]
			Expect(secondGrant.Schema).To(Equal("analytics"))
			Expect(secondGrant.Sequences).To(ContainElement("*"))
			Expect(secondGrant.Privileges).To(ContainElements("USAGE", "SELECT"))
		})

		It("should correctly parse privilege array with various privilege types", func() {
			// Test comprehensive privilege array handling
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: userName,
					},
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								Tables:     []string{"*"},
								Privileges: []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"},
							},
							{
								Database:   "testdb",
								Schema:     "public",
								Functions:  []string{"my_function"},
								Privileges: []string{"EXECUTE"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			fetchedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, fetchedGrant)).To(Succeed())

			// Verify all table privileges are stored
			tableGrant := fetchedGrant.Spec.Postgres.Grants[0]
			Expect(tableGrant.Privileges).To(HaveLen(7))
			Expect(tableGrant.Privileges).To(ContainElements(
				"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER",
			))

			// Verify function privileges
			funcGrant := fetchedGrant.Spec.Postgres.Grants[1]
			Expect(funcGrant.Functions).To(ContainElement("my_function"))
			Expect(funcGrant.Privileges).To(ContainElement("EXECUTE"))
		})

		It("should correctly handle grantee reference with user and role assignments", func() {
			// Test that user reference and role assignments are correctly stored
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name:      userName,
						Namespace: namespace,
					},
					DatabaseRef: &dbopsv1alpha1.DatabaseReference{
						Name:      databaseName,
						Namespace: namespace,
					},
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						// Roles to assign to the user
						Roles: []string{"readonly", "app_user", "reporting"},
						// Direct grants
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								Tables:     []string{"audit_log"},
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			fetchedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, fetchedGrant)).To(Succeed())

			// Verify user reference
			Expect(fetchedGrant.Spec.UserRef.Name).To(Equal(userName))
			Expect(fetchedGrant.Spec.UserRef.Namespace).To(Equal(namespace))

			// Verify database reference
			Expect(fetchedGrant.Spec.DatabaseRef).NotTo(BeNil())
			Expect(fetchedGrant.Spec.DatabaseRef.Name).To(Equal(databaseName))

			// Verify role assignments
			Expect(fetchedGrant.Spec.Postgres.Roles).To(HaveLen(3))
			Expect(fetchedGrant.Spec.Postgres.Roles).To(ContainElements("readonly", "app_user", "reporting"))

			// Verify direct grants exist alongside role assignments
			Expect(fetchedGrant.Spec.Postgres.Grants).To(HaveLen(1))
		})

		It("should correctly store WithGrantOption for grant option spec", func() {
			// Test that WithGrantOption is correctly handled for both true and false cases
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      grantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: userName,
					},
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:        "testdb",
								Schema:          "public",
								Tables:          []string{"public_data"},
								Privileges:      []string{"SELECT", "INSERT"},
								WithGrantOption: true,
							},
							{
								Database:        "testdb",
								Schema:          "restricted",
								Tables:          []string{"internal_data"},
								Privileges:      []string{"SELECT"},
								WithGrantOption: false,
							},
						},
						// Also test default privileges with grant option concept
						DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilegeGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								GrantedBy:  "admin",
								ObjectType: "tables",
								Privileges: []string{"SELECT", "INSERT"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			fetchedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, grantNamespacedName, fetchedGrant)).To(Succeed())

			// Verify WithGrantOption is true for first grant
			Expect(fetchedGrant.Spec.Postgres.Grants[0].WithGrantOption).To(BeTrue())

			// Verify WithGrantOption is false for second grant
			Expect(fetchedGrant.Spec.Postgres.Grants[1].WithGrantOption).To(BeFalse())

			// Verify default privileges are correctly stored
			Expect(fetchedGrant.Spec.Postgres.DefaultPrivileges).To(HaveLen(1))
			defPriv := fetchedGrant.Spec.Postgres.DefaultPrivileges[0]
			Expect(defPriv.Database).To(Equal("testdb"))
			Expect(defPriv.Schema).To(Equal("public"))
			Expect(defPriv.GrantedBy).To(Equal("admin"))
			Expect(defPriv.ObjectType).To(Equal("tables"))
			Expect(defPriv.Privileges).To(ContainElements("SELECT", "INSERT"))
		})

		It("should correctly parse MySQL grant configuration", func() {
			// Create MySQL instance first
			mysqlInstanceName := "mysql-instance-spec"
			mysqlSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mysqlInstanceName + "-creds",
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("root"),
					"password": []byte("rootpass"),
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: mysqlSecret.Name, Namespace: namespace}, &corev1.Secret{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, mysqlSecret)).To(Succeed())
			}

			mysqlInstance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mysqlInstanceName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: dbopsv1alpha1.EngineTypeMySQL,
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: "localhost",
						Port: 3306,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: mysqlInstanceName + "-creds",
						},
					},
				},
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mysqlInstanceName, Namespace: namespace}, &dbopsv1alpha1.DatabaseInstance{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, mysqlInstance)).To(Succeed())
			}

			// Create MySQL user
			mysqlUserName := "mysql-user-spec"
			mysqlUser := &dbopsv1alpha1.DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mysqlUserName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseUserSpec{
					Username: "mysqluser",
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: mysqlInstanceName,
					},
				},
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: mysqlUserName, Namespace: namespace}, &dbopsv1alpha1.DatabaseUser{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, mysqlUser)).To(Succeed())
			}

			// Create grant with MySQL-specific configuration
			mysqlGrantName := "mysql-grant-spec"
			grant := &dbopsv1alpha1.DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mysqlGrantName,
					Namespace: namespace,
				},
				Spec: dbopsv1alpha1.DatabaseGrantSpec{
					UserRef: dbopsv1alpha1.UserReference{
						Name: mysqlUserName,
					},
					MySQL: &dbopsv1alpha1.MySQLGrantConfig{
						Roles: []string{"db_reader", "app_role"},
						Grants: []dbopsv1alpha1.MySQLGrant{
							{
								Level:           dbopsv1alpha1.MySQLGrantLevelDatabase,
								Database:        "appdb",
								Privileges:      []string{"SELECT", "INSERT", "UPDATE"},
								WithGrantOption: false,
							},
							{
								Level:           dbopsv1alpha1.MySQLGrantLevelTable,
								Database:        "appdb",
								Table:           "users",
								Privileges:      []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
								WithGrantOption: true,
							},
							{
								Level:           dbopsv1alpha1.MySQLGrantLevelColumn,
								Database:        "appdb",
								Table:           "sensitive_data",
								Columns:         []string{"name", "email"},
								Privileges:      []string{"SELECT", "UPDATE"},
								WithGrantOption: false,
							},
							{
								Level:           dbopsv1alpha1.MySQLGrantLevelGlobal,
								Privileges:      []string{"PROCESS", "SHOW DATABASES"},
								WithGrantOption: false,
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, grant)).To(Succeed())

			fetchedGrant := &dbopsv1alpha1.DatabaseGrant{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: mysqlGrantName, Namespace: namespace}, fetchedGrant)).To(Succeed())

			// Verify MySQL configuration
			Expect(fetchedGrant.Spec.MySQL).NotTo(BeNil())
			Expect(fetchedGrant.Spec.MySQL.Roles).To(ContainElements("db_reader", "app_role"))
			Expect(fetchedGrant.Spec.MySQL.Grants).To(HaveLen(4))

			// Verify database-level grant
			dbGrant := fetchedGrant.Spec.MySQL.Grants[0]
			Expect(dbGrant.Level).To(Equal(dbopsv1alpha1.MySQLGrantLevelDatabase))
			Expect(dbGrant.Database).To(Equal("appdb"))

			// Verify table-level grant with grant option
			tableGrant := fetchedGrant.Spec.MySQL.Grants[1]
			Expect(tableGrant.Level).To(Equal(dbopsv1alpha1.MySQLGrantLevelTable))
			Expect(tableGrant.Table).To(Equal("users"))
			Expect(tableGrant.WithGrantOption).To(BeTrue())

			// Verify column-level grant
			colGrant := fetchedGrant.Spec.MySQL.Grants[2]
			Expect(colGrant.Level).To(Equal(dbopsv1alpha1.MySQLGrantLevelColumn))
			Expect(colGrant.Columns).To(ContainElements("name", "email"))

			// Verify global-level grant
			globalGrant := fetchedGrant.Spec.MySQL.Grants[3]
			Expect(globalGrant.Level).To(Equal(dbopsv1alpha1.MySQLGrantLevelGlobal))
			Expect(globalGrant.Privileges).To(ContainElements("PROCESS", "SHOW DATABASES"))

			// Clean up MySQL resources
			_ = k8sClient.Delete(ctx, grant)
			_ = k8sClient.Delete(ctx, mysqlUser)
			_ = k8sClient.Delete(ctx, mysqlInstance)
			_ = k8sClient.Delete(ctx, mysqlSecret)
		})
	})
})
