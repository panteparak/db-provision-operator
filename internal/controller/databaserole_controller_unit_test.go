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

// newRoleFakeClient creates a fake client with DatabaseRole status subresource registered.
func newRoleFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(&dbopsv1alpha1.DatabaseRole{}).
		Build()
}

// TestDatabaseRoleReconciler_NotFound tests that reconciliation handles a missing DatabaseRole gracefully.
func TestDatabaseRoleReconciler_NotFound(t *testing.T) {
	fakeClient := newRoleFakeClient()
	r := &DatabaseRoleReconciler{
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

// TestDatabaseRoleReconciler_SkipReconcile tests that reconciliation is skipped when annotation is present.
func TestDatabaseRoleReconciler_SkipReconcile(t *testing.T) {
	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationSkipReconcile: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(role)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should return empty result when skipping reconciliation")
}

// TestDatabaseRoleReconciler_AddsFinalizer tests that the finalizer is added when not present.
func TestDatabaseRoleReconciler_AddsFinalizer(t *testing.T) {
	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-role",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(role)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	_, _ = r.Reconcile(context.Background(), req)

	// Refetch the DatabaseRole
	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	err := fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	require.NoError(t, err)

	assert.Contains(t, updatedRole.Finalizers, util.FinalizerDatabaseRole, "Finalizer should be added")
}

// TestDatabaseRoleReconciler_InstanceNotFound tests handling when DatabaseInstance is not found.
func TestDatabaseRoleReconciler_InstanceNotFound(t *testing.T) {
	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-role",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseRole},
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "nonexistent-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(role)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should not return error but requeue
	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not found")

	// Check status was updated
	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
	assert.Contains(t, updatedRole.Status.Message, "not found")
}

// TestDatabaseRoleReconciler_InstanceNotReady tests handling when DatabaseInstance is not ready.
func TestDatabaseRoleReconciler_InstanceNotReady(t *testing.T) {
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
			Phase: dbopsv1alpha1.PhasePending,
		},
	}

	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-role",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseRole},
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(instance, role)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not ready")

	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedRole.Status.Phase)
	assert.Contains(t, updatedRole.Status.Message, "Waiting for DatabaseInstance")
}

// TestDatabaseRoleReconciler_SecretNotFound tests handling when credentials secret is not found.
func TestDatabaseRoleReconciler_SecretNotFound(t *testing.T) {
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

	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-role",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseRole},
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(instance, role)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.Error(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when secret not found")

	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
}

// TestDatabaseRoleReconciler_DeletionWithRetainPolicy tests deletion removes finalizer without dropping role.
func TestDatabaseRoleReconciler_DeletionWithRetainPolicy(t *testing.T) {
	now := metav1.Now()
	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-role",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseRole},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(role)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Object should be deleted after finalizer removal
	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseRoleReconciler_DeletionWithDeletePolicy tests deletion with Delete policy tries to drop role.
func TestDatabaseRoleReconciler_DeletionWithDeletePolicy(t *testing.T) {
	now := metav1.Now()

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

	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-role",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseRole},
			DeletionTimestamp: &now,
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
	}

	fakeClient := newRoleFakeClient(instance, role, dbSecret)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// The reconciler will try to connect and fail (no real DB), but still removes finalizer
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Object should be deleted after finalizer removal
	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseRoleReconciler_CrossNamespaceInstance tests handling of cross-namespace instance reference.
func TestDatabaseRoleReconciler_CrossNamespaceInstance(t *testing.T) {
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

	role := &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-role",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseRole},
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name:      "test-instance",
				Namespace: "other-namespace",
			},
		},
	}

	fakeClient := newRoleFakeClient(instance, role, dbSecret)
	r := &DatabaseRoleReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-role",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should find instance in other namespace (will fail at connect, not "not found")
	if err != nil {
		assert.NotContains(t, err.Error(), "not found")
	}

	updatedRole := &dbopsv1alpha1.DatabaseRole{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedRole)
	require.NoError(t, getErr)
	assert.NotContains(t, updatedRole.Status.Message, "DatabaseInstance not found",
		"Should find instance in other namespace")
	assert.NotZero(t, result.RequeueAfter)
}
