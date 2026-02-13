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
