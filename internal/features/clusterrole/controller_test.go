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

package clusterrole

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// testDefaultDriftInterval is used as the default drift interval for controllers
// in unit tests. It must match the value used by the production default.
const testDefaultDriftInterval = 8 * time.Hour

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	return scheme
}

func newTestClusterRole(name string) *dbopsv1alpha1.ClusterDatabaseRole {
	return &dbopsv1alpha1.ClusterDatabaseRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: dbopsv1alpha1.ClusterDatabaseRoleSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			RoleName: name,
		},
	}
}

func newTestController(client *fake.ClientBuilder, mockRepo *MockRepository) *Controller {
	c := client.Build()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	return NewController(ControllerConfig{
		Client:               c,
		Scheme:               newTestScheme(),
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		DefaultDriftInterval: testDefaultDriftInterval,
		Logger:               logr.Discard(),
	})
}

// --- Happy path tests ---

func TestController_Reconcile_NewClusterRole(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_ExistingClusterRole(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)
	assert.False(t, mockRepo.WasCalled("Create"))
	assert.True(t, mockRepo.WasCalled("Exists"))
}

func TestController_Reconcile_ClusterRoleNotFound(t *testing.T) {
	scheme := newTestScheme()

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme)

	mockRepo := NewMockRepository()
	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_SkipWithAnnotation(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, mockRepo.WasCalled("Exists"))
	assert.False(t, mockRepo.WasCalled("Create"))
}

// --- Deletion tests ---

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Annotations = map[string]string{
		AnnotationDeletionPolicy: string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	role.Finalizers = []string{util.FinalizerClusterDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error {
		return nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionProtected(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Annotations = map[string]string{
		AnnotationDeletionProtection: "true",
	}
	role.Finalizers = []string{util.FinalizerClusterDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")
	assert.False(t, mockRepo.WasCalled("Delete"))

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
}

func TestController_Reconcile_DeletionProtectedWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Annotations = map[string]string{
		AnnotationDeletionProtection:        "true",
		"dbops.dbprovision.io/force-delete": "true",
		AnnotationDeletionPolicy:            string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	role.Finalizers = []string{util.FinalizerClusterDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error {
		return nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Delete"))
}

// --- Error path tests ---

func TestController_Reconcile_ExistsError(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return false, fmt.Errorf("connection refused")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	// handleError uses ClassifyRequeue — transient errors return err with RequeueAfter
	assert.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
	assert.Contains(t, updatedRole.Status.Message, "check existence")
	assert.False(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_CreateError(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error) {
		return nil, fmt.Errorf("permission denied")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	assert.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRole.Status.Phase)
	assert.Contains(t, updatedRole.Status.Message, "create cluster role")
}

func TestController_Reconcile_DeletionDeleteError(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Annotations = map[string]string{
		AnnotationDeletionPolicy: string(dbopsv1alpha1.DeletionPolicyDelete),
	}
	role.Finalizers = []string{util.FinalizerClusterDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error {
		return fmt.Errorf("database unavailable")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	// Without force delete, the error should be returned and finalizer retained
	require.Error(t, err)
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)
	assert.True(t, mockRepo.WasCalled("Delete"))

	// Verify finalizer is NOT removed
	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Contains(t, updatedRole.Finalizers, util.FinalizerClusterDatabaseRole)
}

func TestController_Reconcile_DeletionDeleteError_WithForce(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Annotations = map[string]string{
		AnnotationDeletionPolicy:            string(dbopsv1alpha1.DeletionPolicyDelete),
		"dbops.dbprovision.io/force-delete": "true",
	}
	role.Finalizers = []string{util.FinalizerClusterDatabaseRole}
	now := metav1.Now()
	role.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error {
		return fmt.Errorf("database unavailable")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	// Force delete continues despite error - finalizer removed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Delete"))
}

// --- Status validation tests ---

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})
	require.NoError(t, err)

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)

	// Phase and Message
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
	assert.Equal(t, "Cluster role is ready", updatedRole.Status.Message)

	// ReconcileID
	assert.NotEmpty(t, updatedRole.Status.ReconcileID)
	assert.Len(t, updatedRole.Status.ReconcileID, 8)
	assert.NotNil(t, updatedRole.Status.LastReconcileTime)

	// Role info
	require.NotNil(t, updatedRole.Status.Role)
	assert.Equal(t, "testrole", updatedRole.Status.Role.Name)

	// Conditions
	assert.Len(t, updatedRole.Status.Conditions, 2)
}

// --- Drift detection tests ---

func TestController_Reconcile_DriftDetected_DetectMode(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		result := drift.NewResult("clusterrole", spec.RoleName)
		result.AddDiff(drift.Diff{
			Field:    "login",
			Expected: "true",
			Actual:   "false",
		})
		return result, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})
	require.NoError(t, err)

	// In detect mode (default), drift is logged but CorrectDrift is NOT called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.NotNil(t, updatedRole.Status.Drift)
	assert.True(t, updatedRole.Status.Drift.Detected)
}

func TestController_Reconcile_DriftDetected_CorrectMode(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	driftResult := drift.NewResult("clusterrole", "testrole")
	driftResult.AddDiff(drift.Diff{
		Field:    "login",
		Expected: "true",
		Actual:   "false",
	})

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		cr.AddCorrected(driftResult.Diffs[0])
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})
	require.NoError(t, err)

	// In correct mode, CorrectDrift should be called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetected_IgnoreMode(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeIgnore,
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})
	require.NoError(t, err)

	// In ignore mode, DetectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("DetectDrift"))
}

