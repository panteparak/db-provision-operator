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

package role

import (
	"context"
	"fmt"
	"testing"
	"time"

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

func newTestRole(name, namespace string) *dbopsv1alpha1.DatabaseRole {
	return &dbopsv1alpha1.DatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: name,
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
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

func TestController_Reconcile_NewRole(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the role was updated
	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_ExistingRole(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil // Role already exists
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify Create was NOT called since role exists
	assert.False(t, mockRepo.WasCalled("Create"))
	assert.True(t, mockRepo.WasCalled("Exists"))
}

func TestController_Reconcile_RoleNotFound(t *testing.T) {
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
	role := newTestRole("testrole", "default")
	role.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no repository methods were called
	assert.False(t, mockRepo.WasCalled("Exists"))
	assert.False(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	role.Finalizers = []string{util.FinalizerDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify delete was called
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionProtected(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-protection": "true",
	}
	role.Finalizers = []string{util.FinalizerDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")

	// Verify delete was NOT called
	assert.False(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionProtectedWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-protection": "true",
		"dbops.dbprovision.io/force-delete":        "true",
		"dbops.dbprovision.io/deletion-policy":     string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	role.Finalizers = []string{util.FinalizerDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify delete was called (force delete overrides protection)
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_ExistsError(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return false, fmt.Errorf("connection refused")
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.Error(t, err)

	// Verify the role status was updated with failure
	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
	assert.Contains(t, updatedRole.Status.Message, "check existence")

	// Verify Create was NOT called
	assert.False(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_CreateError(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return nil, fmt.Errorf("permission denied")
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	require.Error(t, err)

	// Verify the role status was updated with failure
	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
	assert.Contains(t, updatedRole.Status.Message, "create")
}

func TestController_Reconcile_DeletionDeleteError(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	role.Finalizers = []string{util.FinalizerDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
		return fmt.Errorf("database unavailable")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	// Error should be returned for requeue
	require.Error(t, err)
	assert.Greater(t, result.RequeueAfter, time.Duration(0))

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))

	// Verify finalizer is NOT removed (role still has it)
	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.Contains(t, updatedRole.Finalizers, util.FinalizerDatabaseRole)
}

func TestController_Reconcile_DeletionDeleteError_WithForce(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Annotations = map[string]string{
		"dbops.dbprovision.io/deletion-policy": string(dbopsv1alpha1.DeletionPolicyDelete),
		"dbops.dbprovision.io/force-delete":    "true",
	}
	role.Finalizers = []string{util.FinalizerDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
		return fmt.Errorf("database unavailable")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	// No error returned with force delete
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))
}

// --- Status validation tests ---

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
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
			Name:      "testrole",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)

	// Phase and Message
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
	assert.Equal(t, "Role is ready", updatedRole.Status.Message)

	// ObservedGeneration
	assert.Equal(t, role.Generation, updatedRole.Status.ObservedGeneration)

	// ReconcileID
	assert.NotEmpty(t, updatedRole.Status.ReconcileID)
	assert.Len(t, updatedRole.Status.ReconcileID, 8)

	// LastReconcileTime
	assert.NotNil(t, updatedRole.Status.LastReconcileTime)

	// Role info
	require.NotNil(t, updatedRole.Status.Role)
	assert.Equal(t, "testrole", updatedRole.Status.Role.Name)
	assert.NotNil(t, updatedRole.Status.Role.CreatedAt)

	// Conditions: Ready and Synced
	assert.Len(t, updatedRole.Status.Conditions, 2)

	readyCond := util.GetCondition(updatedRole.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, readyCond.Reason)

	syncedCond := util.GetCondition(updatedRole.Status.Conditions, util.ConditionTypeSynced)
	require.NotNil(t, syncedCond)
	assert.Equal(t, metav1.ConditionTrue, syncedCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, syncedCond.Reason)
}

// --- Drift detection tests ---

func TestController_Reconcile_DriftDetected_DetectMode(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	// Default drift policy is detect mode (no DriftPolicy set on spec)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		result := drift.NewResult("role", spec.RoleName)
		result.AddDiff(drift.Diff{
			Field:    "login",
			Expected: "true",
			Actual:   "false",
		})
		return result, nil
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
			Name:      "testrole",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// In detect mode (default), drift is logged but CorrectDrift is NOT called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.NotNil(t, updatedRole.Status.Drift)
	assert.True(t, updatedRole.Status.Drift.Detected)
}

func TestController_Reconcile_DriftDetected_CorrectMode(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	driftResult := drift.NewResult("role", "testrole")
	driftResult.AddDiff(drift.Diff{
		Field:    "login",
		Expected: "true",
		Actual:   "false",
	})

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		cr.AddCorrected(driftResult.Diffs[0])
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

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testrole",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// In correct mode, CorrectDrift should be called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftCorrection_PartialFail(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	loginDiff := drift.Diff{
		Field:    "login",
		Expected: "true",
		Actual:   "false",
	}
	superuserDiff := drift.Diff{
		Field:    "superuser",
		Expected: "false",
		Actual:   "true",
	}

	driftResult := drift.NewResult("role", "testrole")
	driftResult.AddDiff(loginDiff)
	driftResult.AddDiff(superuserDiff)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		cr.AddCorrected(loginDiff)
		cr.AddFailed(superuserDiff, fmt.Errorf("permission denied"))
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

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testrole",
			Namespace: "default",
		},
	})

	// Partial failure does not fail the reconcile
	require.NoError(t, err)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	// Phase is still Ready because drift correction failures do not fail reconcile
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_AllFailed(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	loginDiff := drift.Diff{
		Field:    "login",
		Expected: "true",
		Actual:   "false",
	}

	driftResult := drift.NewResult("role", "testrole")
	driftResult.AddDiff(loginDiff)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		cr.AddFailed(loginDiff, fmt.Errorf("database connection lost"))
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

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testrole",
			Namespace: "default",
		},
	})

	// All corrections failed but reconcile still succeeds (drift errors are non-fatal)
	require.NoError(t, err)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
	// Drift status should still show detected since corrections failed (not cleared)
	assert.NotNil(t, updatedRole.Status.Drift)
	assert.True(t, updatedRole.Status.Drift.Detected)
}

func TestController_Reconcile_DriftDetected_IgnoreMode(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeIgnore,
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
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
			Name:      "testrole",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// In ignore mode, DetectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("DetectDrift"))
}

func TestController_Reconcile_DriftDetection_Error(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return nil, fmt.Errorf("drift detection failed")
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
			Name:      "testrole",
			Namespace: "default",
		},
	})

	// Drift detection error should NOT fail the reconcile
	require.NoError(t, err)

	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_Destructive(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}
	role.Annotations = map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	destructiveDiff := drift.Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	}

	driftResult := drift.NewResult("role", "testrole")
	driftResult.AddDiff(destructiveDiff)

	var capturedAllowDestructive bool
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		capturedAllowDestructive = allowDestructive
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		if allowDestructive {
			cr.AddCorrected(destructiveDiff)
		} else {
			cr.AddSkipped(destructiveDiff, "destructive correction not allowed")
		}
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

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testrole",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// allowDestructive should be true because of the annotation
	assert.True(t, capturedAllowDestructive)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftCorrection_NoDestructive(t *testing.T) {
	scheme := newTestScheme()
	role := newTestRole("testrole", "default")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}
	// No allow-destructive-drift annotation

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role).
		Build()

	destructiveDiff := drift.Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	}

	driftResult := drift.NewResult("role", "testrole")
	driftResult.AddDiff(destructiveDiff)

	var capturedAllowDestructive bool
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		capturedAllowDestructive = allowDestructive
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		// Without destructive allowed, skips destructive changes
		cr.AddSkipped(destructiveDiff, "destructive correction not allowed")
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

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testrole",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// allowDestructive should be false (no annotation)
	assert.False(t, capturedAllowDestructive)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.DatabaseRole
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrole", Namespace: "default"}, &updatedRole)
	require.NoError(t, err)
	// Drift should still be detected since the correction was skipped
	assert.NotNil(t, updatedRole.Status.Drift)
	assert.True(t, updatedRole.Status.Drift.Detected)
}
