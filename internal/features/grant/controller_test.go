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

package grant

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/util"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func newTestGrant(name, namespace string) *dbopsv1alpha1.DatabaseGrant {
	return &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: &dbopsv1alpha1.UserReference{
				Name: "test-user",
			},
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Roles: []string{"app_read"},
			},
		},
	}
}

func newTestUser(name, namespace string) *dbopsv1alpha1.DatabaseUser {
	return &dbopsv1alpha1.DatabaseUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseUserSpec{
			Username: name,
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseUserStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}
}

func newTestInstance(name, namespace string) *dbopsv1alpha1.DatabaseInstance {
	return &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}
}

func TestController_Reconcile_NewGrant(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 0, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the grant was updated
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Apply"))
}

func TestController_Reconcile_WaitingForUser(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	// No user exists - grant should wait

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant).
		Build()

	mockRepo := NewMockRepository()

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the grant is pending
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "Waiting for DatabaseUser")

	// Verify Apply was NOT called
	assert.False(t, mockRepo.WasCalled("Apply"))
}

func TestController_Reconcile_WaitingForUserReady(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	user := newTestUser("test-user", "default")
	user.Status.Phase = dbopsv1alpha1.PhasePending // User exists but not ready

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user).
		WithStatusSubresource(grant, user).
		Build()

	mockRepo := NewMockRepository()

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the grant is pending
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "to be ready")

	// Verify Apply was NOT called
	assert.False(t, mockRepo.WasCalled("Apply"))
}

func TestController_Reconcile_WaitingForInstance(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	user := newTestUser("test-user", "default")
	// No instance exists - grant should wait

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user).
		WithStatusSubresource(grant, user).
		Build()

	mockRepo := NewMockRepository()

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the grant is pending
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "Waiting for DatabaseInstance")
}

func TestController_Reconcile_GrantNotFound(t *testing.T) {
	scheme := newTestScheme()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for not found
}

