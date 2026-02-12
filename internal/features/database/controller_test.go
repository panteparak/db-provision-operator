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
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/util"
)

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

	client := fake.NewClientBuilder().
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
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
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
	err = client.Get(context.Background(), types.NamespacedName{Name: "testdb", Namespace: "default"}, &updatedDB)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedDB.Status.Phase)
	assert.True(t, mockRepo.WasCalled("Create"))
}

func TestController_Reconcile_ExistingDatabase(t *testing.T) {
	scheme := newTestScheme()
	database := newTestDatabase("testdb", "default")

	client := fake.NewClientBuilder().
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
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
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

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockRepo := NewMockRepository()
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

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
	database := newTestDatabase("testdb", "default")
	database.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
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
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
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

	client := fake.NewClientBuilder().
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
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
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

	client := fake.NewClientBuilder().
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
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Handler:  handler,
		Logger:   logr.Discard(),
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
			controller := &Controller{}
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
