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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/storage"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseBackupReconciler reconciles a DatabaseBackup object
type DatabaseBackupReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseBackup resource
	var backup dbopsv1alpha1.DatabaseBackup
	if err := r.Get(ctx, req.NamespacedName, &backup); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseBackup")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(&backup) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(&backup) {
		if controllerutil.ContainsFinalizer(&backup, util.FinalizerDatabaseBackup) {
			if err := r.handleDeletion(ctx, &backup); err != nil {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			controllerutil.RemoveFinalizer(&backup, util.FinalizerDatabaseBackup)
			if err := r.Update(ctx, &backup); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&backup, util.FinalizerDatabaseBackup) {
		controllerutil.AddFinalizer(&backup, util.FinalizerDatabaseBackup)
		if err := r.Update(ctx, &backup); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if backup is already completed or failed - don't re-run
	if backup.Status.Phase == dbopsv1alpha1.PhaseCompleted {
		// Check if expired
		if backup.Status.ExpiresAt != nil && time.Now().After(backup.Status.ExpiresAt.Time) {
			log.Info("Backup has expired, deleting", "expiresAt", backup.Status.ExpiresAt)
			if err := r.Delete(ctx, &backup); err != nil {
				log.Error(err, "Failed to delete expired backup")
				return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}
		// Requeue to check expiration later
		if backup.Status.ExpiresAt != nil {
			requeueAfter := time.Until(backup.Status.ExpiresAt.Time)
			if requeueAfter > 0 {
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
		}
		return ctrl.Result{}, nil
	}

	if backup.Status.Phase == dbopsv1alpha1.PhaseFailed {
		// Don't retry failed backups automatically
		return ctrl.Result{}, nil
	}

	// Set initial phase
	if backup.Status.Phase == "" {
		backup.Status.Phase = dbopsv1alpha1.PhasePending
		backup.Status.Message = "Initializing backup"
		if err := r.Status().Update(ctx, &backup); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch the referenced Database
	dbNamespace := backup.Namespace
	if backup.Spec.DatabaseRef.Namespace != "" {
		dbNamespace = backup.Spec.DatabaseRef.Namespace
	}

	var database dbopsv1alpha1.Database
	if err := r.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.DatabaseRef.Name,
		Namespace: dbNamespace,
	}, &database); err != nil {
		if errors.IsNotFound(err) {
			backup.Status.Phase = dbopsv1alpha1.PhaseFailed
			backup.Status.Message = fmt.Sprintf("Referenced Database %s/%s not found", dbNamespace, backup.Spec.DatabaseRef.Name)
			util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "DatabaseNotFound", backup.Status.Message)
			if statusErr := r.Status().Update(ctx, &backup); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch Database")
		return ctrl.Result{}, err
	}

	// Check if database is ready
	if database.Status.Phase != dbopsv1alpha1.PhaseReady {
		backup.Status.Phase = dbopsv1alpha1.PhasePending
		backup.Status.Message = fmt.Sprintf("Waiting for Database %s to be ready", database.Name)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "DatabaseNotReady", backup.Status.Message)
		if err := r.Status().Update(ctx, &backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Fetch the DatabaseInstance from the database's instanceRef
	instanceNamespace := database.Namespace
	if database.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = database.Spec.InstanceRef.Namespace
	}

	var instance dbopsv1alpha1.DatabaseInstance
	if err := r.Get(ctx, types.NamespacedName{
		Name:      database.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}, &instance); err != nil {
		if errors.IsNotFound(err) {
			backup.Status.Phase = dbopsv1alpha1.PhaseFailed
			backup.Status.Message = fmt.Sprintf("DatabaseInstance %s/%s not found", instanceNamespace, database.Spec.InstanceRef.Name)
			util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "InstanceNotFound", backup.Status.Message)
			if statusErr := r.Status().Update(ctx, &backup); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to fetch DatabaseInstance")
		return ctrl.Result{}, err
	}

	// Check if instance is ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		backup.Status.Phase = dbopsv1alpha1.PhasePending
		backup.Status.Message = fmt.Sprintf("Waiting for DatabaseInstance %s to be ready", instance.Name)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "InstanceNotReady", backup.Status.Message)
		if err := r.Status().Update(ctx, &backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get credentials with retry
	var creds *secret.Credentials
	retryConfig := util.ConnectionRetryConfig()
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		creds, err = r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
		return err
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Failed to get credentials after retries",
			"attempts", result.Attempts)
		backup.Status.Phase = dbopsv1alpha1.PhaseFailed
		backup.Status.Message = fmt.Sprintf("Failed to get credentials: %v", result.LastError)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "CredentialsFailed", backup.Status.Message)
		if err := r.Status().Update(ctx, &backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
	}

	// Get TLS credentials if TLS is enabled
	var tlsCreds *secret.TLSCredentials
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		result := util.RetryWithBackoff(ctx, retryConfig, func() error {
			var err error
			tlsCreds, err = r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
			return err
		})
		if result.LastError != nil {
			log.Error(result.LastError, "Failed to get TLS credentials")
			backup.Status.Phase = dbopsv1alpha1.PhaseFailed
			backup.Status.Message = fmt.Sprintf("Failed to get TLS credentials: %v", result.LastError)
			util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "TLSCredentialsFailed", backup.Status.Message)
			if err := r.Status().Update(ctx, &backup); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
		}
	}

	// Build connection config
	var tlsCA, tlsCert, tlsKey []byte
	if tlsCreds != nil {
		tlsCA = tlsCreds.CA
		tlsCert = tlsCreds.Cert
		tlsKey = tlsCreds.Key
	}
	connConfig := adapter.BuildConnectionConfig(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create database adapter
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, connConfig)
	if err != nil {
		backup.Status.Phase = dbopsv1alpha1.PhaseFailed
		backup.Status.Message = fmt.Sprintf("Unsupported database engine: %s", instance.Spec.Engine)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "UnsupportedEngine", backup.Status.Message)
		if statusErr := r.Status().Update(ctx, &backup); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}
	defer func() { _ = dbAdapter.Close() }()

	// Connect with retry
	result = util.RetryWithBackoff(ctx, retryConfig, func() error {
		return dbAdapter.Connect(ctx)
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Failed to connect to database after retries",
			"attempts", result.Attempts)
		backup.Status.Phase = dbopsv1alpha1.PhaseFailed
		backup.Status.Message = fmt.Sprintf("Failed to connect to database: %v", result.LastError)
		util.SetConnectedCondition(&backup.Status.Conditions, metav1.ConditionFalse, "ConnectionFailed", backup.Status.Message)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "ConnectionFailed", backup.Status.Message)
		if err := r.Status().Update(ctx, &backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
	}

	util.SetConnectedCondition(&backup.Status.Conditions, metav1.ConditionTrue, "Connected", "Successfully connected to database")

	// Update status to Running
	now := metav1.Now()
	backupStartTime := time.Now() // Record start time for metrics
	engine := string(instance.Spec.Engine)
	databaseName := database.Spec.Name
	backup.Status.Phase = dbopsv1alpha1.PhaseRunning
	backup.Status.Message = "Backup in progress"
	backup.Status.StartedAt = &now
	backup.Status.Source = &dbopsv1alpha1.BackupSourceInfo{
		Instance:  instance.Name,
		Database:  databaseName,
		Engine:    engine,
		Version:   instance.Status.Version,
		Timestamp: &now,
	}
	if err := r.Status().Update(ctx, &backup); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Create backup writer with storage backend, compression, and encryption
	backupWriter, err := storage.NewBackupWriter(ctx, &storage.BackupWriterConfig{
		StorageConfig:     &backup.Spec.Storage,
		CompressionConfig: backup.Spec.Compression,
		EncryptionConfig:  backup.Spec.Encryption,
		SecretManager:     r.SecretManager,
		Namespace:         backup.Namespace,
		BackupName:        backup.Name,
		DatabaseName:      database.Spec.Name,
		Timestamp:         now.Time,
		Extension:         r.getBackupExtension(instance.Spec.Engine, &backup),
	})
	if err != nil {
		log.Error(err, "Failed to create backup writer")
		backup.Status.Phase = dbopsv1alpha1.PhaseFailed
		backup.Status.Message = fmt.Sprintf("Failed to create backup writer: %v", err)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "StorageFailed", backup.Status.Message)
		if statusErr := r.Status().Update(ctx, &backup); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Build backup options
	backupOpts := r.buildBackupOptions(&backup, &database, instance.Spec.Engine, backupWriter)

	// Execute backup with retry
	backupRetryConfig := util.BackupRetryConfig()
	var backupResult *adapterpkg.BackupResult
	result = util.RetryWithBackoff(ctx, backupRetryConfig, func() error {
		var err error
		backupResult, err = dbAdapter.Backup(ctx, backupOpts)
		return err
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Backup failed after retries",
			"attempts", result.Attempts,
			"duration", result.TotalTime)
		backup.Status.Phase = dbopsv1alpha1.PhaseFailed
		backup.Status.Message = fmt.Sprintf("Backup failed: %v", result.LastError)
		util.SetSyncedCondition(&backup.Status.Conditions, metav1.ConditionFalse, "BackupFailed", backup.Status.Message)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "BackupFailed", backup.Status.Message)
		// Record failure metric
		metrics.RecordBackupOperation(engine, backup.Namespace, metrics.StatusFailure)
		if err := r.Status().Update(ctx, &backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(backupRetryConfig, result)}, nil
	}

	// Close backup writer to finalize (compress, encrypt, write to storage)
	if err := backupWriter.Close(); err != nil {
		log.Error(err, "Failed to finalize backup")
		backup.Status.Phase = dbopsv1alpha1.PhaseFailed
		backup.Status.Message = fmt.Sprintf("Failed to write backup to storage: %v", err)
		util.SetSyncedCondition(&backup.Status.Conditions, metav1.ConditionFalse, "StorageFailed", backup.Status.Message)
		util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "StorageFailed", backup.Status.Message)
		// Record failure metric
		metrics.RecordBackupOperation(engine, backup.Namespace, metrics.StatusFailure)
		if statusErr := r.Status().Update(ctx, &backup); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Update backup result with path from writer
	backupResult.Path = backupWriter.Path()
	backupResult.SizeBytes = backupWriter.UncompressedSize()

	// Calculate expiration time from TTL
	var expiresAt *metav1.Time
	if backup.Spec.TTL != "" {
		ttlDuration, err := time.ParseDuration(backup.Spec.TTL)
		if err == nil {
			expTime := metav1.NewTime(time.Now().Add(ttlDuration))
			expiresAt = &expTime
		}
	}

	// Update status to Completed
	completedAt := metav1.Now()
	backup.Status.Phase = dbopsv1alpha1.PhaseCompleted
	backup.Status.Message = "Backup completed successfully"
	backup.Status.CompletedAt = &completedAt
	backup.Status.ExpiresAt = expiresAt
	backup.Status.Backup = &dbopsv1alpha1.BackupInfo{
		Path:                backupResult.Path,
		SizeBytes:           backupResult.SizeBytes,
		CompressedSizeBytes: backupResult.CompressedSizeBytes,
		Checksum:            backupResult.Checksum,
		Format:              backupResult.Format,
	}
	util.SetSyncedCondition(&backup.Status.Conditions, metav1.ConditionTrue, "BackupCompleted", "Backup completed successfully")
	util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionTrue, "Ready", "DatabaseBackup is ready")

	// Record success metrics
	duration := time.Since(backupStartTime).Seconds()
	metrics.RecordBackupOperation(engine, backup.Namespace, metrics.StatusSuccess)
	metrics.RecordBackupDuration(engine, backup.Namespace, duration)
	// Use compressed size if available, otherwise use uncompressed size
	backupSize := float64(backupResult.SizeBytes)
	if backupResult.CompressedSizeBytes > 0 {
		backupSize = float64(backupResult.CompressedSizeBytes)
	}
	metrics.SetBackupSize(databaseName, engine, backup.Namespace, backupSize)
	metrics.SetBackupLastSuccess(databaseName, engine, backup.Namespace, float64(time.Now().Unix()))

	if err := r.Status().Update(ctx, &backup); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("DatabaseBackup completed successfully",
		"backup", backup.Name,
		"database", database.Name,
		"path", backupResult.Path,
		"size", backupResult.SizeBytes,
		"duration", time.Since(now.Time))

	// Requeue to handle expiration
	if expiresAt != nil {
		return ctrl.Result{RequeueAfter: time.Until(expiresAt.Time)}, nil
	}

	return ctrl.Result{}, nil
}

