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

// newGrantFakeClient creates a fake client with DatabaseGrant status subresource registered.
func newGrantFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(
			&dbopsv1alpha1.DatabaseGrant{},
			&dbopsv1alpha1.DatabaseUser{},
		).
		Build()
}

// TestDatabaseGrantReconciler_NotFound tests that reconciliation handles a missing DatabaseGrant gracefully.
func TestDatabaseGrantReconciler_NotFound(t *testing.T) {
	fakeClient := newGrantFakeClient()
	r := &DatabaseGrantReconciler{
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

// TestDatabaseGrantReconciler_SkipReconcile tests that reconciliation is skipped when annotation is present.
func TestDatabaseGrantReconciler_SkipReconcile(t *testing.T) {
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationSkipReconcile: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should return empty result when skipping reconciliation")
}

// TestDatabaseGrantReconciler_AddsFinalizer tests that the finalizer is added when not present.
func TestDatabaseGrantReconciler_AddsFinalizer(t *testing.T) {
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	_, _ = r.Reconcile(context.Background(), req)

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	err := fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	require.NoError(t, err)

	assert.Contains(t, updatedGrant.Finalizers, util.FinalizerDatabaseGrant, "Finalizer should be added")
}

// TestDatabaseGrantReconciler_UserNotFound tests handling when referenced DatabaseUser is not found.
func TestDatabaseGrantReconciler_UserNotFound(t *testing.T) {
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-grant",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseGrant},
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "nonexistent-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when user not found")

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "not found")
}

// TestDatabaseGrantReconciler_UserNotReady tests handling when referenced DatabaseUser is not ready.
func TestDatabaseGrantReconciler_UserNotReady(t *testing.T) {
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
		Status: dbopsv1alpha1.DatabaseUserStatus{
			Phase: dbopsv1alpha1.PhasePending, // Not ready
		},
	}

	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-grant",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseGrant},
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(user, grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when user not ready")

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "Waiting for DatabaseUser")
}

// TestDatabaseGrantReconciler_InstanceNotFound tests handling when the referenced instance is not found.
func TestDatabaseGrantReconciler_InstanceNotFound(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "nonexistent-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseUserStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-grant",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseGrant},
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(user, grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not found")

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "not found")
}

// TestDatabaseGrantReconciler_InstanceNotReady tests handling when instance is not ready.
func TestDatabaseGrantReconciler_InstanceNotReady(t *testing.T) {
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
			Name:      "test-user",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseUserStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-grant",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseGrant},
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(instance, user, grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not ready")

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "Waiting for DatabaseInstance")
}

// TestDatabaseGrantReconciler_DeletionRemovesFinalizer tests that deletion removes the finalizer.
func TestDatabaseGrantReconciler_DeletionRemovesFinalizer(t *testing.T) {
	now := metav1.Now()
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-grant",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseGrant},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Object should be deleted after finalizer removal
	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseGrantReconciler_DeletionWithUserNotFound tests deletion when referenced user is gone.
func TestDatabaseGrantReconciler_DeletionWithUserNotFound(t *testing.T) {
	now := metav1.Now()
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-grant",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseGrant},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name: "deleted-user",
			},
		},
	}

	fakeClient := newGrantFakeClient(grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should succeed even if user is not found — graceful cleanup
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseGrantReconciler_CrossNamespaceUserRef tests cross-namespace user reference.
func TestDatabaseGrantReconciler_CrossNamespaceUserRef(t *testing.T) {
	user := &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-user",
			Namespace: "other-namespace",
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseUserStatus{
			Phase: dbopsv1alpha1.PhasePending,
		},
	}

	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-grant",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseGrant},
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: dbopsv1alpha1.UserReference{
				Name:      "test-user",
				Namespace: "other-namespace",
			},
		},
	}

	fakeClient := newGrantFakeClient(user, grant)
	r := &DatabaseGrantReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-grant",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when user not ready")

	updatedGrant := &dbopsv1alpha1.DatabaseGrant{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedGrant)
	require.NoError(t, getErr)
	// Should find the user (pending, not "not found")
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "Waiting for DatabaseUser")
}
