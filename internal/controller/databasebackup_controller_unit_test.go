//go:build !integration && !e2e && !envtest

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

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// newBackupFakeClient creates a fake client with DatabaseBackup status subresource registered.
func newBackupFakeClient(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(objs...).
		WithStatusSubresource(
			&dbopsv1alpha1.DatabaseBackup{},
			&dbopsv1alpha1.Database{},
			&dbopsv1alpha1.DatabaseInstance{},
		).
		Build()
}

// TestDatabaseBackupReconciler_NotFound tests that reconciliation handles a missing DatabaseBackup gracefully.
func TestDatabaseBackupReconciler_NotFound(t *testing.T) {
	fakeClient := newBackupFakeClient()
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

// TestDatabaseBackupReconciler_SkipReconcile tests that reconciliation is skipped when annotation is present.
func TestDatabaseBackupReconciler_SkipReconcile(t *testing.T) {
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationSkipReconcile: "true",
			},
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
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should return empty result when skipping reconciliation")
}

// TestDatabaseBackupReconciler_AddsFinalizer tests that the finalizer is added when not present.
func TestDatabaseBackupReconciler_AddsFinalizer(t *testing.T) {
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "default",
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
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	_, _ = r.Reconcile(context.Background(), req)

	// Refetch the DatabaseBackup
	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	err := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, err)

	assert.Contains(t, updatedBackup.Finalizers, util.FinalizerDatabaseBackup, "Finalizer should be added")
}

// TestDatabaseBackupReconciler_CompletedBackupNoRerun tests that completed backups are not re-run.
func TestDatabaseBackupReconciler_CompletedBackupNoRerun(t *testing.T) {
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
			Phase:   dbopsv1alpha1.PhaseCompleted,
			Message: "Backup completed successfully",
		},
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should not requeue a completed backup without expiration")

	// Verify status was not changed
	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseCompleted, updatedBackup.Status.Phase)
}

// TestDatabaseBackupReconciler_CompletedBackupWithTTLRequeues tests that a completed backup with TTL requeues.
func TestDatabaseBackupReconciler_CompletedBackupWithTTLRequeues(t *testing.T) {
	futureExpiry := metav1.NewTime(time.Now().Add(1 * time.Hour))
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
			TTL: "1h",
		},
		Status: dbopsv1alpha1.DatabaseBackupStatus{
			Phase:     dbopsv1alpha1.PhaseCompleted,
			Message:   "Backup completed successfully",
			ExpiresAt: &futureExpiry,
		},
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue to check expiration later")
}

// TestDatabaseBackupReconciler_FailedBackupNoRetry tests that failed backups are not retried automatically.
func TestDatabaseBackupReconciler_FailedBackupNoRetry(t *testing.T) {
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
			Phase:   dbopsv1alpha1.PhaseFailed,
			Message: "Backup failed: connection timeout",
		},
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should NOT requeue a failed backup (terminal state)")
}

// TestDatabaseBackupReconciler_DatabaseNotFound tests that a missing database reference fails the backup.
func TestDatabaseBackupReconciler_DatabaseNotFound(t *testing.T) {
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
		},
		Spec: dbopsv1alpha1.DatabaseBackupSpec{
			DatabaseRef: dbopsv1alpha1.DatabaseReference{
				Name: "nonexistent-db",
			},
			Storage: dbopsv1alpha1.StorageConfig{
				Type: dbopsv1alpha1.StorageTypePVC,
				PVC: &dbopsv1alpha1.PVCStorageConfig{
					ClaimName: "backup-pvc",
				},
			},
		},
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should not requeue when database not found (terminal)")

	// Verify status was set to Failed
	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "not found")
}

// TestDatabaseBackupReconciler_DatabaseNotReady tests that backup pends when database is not ready.
func TestDatabaseBackupReconciler_DatabaseNotReady(t *testing.T) {
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "mydb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
			Phase: dbopsv1alpha1.PhasePending, // Not ready
		},
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
	}

	fakeClient := newBackupFakeClient(database, backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when database not ready")

	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "Waiting for Database")
}

// TestDatabaseBackupReconciler_InstanceNotFound tests that a missing instance fails the backup.
func TestDatabaseBackupReconciler_InstanceNotFound(t *testing.T) {
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "mydb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "nonexistent-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
	}

	fakeClient := newBackupFakeClient(database, backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should not requeue when instance not found (terminal)")

	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "not found")
}