// handleDeletion handles the cleanup when a DatabaseBackup is being deleted
func (r *DatabaseBackupReconciler) handleDeletion(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
	log := logf.FromContext(ctx)

	// Update status to Deleting
	backup.Status.Phase = dbopsv1alpha1.PhaseDeleting
	backup.Status.Message = "Cleaning up backup"
	if err := r.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Delete backup file from storage
	if backup.Status.Backup != nil && backup.Status.Backup.Path != "" {
		log.Info("Deleting backup file", "path", backup.Status.Backup.Path)

		err := storage.DeleteBackup(ctx, &storage.Config{
			StorageConfig: &backup.Spec.Storage,
			SecretManager: r.SecretManager,
			Namespace:     backup.Namespace,
		}, backup.Status.Backup.Path)
		if err != nil {
			log.Error(err, "Failed to delete backup file", "path", backup.Status.Backup.Path)
			// Continue with deletion even if file deletion fails
		} else {
			log.Info("Backup file deleted successfully", "path", backup.Status.Backup.Path)
		}
	}

	// Clean up backup metrics
	if backup.Status.Source != nil {
		databaseName := backup.Status.Source.Database
		engine := backup.Status.Source.Engine
		metrics.DeleteBackupMetrics(databaseName, engine, backup.Namespace)
	}

	log.Info("DatabaseBackup cleanup completed", "backup", backup.Name)
	return nil
}

