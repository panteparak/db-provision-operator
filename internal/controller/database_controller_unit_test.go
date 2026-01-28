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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// testScheme creates a scheme with all required types registered.
func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dbopsv1alpha1.AddToScheme(scheme))
	return scheme
}

// newFakeClient creates a fake client with the given objects.
func newFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&dbopsv1alpha1.Database{}).
		Build()
}

// TestDatabaseReconciler_NotFound tests that reconciliation handles a missing Database gracefully.
func TestDatabaseReconciler_NotFound(t *testing.T) {
	// Create reconciler with empty client
	fakeClient := newFakeClient()
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	// Reconcile a non-existent Database
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should not return an error for not found
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestDatabaseReconciler_SkipReconcile tests that reconciliation is skipped when annotation is present.
func TestDatabaseReconciler_SkipReconcile(t *testing.T) {
	// Create a Database with skip annotation
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationSkipReconcile: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should return empty result when skipping reconciliation")
}

// TestDatabaseReconciler_AddsFinalizer tests that the finalizer is added when not present.
func TestDatabaseReconciler_AddsFinalizer(t *testing.T) {
	// Create a Database without finalizer
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	_, err := r.Reconcile(context.Background(), req)
	// First reconcile adds finalizer, second reconcile will fail on instance not found
	// but we're testing that finalizer gets added

	// Refetch the Database
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	require.NoError(t, err)

	// Check finalizer was added
	assert.Contains(t, updatedDB.Finalizers, util.FinalizerDatabase, "Finalizer should be added")
}

// TestDatabaseReconciler_InstanceNotFound tests handling when DatabaseInstance is not found.
func TestDatabaseReconciler_InstanceNotFound(t *testing.T) {
	// Create a Database with finalizer but no corresponding instance
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-db",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabase},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "nonexistent-instance",
			},
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should not return error but requeue
	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not found")

	// Check status was updated
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "not found")
}

// TestDatabaseReconciler_InstanceNotReady tests handling when DatabaseInstance is not ready.
func TestDatabaseReconciler_InstanceNotReady(t *testing.T) {
	// Create instance in Pending state
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
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhasePending, // Not ready
		},
	}

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-db",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabase},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeClient(instance, database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not ready")

	// Check status was updated
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "Waiting for DatabaseInstance")
}

// TestDatabaseReconciler_SecretNotFound tests handling when credentials secret is not found.
func TestDatabaseReconciler_SecretNotFound(t *testing.T) {
	// Create ready instance but no secret
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
					Name: "nonexistent-secret",
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-db",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabase},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeClient(instance, database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should return error since secret is required
	require.Error(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when secret not found")

	// Check status was updated to failed
	updatedDB := &dbopsv1alpha1.Database{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
	// Error message contains "not found" for the missing secret
	assert.Contains(t, updatedDB.Status.Message, "not found")
}

// TestDatabaseReconciler_DeletionWithRetainPolicy tests that deletion with Retain policy removes finalizer without dropping database.
func TestDatabaseReconciler_DeletionWithRetainPolicy(t *testing.T) {
	now := metav1.Now()
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-db",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabase},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			DeletionPolicy: dbopsv1alpha1.DeletionPolicyRetain, // Retain policy
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// When finalizer is removed and DeletionTimestamp is set, the object is deleted
	// So Get() will return NotFound - this is the expected behavior
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseReconciler_DeletionProtection tests that deletion is blocked when protection is enabled.
func TestDatabaseReconciler_DeletionProtection(t *testing.T) {
	now := metav1.Now()
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-db",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabase},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			DeletionProtection: true, // Protection enabled
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should return error because deletion is protected
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer was NOT removed
	updatedDB := &dbopsv1alpha1.Database{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	require.NoError(t, getErr)
	assert.Contains(t, updatedDB.Finalizers, util.FinalizerDatabase, "Finalizer should still be present")
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
}

// TestDatabaseReconciler_ForceDeleteBypassesProtection tests that force-delete annotation bypasses protection.
func TestDatabaseReconciler_ForceDeleteBypassesProtection(t *testing.T) {
	now := metav1.Now()
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-db",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabase},
			DeletionTimestamp: &now,
			Annotations: map[string]string{
				util.AnnotationForceDelete: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			DeletionProtection: true, // Protection enabled but force-delete set
			DeletionPolicy:     dbopsv1alpha1.DeletionPolicyRetain,
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should succeed because force-delete bypasses protection
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// When finalizer is removed and DeletionTimestamp is set, the object is deleted
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseReconciler_CrossNamespaceInstance tests handling of cross-namespace instance reference.
func TestDatabaseReconciler_CrossNamespaceInstance(t *testing.T) {
	// Create instance in different namespace
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "other-namespace",
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
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	// Create secret in instance namespace
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "other-namespace",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-db",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabase},
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name:      "test-instance",
				Namespace: "other-namespace", // Cross-namespace reference
			},
		},
	}

	fakeClient := newFakeClient(instance, database, dbSecret)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	// This will fail at the service creation stage (no actual DB),
	// but it should successfully find the instance in the other namespace
	result, err := r.Reconcile(context.Background(), req)

	// Error is expected because we can't connect to real database
	// but it should NOT be "instance not found" error
	if err != nil {
		assert.NotContains(t, err.Error(), "not found")
	}

	// Check that status doesn't show "instance not found"
	updatedDB := &dbopsv1alpha1.Database{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	require.NoError(t, getErr)
	assert.NotContains(t, updatedDB.Status.Message, "DatabaseInstance not found",
		"Should find instance in other namespace")

	// We expect a requeue since we can't actually connect to a database
	assert.NotZero(t, result.RequeueAfter)
}

// TestDatabaseReconciler_DeletionWithDeletePolicy tests deletion policy actually tries to drop database.
func TestDatabaseReconciler_DeletionWithDeletePolicy(t *testing.T) {
	now := metav1.Now()

	// Create ready instance
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
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

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

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-db",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabase},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			DeletionPolicy: dbopsv1alpha1.DeletionPolicyDelete, // Delete policy
		},
	}

	fakeClient := newFakeClient(instance, database, dbSecret)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// The reconciler will try to connect to the actual database and fail,
	// but will still remove the finalizer (graceful degradation)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// When finalizer is removed and DeletionTimestamp is set, the object is deleted
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseReconciler_EmptyDeletionPolicyDefaultsToRetain tests that empty deletion policy defaults to Retain.
func TestDatabaseReconciler_EmptyDeletionPolicyDefaultsToRetain(t *testing.T) {
	now := metav1.Now()
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-db",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabase},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			// DeletionPolicy is empty - should default to Retain
		},
	}

	fakeClient := newFakeClient(database)
	r := &DatabaseReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-db",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should succeed - empty policy defaults to Retain, no database drop needed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// When finalizer is removed and DeletionTimestamp is set, the object is deleted
	updatedDB := &dbopsv1alpha1.Database{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedDB)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}
