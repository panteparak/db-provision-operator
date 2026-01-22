//go:build integration

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
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// Integration tests run controllers with the full manager to test:
// - Cross-resource dependency resolution
// - Finalizer chain execution
// - Status propagation between resources
// - Controller event handling

// IntegrationDBConfig holds database configuration from environment variables
type IntegrationDBConfig struct {
	Database string // postgresql, mysql, mariadb
	Host     string
	Port     int32
	User     string
	Password string
}

var (
	intCtx       context.Context
	intCancel    context.CancelFunc
	intTestEnv   *envtest.Environment
	intCfg       *rest.Config
	intK8sClient client.Client
	intDBConfig  IntegrationDBConfig
)

// getIntegrationDBConfig reads database configuration from environment variables
func getIntegrationDBConfig() IntegrationDBConfig {
	database := os.Getenv("INTEGRATION_TEST_DATABASE")
	if database == "" {
		database = "postgresql" // default
	}

	host := os.Getenv("INTEGRATION_TEST_HOST")
	if host == "" {
		host = "localhost"
	}

	portStr := os.Getenv("INTEGRATION_TEST_PORT")
	port := int32(5432) // default PostgreSQL port
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = int32(p)
		}
	}

	user := os.Getenv("INTEGRATION_TEST_USER")
	if user == "" {
		user = "admin"
	}

	password := os.Getenv("INTEGRATION_TEST_PASSWORD")
	if password == "" {
		password = "password123"
	}

	return IntegrationDBConfig{
		Database: database,
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
	}
}

