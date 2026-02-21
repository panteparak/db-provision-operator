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

package database

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/util"
)

// testDefaultDriftInterval is used as the default drift interval for controllers
// created in tests. It mirrors the production default of 8h.
const testDefaultDriftInterval = 8 * time.Hour

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	return scheme
}

func newTestDatabase(name, namespace string) *dbopsv1alpha1.Database {
	return &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: name,
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

func TestController_Reconcile_NewDatabase(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
		return &Info{Name: name, SizeBytes: 1024}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return drift.NewResult("database", spec.Name), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the database was updated
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_ExistingDatabase(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return true, nil // Database already exists
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
		return &Info{Name: name, SizeBytes: 2048}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return drift.NewResult("database", spec.Name), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify Create was NOT called since database exists
	assert.False(t, mockRepo.WasCalled("Create"))
	assert.True(t, mockRepo.WasCalled("Exists"))
}

func TestController_Reconcile_DatabaseNotFound(t *testing.T) {
	scheme := newTestScheme()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
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
	database := newTestDatabase("testdb", "default")
	database.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
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
	database := newTestDatabase("testdb", "default")
	database.Spec.DeletionPolicy = dbopsv1alpha1.DeletionPolicyDelete
	database.Finalizers = []string{util.FinalizerDatabase}
	now := metav1.Now()
	database.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
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
	database := newTestDatabase("testdb", "default")
	database.Spec.DeletionProtection = true
	database.Finalizers = []string{util.FinalizerDatabase}
	now := metav1.Now()
	database.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")

	// Verify delete was NOT called
	assert.False(t, mockRepo.WasCalled("Delete"))
}

func TestController_GetEffectiveDriftPolicy(t *testing.T) {
	tests := []struct {
		name         string
		database     *dbopsv1alpha1.Database
		resolved     *instanceresolver.ResolvedInstance
		expectedMode dbopsv1alpha1.DriftMode
	}{
		{
			name: "use database-level policy",
			database: &dbopsv1alpha1.Database{
				Spec: dbopsv1alpha1.DatabaseSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{
						Mode: dbopsv1alpha1.DriftModeCorrect,
					},
				},
			},
			resolved:     nil,
			expectedMode: dbopsv1alpha1.DriftModeCorrect,
		},
		{
			name: "fallback to instance policy",
			database: &dbopsv1alpha1.Database{
				Spec: dbopsv1alpha1.DatabaseSpec{},
			},
			resolved: &instanceresolver.ResolvedInstance{
				Spec: &dbopsv1alpha1.DatabaseInstanceSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{
						Mode: dbopsv1alpha1.DriftModeIgnore,
					},
				},
			},
			expectedMode: dbopsv1alpha1.DriftModeIgnore,
		},
		{
			name: "use default policy when neither specified",
			database: &dbopsv1alpha1.Database{
				Spec: dbopsv1alpha1.DatabaseSpec{},
			},
			resolved:     &instanceresolver.ResolvedInstance{Spec: &dbopsv1alpha1.DatabaseInstanceSpec{}},
			expectedMode: dbopsv1alpha1.DriftModeDetect,
		},
		{
			name: "database policy takes precedence over instance",
			database: &dbopsv1alpha1.Database{
				Spec: dbopsv1alpha1.DatabaseSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{
						Mode: dbopsv1alpha1.DriftModeCorrect,
					},
				},
			},
			resolved: &instanceresolver.ResolvedInstance{
				Spec: &dbopsv1alpha1.DatabaseInstanceSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{
						Mode: dbopsv1alpha1.DriftModeIgnore,
					},
				},
			},
			expectedMode: dbopsv1alpha1.DriftModeCorrect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{defaultDriftInterval: testDefaultDriftInterval}
			policy := controller.getEffectiveDriftPolicy(tt.database, tt.resolved)
			assert.Equal(t, tt.expectedMode, policy.Mode)
		})
	}
}

