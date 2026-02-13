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

package restore

import (
	"context"
	"fmt"
	"io"
	"strings"
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
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/storage"
	"github.com/db-provision-operator/internal/util"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func newTestRestoreResource(name, namespace string) *dbopsv1alpha1.DatabaseRestore {
	return &dbopsv1alpha1.DatabaseRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: dbopsv1alpha1.DatabaseRestoreSpec{
			BackupRef: &dbopsv1alpha1.BackupReference{
				Name: "test-backup",
			},
			Target: dbopsv1alpha1.RestoreTarget{
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
				DatabaseName: "restored-db",
			},
			ActiveDeadlineSeconds: 3600,
		},
	}
}

func newTestBackup(name, namespace string) *dbopsv1alpha1.DatabaseBackup {
	return &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseBackupSpec{
			DatabaseRef: dbopsv1alpha1.DatabaseReference{
				Name: "test-db",
			},
			Storage: dbopsv1alpha1.StorageConfig{
				Type: dbopsv1alpha1.StorageTypePVC,
				PVC: &dbopsv1alpha1.PVCStorageConfig{
					ClaimName: "backup-pvc",
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseBackupStatus{
			Phase: dbopsv1alpha1.PhaseCompleted,
			Backup: &dbopsv1alpha1.BackupInfo{
				Path: "/backups/test.dump",
			},
			Source: &dbopsv1alpha1.BackupSourceInfo{
				Instance: "test-instance",
				Database: "test-db",
				Engine:   "postgres",
			},
		},
	}
}

func newTestInstanceResource(name, namespace string) *dbopsv1alpha1.DatabaseInstance {
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
			Phase:   dbopsv1alpha1.PhaseReady,
			Version: "15.1",
		},
	}
}

