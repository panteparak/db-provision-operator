//go:build e2e

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

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	// k8sClient is the standard Kubernetes client for core resources
	k8sClient kubernetes.Interface

	// dynamicClient is used to interact with CRDs (DatabaseInstance, Database, etc.)
	dynamicClient dynamic.Interface

	// E2E_DATABASE_ENGINE environment variable specifies which database to test (postgresql or mysql)
	databaseEngine = os.Getenv("E2E_DATABASE_ENGINE")

	// Default test timeout
	defaultTimeout = 2 * time.Minute

	// Polling interval for Eventually checks
	pollingInterval = 2 * time.Second
)

// GVRs for custom resources
var (
	databaseInstanceGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databaseinstances",
	}

	databaseGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databases",
	}

	databaseUserGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databaseusers",
	}

	databaseRoleGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databaseroles",
	}

	databaseGrantGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databasegrants",
	}

	databaseBackupGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databasebackups",
	}

	databaseRestoreGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databaserestores",
	}

	databaseBackupScheduleGVR = schema.GroupVersionResource{
		Group:    "dbops.dbprovision.io",
		Version:  "v1alpha1",
		Resource: "databasebackupschedules",
	}
)

// TestE2E runs the end-to-end (e2e) test suite for the project.
// These tests execute against a real Kubernetes cluster (k3d) with real database instances.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting db-provision-operator E2E test suite\n")
	_, _ = fmt.Fprintf(GinkgoWriter, "Database engine: %s\n", databaseEngine)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	By("setting up Kubernetes clients")

	cfg, err := config.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "Failed to get kubeconfig")

	k8sClient, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred(), "Failed to create Kubernetes client")

	dynamicClient, err = dynamic.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred(), "Failed to create dynamic client")

	By("verifying cluster connectivity")
	_, err = k8sClient.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to connect to Kubernetes cluster")

	By("verifying operator is running")
	Eventually(func() bool {
		pods, err := k8sClient.CoreV1().Pods("db-provision-operator-system").List(
			context.Background(),
			metav1.ListOptions{LabelSelector: "control-plane=controller-manager"},
		)
		if err != nil {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == "Running" {
				return true
			}
		}
		return false
	}, defaultTimeout, pollingInterval).Should(BeTrue(), "Operator pod should be running")

	_, _ = fmt.Fprintf(GinkgoWriter, "E2E test setup complete\n")
})

var _ = AfterSuite(func() {
	By("E2E test suite cleanup complete")
})

// Helper functions for tests

// createDatabaseInstance creates a DatabaseInstance CR and waits for it to become ready
func createDatabaseInstance(ctx context.Context, name, namespace, engine, host string, port int32, secretName, secretNamespace string) error {
	instance := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dbops.dbprovision.io/v1alpha1",
			"kind":       "DatabaseInstance",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"engine": engine,
				"connection": map[string]interface{}{
					"host":     host,
					"port":     port,
					"database": "postgres",
					"secretRef": map[string]interface{}{
						"name":      secretName,
						"namespace": secretNamespace,
					},
				},
			},
		},
	}

	if engine == "mysql" {
		instance.Object["spec"].(map[string]interface{})["connection"].(map[string]interface{})["database"] = "mysql"
	}

	_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Create(ctx, instance, metav1.CreateOptions{})
	return err
}

// waitForPhase waits for a CR to reach the specified phase
func waitForPhase(ctx context.Context, gvr schema.GroupVersionResource, name, namespace, expectedPhase string, timeout time.Duration) error {
	var lastPhase string
	err := wait.PollUntilContextTimeout(ctx, pollingInterval, timeout, true, func(ctx context.Context) (bool, error) {
		obj, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil // Keep polling on error
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		lastPhase = phase
		return phase == expectedPhase, nil
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for phase %s, last phase was %s: %w", expectedPhase, lastPhase, err)
	}
	return nil
}

// deleteResource deletes a CR by name and namespace
func deleteResource(ctx context.Context, gvr schema.GroupVersionResource, name, namespace string) error {
	return dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// getResourcePhase returns the current phase of a CR
func getResourcePhase(ctx context.Context, gvr schema.GroupVersionResource, name, namespace string) (string, error) {
	obj, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	phase, _, err := unstructured.NestedString(obj.Object, "status", "phase")
	return phase, err
}
