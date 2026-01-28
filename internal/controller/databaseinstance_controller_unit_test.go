//go:build !integration && !e2e && !envtest

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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// newFakeInstanceClient creates a fake client for DatabaseInstance tests.
func newFakeInstanceClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&dbopsv1alpha1.DatabaseInstance{}).
		Build()
}

// TestDatabaseInstanceReconciler_NotFound tests that reconciliation handles a missing instance gracefully.
func TestDatabaseInstanceReconciler_NotFound(t *testing.T) {
	fakeClient := newFakeInstanceClient()
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestDatabaseInstanceReconciler_SkipReconcile tests that reconciliation is skipped when annotation is present.
func TestDatabaseInstanceReconciler_SkipReconcile(t *testing.T) {
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
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
					Name: "test-secret",
				},
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should return empty result when skipping reconciliation")
}

// TestDatabaseInstanceReconciler_AddsFinalizer tests that the finalizer is added when not present.
func TestDatabaseInstanceReconciler_AddsFinalizer(t *testing.T) {
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "test-secret",
				},
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	// First reconcile adds finalizer
	_, _ = r.Reconcile(context.Background(), req)

	// Refetch the instance
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	err := fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	require.NoError(t, err)

	assert.Contains(t, updatedInstance.Finalizers, util.FinalizerDatabaseInstance, "Finalizer should be added")
}

// TestDatabaseInstanceReconciler_SecretNotFound tests handling when credentials secret is not found.
func TestDatabaseInstanceReconciler_SecretNotFound(t *testing.T) {
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-instance",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseInstance},
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

	fakeClient := newFakeInstanceClient(instance)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.Error(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when secret not found")

	// Check status was updated to failed
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	// Error message contains "not found" for the missing secret
	assert.Contains(t, updatedInstance.Status.Message, "not found")
}

// TestDatabaseInstanceReconciler_CrossNamespaceSecret tests handling of cross-namespace secret reference.
func TestDatabaseInstanceReconciler_CrossNamespaceSecret(t *testing.T) {
	// Create secret in different namespace
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "secrets-namespace",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-instance",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseInstance},
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name:      "test-secret",
					Namespace: "secrets-namespace", // Cross-namespace reference
				},
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance, dbSecret)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	// This will fail at the service creation stage (no actual DB),
	// but it should successfully find the secret in the other namespace
	result, err := r.Reconcile(context.Background(), req)

	// Error is expected because we can't connect to real database
	// but it should NOT be "secret not found" error
	if err != nil {
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "not found")
	}

	// Check that status doesn't show "credentials" error
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	require.NoError(t, getErr)
	// The error should be about connection, not about credentials
	if updatedInstance.Status.Phase == dbopsv1alpha1.PhaseFailed {
		assert.NotContains(t, updatedInstance.Status.Message, "credentials",
			"Should find secret in other namespace")
	}

	assert.NotZero(t, result.RequeueAfter)
}

// TestDatabaseInstanceReconciler_Deletion tests that deletion removes finalizer and cleans up metrics.
func TestDatabaseInstanceReconciler_Deletion(t *testing.T) {
	now := metav1.Now()
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-instance",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseInstance},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "test-secret",
				},
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// When finalizer is removed and DeletionTimestamp is set, the object is deleted
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseInstanceReconciler_TLSSecretNotFound tests handling when TLS secret is not found.
func TestDatabaseInstanceReconciler_TLSSecretNotFound(t *testing.T) {
	// Create credentials secret
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-instance",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseInstance},
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "test-secret",
				},
			},
			TLS: &dbopsv1alpha1.TLSConfig{
				Enabled: true,
				SecretRef: &dbopsv1alpha1.TLSSecretRef{
					Name: "nonexistent-tls-secret", // TLS secret doesn't exist
				},
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance, dbSecret)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.Error(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when TLS secret not found")

	// Check status was updated
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	assert.Contains(t, updatedInstance.Status.Message, "TLS")
}

// TestDatabaseInstanceReconciler_MySQLEngine tests that MySQL engine type is handled correctly.
func TestDatabaseInstanceReconciler_MySQLEngine(t *testing.T) {
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-instance",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseInstance},
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypeMySQL,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 3306,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "test-secret",
				},
			},
			MySQL: &dbopsv1alpha1.MySQLInstanceConfig{
				Charset:   "utf8mb4",
				Collation: "utf8mb4_unicode_ci",
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance, dbSecret)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	// This will fail at connection time (no actual DB), but should get past config creation
	result, err := r.Reconcile(context.Background(), req)

	// Error is expected because we can't connect to real database
	// but it should not be about engine type or configuration
	if err != nil {
		assert.NotContains(t, err.Error(), "engine")
		assert.NotContains(t, err.Error(), "mysql")
	}

	// Should requeue for retry
	assert.NotZero(t, result.RequeueAfter)

	// Check that the instance entered a state past credential retrieval
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	require.NoError(t, getErr)
	// Status should reflect connection failure, not credential failure
	if updatedInstance.Status.Phase == dbopsv1alpha1.PhaseFailed {
		assert.NotContains(t, updatedInstance.Status.Message, "credentials")
	}
}

// TestDatabaseInstanceReconciler_HealthCheckInterval tests that custom health check interval is used.
func TestDatabaseInstanceReconciler_HealthCheckIntervalConfig(t *testing.T) {
	// This test verifies that the health check interval configuration is properly read.
	// We can't test the actual requeue interval without a working database connection,
	// but we can verify the configuration is accepted without error.

	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "test-secret",
				},
			},
			HealthCheck: &dbopsv1alpha1.HealthCheckConfig{
				Enabled:         true,
				IntervalSeconds: 120, // Custom interval
			},
		},
	}

	fakeClient := newFakeInstanceClient(instance)
	r := &DatabaseInstanceReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-instance",
			Namespace: "default",
		},
	}

	// First reconcile adds finalizer, subsequent reconciles will fail on credentials
	_, _ = r.Reconcile(context.Background(), req)

	// Verify the instance was processed (finalizer added)
	updatedInstance := &dbopsv1alpha1.DatabaseInstance{}
	err := fakeClient.Get(context.Background(), req.NamespacedName, updatedInstance)
	require.NoError(t, err)
	assert.Contains(t, updatedInstance.Finalizers, util.FinalizerDatabaseInstance)
}
