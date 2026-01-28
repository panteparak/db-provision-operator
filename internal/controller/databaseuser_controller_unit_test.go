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

// newFakeUserClient creates a fake client for DatabaseUser tests.
func newFakeUserClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&dbopsv1alpha1.DatabaseUser{}).
		Build()
}

// TestDatabaseUserReconciler_NotFound tests that reconciliation handles a missing user gracefully.
func TestDatabaseUserReconciler_NotFound(t *testing.T) {
	fakeClient := newFakeUserClient()
	r := &DatabaseUserReconciler{
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

// TestDatabaseUserReconciler_SkipReconcile tests that reconciliation is skipped when annotation is present.
func TestDatabaseUserReconciler_SkipReconcile(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationSkipReconcile: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeUserClient(user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should return empty result when skipping reconciliation")
}

// TestDatabaseUserReconciler_AddsFinalizer tests that the finalizer is added when not present.
func TestDatabaseUserReconciler_AddsFinalizer(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeUserClient(user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	// First reconcile adds finalizer
	_, _ = r.Reconcile(context.Background(), req)

	// Refetch the user
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	err := fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	require.NoError(t, err)

	assert.Contains(t, updatedUser.Finalizers, util.FinalizerDatabaseUser, "Finalizer should be added")
}

// TestDatabaseUserReconciler_InstanceNotFound tests handling when DatabaseInstance is not found.
func TestDatabaseUserReconciler_InstanceNotFound(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-user",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseUser},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "nonexistent-instance",
			},
		},
	}

	fakeClient := newFakeUserClient(user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should not return error but requeue
	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not found")

	// Check status was updated
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedUser.Status.Phase)
	assert.Contains(t, updatedUser.Status.Message, "not found")
}

// TestDatabaseUserReconciler_InstanceNotReady tests handling when DatabaseInstance is not ready.
func TestDatabaseUserReconciler_InstanceNotReady(t *testing.T) {
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

	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-user",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseUser},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeUserClient(instance, user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not ready")

	// Check status was updated
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedUser.Status.Phase)
	assert.Contains(t, updatedUser.Status.Message, "Waiting for DatabaseInstance")
}

// TestDatabaseUserReconciler_AdminSecretNotFound tests handling when admin credentials secret is not found.
func TestDatabaseUserReconciler_AdminSecretNotFound(t *testing.T) {
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

	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-user",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseUser},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeUserClient(instance, user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.Error(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when admin secret not found")

	// Check status was updated to failed
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedUser.Status.Phase)
	// Error message contains "not found" for the missing secret
	assert.Contains(t, updatedUser.Status.Message, "not found")
}

// TestDatabaseUserReconciler_Deletion tests that deletion removes finalizer and allows resource cleanup.
func TestDatabaseUserReconciler_Deletion(t *testing.T) {
	now := metav1.Now()
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-user",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseUser},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newFakeUserClient(user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// When finalizer is removed and DeletionTimestamp is set, the object is deleted
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseUserReconciler_CrossNamespaceInstance tests handling of cross-namespace instance reference.
func TestDatabaseUserReconciler_CrossNamespaceInstance(t *testing.T) {
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

	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-user",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseUser},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name:      "test-instance",
				Namespace: "other-namespace", // Cross-namespace reference
			},
		},
	}

	fakeClient := newFakeUserClient(instance, user, dbSecret)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	// This will fail at the service creation stage (no actual DB),
	// but it should successfully find the instance in the other namespace
	result, err := r.Reconcile(context.Background(), req)

	// Check that status doesn't show "instance not found"
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	require.NoError(t, getErr)
	assert.NotContains(t, updatedUser.Status.Message, "DatabaseInstance not found",
		"Should find instance in other namespace")

	// We expect a requeue since we can't actually connect to a database
	if err != nil {
		assert.NotContains(t, err.Error(), "not found")
	}
	assert.NotZero(t, result.RequeueAfter)
}

// TestDatabaseUserReconciler_ExternalSecretRef tests handling when external secret reference is provided.
func TestDatabaseUserReconciler_ExternalSecretRef(t *testing.T) {
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
					Name: "admin-secret",
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	// Admin secret for the instance
	adminSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "admin-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("admin-secret"),
		},
	}

	// User's external password secret (pre-existing)
	userSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-user-password",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"password": []byte("my-external-password"),
		},
	}

	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-user",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseUser},
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
			ExistingPasswordSecret: &dbopsv1alpha1.ExistingPasswordSecret{
				Name: "my-user-password", // External secret reference
				Key:  "password",
			},
		},
	}

	fakeClient := newFakeUserClient(instance, adminSecret, userSecret, user)
	r := &DatabaseUserReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-user",
			Namespace: "default",
		},
	}

	// This will fail at the service creation stage (no actual DB),
	// but should get past the credential retrieval
	result, err := r.Reconcile(context.Background(), req)

	// Check that status doesn't show secret-related errors
	updatedUser := &dbopsv1alpha1.DatabaseUser{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedUser)
	require.NoError(t, getErr)
	assert.NotContains(t, updatedUser.Status.Message, "admin credentials")

	// We expect a requeue since we can't actually connect to a database
	if err != nil {
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "password")
	}
	assert.NotZero(t, result.RequeueAfter)
}