func TestController_Reconcile_NewRestore(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	// Set phase to Pending to skip initialization step
	restore.Status.Phase = dbopsv1alpha1.PhasePending
	backup := newTestBackup("test-backup", "default")
	instance := newTestInstanceResource("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, backup, instance).
		WithStatusSubresource(restore, backup, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
		return backup, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("mock data")), nil
	}
	mockRepo.ExecuteRestoreFunc = func(ctx context.Context, inst *dbopsv1alpha1.DatabaseInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
		return &adapterpkg.RestoreResult{
			TargetDatabase: opts.Database,
			TablesRestored: 10,
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue on success

	// Verify the restore was updated
	var updatedRestore dbopsv1alpha1.DatabaseRestore
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrestore", Namespace: "default"}, &updatedRestore)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseCompleted, updatedRestore.Status.Phase)
	assert.True(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_BackupNotCompleted(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	// Set phase to Pending to skip initialization step
	restore.Status.Phase = dbopsv1alpha1.PhasePending

	// Backup is still running - ValidateSpec will catch this as a validation error
	backup := newTestBackup("test-backup", "default")
	backup.Status.Phase = dbopsv1alpha1.PhaseRunning

	instance := newTestInstanceResource("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, backup, instance).
		WithStatusSubresource(restore, backup, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
		return backup, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	// Validation error results in no error returned (terminal state)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for validation failure

	// Verify the restore is failed due to validation error
	var updatedRestore dbopsv1alpha1.DatabaseRestore
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrestore", Namespace: "default"}, &updatedRestore)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRestore.Status.Phase)
	assert.Contains(t, updatedRestore.Status.Message, "is not completed")

	// Verify ExecuteRestore was NOT called
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_RestoreNotFound(t *testing.T) {
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
	restore := newTestRestoreResource("testrestore", "default")
	restore.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(restore).
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no repository methods were called
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	restore.Finalizers = []string{util.FinalizerDatabaseRestore}
	now := metav1.Now()
	restore.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(restore).
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_TerminalStateCompleted(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	restore.Status.Phase = dbopsv1alpha1.PhaseCompleted
	restore.Status.Message = "Restore completed"

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(restore).
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for terminal state

	// Verify ExecuteRestore was NOT called - terminal state is skipped
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_TerminalStateFailed(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	restore.Status.Phase = dbopsv1alpha1.PhaseFailed
	restore.Status.Message = "Previous restore failed"

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(restore).
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for failed

	// Verify ExecuteRestore was NOT called - failed state is terminal
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_DeadlineExceeded(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	restore.Spec.ActiveDeadlineSeconds = 60
	// Started 2 minutes ago
	startedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
	restore.Status.StartedAt = &startedAt
	restore.Status.Phase = dbopsv1alpha1.PhaseRunning

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(restore).
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for deadline exceeded

	// Verify the restore is marked as failed
	var updatedRestore dbopsv1alpha1.DatabaseRestore
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrestore", Namespace: "default"}, &updatedRestore)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRestore.Status.Phase)
	assert.Contains(t, updatedRestore.Status.Message, "deadline")

	// Verify ExecuteRestore was NOT called
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_WaitingForInstance(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	// Set phase to Pending to skip initialization step
	restore.Status.Phase = dbopsv1alpha1.PhasePending
	backup := newTestBackup("test-backup", "default")

	// Instance not ready
	instance := newTestInstanceResource("test-instance", "default")
	instance.Status.Phase = dbopsv1alpha1.PhasePending

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, backup, instance).
		WithStatusSubresource(restore, backup, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
		return backup, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	// Override ResolveInstance to return a pending instance (controller uses this instead of GetInstance)
	mockRepo.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
		return &instanceresolver.ResolvedInstance{
			Spec:                &instance.Spec,
			CredentialNamespace: namespace,
			Phase:               dbopsv1alpha1.PhasePending, // Not ready
			Name:                "test-instance",
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the restore is pending
	var updatedRestore dbopsv1alpha1.DatabaseRestore
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrestore", Namespace: "default"}, &updatedRestore)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedRestore.Status.Phase)
	assert.Contains(t, updatedRestore.Status.Message, "Waiting for instance")

	// Verify ExecuteRestore was NOT called
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_RestoreError(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	// Set phase to Pending to skip initialization step
	restore.Status.Phase = dbopsv1alpha1.PhasePending
	backup := newTestBackup("test-backup", "default")
	instance := newTestInstanceResource("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, backup, instance).
		WithStatusSubresource(restore, backup, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
		return backup, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("mock data")), nil
	}
	mockRepo.ExecuteRestoreWithResolvedFunc = func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
		return nil, fmt.Errorf("connection refused: database server is unreachable")
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	// handleError returns the error and requeues after RequeueAfterError
	require.Error(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: RequeueAfterError}, result)

	// Verify the restore status was set to Failed with error info
	var updatedRestore dbopsv1alpha1.DatabaseRestore
	err = client.Get(context.Background(), types.NamespacedName{Name: "testrestore", Namespace: "default"}, &updatedRestore)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedRestore.Status.Phase)
	assert.Contains(t, updatedRestore.Status.Message, "execute restore")
	assert.Contains(t, updatedRestore.Status.Message, "connection refused")

	// Verify ExecuteRestoreWithResolved WAS called (it failed)
	assert.True(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}

func TestController_Reconcile_DeletionError(t *testing.T) {
	scheme := newTestScheme()
	restore := newTestRestoreResource("testrestore", "default")
	restore.Finalizers = []string{util.FinalizerDatabaseRestore}
	now := metav1.Now()
	restore.DeletionTimestamp = &now

	// Use an interceptor to make Update fail during finalizer removal.
	// The deletion handler calls c.Update(ctx, restore) to remove the finalizer;
	// if that update fails the controller should propagate the error.
	updateCallCount := 0
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore).
		WithStatusSubresource(restore).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c crclient.WithWatch, obj crclient.Object, opts ...crclient.UpdateOption) error {
				updateCallCount++
				return fmt.Errorf("conflict: the object has been modified")
			},
		}).
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
			Name:      "testrestore",
			Namespace: "default",
		},
	})

	// The deletion handler returns the Update error directly
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflict")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify Update was attempted
	assert.Greater(t, updateCallCount, 0)

	// Verify no restore execution occurred during deletion
	assert.False(t, mockRepo.WasCalled("ExecuteRestoreWithResolved"))
}