func TestController_HasDestructiveDriftAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "annotation not set",
			annotations: map[string]string{"other": "value"},
			expected:    false,
		},
		{
			name:        "annotation set to true",
			annotations: map[string]string{dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true"},
			expected:    true,
		},
		{
			name:        "annotation set to false",
			annotations: map[string]string{dbopsv1alpha1.AnnotationAllowDestructiveDrift: "false"},
			expected:    false,
		},
		{
			name:        "annotation set to other value",
			annotations: map[string]string{dbopsv1alpha1.AnnotationAllowDestructiveDrift: "yes"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{}
			database := &dbopsv1alpha1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			result := controller.hasDestructiveDriftAnnotation(database)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestController_Reconcile_ExistsError(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return false, fmt.Errorf("connection refused")
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.NotEqual(t, time.Duration(0), result.RequeueAfter)

	// Verify the database status was updated
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "check existence")

	// Verify Create was NOT called
	assert.False(t, mockRepo.WasCalled("Create"))

	// Verify Ready condition set to False
	readyCond := util.GetCondition(updatedDB.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
}

func TestController_Reconcile_CreateError(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return nil, fmt.Errorf("permission denied")
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.NotEqual(t, time.Duration(0), result.RequeueAfter)

	// Verify the database status was updated
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "create database")

	// Verify VerifyAccess was NOT called
	assert.False(t, mockRepo.WasCalled("VerifyAccess"))
}

func TestController_Reconcile_VerifyAccessError(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return fmt.Errorf("database not yet accepting connections")
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue
	assert.NotEqual(t, time.Duration(0), result.RequeueAfter)

	// Verify the database status was updated
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseCreating, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "Waiting for database to accept connections")
}

func TestController_Reconcile_DeletionDeleteError(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	database.Spec.DeletionPolicy = dbopsv1alpha1.DeletionPolicyDelete
	database.Finalizers = []string{util.FinalizerDatabase}
	now := metav1.Now()
	database.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
		return fmt.Errorf("database unavailable")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	// Error is returned
	require.Error(t, err)
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))

	// Verify finalizer is NOT removed
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updatedDB, util.FinalizerDatabase))
}

func TestController_Reconcile_DeletionDeleteError_WithForce(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	database.Spec.DeletionPolicy = dbopsv1alpha1.DeletionPolicyDelete
	database.Finalizers = []string{util.FinalizerDatabase}
	database.Annotations = map[string]string{
		util.AnnotationForceDelete: "true",
	}
	now := metav1.Now()
	database.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
		return fmt.Errorf("database unavailable")
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	// No error returned
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))

	// Verify finalizer IS removed (object is fully deleted by fake client once finalizer is removed)
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	// The fake client deletes the object when the last finalizer is removed from a deletion-marked object
	assert.True(t, err != nil || !controllerutil.ContainsFinalizer(&updatedDB, util.FinalizerDatabase))
}

func TestController_Reconcile_DeletionProtectedWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	database.Spec.DeletionProtection = true
	database.Spec.DeletionPolicy = dbopsv1alpha1.DeletionPolicyDelete
	database.Finalizers = []string{util.FinalizerDatabase}
	database.Annotations = map[string]string{
		util.AnnotationForceDelete: "true",
	}
	now := metav1.Now()
	database.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
		return nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	// No error returned - force-delete bypasses deletion protection
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Delete WAS called
	assert.True(t, mockRepo.WasCalled("Delete"))
}

// =============================================================================
// Phase 2: Status field verification tests
// =============================================================================

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
		return &Info{Name: name, Owner: "app_user", SizeBytes: 4096}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return drift.NewResult("database", spec.Name), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Fetch the updated database
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)

	// Verify Phase
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)

	// Verify Message
	assert.Equal(t, "Database is ready", updatedDB.Status.Message)

	// Verify Database info
	require.NotNil(t, updatedDB.Status.Database)
	assert.Equal(t, "testdb", updatedDB.Status.Database.Name)
	assert.Equal(t, "app_user", updatedDB.Status.Database.Owner)
	assert.Equal(t, int64(4096), updatedDB.Status.Database.SizeBytes)

	// Verify Ready condition
	readyCond := util.GetCondition(updatedDB.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, readyCond.Reason)

	// Verify Synced condition
	syncedCond := util.GetCondition(updatedDB.Status.Conditions, util.ConditionTypeSynced)
	require.NotNil(t, syncedCond)
	assert.Equal(t, metav1.ConditionTrue, syncedCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, syncedCond.Reason)

	// Verify ReconcileID is set (non-empty)
	assert.NotEmpty(t, updatedDB.Status.ReconcileID)

	// Verify LastReconcileTime is set
	require.NotNil(t, updatedDB.Status.LastReconcileTime)
	assert.False(t, updatedDB.Status.LastReconcileTime.IsZero())
}

// =============================================================================
// Phase 2: Status transition tests
// =============================================================================

