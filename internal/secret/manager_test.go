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

package secret

import (
	"context"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret/testutil"
)

func TestSecretManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Secret Manager Suite")
}

var _ = Describe("Secret Manager", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		fakeClient client.Client
		manager   *Manager
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(dbopsv1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("NewManager", func() {
		It("should create a new manager with the provided client", func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
			Expect(manager).NotTo(BeNil())
			Expect(manager.client).To(Equal(fakeClient))
		})
	})

	Describe("GetCredentials", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the credential secret exists", func() {
			It("should retrieve credentials with default keys", func() {
				secret := testutil.NewCredentialSecret(testutil.TestCredentialSecretName, testutil.TestNamespace)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				ref := testutil.NewCredentialSecretRef(testutil.TestCredentialSecretName)
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, ref)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).NotTo(BeNil())
				Expect(creds.Username).To(Equal(testutil.TestUsername))
				Expect(creds.Password).To(Equal(testutil.TestPassword))
			})

			It("should retrieve credentials with custom keys", func() {
				secret := testutil.NewCredentialSecretWithKeys(
					testutil.TestCredentialSecretName,
					testutil.TestNamespace,
					"user",
					"pass",
				)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				ref := testutil.NewCredentialSecretRefWithKeys(testutil.TestCredentialSecretName, "user", "pass")
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, ref)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).NotTo(BeNil())
				Expect(creds.Username).To(Equal(testutil.TestUsername))
				Expect(creds.Password).To(Equal(testutil.TestPassword))
			})

			It("should use secretRef namespace when specified", func() {
				otherNamespace := "other-namespace"
				secret := testutil.NewCredentialSecret(testutil.TestCredentialSecretName, otherNamespace)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				ref := testutil.NewCredentialSecretRefWithNamespace(testutil.TestCredentialSecretName, otherNamespace)
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, ref)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).NotTo(BeNil())
				Expect(creds.Username).To(Equal(testutil.TestUsername))
			})
		})

		Context("when the credential secret does not exist", func() {
			It("should return an error", func() {
				ref := testutil.NewCredentialSecretRef("nonexistent-secret")
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, ref)

				Expect(err).To(HaveOccurred())
				Expect(creds).To(BeNil())
			})
		})

		Context("when the reference is nil", func() {
			It("should return an error", func() {
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("credential reference is nil"))
				Expect(creds).To(BeNil())
			})
		})

		Context("when the secret is missing required keys", func() {
			It("should return an error when username key is missing", func() {
				secret := testutil.NewOpaqueSecret(
					testutil.TestCredentialSecretName,
					testutil.TestNamespace,
					map[string][]byte{"password": []byte("pass")},
				)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				ref := testutil.NewCredentialSecretRef(testutil.TestCredentialSecretName)
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, ref)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not contain key username"))
				Expect(creds).To(BeNil())
			})

			It("should return an error when password key is missing", func() {
				secret := testutil.NewOpaqueSecret(
					testutil.TestCredentialSecretName,
					testutil.TestNamespace,
					map[string][]byte{"username": []byte("user")},
				)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				ref := testutil.NewCredentialSecretRef(testutil.TestCredentialSecretName)
				creds, err := manager.GetCredentials(ctx, testutil.TestNamespace, ref)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not contain key password"))
				Expect(creds).To(BeNil())
			})
		})
	})

	Describe("GetTLSCredentials", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when TLS is enabled and secret exists", func() {
			It("should retrieve TLS credentials with default keys", func() {
				secret := testutil.NewTLSSecret(testutil.TestTLSSecretName, testutil.TestNamespace)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				tlsConfig := testutil.NewTLSConfig(true, testutil.TestTLSSecretName)
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, tlsConfig)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).NotTo(BeNil())
				Expect(creds.CA).To(Equal([]byte(testutil.TestCACert)))
				Expect(creds.Cert).To(Equal([]byte(testutil.TestClientCert)))
				Expect(creds.Key).To(Equal([]byte(testutil.TestClientKey)))
			})

			It("should retrieve TLS credentials with custom keys", func() {
				secret := testutil.NewTLSSecretWithKeys(
					testutil.TestTLSSecretName,
					testutil.TestNamespace,
					"custom-ca",
					"custom-cert",
					"custom-key",
				)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				tlsConfig := testutil.NewTLSConfigWithKeys(
					testutil.TestTLSSecretName,
					"custom-ca",
					"custom-cert",
					"custom-key",
				)
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, tlsConfig)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).NotTo(BeNil())
				Expect(creds.CA).To(Equal([]byte(testutil.TestCACert)))
				Expect(creds.Cert).To(Equal([]byte(testutil.TestClientCert)))
				Expect(creds.Key).To(Equal([]byte(testutil.TestClientKey)))
			})

			It("should retrieve partial TLS credentials when only CA is present", func() {
				secret := testutil.NewTLSSecretWithCA(testutil.TestTLSSecretName, testutil.TestNamespace)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				tlsConfig := testutil.NewTLSConfig(true, testutil.TestTLSSecretName)
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, tlsConfig)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).NotTo(BeNil())
				Expect(creds.CA).To(Equal([]byte(testutil.TestCACert)))
				Expect(creds.Cert).To(BeNil())
				Expect(creds.Key).To(BeNil())
			})
		})

		Context("when TLS is disabled", func() {
			It("should return nil without error", func() {
				tlsConfig := testutil.NewTLSConfig(false, "")
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, tlsConfig)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).To(BeNil())
			})
		})

		Context("when TLSConfig is nil", func() {
			It("should return nil without error", func() {
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, nil)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).To(BeNil())
			})
		})

		Context("when TLS is enabled but SecretRef is nil", func() {
			It("should return nil without error", func() {
				tlsConfig := &dbopsv1alpha1.TLSConfig{
					Enabled:   true,
					SecretRef: nil,
				}
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, tlsConfig)

				Expect(err).NotTo(HaveOccurred())
				Expect(creds).To(BeNil())
			})
		})

		Context("when the TLS secret does not exist", func() {
			It("should return an error", func() {
				tlsConfig := testutil.NewTLSConfig(true, "nonexistent-secret")
				creds, err := manager.GetTLSCredentials(ctx, testutil.TestNamespace, tlsConfig)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get TLS secret"))
				Expect(creds).To(BeNil())
			})
		})
	})

	Describe("GetPassword", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the password secret exists", func() {
			It("should retrieve the password", func() {
				secret := testutil.NewPasswordSecret("password-secret", testutil.TestNamespace, "password", "mysecretpassword")
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				secretRef := testutil.NewExistingPasswordSecret("password-secret", "password")
				password, err := manager.GetPassword(ctx, testutil.TestNamespace, secretRef)

				Expect(err).NotTo(HaveOccurred())
				Expect(password).To(Equal("mysecretpassword"))
			})

			It("should use secretRef namespace when specified", func() {
				otherNamespace := "other-namespace"
				secret := testutil.NewPasswordSecret("password-secret", otherNamespace, "password", "mysecretpassword")
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				secretRef := testutil.NewExistingPasswordSecretWithNamespace("password-secret", otherNamespace, "password")
				password, err := manager.GetPassword(ctx, testutil.TestNamespace, secretRef)

				Expect(err).NotTo(HaveOccurred())
				Expect(password).To(Equal("mysecretpassword"))
			})
		})

		Context("when the password secret does not exist", func() {
			It("should return an error", func() {
				secretRef := testutil.NewExistingPasswordSecret("nonexistent-secret", "password")
				password, err := manager.GetPassword(ctx, testutil.TestNamespace, secretRef)

				Expect(err).To(HaveOccurred())
				Expect(password).To(BeEmpty())
			})
		})

		Context("when the reference is nil", func() {
			It("should return an error", func() {
				password, err := manager.GetPassword(ctx, testutil.TestNamespace, nil)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("password secret reference is nil"))
				Expect(password).To(BeEmpty())
			})
		})

		Context("when the key does not exist in the secret", func() {
			It("should return an error", func() {
				secret := testutil.NewPasswordSecret("password-secret", testutil.TestNamespace, "password", "mysecretpassword")
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				secretRef := testutil.NewExistingPasswordSecret("password-secret", "wrong-key")
				password, err := manager.GetPassword(ctx, testutil.TestNamespace, secretRef)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not contain key"))
				Expect(password).To(BeEmpty())
			})
		})
	})

	Describe("GetSecretData", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the secret exists", func() {
			It("should retrieve all data as string map", func() {
				data := map[string][]byte{
					"key1": []byte("value1"),
					"key2": []byte("value2"),
				}
				secret := testutil.NewOpaqueSecret("test-secret", testutil.TestNamespace, data)
				Expect(fakeClient.Create(ctx, secret)).To(Succeed())

				result, err := manager.GetSecretData(ctx, "test-secret", testutil.TestNamespace)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(HaveLen(2))
				Expect(result["key1"]).To(Equal("value1"))
				Expect(result["key2"]).To(Equal("value2"))
			})
		})

		Context("when the secret does not exist", func() {
			It("should return an error", func() {
				result, err := manager.GetSecretData(ctx, "nonexistent-secret", testutil.TestNamespace)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("CreateSecret", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when creating a new secret", func() {
			It("should create the secret without owner reference", func() {
				data := map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				}

				err := manager.CreateSecret(ctx, testutil.TestNamespace, "new-secret", data, nil)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was created
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "new-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(Equal(data))
				Expect(secret.Type).To(Equal(corev1.SecretTypeOpaque))
				Expect(secret.OwnerReferences).To(BeEmpty())
			})

			It("should create the secret with owner reference", func() {
				data := map[string][]byte{
					"username": []byte("admin"),
				}
				owner := &OwnerInfo{
					APIVersion: "dbops.io/v1alpha1",
					Kind:       "DatabaseUser",
					Name:       "test-user",
					UID:        types.UID("12345"),
				}

				err := manager.CreateSecret(ctx, testutil.TestNamespace, "new-secret", data, owner)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was created with owner reference
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "new-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.OwnerReferences).To(HaveLen(1))
				Expect(secret.OwnerReferences[0].Name).To(Equal("test-user"))
				Expect(secret.OwnerReferences[0].Kind).To(Equal("DatabaseUser"))
				Expect(*secret.OwnerReferences[0].Controller).To(BeTrue())
			})
		})

		Context("when the secret already exists", func() {
			It("should return an error", func() {
				existingSecret := testutil.NewOpaqueSecret("existing-secret", testutil.TestNamespace, map[string][]byte{"key": []byte("value")})
				Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

				data := map[string][]byte{"newkey": []byte("newvalue")}
				err := manager.CreateSecret(ctx, testutil.TestNamespace, "existing-secret", data, nil)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("CreateSecretWithOwner", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		It("should create a secret with owner reference from client.Object", func() {
			data := map[string][]byte{
				"username": []byte("admin"),
			}

			// Create a DatabaseUser as owner
			owner := &dbopsv1alpha1.DatabaseUser{}
			owner.SetName("test-user")
			owner.SetNamespace(testutil.TestNamespace)
			owner.SetUID(types.UID("owner-uid-123"))

			err := manager.CreateSecretWithOwner(ctx, testutil.TestNamespace, "new-secret", data, owner, scheme)
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Namespace: testutil.TestNamespace,
				Name:      "new-secret",
			}, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.OwnerReferences).To(HaveLen(1))
			Expect(secret.OwnerReferences[0].Name).To(Equal("test-user"))
			Expect(secret.OwnerReferences[0].UID).To(Equal(types.UID("owner-uid-123")))
		})

		It("should create a secret without owner reference when owner is nil", func() {
			data := map[string][]byte{
				"username": []byte("admin"),
			}

			err := manager.CreateSecretWithOwner(ctx, testutil.TestNamespace, "new-secret", data, nil, scheme)
			Expect(err).NotTo(HaveOccurred())

			// Verify secret was created without owner reference
			secret := &corev1.Secret{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Namespace: testutil.TestNamespace,
				Name:      "new-secret",
			}, secret)
			Expect(err).NotTo(HaveOccurred())
			Expect(secret.OwnerReferences).To(BeEmpty())
		})
	})

	Describe("UpdateSecret", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the secret exists", func() {
			It("should update the secret data", func() {
				existingSecret := testutil.NewOpaqueSecret("test-secret", testutil.TestNamespace, map[string][]byte{"old": []byte("data")})
				Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

				newData := map[string][]byte{"new": []byte("data")}
				err := manager.UpdateSecret(ctx, testutil.TestNamespace, "test-secret", newData)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was updated
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "test-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(Equal(newData))
			})
		})

		Context("when the secret does not exist", func() {
			It("should return an error", func() {
				newData := map[string][]byte{"new": []byte("data")}
				err := manager.UpdateSecret(ctx, testutil.TestNamespace, "nonexistent-secret", newData)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("EnsureSecret", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the secret does not exist", func() {
			It("should create the secret", func() {
				data := map[string][]byte{"key": []byte("value")}
				err := manager.EnsureSecret(ctx, testutil.TestNamespace, "new-secret", data, nil)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was created
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "new-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(Equal(data))
			})
		})

		Context("when the secret already exists", func() {
			It("should update the secret", func() {
				existingSecret := testutil.NewOpaqueSecret("existing-secret", testutil.TestNamespace, map[string][]byte{"old": []byte("data")})
				Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

				newData := map[string][]byte{"new": []byte("data")}
				err := manager.EnsureSecret(ctx, testutil.TestNamespace, "existing-secret", newData, nil)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was updated
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "existing-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(Equal(newData))
			})
		})
	})

	Describe("EnsureSecretWithOwner", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the secret does not exist", func() {
			It("should create the secret with owner reference", func() {
				data := map[string][]byte{"key": []byte("value")}

				owner := &dbopsv1alpha1.DatabaseUser{}
				owner.SetName("test-user")
				owner.SetNamespace(testutil.TestNamespace)
				owner.SetUID(types.UID("owner-uid-123"))

				err := manager.EnsureSecretWithOwner(ctx, testutil.TestNamespace, "new-secret", data, owner, scheme)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was created
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "new-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(Equal(data))
				Expect(secret.OwnerReferences).To(HaveLen(1))
			})
		})

		Context("when the secret already exists", func() {
			It("should update the secret", func() {
				existingSecret := testutil.NewOpaqueSecret("existing-secret", testutil.TestNamespace, map[string][]byte{"old": []byte("data")})
				Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

				newData := map[string][]byte{"new": []byte("data")}

				owner := &dbopsv1alpha1.DatabaseUser{}
				owner.SetName("test-user")
				owner.SetNamespace(testutil.TestNamespace)
				owner.SetUID(types.UID("owner-uid-123"))

				err := manager.EnsureSecretWithOwner(ctx, testutil.TestNamespace, "existing-secret", newData, owner, scheme)
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was updated
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "existing-secret",
				}, secret)
				Expect(err).NotTo(HaveOccurred())
				Expect(secret.Data).To(Equal(newData))
			})
		})
	})

	Describe("DeleteSecret", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the secret exists", func() {
			It("should delete the secret", func() {
				existingSecret := testutil.NewOpaqueSecret("test-secret", testutil.TestNamespace, map[string][]byte{"key": []byte("value")})
				Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

				err := manager.DeleteSecret(ctx, testutil.TestNamespace, "test-secret")
				Expect(err).NotTo(HaveOccurred())

				// Verify secret was deleted
				secret := &corev1.Secret{}
				err = fakeClient.Get(ctx, types.NamespacedName{
					Namespace: testutil.TestNamespace,
					Name:      "test-secret",
				}, secret)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the secret does not exist", func() {
			It("should return nil without error", func() {
				err := manager.DeleteSecret(ctx, testutil.TestNamespace, "nonexistent-secret")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("SecretExists", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
			manager = NewManager(fakeClient)
		})

		Context("when the secret exists", func() {
			It("should return true", func() {
				existingSecret := testutil.NewOpaqueSecret("test-secret", testutil.TestNamespace, map[string][]byte{"key": []byte("value")})
				Expect(fakeClient.Create(ctx, existingSecret)).To(Succeed())

				exists, err := manager.SecretExists(ctx, testutil.TestNamespace, "test-secret")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
			})
		})

		Context("when the secret does not exist", func() {
			It("should return false", func() {
				exists, err := manager.SecretExists(ctx, testutil.TestNamespace, "nonexistent-secret")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})
	})
})

var _ = Describe("GeneratePassword", func() {
	Context("with default options", func() {
		It("should generate a password with default length of 32", func() {
			password, err := GeneratePassword(nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(password).To(HaveLen(32))
		})
	})

	Context("with custom length", func() {
		It("should generate a password with specified length", func() {
			config := testutil.NewPasswordConfig(16, true)
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			Expect(password).To(HaveLen(16))
		})

		It("should generate a long password", func() {
			config := testutil.NewPasswordConfig(64, true)
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			Expect(password).To(HaveLen(64))
		})
	})

	Context("with special characters", func() {
		It("should include special characters when enabled", func() {
			config := testutil.NewPasswordConfig(100, true)
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			// With 100 characters, there should likely be at least one special char
			hasSpecial := strings.ContainsAny(password, "!@#$%^&*")
			Expect(hasSpecial).To(BeTrue())
		})

		It("should not include special characters when disabled", func() {
			config := testutil.NewPasswordConfig(100, false)
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			hasSpecial := strings.ContainsAny(password, "!@#$%^&*")
			Expect(hasSpecial).To(BeFalse())
		})
	})

	Context("with excluded characters", func() {
		It("should not include excluded characters", func() {
			config := testutil.NewPasswordConfigWithExclusions(100, true, "aeiouAEIOU")
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			// Verify no vowels are included
			for _, c := range "aeiouAEIOU" {
				Expect(password).NotTo(ContainSubstring(string(c)))
			}
		})

		It("should exclude special characters when specified", func() {
			config := testutil.NewPasswordConfigWithExclusions(100, true, "!@#$%^&*")
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			hasSpecial := strings.ContainsAny(password, "!@#$%^&*")
			Expect(hasSpecial).To(BeFalse())
		})

		It("should fallback to default charset when all chars are excluded", func() {
			// Exclude everything - should fallback to default alphanumeric
			config := testutil.NewPasswordConfigWithExclusions(32, false, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
			password, err := GeneratePassword(config)

			Expect(err).NotTo(HaveOccurred())
			Expect(password).To(HaveLen(32))
		})
	})

	Context("randomness", func() {
		It("should generate unique passwords", func() {
			passwords := make(map[string]bool)
			for i := 0; i < 100; i++ {
				password, err := GeneratePassword(nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(passwords[password]).To(BeFalse(), "Duplicate password generated")
				passwords[password] = true
			}
		})
	})
})

var _ = Describe("RenderSecretTemplate", func() {
	Context("with valid template", func() {
		It("should render a simple template", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{
				"username": "{{ .Username }}",
				"password": "{{ .Password }}",
			})

			data := TemplateData{
				Username: "testuser",
				Password: "testpassword",
			}

			result, err := RenderSecretTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
			Expect(string(result["username"])).To(Equal("testuser"))
			Expect(string(result["password"])).To(Equal("testpassword"))
		})

		It("should render a connection string template", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{
				"connection-string": "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode={{ .SSLMode }}",
			})

			data := TemplateData{
				Username: "admin",
				Password: "secret123",
				Host:     "db.example.com",
				Port:     5432,
				Database: "mydb",
				SSLMode:  "require",
			}

			result, err := RenderSecretTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(result["connection-string"])).To(Equal("postgresql://admin:secret123@db.example.com:5432/mydb?sslmode=require"))
		})

		It("should render all available fields", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{
				"all-fields": "user={{ .Username }},pass={{ .Password }},host={{ .Host }},port={{ .Port }},db={{ .Database }},ssl={{ .SSLMode }},ns={{ .Namespace }},name={{ .Name }}",
			})

			data := TemplateData{
				Username:  "admin",
				Password:  "secret",
				Host:      "localhost",
				Port:      5432,
				Database:  "testdb",
				SSLMode:   "disable",
				Namespace: "default",
				Name:      "my-secret",
			}

			result, err := RenderSecretTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			expected := "user=admin,pass=secret,host=localhost,port=5432,db=testdb,ssl=disable,ns=default,name=my-secret"
			Expect(string(result["all-fields"])).To(Equal(expected))
		})
	})

	Context("with nil template", func() {
		It("should return an error", func() {
			result, err := RenderSecretTemplate(nil, TemplateData{})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template is nil"))
			Expect(result).To(BeNil())
		})
	})

	Context("with invalid template syntax", func() {
		It("should return an error for malformed template", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{
				"invalid": "{{ .Username",
			})

			data := TemplateData{
				Username: "testuser",
			}

			result, err := RenderSecretTemplate(tmpl, data)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
		})

		It("should return an error for invalid field reference", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{
				"invalid": "{{ .NonExistentField }}",
			})

			data := TemplateData{
				Username: "testuser",
			}

			result, err := RenderSecretTemplate(tmpl, data)

			// Go templates error on invalid field references
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("can't evaluate field"))
			Expect(result).To(BeNil())
		})
	})

	Context("with empty data", func() {
		It("should render empty template data", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{})

			data := TemplateData{}

			result, err := RenderSecretTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(0))
		})
	})

	Context("with static values", func() {
		It("should handle templates with no variables", func() {
			tmpl := testutil.NewSecretTemplate(map[string]string{
				"static": "this-is-a-static-value",
			})

			data := TemplateData{}

			result, err := RenderSecretTemplate(tmpl, data)

			Expect(err).NotTo(HaveOccurred())
			Expect(string(result["static"])).To(Equal("this-is-a-static-value"))
		})
	})
})
