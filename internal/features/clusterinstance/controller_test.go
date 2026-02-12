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

package clusterinstance

import (
	"context"
	"errors"
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

// newTestClusterInstanceResource creates a ClusterDatabaseInstance for testing.
// Note: Cluster-scoped resources don't have a namespace.
func newTestClusterInstanceResource(name string) *dbopsv1alpha1.ClusterDatabaseInstance {
	return &dbopsv1alpha1.ClusterDatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  "test-uid",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name:      "test-secret",
					Namespace: "default", // Cluster-scoped resources require namespace in secretRef
				},
			},
		},
	}
}

func TestController_Reconcile_NewClusterInstance(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
			Message:   "Connected successfully",
		}, nil
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

	// Note: For cluster-scoped resources, NamespacedName has no namespace
	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-cluster-instance",
			// No namespace for cluster-scoped resources
		},
	})

	require.NoError(t, err)
	// Should requeue for health check
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify the instance was updated
	var updatedInstance dbopsv1alpha1.ClusterDatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-cluster-instance"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedInstance.Status.Phase)
	assert.Equal(t, "15.1", updatedInstance.Status.Version)
	assert.True(t, mockRepo.WasCalled("Connect"))
}

func TestController_Reconcile_ConnectionFailed(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return nil, errors.New("connection refused")
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
			Name: "test-cluster-instance",
		},
	})

	// reconcileutil.ClassifyRequeue may return an error depending on error type
	// but should always requeue for retry
	assert.NotEqual(t, ctrl.Result{}, result)
	_ = err // Error may or may not be returned based on ClassifyRequeue

	// Verify the instance status was updated to Failed
	var updatedInstance dbopsv1alpha1.ClusterDatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-cluster-instance"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	assert.Contains(t, updatedInstance.Status.Message, "connection refused")
}

func TestController_Reconcile_InstanceNotFound(t *testing.T) {
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
			Name: "nonexistent",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for not found
}

func TestController_Reconcile_SkipWithAnnotation(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")
	instance.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
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
			Name: "test-cluster-instance",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Connect was NOT called
	assert.False(t, mockRepo.WasCalled("Connect"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")
	instance.Finalizers = []string{FinalizerClusterDatabaseInstance}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
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
			Name: "test-cluster-instance",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_DeletionProtection(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")
	instance.Spec.DeletionProtection = true
	instance.Finalizers = []string{FinalizerClusterDatabaseInstance}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	recorder := record.NewFakeRecorder(10)
	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: recorder,
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-cluster-instance",
		},
	})

	// Should return error for deletion protection
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify status was updated
	var updatedInstance dbopsv1alpha1.ClusterDatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-cluster-instance"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	assert.Contains(t, updatedInstance.Status.Message, "deletion protection")
}

func TestController_Reconcile_DeletionProtectionWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")
	instance.Spec.DeletionProtection = true
	instance.Finalizers = []string{FinalizerClusterDatabaseInstance}
	instance.Annotations = map[string]string{
		util.AnnotationForceDelete: "true",
	}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
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
			Name: "test-cluster-instance",
		},
	})

	// Should succeed with force delete annotation
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_FinalizerAdded(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")
	// No finalizer initially

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
		}, nil
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
			Name: "test-cluster-instance",
		},
	})

	require.NoError(t, err)

	// Verify finalizer was added
	var updatedInstance dbopsv1alpha1.ClusterDatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-cluster-instance"}, &updatedInstance)
	require.NoError(t, err)
	assert.Contains(t, updatedInstance.Finalizers, FinalizerClusterDatabaseInstance)
}

func TestController_Reconcile_CustomHealthCheckInterval(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")
	instance.Spec.HealthCheck = &dbopsv1alpha1.HealthCheckConfig{
		IntervalSeconds: 120, // 2 minutes
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
		}, nil
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
			Name: "test-cluster-instance",
		},
	})

	require.NoError(t, err)
	// Should requeue after custom interval (120 seconds)
	assert.Equal(t, 120, int(result.RequeueAfter.Seconds()))
}

func TestController_Reconcile_ConditionsSet(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
		}, nil
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
			Name: "test-cluster-instance",
		},
	})

	require.NoError(t, err)

	// Verify conditions were set
	var updatedInstance dbopsv1alpha1.ClusterDatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-cluster-instance"}, &updatedInstance)
	require.NoError(t, err)

	// Check conditions
	var connectedCondition, healthyCondition, readyCondition *metav1.Condition
	for i := range updatedInstance.Status.Conditions {
		cond := &updatedInstance.Status.Conditions[i]
		switch cond.Type {
		case "Connected":
			connectedCondition = cond
		case "Healthy":
			healthyCondition = cond
		case "Ready":
			readyCondition = cond
		}
	}

	assert.NotNil(t, connectedCondition)
	assert.NotNil(t, healthyCondition)
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, connectedCondition.Status)
	assert.Equal(t, metav1.ConditionTrue, healthyCondition.Status)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
}

func TestController_Reconcile_EventPublished(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-cluster-instance")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
		}, nil
	}

	mockEventBus := NewMockEventBus()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: mockEventBus,
		logger:   logr.Discard(),
	}

	recorder := record.NewFakeRecorder(10)
	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: recorder,
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "test-cluster-instance",
		},
	})

	require.NoError(t, err)

	// Verify event bus received events
	assert.True(t, len(mockEventBus.PublishedEvents) > 0, "Expected at least one event to be published")
}

func TestController_Reconcile_MySQL(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestClusterInstanceResource("test-mysql-cluster")
	instance.Spec.Engine = dbopsv1alpha1.EngineTypeMySQL
	instance.Spec.Connection.Port = 3306

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "8.0.32",
			Message:   "Connected successfully",
		}, nil
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
			Name: "test-mysql-cluster",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify the instance was updated
	var updatedInstance dbopsv1alpha1.ClusterDatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-mysql-cluster"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedInstance.Status.Phase)
	assert.Equal(t, "8.0.32", updatedInstance.Status.Version)
}
