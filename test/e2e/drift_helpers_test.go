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
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/db-provision-operator/test/e2e/testutil"
)

// Common drift test constants
const (
	driftTestNamespace   = "default"
	driftPostgresHost    = "host.k3d.internal"
	driftSecretName      = "postgres-credentials"
	driftSecretNamespace = "postgres"
	driftReconcileTimeout = 30 * time.Second
	driftPollingInterval  = 2 * time.Second
	driftTimeout          = 60 * time.Second
	driftInstanceTimeout  = 2 * time.Minute
)

// driftGetVerifierHost returns the host for the verifier to connect to.
func driftGetVerifierHost() string {
	if host := os.Getenv("E2E_DATABASE_HOST"); host != "" {
		return host
	}
	return driftPostgresHost
}

// driftGetVerifierPort returns the port for the verifier to connect to.
func driftGetVerifierPort() int32 {
	if portStr := os.Getenv("E2E_DATABASE_PORT"); portStr != "" {
		if port, err := strconv.ParseInt(portStr, 10, 32); err == nil {
			return int32(port)
		}
	}
	return 5432
}

// driftGetInstanceHost returns the host for DatabaseInstance CRs.
func driftGetInstanceHost() string {
	if host := os.Getenv("E2E_INSTANCE_HOST"); host != "" {
		return host
	}
	return driftPostgresHost
}

// driftGetInstancePort returns the port for DatabaseInstance CRs.
func driftGetInstancePort() int64 {
	if portStr := os.Getenv("E2E_INSTANCE_PORT"); portStr != "" {
		if port, err := strconv.ParseInt(portStr, 10, 64); err == nil {
			return port
		}
	}
	return 5432
}

// setupDriftVerifier creates and connects a PostgresVerifier using K8s secret credentials.
func setupDriftVerifier(ctx context.Context) (*testutil.PostgresVerifier, string, string) {
	adminUsername, err := getSecretValue(ctx, driftSecretNamespace, driftSecretName, "username")
	Expect(err).NotTo(HaveOccurred(), "Failed to get PostgreSQL username from secret")

	adminPassword, err := getSecretValue(ctx, driftSecretNamespace, driftSecretName, "password")
	Expect(err).NotTo(HaveOccurred(), "Failed to get PostgreSQL password from secret")

	verifierHost := driftGetVerifierHost()
	verifierPort := driftGetVerifierPort()
	GinkgoWriter.Printf("Drift test verifier: %s:%d user: %s\n", verifierHost, verifierPort, adminUsername)

	cfg := testutil.PostgresEngineConfig(verifierHost, verifierPort, adminUsername, adminPassword)
	verifier := testutil.NewPostgresVerifier(cfg)

	Eventually(func() error {
		return verifier.Connect(ctx)
	}, driftInstanceTimeout, driftPollingInterval).Should(Succeed(), "Should connect to PostgreSQL for drift verification")

	return verifier, adminUsername, adminPassword
}

// createNamespacedInstance creates a DatabaseInstance CR and waits for it to become Ready.
func createNamespacedInstance(ctx context.Context, name, namespace string) {
	By("creating DatabaseInstance " + name)
	instance := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dbops.dbprovision.io/v1alpha1",
			"kind":       "DatabaseInstance",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"engine": "postgres",
				"connection": map[string]interface{}{
					"host":     driftGetInstanceHost(),
					"port":     driftGetInstancePort(),
					"database": "postgres",
					"secretRef": map[string]interface{}{
						"name":      driftSecretName,
						"namespace": driftSecretNamespace,
					},
				},
			},
		},
	}

	_, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Create(ctx, instance, metav1.CreateOptions{})
	if err != nil {
		GinkgoWriter.Printf("DatabaseInstance %s creation error (may already exist): %v\n", name, err)
	}

	By("waiting for DatabaseInstance " + name + " to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(databaseInstanceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, driftInstanceTimeout, driftPollingInterval).Should(Equal("Ready"), "DatabaseInstance should become Ready")
}

// createClusterInstance creates a ClusterDatabaseInstance CR and waits for it to become Ready.
func createClusterInstance(ctx context.Context, name string) {
	By("creating ClusterDatabaseInstance " + name)
	instance := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dbops.dbprovision.io/v1alpha1",
			"kind":       "ClusterDatabaseInstance",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"engine": "postgres",
				"connection": map[string]interface{}{
					"host":     driftGetInstanceHost(),
					"port":     driftGetInstancePort(),
					"database": "postgres",
					"secretRef": map[string]interface{}{
						"name":      driftSecretName,
						"namespace": driftSecretNamespace,
					},
				},
				"healthCheck": map[string]interface{}{
					"enabled":         true,
					"intervalSeconds": int64(30),
				},
			},
		},
	}

	_, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Create(ctx, instance, metav1.CreateOptions{})
	if err != nil {
		GinkgoWriter.Printf("ClusterDatabaseInstance %s creation error (may already exist): %v\n", name, err)
	}

	By("waiting for ClusterDatabaseInstance " + name + " to become Ready")
	Eventually(func() string {
		obj, err := dynamicClient.Resource(clusterDatabaseInstanceGVR).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, driftInstanceTimeout, driftPollingInterval).Should(Equal("Ready"), "ClusterDatabaseInstance should become Ready")
}