func TestController_Reconcile_SkipWithAnnotation(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no repository methods were called
	assert.False(t, mockRepo.WasCalled("Apply"))
	assert.False(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify revoke was called
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_DeletionWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Annotations = map[string]string{
		"dbops.dbprovision.io/force-delete": "true",
	}
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	// Revoke fails but force delete should proceed
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify revoke was called
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_AppliedGrantsInStatus(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{
			Applied:           true,
			Roles:             []string{"app_read", "app_write"},
			DirectGrants:      5,
			DefaultPrivileges: 2,
			Message:           "applied",
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify the status has applied grants info
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)

	require.NotNil(t, updatedGrant.Status.AppliedGrants)
	assert.Equal(t, []string{"app_read", "app_write"}, updatedGrant.Status.AppliedGrants.Roles)
	assert.Equal(t, int32(5), updatedGrant.Status.AppliedGrants.DirectGrants)
	assert.Equal(t, int32(2), updatedGrant.Status.AppliedGrants.DefaultPrivileges)
}

// --- Error path tests ---

func TestController_Reconcile_ApplyError(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return nil, fmt.Errorf("permission denied")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify the grant status reflects the failure
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "apply")
}

func TestController_Reconcile_RevokeError(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
		return fmt.Errorf("revoke failed: database unavailable")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	// Without force delete, revoke errors prevent finalizer removal
	require.Error(t, err)
	assert.True(t, result.RequeueAfter > 0)
	assert.True(t, mockRepo.WasCalled("Revoke"))

	// Verify finalizer is NOT removed
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Contains(t, updatedGrant.Finalizers, util.FinalizerDatabaseGrant)
}

func TestController_Reconcile_RevokeError_WithForce(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Annotations = map[string]string{
		"dbops.dbprovision.io/force-delete": "true",
	}
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
		return fmt.Errorf("revoke failed: database unavailable")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	// With force delete, revoke errors are ignored and finalizer is removed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_Deletion_TargetNotReady(t *testing.T) {
	// When the target user/role is not ready (e.g., being deleted with DependenciesExist),
	// the grant controller should treat the revoke as a no-op and proceed with deletion.
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	// Simulate: withService → ResolveTarget fails because user is not ready
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
		return fmt.Errorf("resolve target: %w", &TargetResolutionError{
			Err: fmt.Errorf("user not ready: phase is Failed"),
		})
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*TargetInfo, error) {
		return nil, &TargetResolutionError{Err: fmt.Errorf("user not ready: phase is Failed")}
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	// TargetResolutionError should be treated as success — finalizer removed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_Deletion_TargetNotFound(t *testing.T) {
	// When the target user/role has already been deleted, the grant controller
	// should treat the revoke as a no-op and proceed with deletion.
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	// Note: no user object — simulates the user already being deleted
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance).
		Build()

	mockRepo := NewMockRepository()
	// Simulate: withService → ResolveTarget fails because user is not found
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
		return fmt.Errorf("resolve target: %w", &TargetResolutionError{
			Err: fmt.Errorf("get user: not found"),
		})
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*TargetInfo, error) {
		return nil, &TargetResolutionError{Err: fmt.Errorf("get user: not found")}
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	// TargetResolutionError should be treated as success — finalizer removed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

// --- Phase 2: Status validation tests ---

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{
			Applied:           true,
			Roles:             []string{"app_read", "app_write"},
			DirectGrants:      3,
			DefaultPrivileges: 1,
			Message:           "applied",
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Fetch the updated grant
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)

	// Verify Phase
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// Verify Message
	assert.Equal(t, "Grants applied successfully", updatedGrant.Status.Message)

	// Verify AppliedGrants
	require.NotNil(t, updatedGrant.Status.AppliedGrants, "AppliedGrants should be populated")
	assert.Equal(t, []string{"app_read", "app_write"}, updatedGrant.Status.AppliedGrants.Roles)
	assert.Equal(t, int32(3), updatedGrant.Status.AppliedGrants.DirectGrants)
	assert.Equal(t, int32(1), updatedGrant.Status.AppliedGrants.DefaultPrivileges)

	// Verify ObservedGeneration
	assert.Equal(t, updatedGrant.Generation, updatedGrant.Status.ObservedGeneration)

	// Verify Ready condition
	readyCond := util.GetCondition(updatedGrant.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond, "Ready condition should be present")
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, readyCond.Reason)
	assert.Equal(t, "Grants are ready", readyCond.Message)

	// Verify Synced condition
	syncedCond := util.GetCondition(updatedGrant.Status.Conditions, util.ConditionTypeSynced)
	require.NotNil(t, syncedCond, "Synced condition should be present")
	assert.Equal(t, metav1.ConditionTrue, syncedCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, syncedCond.Reason)
	assert.Equal(t, "Grants are synced", syncedCond.Message)

	// Verify ReconcileID is populated (non-empty)
	assert.NotEmpty(t, updatedGrant.Status.ReconcileID, "ReconcileID should be populated")

	// Verify LastReconcileTime is set
	require.NotNil(t, updatedGrant.Status.LastReconcileTime, "LastReconcileTime should be set")
	assert.False(t, updatedGrant.Status.LastReconcileTime.IsZero(), "LastReconcileTime should not be zero")
}

func TestController_Reconcile_DeletionProtected(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DeletionProtection = true
	grant.Finalizers = []string{util.FinalizerDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant).
		Build()

	mockRepo := NewMockRepository()

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testgrant",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")
	assert.False(t, mockRepo.WasCalled("Revoke"))
}

// --- Drift detection tests ---

func TestController_Reconcile_DriftDetected_DetectMode(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	// Default drift policy is detect mode (no DriftPolicy set)
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(drift.Diff{
			Field:    "roles",
			Expected: "app_read",
			Actual:   "app_write",
		})
		return r, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify drift status is set
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)

	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.True(t, updatedGrant.Status.Drift.Detected)
	require.Len(t, updatedGrant.Status.Drift.Diffs, 1)
	assert.Equal(t, "roles", updatedGrant.Status.Drift.Diffs[0].Field)
	assert.Equal(t, "app_read", updatedGrant.Status.Drift.Diffs[0].Expected)
	assert.Equal(t, "app_write", updatedGrant.Status.Drift.Diffs[0].Actual)

	// In detect mode, CorrectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetected_CorrectMode(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	rolesDiff := drift.Diff{
		Field:    "roles",
		Expected: "app_read",
		Actual:   "app_write",
	}
	// DetectDrift is called twice: once by performDriftDetection, once by correctDrift
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(rolesDiff)
		return r, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddCorrected(rolesDiff)
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify drift status is cleared after successful correction
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// After successful correction, drift should be cleared
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.False(t, updatedGrant.Status.Drift.Detected)
}