// TestDatabaseBackupReconciler_InstanceNotReady tests that backup pends when instance is not ready.
func TestDatabaseBackupReconciler_InstanceNotReady(t *testing.T) {
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
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
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhasePending, // Not ready
		},
	}

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "mydb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
	}

	fakeClient := newBackupFakeClient(instance, database, backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when instance not ready")

	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "Waiting for DatabaseInstance")
}

// TestDatabaseBackupReconciler_CredentialsSecretNotFound tests that missing credentials fail the backup.
func TestDatabaseBackupReconciler_CredentialsSecretNotFound(t *testing.T) {
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "nonexistent-secret",
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "mydb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
	}

	fakeClient := newBackupFakeClient(instance, database, backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when credentials not found")

	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "credentials")
}

// TestDatabaseBackupReconciler_DeletionRemovesFinalizer tests that deletion cleans up and removes finalizer.
func TestDatabaseBackupReconciler_DeletionRemovesFinalizer(t *testing.T) {
	now := metav1.Now()
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-backup",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseBackup},
			DeletionTimestamp: &now,
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
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Object should be deleted after finalizer removal
	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseBackupReconciler_DeletionWithBackupPath tests that deletion attempts to clean up backup files.
func TestDatabaseBackupReconciler_DeletionWithBackupPath(t *testing.T) {
	now := metav1.Now()
	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-backup",
			Namespace:         "default",
			Finalizers:        []string{util.FinalizerDatabaseBackup},
			DeletionTimestamp: &now,
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
				Path:      "/backups/test-backup.dump",
				SizeBytes: 1024,
			},
			Source: &dbopsv1alpha1.BackupSourceInfo{
				Instance: "test-instance",
				Database: "mydb",
				Engine:   "postgres",
			},
		},
	}

	fakeClient := newBackupFakeClient(backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should succeed even if the actual file deletion fails (no real storage backend)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Finalizer should be removed
	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	err = fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	assert.True(t, err != nil, "Object should be deleted after finalizer removal")
}

// TestDatabaseBackupReconciler_CrossNamespaceDatabaseRef tests cross-namespace database reference.
func TestDatabaseBackupReconciler_CrossNamespaceDatabaseRef(t *testing.T) {
	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "other-namespace",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "mydb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
			Phase: dbopsv1alpha1.PhasePending, // Not ready
		},
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
		},
		Spec: dbopsv1alpha1.DatabaseBackupSpec{
			DatabaseRef: dbopsv1alpha1.DatabaseReference{
				Name:      "test-db",
				Namespace: "other-namespace",
			},
			Storage: dbopsv1alpha1.StorageConfig{
				Type: dbopsv1alpha1.StorageTypePVC,
				PVC: &dbopsv1alpha1.PVCStorageConfig{
					ClaimName: "backup-pvc",
				},
			},
		},
	}

	fakeClient := newBackupFakeClient(database, backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue when database not ready")

	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	// Should find the database in other namespace (pending, not "not found")
	assert.Equal(t, dbopsv1alpha1.PhasePending, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "Waiting for Database")
}

// TestDatabaseBackupReconciler_ConnectFailsWithCredentials tests that connection failure after
// getting credentials sets the proper status.
func TestDatabaseBackupReconciler_ConnectFailsWithCredentials(t *testing.T) {
	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "default",
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
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	database := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "mydb",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		},
		Status: dbopsv1alpha1.DatabaseStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backup",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackup},
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
	}

	fakeClient := newBackupFakeClient(instance, database, dbSecret, backup)
	r := &DatabaseBackupReconciler{
		Client:        fakeClient,
		Scheme:        testScheme(),
		SecretManager: secret.NewManager(fakeClient),
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-backup",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(context.Background(), req)

	// Should fail at connect (no real DB), set Failed phase
	require.NoError(t, err)
	assert.NotZero(t, result.RequeueAfter, "Should requeue after connection failure")

	updatedBackup := &dbopsv1alpha1.DatabaseBackup{}
	getErr := fakeClient.Get(context.Background(), req.NamespacedName, updatedBackup)
	require.NoError(t, getErr)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updatedBackup.Status.Phase)
	assert.Contains(t, updatedBackup.Status.Message, "connect")
}
