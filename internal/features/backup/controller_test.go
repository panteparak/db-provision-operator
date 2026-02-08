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

package backup

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func newTestBackupResource(name, namespace string) *dbopsv1alpha1.DatabaseBackup {
	return &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: dbopsv1alpha1.DatabaseBackupSpec{
			DatabaseRef: dbopsv1alpha1.DatabaseReference{
				Name: "test-db",
			},
			Storage: dbopsv1alpha1.StorageConfig{
				Type: dbopsv1alpha1.StorageTypePVC,
				PVC: &dbopsv1alpha1.PVCStorageConfig{
					ClaimName: "backup-pvc",
					SubPath:   "backups",
				},
			},
		},
	}
}

func newTestDatabase(name, namespace string) *dbopsv1alpha1.Database {
	return &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: name,
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
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
			Phase:   dbopsv1alpha1.PhaseReady,
			Version: "15.1",
		},
	}
}

func TestController_Reconcile_NewBackup(t *testing.T) {
	scheme := newTestScheme()
	backup := newTestBackupResource("testbackup", "default")
	database := newTestDatabase("test-db", "default")
	instance := newTestInstance("test-instance", "default")

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, database, instance).
		WithStatusSubresource(backup, database, instance).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetDatabaseFunc = func(ctx context.Context, b *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error) {
		return database, nil
	}
	mockRepo.GetInstanceFunc = func(ctx context.Context, db *dbopsv1alpha1.Database) (*dbopsv1alpha1.DatabaseInstance, error) {
		return instance, nil
	}
	mockRepo.GetEngineFunc = func(ctx context.Context, b *dbopsv1alpha1.DatabaseBackup) (string, error) {
		return "postgres", nil
	}
	mockRepo.ExecuteBackupFunc = func(ctx context.Context, b *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
		return &BackupExecutionResult{
			Path:      "/backups/testdb-backup.dump",
			SizeBytes: 1024 * 1024,
			Checksum:  "abc123",
			Format:    "custom",
			Duration:  5 * time.Second,
			Database:  "test-db",
		}, nil
	}

	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	require.NoError(t, err)

	// Verify the backup was updated
	var updatedBackup dbopsv1alpha1.DatabaseBackup
	err = client.Get(context.Background(), types.NamespacedName{Name: "testbackup", Namespace: "default"}, &updatedBackup)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseCompleted, updatedBackup.Status.Phase)
	assert.True(t, mockRepo.WasCalled("ExecuteBackup"))

	// If TTL was not set, result should not requeue
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_WaitingForDatabase(t *testing.T) {
	scheme := newTestScheme()
	backup := newTestBackupResource("testbackup", "default")
	// No database exists - backup should wait

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup).
		WithStatusSubresource(backup).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetDatabaseFunc = func(ctx context.Context, b *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error) {
		return nil, apierrors.NewNotFound(schema.GroupResource{Group: "dbops.dbprovision.io", Resource: "databases"}, "test-db")
	}

	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	// Controller returns error for requeue and sets status to Failed
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the backup is in failed state (database not found is an error)
	var updatedBackup dbopsv1alpha1.DatabaseBackup
	getErr := client.Get(context.Background(), types.NamespacedName{Name: "testbackup", Namespace: "default"}, &updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedBackup.Status.Phase)

	// Verify ExecuteBackup was NOT called
	assert.False(t, mockRepo.WasCalled("ExecuteBackup"))
}

func TestController_Reconcile_BackupNotFound(t *testing.T) {
	scheme := newTestScheme()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
	backup := newTestBackupResource("testbackup", "default")
	backup.Annotations = map[string]string{
		util.AnnotationSkipReconcile: "true",
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup).
		WithStatusSubresource(backup).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify no repository methods were called
	assert.False(t, mockRepo.WasCalled("ExecuteBackup"))
}

func TestController_Reconcile_Deletion(t *testing.T) {
	scheme := newTestScheme()
	backup := newTestBackupResource("testbackup", "default")
	backup.Finalizers = []string{util.FinalizerDatabaseBackup}
	now := metav1.Now()
	backup.DeletionTimestamp = &now

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup).
		WithStatusSubresource(backup).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.DeleteBackupFunc = func(ctx context.Context, b *dbopsv1alpha1.DatabaseBackup) error {
		return nil
	}

	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify delete was called
	assert.True(t, mockRepo.WasCalled("DeleteBackup"))
}

func TestController_Reconcile_FailedBackupNoRetry(t *testing.T) {
	scheme := newTestScheme()
	backup := newTestBackupResource("testbackup", "default")
	backup.Status.Phase = dbopsv1alpha1.PhaseFailed
	backup.Status.Message = "Previous backup failed"

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup).
		WithStatusSubresource(backup).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result) // No requeue for failed

	// Verify ExecuteBackup was NOT called - failed backups don't retry
	assert.False(t, mockRepo.WasCalled("ExecuteBackup"))
}

func TestController_Reconcile_CompletedWithExpiration(t *testing.T) {
	scheme := newTestScheme()
	backup := newTestBackupResource("testbackup", "default")
	backup.Status.Phase = dbopsv1alpha1.PhaseCompleted
	backup.Status.Message = "Backup completed"

	// Set expiration in the future
	future := metav1.NewTime(time.Now().Add(1 * time.Hour))
	backup.Status.ExpiresAt = &future

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup).
		WithStatusSubresource(backup).
		Build()

	mockRepo := NewMockRepository()
	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	// Should requeue for expiration check
	assert.True(t, result.RequeueAfter > 0)
}

func TestController_Reconcile_WaitingForDatabaseReady(t *testing.T) {
	scheme := newTestScheme()
	backup := newTestBackupResource("testbackup", "default")
	database := newTestDatabase("test-db", "default")
	database.Status.Phase = dbopsv1alpha1.PhasePending // Database not ready

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, database).
		WithStatusSubresource(backup, database).
		Build()

	mockRepo := NewMockRepository()
	mockRepo.GetDatabaseFunc = func(ctx context.Context, b *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error) {
		return database, nil
	}

	handler := &Handler{
		repo:              mockRepo,
		eventBus:          NewMockEventBus(),
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
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
			Name:      "testbackup",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.NotEqual(t, ctrl.Result{}, result) // Should requeue

	// Verify the backup is pending
	var updatedBackup dbopsv1alpha1.DatabaseBackup
	err = client.Get(context.Background(), types.NamespacedName{Name: "testbackup", Namespace: "default"}, &updatedBackup)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "Waiting for Database")

	// Verify ExecuteBackup was NOT called
	assert.False(t, mockRepo.WasCalled("ExecuteBackup"))
}
