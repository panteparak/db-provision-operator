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
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// --- Error path tests ---

func TestController_Reconcile_ConnectionError(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return nil, errors.New("secret not found: test-secret")
	}
	// Connect should propagate the GetInstance error via handler.Connect
	mockRepo.ConnectFunc = func(ctx context.Context, inst *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
		return nil, errors.New("get instance: secret not found: test-secret")
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	recorder := record.NewFakeRecorder(10)
	controller := NewController(ControllerConfig{
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	// handleError -> ClassifyRequeue: transient error returns err with RequeueAfter
	assert.Error(t, err)
	assert.NotEqual(t, ctrl.Result{}, result)
	assert.True(t, result.RequeueAfter > 0, "should requeue after error")

	// Verify status updated to Failed with connection error details
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	assert.Contains(t, updatedInstance.Status.Message, "secret not found")

	// Verify Connected condition is set to False with ConnectionFailed reason
	connCond := util.GetCondition(updatedInstance.Status.Conditions, util.ConditionTypeConnected)
	require.NotNil(t, connCond, "Connected condition should be set")
	assert.Equal(t, metav1.ConditionFalse, connCond.Status)
	assert.Equal(t, util.ReasonConnectionFailed, connCond.Reason)
}

func TestController_Reconcile_DeletionProtectionBlocked(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Spec.DeletionProtection = true
	instance.Finalizers = []string{util.FinalizerDatabaseInstance}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
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
		Client:   fakeClient,
		Scheme:   scheme,
		Recorder: recorder,
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "testinstance",
			Namespace: "default",
		},
	})

	// Deletion protection returns an error
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify status updated to Failed with protection message
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	assert.Contains(t, updatedInstance.Status.Message, "deletion protection")

	// Verify Ready condition is set to False with DeletionProtected reason
	readyCond := util.GetCondition(updatedInstance.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond, "Ready condition should be set")
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, util.ReasonDeletionProtected, readyCond.Reason)

	// Verify finalizer is NOT removed (instance stays protected)
	assert.Contains(t, updatedInstance.Finalizers, util.FinalizerDatabaseInstance)
}

func TestController_Reconcile_DeletionProtectionBypassedWithForceDelete(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Spec.DeletionProtection = true
	instance.Finalizers = []string{util.FinalizerDatabaseInstance}
	instance.Annotations = map[string]string{
		util.AnnotationForceDelete: "true",
	}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	fakeClient := fake.NewClientBuilder().
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
		Client:   fakeClient,
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

	// Force delete bypasses deletion protection
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// --- Status validation tests ---

func TestController_Reconcile_StatusFieldsPopulated(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")

	fakeClient := fake.NewClientBuilder().
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
			Version:   "14.8",
			Message:   "Connected successfully",
		}, nil
	}

	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   fakeClient,
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

	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	require.NoError(t, err)

	// Phase and Message
	assert.Equal(t, dbopsv1alpha1.PhaseReady, updatedInstance.Status.Phase)
	assert.Equal(t, "Connected successfully", updatedInstance.Status.Message)

	// Version from ConnectResult
	assert.Equal(t, "14.8", updatedInstance.Status.Version)

	// ReconcileID should be an 8-character hex string
	assert.NotEmpty(t, updatedInstance.Status.ReconcileID)
	assert.Len(t, updatedInstance.Status.ReconcileID, 8)

	// LastReconcileTime should be set
	assert.NotNil(t, updatedInstance.Status.LastReconcileTime)

	// LastCheckedAt should be set
	assert.NotNil(t, updatedInstance.Status.LastCheckedAt)

	// Conditions: Connected, Healthy, and Ready should all be present and True
	assert.Len(t, updatedInstance.Status.Conditions, 3)

	connCond := util.GetCondition(updatedInstance.Status.Conditions, util.ConditionTypeConnected)
	require.NotNil(t, connCond)
	assert.Equal(t, metav1.ConditionTrue, connCond.Status)
	assert.Equal(t, util.ReasonConnectionSuccess, connCond.Reason)

	healthyCond := util.GetCondition(updatedInstance.Status.Conditions, util.ConditionTypeHealthy)
	require.NotNil(t, healthyCond)
	assert.Equal(t, metav1.ConditionTrue, healthyCond.Status)
	assert.Equal(t, util.ReasonHealthCheckPassed, healthyCond.Reason)

	readyCond := util.GetCondition(updatedInstance.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond)
	assert.Equal(t, metav1.ConditionTrue, readyCond.Status)
	assert.Equal(t, util.ReasonReconcileSuccess, readyCond.Reason)
}

// --- Dependency-checking deletion tests ---

func TestController_Reconcile_DeletionBlockedByChildDependencies(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Finalizers = []string{util.FinalizerDatabaseInstance}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	// Create a Database that references this instance
	childDB := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "testinstance"},
			Name:        "mydb",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance, childDB).
		WithStatusSubresource(instance, childDB).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   fakeClient,
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

	// Dependency check blocks deletion: returns RequeueAfter with no error
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter)

	// Verify the instance still has its finalizer (not removed)
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	require.NoError(t, err)
	assert.Contains(t, updatedInstance.Finalizers, util.FinalizerDatabaseInstance)

	// Verify status: Phase=Failed, Ready condition = DependenciesExist
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedInstance.Status.Phase)
	assert.Contains(t, updatedInstance.Status.Message, "child-db")

	readyCond := util.GetCondition(updatedInstance.Status.Conditions, util.ConditionTypeReady)
	require.NotNil(t, readyCond, "Ready condition should be set")
	assert.Equal(t, metav1.ConditionFalse, readyCond.Status)
	assert.Equal(t, util.ReasonDependenciesExist, readyCond.Reason)
}

func TestController_Reconcile_DeletionSucceedsWhenNoChildren(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Finalizers = []string{util.FinalizerDatabaseInstance}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	// No child resources created — instance has no dependencies

	fakeClient := fake.NewClientBuilder().
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
		Client:   fakeClient,
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

	// No children: deletion proceeds without dependency blocking
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the instance was fully deleted (fake client removes objects
	// once all finalizers are cleared and DeletionTimestamp is set)
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	assert.True(t, apierrors.IsNotFound(err), "instance should be deleted after finalizer removal")
}

func TestController_Reconcile_ForceDeleteBypassesChildCheck(t *testing.T) {
	scheme := newTestScheme()
	instance := newTestInstanceResource("testinstance", "default")
	instance.Finalizers = []string{util.FinalizerDatabaseInstance}
	instance.Annotations = map[string]string{
		util.AnnotationForceDelete: "true",
	}
	now := metav1.Now()
	instance.DeletionTimestamp = &now

	// Create a Database that references this instance — should be bypassed
	childDB := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "testinstance"},
			Name:        "mydb",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance, childDB).
		WithStatusSubresource(instance, childDB).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:     mockRepo,
		eventBus: NewMockEventBus(),
		logger:   logr.Discard(),
	}

	controller := NewController(ControllerConfig{
		Client:   fakeClient,
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

	// Force-delete bypasses child dependency check: deletion proceeds
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the instance was fully deleted (fake client removes objects
	// once all finalizers are cleared and DeletionTimestamp is set)
	var updatedInstance dbopsv1alpha1.DatabaseInstance
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "testinstance", Namespace: "default"}, &updatedInstance)
	assert.True(t, apierrors.IsNotFound(err), "instance should be deleted despite having children")
}