func TestController_Reconcile_StatusTransition_PendingToReady(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	// Database starts with no phase set (effectively Pending)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return false, nil
	}
	mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
		return &Info{Name: name, SizeBytes: 1024}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return drift.NewResult("database", spec.Name), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	// Verify initial state has no phase
	var initialDB dbopsv1alpha1.Database
	err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &initialDB)
	require.NoError(t, err)
	assert.Empty(t, string(initialDB.Status.Phase))

	// First reconcile: should transition to Ready
	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify the database transitioned to Ready
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
	assert.Equal(t, "Database is ready", updatedDB.Status.Message)
	assert.True(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_StatusTransition_ReadyToFailed(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	// Set status to Ready initially
	database.Status.Phase = dbopsv1alpha1.PhaseReady
	database.Status.Message = "Database is ready"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	// Simulate an error on existence check
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return false, fmt.Errorf("connection lost")
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.NotEqual(t, time.Duration(0), result.RequeueAfter)

	// Verify transition: Ready -> Failed
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "check existence")

	// Verify Ready condition set to False
	readyCond := util.GetCondition(updatedDB.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
}

func TestController_Reconcile_StatusTransition_FailedToReady(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	// Set status to Failed initially
	database.Status.Phase = dbopsv1alpha1.PhaseFailed
	database.Status.Message = "Failed to check existence: connection lost"

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	// Now the database exists and everything succeeds (recovery)
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
		return &Info{Name: name, SizeBytes: 2048}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return newTestInstance("test-instance", namespace), nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return drift.NewResult("database", spec.Name), nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify transition: Failed -> Ready
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
	assert.Equal(t, "Database is ready", updatedDB.Status.Message)

	// Verify Ready condition set to True
	readyCond := util.GetCondition(updatedDB.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, readyCond.Reason)
}

// =============================================================================
// Phase 3: Drift detection tests
// =============================================================================

// newTestDatabaseWithDriftPolicy creates a test database with a drift policy configured.
func newTestDatabaseWithDriftPolicy(name, namespace string, mode dbopsv1alpha1.DriftMode) *dbopsv1alpha1.Database {
	db := newTestDatabase(name, namespace)
	db.Spec.DriftPolicy = &dbopsv1alpha1.DriftPolicy{
		Mode: mode,
	}
	return db
}

// setupDriftTest creates a controller with the given mock repository for drift testing.
// It returns the controller, the fake client, and the mock repository.
func setupDriftTest(t *testing.T, database *dbopsv1alpha1.Database, mockRepo *MockRepository) (*Controller, client.Client) {
	t.Helper()
	scheme := newTestScheme()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	return controller, fakeClient
}

// newDriftReadyMockRepo creates a mock repository that simulates a database
// that already exists and is accessible, suitable for drift detection tests.
func newDriftReadyMockRepo() *MockRepository {
	mockRepo := NewMockRepository()
	mockRepo.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
		return true, nil
	}
	mockRepo.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
		return nil
	}
	mockRepo.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
		return &Result{Updated: false}, nil
	}
	mockRepo.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
		return &Info{Name: name, SizeBytes: 1024}, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: namespace},
			Spec:       dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
			Status:     dbopsv1alpha1.DatabaseInstanceStatus{Phase: dbopsv1alpha1.PhaseReady},
		}, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	return mockRepo
}

func TestController_Reconcile_DriftDetected_DetectMode(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeDetect)

	mockRepo := newDriftReadyMockRepo()
	// Return drift result with diffs
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(drift.Diff{
			Field:    "owner",
			Expected: "app_user",
			Actual:   "postgres",
		})
		return r, nil
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify drift status is set
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)

	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
	require.NotNil(t, updatedDB.Status.Drift)
	assert.True(t, updatedDB.Status.Drift.Detected)
	require.Len(t, updatedDB.Status.Drift.Diffs, 1)
	assert.Equal(t, "owner", updatedDB.Status.Drift.Diffs[0].Field)
	assert.Equal(t, "app_user", updatedDB.Status.Drift.Diffs[0].Expected)
	assert.Equal(t, "postgres", updatedDB.Status.Drift.Diffs[0].Actual)

	// In detect mode, CorrectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetected_CorrectMode(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeCorrect)

	mockRepo := newDriftReadyMockRepo()
	ownerDiff := drift.Diff{
		Field:    "owner",
		Expected: "app_user",
		Actual:   "postgres",
	}
	// DetectDrift returns a drift result with a diff
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(ownerDiff)
		return r, nil
	}
	// CorrectDrift succeeds and marks the diff as corrected
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Name)
		cr.AddCorrected(ownerDiff)
		return cr, nil
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify drift status is cleared after successful correction
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)

	// After successful correction, drift should be cleared
	require.NotNil(t, updatedDB.Status.Drift)
	assert.False(t, updatedDB.Status.Drift.Detected)
}