func TestController_Reconcile_DriftCorrection_PartialFail(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	rolesDiff := drift.Diff{
		Field:    "roles",
		Expected: "app_read",
		Actual:   "app_write",
	}
	privilegesDiff := drift.Diff{
		Field:    "privileges",
		Expected: "SELECT",
		Actual:   "ALL",
	}
	// DetectDrift returns two diffs
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(rolesDiff)
		r.AddDiff(privilegesDiff)
		return r, nil
	}
	// CorrectDrift: one succeeds, one fails
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddCorrected(rolesDiff)
		cr.AddFailed(privilegesDiff, fmt.Errorf("privilege change not supported"))
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Still ends up Ready (drift correction failures do not fail reconciliation)
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_AllFailed(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	rolesDiff := drift.Diff{
		Field:    "roles",
		Expected: "app_read",
		Actual:   "app_write",
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(rolesDiff)
		return r, nil
	}
	// CorrectDrift returns error
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		return nil, fmt.Errorf("permission denied")
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	// Reconcile itself should still succeed (drift correction errors are non-fatal)
	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify the grant is still Ready
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// Drift status should show detected since correction failed
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.True(t, updatedGrant.Status.Drift.Detected)

	// CorrectDrift should have been called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetected_IgnoreMode(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeIgnore}
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	// DetectDrift should NOT be called when mode is ignore, but set it up just in case
	detectDriftCalled := false
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		detectDriftCalled = true
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(drift.Diff{Field: "roles", Expected: "app_read", Actual: "app_write"})
		return r, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// In ignore mode, DetectDrift should NOT be called
	assert.False(t, detectDriftCalled, "DetectDrift should not be called in ignore mode")

	// No drift status should be set
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	assert.Nil(t, updatedGrant.Status.Drift)

	// CorrectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetection_Error(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	// Default drift policy is detect mode
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	// DetectDrift returns an error
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return nil, fmt.Errorf("unable to query grant state")
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	// Drift detection errors are non-fatal; reconcile should still succeed
	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Grant should still be Ready
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// No drift status set because detection failed
	assert.Nil(t, updatedGrant.Status.Drift)
}

func TestController_Reconcile_DriftCorrection_Destructive(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	// Set the destructive drift annotation
	grant.Annotations = map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	}
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	destructiveDiff := drift.Diff{
		Field:       "roles",
		Expected:    "app_read",
		Actual:      "app_write",
		Destructive: true,
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		// Verify that allowDestructive is true
		assert.True(t, allowDestructive, "allowDestructive should be true when annotation is set")
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(destructiveDiff)
		return r, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		assert.True(t, allowDestructive, "allowDestructive should be passed through to CorrectDrift")
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddCorrected(destructiveDiff)
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify Ready state
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_NoDestructive(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestGrant("testgrant", "default")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	// NO destructive annotation set (default behavior)
	user := newTestUser("test-user", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, user, instance).
		WithStatusSubresource(grant, user, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	destructiveDiff := drift.Diff{
		Field:       "roles",
		Expected:    "app_read",
		Actual:      "app_write",
		Destructive: true,
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		// Verify that allowDestructive is false (no annotation)
		assert.False(t, allowDestructive, "allowDestructive should be false when annotation is not set")
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(destructiveDiff)
		return r, nil
	}
	// CorrectDrift skips the destructive diff
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		assert.False(t, allowDestructive, "allowDestructive should be false")
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddSkipped(destructiveDiff, "destructive corrections not allowed")
		return cr, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called (it attempts correction but skips destructive ones)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify the grant is still Ready
	var updatedGrant dbopsv1alpha1.DatabaseGrant
	err = client.Get(context.Background(), types.NamespacedName{Name: "testgrant", Namespace: "default"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// Drift status should still show detected drift since correction was skipped
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.True(t, updatedGrant.Status.Drift.Detected)
	require.Len(t, updatedGrant.Status.Drift.Diffs, 1)
	assert.True(t, updatedGrant.Status.Drift.Diffs[0].Destructive)
}