// getEngineType converts database name to EngineType
func getEngineType(database string) dbopsv1alpha1.EngineType {
	switch database {
	case "mysql":
		return dbopsv1alpha1.EngineTypeMySQL
	case "mariadb":
		return dbopsv1alpha1.EngineTypeMariaDB
	default:
		return dbopsv1alpha1.EngineTypePostgres
	}
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Load database configuration from environment
	intDBConfig = getIntegrationDBConfig()
	GinkgoWriter.Printf("Integration test database: %s at %s:%d\n",
		intDBConfig.Database, intDBConfig.Host, intDBConfig.Port)

	intCtx, intCancel = context.WithCancel(context.TODO())

	var err error
	err = dbopsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	By("bootstrapping integration test environment")
	intTestEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDirIntegration() != "" {
		intTestEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDirIntegration()
	}

	intCfg, err = intTestEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(intCfg).NotTo(BeNil())

	intK8sClient, err = client.New(intCfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(intK8sClient).NotTo(BeNil())

	// Create default namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	_ = intK8sClient.Create(intCtx, ns)

	// Start the controller manager
	By("starting the controller manager")
	mgr, err := ctrl.NewManager(intCfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // Disable metrics server for tests
		},
	})
	Expect(err).NotTo(HaveOccurred())

	// Register controllers
	secretManager := secret.NewManager(mgr.GetClient())

	err = (&DatabaseInstanceReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		SecretManager: secretManager,
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&DatabaseReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&DatabaseUserReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		SecretManager: secretManager,
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&DatabaseRoleReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	err = (&DatabaseGrantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	// Start manager in goroutine
	go func() {
		defer GinkgoRecover()
		err = mgr.Start(intCtx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	By("tearing down the integration test environment")
	intCancel()
	err := intTestEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// getFirstFoundEnvTestBinaryDirIntegration locates the first binary in the specified path.
func getFirstFoundEnvTestBinaryDirIntegration() string {
	basePath := filepath.Join("..", "..", "bin", "k8s", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

var _ = Describe("Integration Tests", func() {
	Context("Cross-resource Dependencies", func() {
		const (
			testNamespace = "default"
			timeout       = time.Second * 60
			interval      = time.Millisecond * 500
		)

		It("should add finalizer to DatabaseInstance when created", func() {
			instanceName := fmt.Sprintf("int-test-instance-1-%s", intDBConfig.Database)

			// Create credentials secret with actual database credentials
			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"username": []byte(intDBConfig.User),
					"password": []byte(intDBConfig.Password),
				},
			}
			Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

			// Create DatabaseInstance with actual database connection
			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: getEngineType(intDBConfig.Database),
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: intDBConfig.Host,
						Port: intDBConfig.Port,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

			// Wait for finalizer to be added by the controller
			Eventually(func() []string {
				updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      instanceName,
					Namespace: testNamespace,
				}, updatedInstance)
				if err != nil {
					return nil
				}
				return updatedInstance.Finalizers
			}, timeout, interval).Should(ContainElement("dbops.dbprovision.io/databaseinstance"))

			// Cleanup
			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			_ = intK8sClient.Get(intCtx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updatedInstance)
			updatedInstance.Finalizers = nil
			_ = intK8sClient.Update(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, credSecret)
		})

		It("should set status phase to Ready when connected to real database", func() {
			instanceName := fmt.Sprintf("int-test-instance-2-%s", intDBConfig.Database)

			// Create credentials secret with actual database credentials
			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"username": []byte(intDBConfig.User),
					"password": []byte(intDBConfig.Password),
				},
			}
			Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

			// Create DatabaseInstance with actual database connection
			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: getEngineType(intDBConfig.Database),
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: intDBConfig.Host,
						Port: intDBConfig.Port,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

			// Wait for status phase to be Ready (connected to real database)
			Eventually(func() string {
				updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      instanceName,
					Namespace: testNamespace,
				}, updatedInstance)
				if err != nil {
					return ""
				}
				GinkgoWriter.Printf("Instance %s phase: %s\n", instanceName, updatedInstance.Status.Phase)
				return string(updatedInstance.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Cleanup
			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			_ = intK8sClient.Get(intCtx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updatedInstance)
			updatedInstance.Finalizers = nil
			_ = intK8sClient.Update(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, credSecret)
		})

		It("should handle Database creation with instanceRef", func() {
			instanceName := fmt.Sprintf("int-test-instance-3-%s", intDBConfig.Database)
			dbName := fmt.Sprintf("int_test_db_%s", intDBConfig.Database)

			// Create credentials secret with actual database credentials
			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"username": []byte(intDBConfig.User),
					"password": []byte(intDBConfig.Password),
				},
			}
			Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

			// Create DatabaseInstance first with actual database connection
			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: getEngineType(intDBConfig.Database),
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: intDBConfig.Host,
						Port: intDBConfig.Port,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

			// Wait for instance to be Ready first
			Eventually(func() string {
				updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      instanceName,
					Namespace: testNamespace,
				}, updatedInstance)
				if err != nil {
					return ""
				}
				return string(updatedInstance.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Create Database referencing the instance
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseSpec{
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, database)).To(Succeed())

			// Wait for Database to have a finalizer added
			Eventually(func() []string {
				updatedDB := &dbopsv1alpha1.Database{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      dbName,
					Namespace: testNamespace,
				}, updatedDB)
				if err != nil {
					return nil
				}
				return updatedDB.Finalizers
			}, timeout, interval).Should(ContainElement("dbops.dbprovision.io/database"))

			// Wait for Database to be Ready
			Eventually(func() string {
				updatedDB := &dbopsv1alpha1.Database{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      dbName,
					Namespace: testNamespace,
				}, updatedDB)
				if err != nil {
					return ""
				}
				GinkgoWriter.Printf("Database %s phase: %s\n", dbName, updatedDB.Status.Phase)
				return string(updatedDB.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Cleanup - delete database first (it depends on instance)
			updatedDB := &dbopsv1alpha1.Database{}
			_ = intK8sClient.Get(intCtx, types.NamespacedName{Name: dbName, Namespace: testNamespace}, updatedDB)
			updatedDB.Finalizers = nil
			_ = intK8sClient.Update(intCtx, updatedDB)
			_ = intK8sClient.Delete(intCtx, updatedDB)

			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			_ = intK8sClient.Get(intCtx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updatedInstance)
			updatedInstance.Finalizers = nil
			_ = intK8sClient.Update(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, credSecret)
		})

		It("should create DatabaseUser with generated credentials", func() {
			instanceName := fmt.Sprintf("int-test-instance-4-%s", intDBConfig.Database)
			userName := fmt.Sprintf("int-test-user-%s", intDBConfig.Database)

			// Create credentials secret with actual database credentials
			credSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName + "-creds",
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"username": []byte(intDBConfig.User),
					"password": []byte(intDBConfig.Password),
				},
			}
			Expect(intK8sClient.Create(intCtx, credSecret)).To(Succeed())

			// Create DatabaseInstance first
			instance := &dbopsv1alpha1.DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: getEngineType(intDBConfig.Database),
					Connection: dbopsv1alpha1.ConnectionConfig{
						Host: intDBConfig.Host,
						Port: intDBConfig.Port,
						SecretRef: &dbopsv1alpha1.CredentialSecretRef{
							Name: instanceName + "-creds",
						},
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, instance)).To(Succeed())

			// Wait for instance to be Ready
			Eventually(func() string {
				updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      instanceName,
					Namespace: testNamespace,
				}, updatedInstance)
				if err != nil {
					return ""
				}
				return string(updatedInstance.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Create DatabaseUser
			user := &dbopsv1alpha1.DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: testNamespace,
				},
				Spec: dbopsv1alpha1.DatabaseUserSpec{
					Username: fmt.Sprintf("testuser_%s", intDBConfig.Database),
					InstanceRef: dbopsv1alpha1.InstanceReference{
						Name: instanceName,
					},
				},
			}
			Expect(intK8sClient.Create(intCtx, user)).To(Succeed())

			// Wait for DatabaseUser to be Ready
			Eventually(func() string {
				updatedUser := &dbopsv1alpha1.DatabaseUser{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      userName,
					Namespace: testNamespace,
				}, updatedUser)
				if err != nil {
					return ""
				}
				GinkgoWriter.Printf("User %s phase: %s\n", userName, updatedUser.Status.Phase)
				return string(updatedUser.Status.Phase)
			}, timeout, interval).Should(Equal(string(dbopsv1alpha1.PhaseReady)))

			// Verify credentials secret was created
			Eventually(func() bool {
				userCredSecret := &corev1.Secret{}
				err := intK8sClient.Get(intCtx, types.NamespacedName{
					Name:      userName + "-credentials",
					Namespace: testNamespace,
				}, userCredSecret)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			updatedUser := &dbopsv1alpha1.DatabaseUser{}
			_ = intK8sClient.Get(intCtx, types.NamespacedName{Name: userName, Namespace: testNamespace}, updatedUser)
			updatedUser.Finalizers = nil
			_ = intK8sClient.Update(intCtx, updatedUser)
			_ = intK8sClient.Delete(intCtx, updatedUser)

			updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
			_ = intK8sClient.Get(intCtx, types.NamespacedName{Name: instanceName, Namespace: testNamespace}, updatedInstance)
			updatedInstance.Finalizers = nil
			_ = intK8sClient.Update(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, updatedInstance)
			_ = intK8sClient.Delete(intCtx, credSecret)
		})
	})
})