func TestController_Reconcile_DriftCorrection_PartialFail(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeCorrect)

	mockRepo := newDriftReadyMockRepo()
	ownerDiff := drift.Diff{
		Field:    "owner",
		Expected: "app_user",
		Actual:   "postgres",
	}
	encodingDiff := drift.Diff{
		Field:    "encoding",
		Expected: "UTF8",
		Actual:   "LATIN1",
	}
	// DetectDrift returns two diffs
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(ownerDiff)
		r.AddDiff(encodingDiff)
		return r, nil
	}
	// CorrectDrift: one succeeds, one fails
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		cr := drift.NewCorrectionResult(spec.Name)
		cr.AddCorrected(ownerDiff)
		cr.AddFailed(encodingDiff, fmt.Errorf("encoding is immutable"))
		return cr, nil
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Still ends up Ready (drift correction failures do not fail reconciliation)
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_AllFailed(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeCorrect)

	mockRepo := newDriftReadyMockRepo()
	ownerDiff := drift.Diff{
		Field:    "owner",
		Expected: "app_user",
		Actual:   "postgres",
	}
	// DetectDrift returns a single diff
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(ownerDiff)
		return r, nil
	}
	// CorrectDrift returns error
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		return nil, fmt.Errorf("permission denied")
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	// Reconcile itself should still succeed (drift correction errors are non-fatal)
	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify the database is still Ready
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)

	// CorrectDrift should have been called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetected_IgnoreMode(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeIgnore)

	mockRepo := newDriftReadyMockRepo()
	// DetectDrift should NOT be called when mode is ignore, but set it up just in case
	detectDriftCalled := false
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		detectDriftCalled = true
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(drift.Diff{Field: "owner", Expected: "app_user", Actual: "postgres"})
		return r, nil
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// In ignore mode, DetectDrift should NOT be called by performDriftDetection
	// (although handler.DetectDrift may still be called during the normal reconcile flow
	// which goes through the handler directly -- but the controller's performDriftDetection
	// returns early for ignore mode)
	assert.False(t, detectDriftCalled, "DetectDrift should not be called in ignore mode")

	// No drift status should be set
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
	assert.Nil(t, updatedDB.Status.Drift)

	// CorrectDrift should NOT be called
	assert.False(t, mockRepo.WasCalled("CorrectDrift"))
}

func TestController_Reconcile_DriftDetection_Error(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeDetect)

	mockRepo := newDriftReadyMockRepo()
	// DetectDrift returns an error
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return nil, fmt.Errorf("unable to query database state")
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	// Drift detection errors are non-fatal; reconcile should still succeed
	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Database should still be Ready
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)

	// No drift status set because detection failed
	assert.Nil(t, updatedDB.Status.Drift)
}

func TestController_Reconcile_DriftCorrection_Destructive(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeCorrect)
	// Set the destructive drift annotation
	database.Annotations = map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	}

	mockRepo := newDriftReadyMockRepo()
	destructiveDiff := drift.Diff{
		Field:       "owner",
		Expected:    "app_user",
		Actual:      "postgres",
		Destructive: true,
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		// Verify that allowDestructive is true
		assert.True(t, allowDestructive, "allowDestructive should be true when annotation is set")
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(destructiveDiff)
		return r, nil
	}
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		assert.True(t, allowDestructive, "allowDestructive should be passed through to CorrectDrift")
		cr := drift.NewCorrectionResult(spec.Name)
		cr.AddCorrected(destructiveDiff)
		return cr, nil
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify CorrectDrift was called
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify Ready state
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
}

