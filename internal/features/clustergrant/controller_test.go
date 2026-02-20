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

package clustergrant

import (
	"context"
	"fmt"
	"testing"

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

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	return scheme
}

func newTestClusterGrant(name string) *dbopsv1alpha1.ClusterDatabaseGrant {
	return &dbopsv1alpha1.ClusterDatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "test-user",
				Namespace: "default",
			},
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Roles: []string{"app_read"},
			},
		},
	}
}

func newTestClusterInstance(name string) *dbopsv1alpha1.ClusterDatabaseInstance {
	return &dbopsv1alpha1.ClusterDatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
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

func newTestController(clientBuilder *fake.ClientBuilder, mockRepo *MockRepository) *Controller {
	c := clientBuilder.Build()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	return NewController(ControllerConfig{
		Client:   c,
		Scheme:   newTestScheme(),
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
	})
}

// --- Happy path tests ---

func TestController_Reconcile_NewClusterGrant(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 0, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Apply"))
}

func TestController_Reconcile_ClusterGrantNotFound(t *testing.T) {
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
	grant := newTestClusterGrant("testgrant")
	grant.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.False(t, mockRepo.WasCalled("Apply"))
	assert.False(t, mockRepo.WasCalled("Revoke"))
}

// --- Deletion tests ---

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Finalizers = []string{util.FinalizerClusterDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
		return nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_DeletionProtected(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DeletionProtection = true
	grant.Finalizers = []string{util.FinalizerClusterDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")
	assert.False(t, mockRepo.WasCalled("Revoke"))

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
}

func TestController_Reconcile_DeletionProtectedWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DeletionProtection = true
	grant.Annotations = map[string]string{
		"dbops.dbprovision.io/force-delete": "true",
	}
	grant.Finalizers = []string{util.FinalizerClusterDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
		return nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

// --- Error path tests ---

func TestController_Reconcile_ApplyError(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return nil, fmt.Errorf("permission denied")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	assert.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "apply grants")
}

func TestController_Reconcile_RevokeError(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Finalizers = []string{util.FinalizerClusterDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
		return fmt.Errorf("revoke failed: database unavailable")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	// Without force delete, revoke errors prevent finalizer removal
	require.Error(t, err)
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)
	assert.True(t, mockRepo.WasCalled("Revoke"))

	// Verify finalizer is NOT removed
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Contains(t, updatedGrant.Finalizers, util.FinalizerClusterDatabaseGrant)
}

func TestController_Reconcile_RevokeError_WithForce(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Annotations = map[string]string{
		"dbops.dbprovision.io/force-delete": "true",
	}
	grant.Finalizers = []string{util.FinalizerClusterDatabaseGrant}
	now := metav1.Now()
	grant.DeletionTimestamp = &now

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
		return fmt.Errorf("revoke failed: database unavailable")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	// With force delete, revoke errors are ignored and finalizer is removed
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.True(t, mockRepo.WasCalled("Revoke"))
}

func TestController_Reconcile_ResolveTargetError(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return nil, fmt.Errorf("target user not found")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	assert.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "resolve target")
}

func TestController_Reconcile_GetInstanceError(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return nil, fmt.Errorf("connection refused to API server")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	assert.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "get cluster instance")
}

// --- Status validation tests ---

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 3, DefaultPrivileges: 1, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})
	require.NoError(t, err)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)

	// Phase and Message
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	assert.Equal(t, "Grants applied successfully", updatedGrant.Status.Message)

	// ReconcileID
	assert.NotEmpty(t, updatedGrant.Status.ReconcileID)
	assert.Len(t, updatedGrant.Status.ReconcileID, 8)
	assert.NotNil(t, updatedGrant.Status.LastReconcileTime)

	// AppliedGrants
	require.NotNil(t, updatedGrant.Status.AppliedGrants)
	assert.Equal(t, []string{"app_read"}, updatedGrant.Status.AppliedGrants.Roles)
	assert.Equal(t, int32(3), updatedGrant.Status.AppliedGrants.DirectGrants)
	assert.Equal(t, int32(1), updatedGrant.Status.AppliedGrants.DefaultPrivileges)

	// TargetInfo
	require.NotNil(t, updatedGrant.Status.TargetInfo)
	assert.Equal(t, "user", updatedGrant.Status.TargetInfo.Type)
	assert.Equal(t, "test-user", updatedGrant.Status.TargetInfo.Name)
	assert.Equal(t, "default", updatedGrant.Status.TargetInfo.Namespace)
	assert.Equal(t, "test-user", updatedGrant.Status.TargetInfo.ResolvedUsername)

	// Conditions
	assert.Len(t, updatedGrant.Status.Conditions, 2)
}

func TestController_Reconcile_WaitingForInstanceReady(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")
	instance.Status.Phase = dbopsv1alpha1.PhasePending

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.Equal(t, RequeueAfterPending, result.RequeueAfter)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedGrant.Status.Phase)
	assert.Contains(t, updatedGrant.Status.Message, "to be ready")
	assert.False(t, mockRepo.WasCalled("Apply"))
}

// --- Drift detection tests ---