func TestController_Reconcile_DriftDetection_Error(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return nil, fmt.Errorf("drift detection failed")
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	// Drift detection error should NOT fail the reconcile
	require.NoError(t, err)

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_PartialFail(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

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

	driftResult := drift.NewResult("clusterrole", "testrole")
	driftResult.AddDiff(loginDiff)
	driftResult.AddDiff(superuserDiff)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		cr.AddCorrected(loginDiff)
		cr.AddFailed(superuserDiff, fmt.Errorf("permission denied"))
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	// Partial failure does not fail the reconcile
	require.NoError(t, err)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	// Phase is still Ready because drift correction failures do not fail reconcile
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_AllFailed(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	loginDiff := drift.Diff{
		Field:    "login",
		Expected: "true",
		Actual:   "false",
	}

	driftResult := drift.NewResult("clusterrole", "testrole")
	driftResult.AddDiff(loginDiff)

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		cr.AddFailed(loginDiff, fmt.Errorf("database connection lost"))
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})

	// All corrections failed but reconcile still succeeds (drift errors are non-fatal)
	require.NoError(t, err)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedRole.Status.Phase)
	// Drift status should still show detected since corrections failed (not cleared)
	assert.NotNil(t, updatedRole.Status.Drift)
	assert.True(t, updatedRole.Status.Drift.Detected)
}

func TestController_Reconcile_DriftCorrection_Destructive(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}
	role.Annotations = map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	destructiveDiff := drift.Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	}

	driftResult := drift.NewResult("clusterrole", "testrole")
	driftResult.AddDiff(destructiveDiff)

	var capturedAllowDestructive bool
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		capturedAllowDestructive = allowDestructive
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		if allowDestructive {
			cr.AddCorrected(destructiveDiff)
		} else {
			cr.AddSkipped(destructiveDiff, "destructive correction not allowed")
		}
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})
	require.NoError(t, err)

	// allowDestructive should be true because of the annotation
	assert.True(t, capturedAllowDestructive)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftCorrection_NoDestructive(t *testing.T) {
	scheme := newTestScheme()
	role := newTestClusterRole("testrole")
	role.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}
	// No allow-destructive-drift annotation

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(role).
		WithStatusSubresource(role)

	destructiveDiff := drift.Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	}

	driftResult := drift.NewResult("clusterrole", "testrole")
	driftResult.AddDiff(destructiveDiff)

	var capturedAllowDestructive bool
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
		return true, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
		capturedAllowDestructive = allowDestructive
		return driftResult, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, dr *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.RoleName)
		// Without destructive allowed, skips destructive changes
		cr.AddSkipped(destructiveDiff, "destructive correction not allowed")
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testrole"},
	})
	require.NoError(t, err)

	// allowDestructive should be false (no annotation)
	assert.False(t, capturedAllowDestructive)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	var updatedRole dbopsv1alpha1.ClusterDatabaseRole
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testrole"}, &updatedRole)
	require.NoError(t, err)
	// Drift should still be detected since the correction was skipped
	assert.NotNil(t, updatedRole.Status.Drift)
	assert.True(t, updatedRole.Status.Drift.Detected)
}
