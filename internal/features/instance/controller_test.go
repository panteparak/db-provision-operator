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

package instance

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

func newTestInstanceResource(name, namespace string) *dbopsv1alpha1.DatabaseInstance {
	return &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
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
	}
}

func TestController_Reconcile_NewInstance(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	// Should requeue for health check
	assert.NotEqual(t, ctrl.Result{}, result)

	// Verify the instance was updated
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedInstance.Status.Phase)
	assert.Equal(t, "15.1", updatedInstance.Status.Version)
	assert.True(t, mockRepo.WasCalled("Connect"))
}

func TestController_Reconcile_ConnectionFailed(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	// reconcileutil.ClassifyRequeue may return an error depending on error type
	// but should always requeue for retry
	assert.NotEqual(t, ctrl.Result{}, result)
	_ = err // Error may or may not be returned based on ClassifyRequeue

	// Verify the instance status was updated to Failed
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
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
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for not found
}

func TestController_Reconcile_SkipWithAnnotation(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
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
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Connect was NOT called
	assert.False(t, mockRepo.WasCalled("Connect"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Finalizers = []string{util.FinalizerDatabaseInstance}
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
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_FinalizerAdded(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	// No finalizer initially

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify finalizer was added
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = client.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	require.NoError(t, err)
	assert.Contains(t, updatedInstance.Finalizers, util.FinalizerDatabaseInstance)
}

func TestController_Reconcile_CustomHealthCheckInterval(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Spec.HealthCheck = &dbopsv1alpha1.HealthCheckConfig{
		IntervalSeconds: 120, // 2 minutes
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	// Should requeue after custom interval (120 seconds)
	assert.Equal(t, 120, int(result.RequeueAfter.Seconds()))
}