// getBackupExtension returns the appropriate file extension based on engine and backup config
func (r *DatabaseBackupReconciler) getBackupExtension(engine dbopsv1alpha1.EngineType, backup *dbopsv1alpha1.DatabaseBackup) string {
	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if backup.Spec.Postgres != nil {
			switch backup.Spec.Postgres.Format {
			case "plain":
				return ".sql"
			case "custom":
				return ".dump"
			case "directory":
				return ".dir"
			case "tar":
				return ".tar"
			}
		}
		return ".dump" // default for PostgreSQL
	case dbopsv1alpha1.EngineTypeMySQL:
		return ".sql"
	default:
		return ".bak"
	}
}

// buildBackupOptions builds backup options from the spec
func (r *DatabaseBackupReconciler) buildBackupOptions(backup *dbopsv1alpha1.DatabaseBackup, database *dbopsv1alpha1.Database, engine dbopsv1alpha1.EngineType, writer *storage.BackupWriter) adapterpkg.BackupOptions {
	opts := adapterpkg.BackupOptions{
		Database: database.Spec.Name,
		BackupID: string(backup.UID),
		Writer:   writer,
	}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if backup.Spec.Postgres != nil {
			opts.Method = string(backup.Spec.Postgres.Method)
			opts.Format = string(backup.Spec.Postgres.Format)
			opts.Jobs = backup.Spec.Postgres.Jobs
			opts.DataOnly = backup.Spec.Postgres.DataOnly
			opts.SchemaOnly = backup.Spec.Postgres.SchemaOnly
			opts.Blobs = backup.Spec.Postgres.Blobs
			opts.NoOwner = backup.Spec.Postgres.NoOwner
			opts.NoPrivileges = backup.Spec.Postgres.NoPrivileges
			opts.Schemas = backup.Spec.Postgres.Schemas
			opts.ExcludeSchemas = backup.Spec.Postgres.ExcludeSchemas
			opts.Tables = backup.Spec.Postgres.Tables
			opts.ExcludeTables = backup.Spec.Postgres.ExcludeTables
			opts.LockWaitTimeout = backup.Spec.Postgres.LockWaitTimeout
		} else {
			// Set defaults for PostgreSQL
			opts.Method = "pg_dump"
			opts.Format = "custom"
			opts.Jobs = 1
			opts.Blobs = true
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if backup.Spec.MySQL != nil {
			opts.SingleTransaction = backup.Spec.MySQL.SingleTransaction
			opts.Quick = backup.Spec.MySQL.Quick
			opts.LockTables = backup.Spec.MySQL.LockTables
			opts.Routines = backup.Spec.MySQL.Routines
			opts.Triggers = backup.Spec.MySQL.Triggers
			opts.Events = backup.Spec.MySQL.Events
			opts.ExtendedInsert = backup.Spec.MySQL.ExtendedInsert
			opts.SetGtidPurged = string(backup.Spec.MySQL.SetGtidPurged)
		} else {
			// Set defaults for MySQL
			opts.SingleTransaction = true
			opts.Quick = true
			opts.Routines = true
			opts.Triggers = true
		}
	}

	return opts
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseBackup{}).
		Named("databasebackup").
		Complete(r)
}