func TestController_Reconcile_DriftCorrection_NoDestructive(t *testing.T) {
	database := newTestDatabaseWithDriftPolicy("testdb", "default", dbopsv1alpha1.DriftModeCorrect)
	// NO destructive annotation set (default behavior)

	mockRepo := newDriftReadyMockRepo()
	destructiveDiff := drift.Diff{
		Field:       "owner",
		Expected:    "app_user",
		Actual:      "postgres",
		Destructive: true,
	}
	mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		// Verify that allowDestructive is false (no annotation)
		assert.False(t, allowDestructive, "allowDestructive should be false when annotation is not set")
		r := drift.NewResult("database", spec.Name)
		r.AddDiff(destructiveDiff)
		return r, nil
	}
	// CorrectDrift skips the destructive diff
	mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		assert.False(t, allowDestructive, "allowDestructive should be false")
		cr := drift.NewCorrectionResult(spec.Name)
		cr.AddSkipped(destructiveDiff, "destructive corrections not allowed")
		return cr, nil
	}

	controller, fakeClient := setupDriftTest(t, database, mockRepo)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "testdb", Namespace: "default"},
	})

	require.NoError(t, err)
	assert.Equal(t, testDefaultDriftInterval, result.RequeueAfter)

	// Verify CorrectDrift was called (it attempts correction but skips destructive ones)
	assert.True(t, mockRepo.WasCalled("CorrectDrift"))

	// Verify the database is still Ready
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)

	// Drift status should still show detected drift since correction was skipped
	require.NotNil(t, updatedDB.Status.Drift)
	assert.True(t, updatedDB.Status.Drift.Detected)
	require.Len(t, updatedDB.Status.Drift.Diffs, 1)
	assert.True(t, updatedDB.Status.Drift.Diffs[0].Destructive)
}

// =============================================================================
// Phase 4: Dependency-checking deletion tests
// =============================================================================

func TestController_Reconcile_DeletionBlockedByGrantDependencies(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	database.Finalizers = []string{util.FinalizerDatabase}
	now := metav1.Now()
	database.DeletionTimestamp = &now

	// Create a DatabaseGrant that references this database
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			DatabaseRef: &dbopsv1alpha1.DatabaseReference{
				Name: "testdb",
			},
			UserRef: &dbopsv1alpha1.UserReference{
				Name: "someuser",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database, grant).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	// Dependency check blocks deletion: returns RequeueAfter with no error
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)

	// Verify the database still has its finalizer (not removed)
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.True(t, controllerutil.ContainsFinalizer(&updatedDB, util.FinalizerDatabase))

	// Verify status: Phase=Failed, Ready condition = DependenciesExist
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedDB.Status.Phase)
	assert.Contains(t, updatedDB.Status.Message, "test-grant")

	readyCond := util.GetCondition(updatedDB.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond, "Ready condition should be set")
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, util.ReasonDependenciesExist, readyCond.Reason)

	// Verify Delete was NOT called (we never reached the deletion handler)
	assert.False(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_DeletionSucceedsWhenNoGrants(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	database.Finalizers = []string{util.FinalizerDatabase}
	now := metav1.Now()
	database.DeletionTimestamp = &now
	// Default deletion policy is Retain — no external deletion, just finalizer removal

	// No grant resources created — database has no dependencies

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	// No children: deletion proceeds without dependency blocking
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the database was fully deleted (fake client removes objects
	// once all finalizers are cleared and DeletionTimestamp is set)
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	assert.True(t, apierrors.IsNotFound(err), "database should be deleted after finalizer removal")

	// Verify Delete was NOT called (Retain policy)
	assert.False(t, mockRepo.WasCalled("Delete"))
}

func TestController_Reconcile_ForceDeleteBypassesGrantCheck(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")
	database.Finalizers = []string{util.FinalizerDatabase}
	database.Annotations = map[string]string{
		util.AnnotationForceDelete: "true",
	}
	now := metav1.Now()
	database.DeletionTimestamp = &now
	// Default deletion policy is Retain

	// Create a DatabaseGrant that references this database
	grant := &dbopsv1alpha1.DatabaseGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grant",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseGrantSpec{
			DatabaseRef: &dbopsv1alpha1.DatabaseReference{
				Name: "testdb",
			},
			UserRef: &dbopsv1alpha1.UserReference{
				Name: "someuser",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(database, grant).
		WithStatusSubresource(database).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:               fakeClient,
		Scheme:               scheme,
		Recorder:             record.NewFakeRecorder(10),
		Handler:              handler,
		Logger:               logr.Discard(),
		DefaultDriftInterval: testDefaultDriftInterval,
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testdb",
			Namespace: "default",
		},
	})

	// Force delete bypasses dependency check: deletion proceeds
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the database was fully deleted (fake client removes objects
	// once all finalizers are cleared and DeletionTimestamp is set)
	var updatedDB dbopsv1alpha1.Database
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	assert.True(t, apierrors.IsNotFound(err), "database should be deleted after force-delete bypasses grant check")

	// Verify Delete was NOT called (Retain policy — force-delete only bypasses dependency check,
	// it does not change the deletion policy)
	assert.False(t, mockRepo.WasCalled("Delete"))
}