func TestController_Reconcile_DriftDetected_DetectMode(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(drift.Diff{
			Field:    "roles.missing",
			Expected: "editor",
			Actual:   "",
		})
		return r, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)

	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.True(t, updatedGrant.Status.Drift.Detected)
	require.Len(t, updatedGrant.Status.Drift.Diffs, 1)
	assert.Equal(t, "roles.missing", updatedGrant.Status.Drift.Diffs[0].Field)
	assert.Equal(t, "editor", updatedGrant.Status.Drift.Diffs[0].Expected)
	assert.Equal(t, "", updatedGrant.Status.Drift.Diffs[0].Actual)

	// In detect mode (default), CorrectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetected_CorrectMode(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	rolesDiff := drift.Diff{
		Field:    "roles.missing",
		Expected: "editor",
		Actual:   "",
	}

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	// DetectDrift is called twice in correct mode (once for detection, once inside correctDrift)
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(rolesDiff)
		return r, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddCorrected(rolesDiff)
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// DetectDrift called twice (once for detection, once inside correctDrift)
	assert.Equal(t, 2, mockRepo.CallCount("DetectDrift"))

	// After successful correction, drift should be cleared
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.False(t, updatedGrant.Status.Drift.Detected)
}

func TestController_Reconcile_DriftCorrection_PartialFail(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	rolesDiff := drift.Diff{
		Field:    "roles.missing",
		Expected: "editor",
		Actual:   "",
	}
	privilegeDiff := drift.Diff{
		Field:    "privileges.select",
		Expected: "GRANT",
		Actual:   "REVOKE",
	}

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(rolesDiff)
		r.AddDiff(privilegeDiff)
		return r, nil
	}
	// CorrectDrift: one succeeds, one fails
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddCorrected(rolesDiff)
		cr.AddFailed(privilegeDiff, fmt.Errorf("privilege correction failed"))
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	// Reconcile should still succeed (drift correction failures are non-fatal)
	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Still ends up Ready
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_AllFailed(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	rolesDiff := drift.Diff{
		Field:    "roles.missing",
		Expected: "editor",
		Actual:   "",
	}

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(rolesDiff)
		return r, nil
	}
	// CorrectDrift returns error (all failed)
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		return nil, fmt.Errorf("permission denied")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	// Reconcile itself should still succeed (drift correction errors are non-fatal)
	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// CorrectDrift should have been called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Grant should still be Ready with drift status showing detected
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.True(t, updatedGrant.Status.Drift.Detected)
}

func TestController_Reconcile_DriftDetected_IgnoreMode(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeIgnore}
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	detectDriftCalled := false

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	// DetectDrift should NOT be called when mode is ignore, but set it up just in case
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		detectDriftCalled = true
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(drift.Diff{Field: "roles.missing", Expected: "editor", Actual: ""})
		return r, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// In ignore mode, DetectDrift should NOT be called
	assert.False(t, detectDriftCalled, "DetectDrift should not be called in ignore mode")

	// No drift status should be set
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
	assert.Nil(t, updatedGrant.Status.Drift)

	// CorrectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetection_Error(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	// DetectDrift returns an error
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		return nil, fmt.Errorf("unable to query database state")
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	// Drift detection errors are non-fatal; reconcile should still succeed
	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Grant should still be Ready
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// No drift status set because detection failed
	assert.Nil(t, updatedGrant.Status.Drift)
}

func TestController_Reconcile_DriftCorrection_Destructive(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	grant.Annotations = map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	}
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	destructiveDiff := drift.Diff{
		Field:       "roles.extra",
		Expected:    "",
		Actual:      "admin",
		Destructive: true,
	}

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		// Verify that allowDestructive is true when annotation is set
		assert.True(t, allowDestructive, "allowDestructive should be true when annotation is set")
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(destructiveDiff)
		return r, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		assert.True(t, allowDestructive, "allowDestructive should be passed through to CorrectDrift")
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddCorrected(destructiveDiff)
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify Ready state
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_NoDestructive(t *testing.T) {
	scheme := newTestScheme()
	grant := newTestClusterGrant("testgrant")
	grant.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect}
	// NO destructive annotation set (default behavior)
	instance := newTestClusterInstance("test-cluster-instance")

	clientBuilder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, instance).
		WithStatusSubresource(grant, instance)

	destructiveDiff := drift.Diff{
		Field:       "roles.extra",
		Expected:    "",
		Actual:      "admin",
		Destructive: true,
	}

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		return &TargetInfo{Type: "user", Name: "test-user", Namespace: "default", DatabaseName: "test-user"}, nil
	}
	mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		// Verify that allowDestructive is false (no annotation)
		assert.False(t, allowDestructive, "allowDestructive should be false when annotation is not set")
		r := drift.NewResult("grant", "testgrant")
		r.AddDiff(destructiveDiff)
		return r, nil
	}
	// CorrectDrift skips the destructive diff
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		assert.False(t, allowDestructive, "allowDestructive should be false")
		cr := drift.NewCorrectionResult("testgrant")
		cr.AddSkipped(destructiveDiff, "destructive corrections not allowed")
		return cr, nil
	}

	controller := newTestController(clientBuilder, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testgrant"},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify CorrectDrift was called (it attempts correction but skips destructive ones)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify the grant is still Ready
	var updatedGrant dbopsv1alpha1.ClusterDatabaseGrant
	err = controller.Get(context.Background(), types.NamespacedName{Name: "testgrant"}, &updatedGrant)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedGrant.Status.Phase)

	// Drift status should still show detected drift since correction was skipped
	require.NotNil(t, updatedGrant.Status.Drift)
	assert.True(t, updatedGrant.Status.Drift.Detected)
	require.Len(t, updatedGrant.Status.Drift.Diffs, 1)
	assert.True(t, updatedGrant.Status.Drift.Diffs[0].Destructive)
}