// waitForDriftDetected polls CR status.drift.detected until true.
func waitForDriftDetected(gvr schema.GroupVersionResource, name, namespace string, timeout time.Duration) {
	Eventually(func() bool {
		var obj *unstructured.Unstructured
		var err error
		if namespace == "" {
			obj, err = dynamicClient.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
		} else {
			obj, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		}
		if err != nil {
			return false
		}
		detected, _, _ := unstructured.NestedBool(obj.Object, "status", "drift", "detected")
		return detected
	}, timeout, driftPollingInterval).Should(BeTrue(), "Drift should be detected in CR status for %s/%s", namespace, name)
}

// waitForDriftCleared polls CR status.drift.detected until false (or drift block removed).
func waitForDriftCleared(gvr schema.GroupVersionResource, name, namespace string, timeout time.Duration) {
	Eventually(func() bool {
		var obj *unstructured.Unstructured
		var err error
		if namespace == "" {
			obj, err = dynamicClient.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
		} else {
			obj, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		}
		if err != nil {
			return false
		}
		detected, found, _ := unstructured.NestedBool(obj.Object, "status", "drift", "detected")
		// Cleared if drift block is missing or detected is false
		return !found || !detected
	}, timeout, driftPollingInterval).Should(BeTrue(), "Drift should be cleared in CR status for %s/%s", namespace, name)
}

// assertNoDriftDetected asserts that drift is NOT detected within a time window (Consistently).
func assertNoDriftDetected(gvr schema.GroupVersionResource, name, namespace string, duration time.Duration) {
	Consistently(func() bool {
		var obj *unstructured.Unstructured
		var err error
		if namespace == "" {
			obj, err = dynamicClient.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
		} else {
			obj, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		}
		if err != nil {
			return false
		}
		detected, _, _ := unstructured.NestedBool(obj.Object, "status", "drift", "detected")
		return !detected
	}, duration, driftPollingInterval).Should(BeTrue(), "Drift should NOT be detected in ignore mode for %s/%s", namespace, name)
}

// getDriftDiffs reads status.drift.diffs from a CR.
func getDriftDiffs(gvr schema.GroupVersionResource, name, namespace string) ([]map[string]interface{}, error) {
	var obj *unstructured.Unstructured
	var err error
	if namespace == "" {
		obj, err = dynamicClient.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
	} else {
		obj, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
	}
	if err != nil {
		return nil, err
	}

	diffsRaw, found, err := unstructured.NestedSlice(obj.Object, "status", "drift", "diffs")
	if err != nil || !found {
		return nil, fmt.Errorf("no diffs found in status.drift.diffs")
	}

	diffs := make([]map[string]interface{}, 0, len(diffsRaw))
	for _, d := range diffsRaw {
		if m, ok := d.(map[string]interface{}); ok {
			diffs = append(diffs, m)
		}
	}
	return diffs, nil
}

// waitForReady polls CR status.phase until "Ready".
func waitForReady(gvr schema.GroupVersionResource, name, namespace string, timeout time.Duration) {
	Eventually(func() string {
		var obj *unstructured.Unstructured
		var err error
		if namespace == "" {
			obj, err = dynamicClient.Resource(gvr).Get(context.Background(), name, metav1.GetOptions{})
		} else {
			obj, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(context.Background(), name, metav1.GetOptions{})
		}
		if err != nil {
			return ""
		}
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		return phase
	}, timeout, driftPollingInterval).Should(Equal("Ready"), "%s/%s should become Ready", namespace, name)
}

// cleanupCR deletes a CR and waits for it to disappear.
func cleanupCR(gvr schema.GroupVersionResource, name, namespace string) {
	ctx := context.Background()
	if namespace == "" {
		_ = dynamicClient.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
			return err != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())
	} else {
		_ = dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		Eventually(func() bool {
			_, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			return err != nil
		}, getDeletionTimeout(), driftPollingInterval).Should(BeTrue())
	}
}

// updateCRSpec gets a CR, applies a mutator function to its spec, and updates it.
func updateCRSpec(gvr schema.GroupVersionResource, name, namespace string, mutator func(spec map[string]interface{})) {
	ctx := context.Background()
	var obj *unstructured.Unstructured
	var err error
	if namespace == "" {
		obj, err = dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	} else {
		obj, err = dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	Expect(err).NotTo(HaveOccurred())

	spec := obj.Object["spec"].(map[string]interface{})
	mutator(spec)

	if namespace == "" {
		_, err = dynamicClient.Resource(gvr).Update(ctx, obj, metav1.UpdateOptions{})
	} else {
		_, err = dynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	}
	Expect(err).NotTo(HaveOccurred())
}
