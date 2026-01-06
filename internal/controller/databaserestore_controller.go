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
	"io"
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

// DatabaseRestoreReconciler reconciles a DatabaseRestore object
type DatabaseRestoreReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaserestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaserestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaserestores/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseRestore resource
	var restore dbopsv1alpha1.DatabaseRestore
	if err := r.Get(ctx, req.NamespacedName, &restore); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseRestore")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(&restore) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion - minimal cleanup for restore
	if util.IsMarkedForDeletion(&restore) {
		if controllerutil.ContainsFinalizer(&restore, util.FinalizerDatabaseRestore) {
			// Nothing to clean up for restore - just remove finalizer
			controllerutil.RemoveFinalizer(&restore, util.FinalizerDatabaseRestore)
			if err := r.Update(ctx, &restore); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&restore, util.FinalizerDatabaseRestore) {
		controllerutil.AddFinalizer(&restore, util.FinalizerDatabaseRestore)
		if err := r.Update(ctx, &restore); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if restore is already completed or failed - don't re-run
	if restore.Status.Phase == dbopsv1alpha1.PhaseCompleted {
		return ctrl.Result{}, nil
	}

	if restore.Status.Phase == dbopsv1alpha1.PhaseFailed {
		// Don't retry failed restores automatically
		return ctrl.Result{}, nil
	}

	// Check active deadline
	if restore.Status.StartedAt != nil && restore.Spec.ActiveDeadlineSeconds > 0 {
		deadline := restore.Status.StartedAt.Add(time.Duration(restore.Spec.ActiveDeadlineSeconds) * time.Second)
		if time.Now().After(deadline) {
			restore.Status.Phase = dbopsv1alpha1.PhaseFailed
			restore.Status.Message = "Restore exceeded active deadline"
			util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "DeadlineExceeded", restore.Status.Message)
			if err := r.Status().Update(ctx, &restore); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{}, nil
		}
	}

	// Set initial phase
	if restore.Status.Phase == "" {
		restore.Status.Phase = dbopsv1alpha1.PhasePending
		restore.Status.Message = "Initializing restore"
		if err := r.Status().Update(ctx, &restore); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate source - must have either BackupRef or FromPath
	if restore.Spec.BackupRef == nil && restore.Spec.FromPath == nil {
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = "Either backupRef or fromPath must be specified"
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "InvalidSpec", restore.Status.Message)
		if err := r.Status().Update(ctx, &restore); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	// Validate safety confirmation for in-place restore
	if restore.Spec.Target.InPlace {
		if restore.Spec.Confirmation == nil ||
			restore.Spec.Confirmation.AcknowledgeDataLoss != dbopsv1alpha1.RestoreConfirmDataLoss {
			restore.Status.Phase = dbopsv1alpha1.PhaseFailed
			restore.Status.Message = fmt.Sprintf("In-place restore requires confirmation.acknowledgeDataLoss to be set to '%s'",
				dbopsv1alpha1.RestoreConfirmDataLoss)
			util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "ConfirmationRequired", restore.Status.Message)
			if err := r.Status().Update(ctx, &restore); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{}, nil
		}
	}

	// Get source backup information if using BackupRef
	var sourceBackup *dbopsv1alpha1.DatabaseBackup
	if restore.Spec.BackupRef != nil {
		backupNamespace := restore.Namespace
		if restore.Spec.BackupRef.Namespace != "" {
			backupNamespace = restore.Spec.BackupRef.Namespace
		}

		sourceBackup = &dbopsv1alpha1.DatabaseBackup{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      restore.Spec.BackupRef.Name,
			Namespace: backupNamespace,
		}, sourceBackup); err != nil {
			if errors.IsNotFound(err) {
				restore.Status.Phase = dbopsv1alpha1.PhaseFailed
				restore.Status.Message = fmt.Sprintf("Referenced DatabaseBackup %s/%s not found", backupNamespace, restore.Spec.BackupRef.Name)
				util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "BackupNotFound", restore.Status.Message)
				if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
					log.Error(statusErr, "Failed to update status")
				}
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to fetch DatabaseBackup")
			return ctrl.Result{}, err
		}

		// Validate backup is completed
		if sourceBackup.Status.Phase != dbopsv1alpha1.PhaseCompleted {
			restore.Status.Phase = dbopsv1alpha1.PhasePending
			restore.Status.Message = fmt.Sprintf("Waiting for DatabaseBackup %s to complete", sourceBackup.Name)
			util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "BackupNotReady", restore.Status.Message)
			if err := r.Status().Update(ctx, &restore); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Determine target instance and database
	var targetInstance *dbopsv1alpha1.DatabaseInstance
	var targetDatabaseName string

	if restore.Spec.Target.InPlace && restore.Spec.Target.DatabaseRef != nil {
		// In-place restore: get instance from the target database's instanceRef
		dbNamespace := restore.Namespace
		if restore.Spec.Target.DatabaseRef.Namespace != "" {
			dbNamespace = restore.Spec.Target.DatabaseRef.Namespace
		}

		var targetDB dbopsv1alpha1.Database
		if err := r.Get(ctx, types.NamespacedName{
			Name:      restore.Spec.Target.DatabaseRef.Name,
			Namespace: dbNamespace,
		}, &targetDB); err != nil {
			if errors.IsNotFound(err) {
				restore.Status.Phase = dbopsv1alpha1.PhaseFailed
				restore.Status.Message = fmt.Sprintf("Target Database %s/%s not found", dbNamespace, restore.Spec.Target.DatabaseRef.Name)
				util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "TargetNotFound", restore.Status.Message)
				if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
					log.Error(statusErr, "Failed to update status")
				}
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to fetch target Database")
			return ctrl.Result{}, err
		}

		targetDatabaseName = targetDB.Spec.Name

		// Get the instance from the target database
		instanceNamespace := targetDB.Namespace
		if targetDB.Spec.InstanceRef.Namespace != "" {
			instanceNamespace = targetDB.Spec.InstanceRef.Namespace
		}

		targetInstance = &dbopsv1alpha1.DatabaseInstance{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      targetDB.Spec.InstanceRef.Name,
			Namespace: instanceNamespace,
		}, targetInstance); err != nil {
			if errors.IsNotFound(err) {
				restore.Status.Phase = dbopsv1alpha1.PhaseFailed
				restore.Status.Message = fmt.Sprintf("Target DatabaseInstance %s/%s not found", instanceNamespace, targetDB.Spec.InstanceRef.Name)
				util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "InstanceNotFound", restore.Status.Message)
				if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
					log.Error(statusErr, "Failed to update status")
				}
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to fetch DatabaseInstance")
			return ctrl.Result{}, err
		}
	} else if restore.Spec.Target.InstanceRef != nil {
		// Restore to new database on specified instance
		instanceNamespace := restore.Namespace
		if restore.Spec.Target.InstanceRef.Namespace != "" {
			instanceNamespace = restore.Spec.Target.InstanceRef.Namespace
		}

		targetInstance = &dbopsv1alpha1.DatabaseInstance{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      restore.Spec.Target.InstanceRef.Name,
			Namespace: instanceNamespace,
		}, targetInstance); err != nil {
			if errors.IsNotFound(err) {
				restore.Status.Phase = dbopsv1alpha1.PhaseFailed
				restore.Status.Message = fmt.Sprintf("Target DatabaseInstance %s/%s not found", instanceNamespace, restore.Spec.Target.InstanceRef.Name)
				util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "InstanceNotFound", restore.Status.Message)
				if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
					log.Error(statusErr, "Failed to update status")
				}
				return ctrl.Result{}, nil
			}
			log.Error(err, "Failed to fetch DatabaseInstance")
			return ctrl.Result{}, err
		}

		targetDatabaseName = restore.Spec.Target.DatabaseName
		if targetDatabaseName == "" && sourceBackup != nil && sourceBackup.Status.Source != nil {
			// Default to source database name from backup
			targetDatabaseName = sourceBackup.Status.Source.Database
		}
	} else {
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = "Either target.instanceRef or target.databaseRef (with inPlace: true) must be specified"
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "InvalidTarget", restore.Status.Message)
		if err := r.Status().Update(ctx, &restore); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	// Check if instance is ready
	if targetInstance.Status.Phase != dbopsv1alpha1.PhaseReady {
		restore.Status.Phase = dbopsv1alpha1.PhasePending
		restore.Status.Message = fmt.Sprintf("Waiting for DatabaseInstance %s to be ready", targetInstance.Name)
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "InstanceNotReady", restore.Status.Message)
		if err := r.Status().Update(ctx, &restore); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get credentials with retry
	var creds *secret.Credentials
	retryConfig := util.ConnectionRetryConfig()
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		creds, err = r.SecretManager.GetCredentials(ctx, targetInstance.Namespace, targetInstance.Spec.Connection.SecretRef)
		return err
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Failed to get credentials after retries",
			"attempts", result.Attempts)
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = fmt.Sprintf("Failed to get credentials: %v", result.LastError)
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "CredentialsFailed", restore.Status.Message)
		if err := r.Status().Update(ctx, &restore); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
	}

	// Get TLS credentials if TLS is enabled
	var tlsCreds *secret.TLSCredentials
	if targetInstance.Spec.TLS != nil && targetInstance.Spec.TLS.Enabled {
		result := util.RetryWithBackoff(ctx, retryConfig, func() error {
			var err error
			tlsCreds, err = r.SecretManager.GetTLSCredentials(ctx, targetInstance.Namespace, targetInstance.Spec.TLS)
			return err
		})
		if result.LastError != nil {
			log.Error(result.LastError, "Failed to get TLS credentials")
			restore.Status.Phase = dbopsv1alpha1.PhaseFailed
			restore.Status.Message = fmt.Sprintf("Failed to get TLS credentials: %v", result.LastError)
			util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "TLSCredentialsFailed", restore.Status.Message)
			if err := r.Status().Update(ctx, &restore); err != nil {
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
	connConfig := adapter.BuildConnectionConfig(&targetInstance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create database adapter
	dbAdapter, err := adapter.NewAdapter(targetInstance.Spec.Engine, connConfig)
	if err != nil {
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = fmt.Sprintf("Unsupported database engine: %s", targetInstance.Spec.Engine)
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "UnsupportedEngine", restore.Status.Message)
		if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
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
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = fmt.Sprintf("Failed to connect to database: %v", result.LastError)
		util.SetConnectedCondition(&restore.Status.Conditions, metav1.ConditionFalse, "ConnectionFailed", restore.Status.Message)
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "ConnectionFailed", restore.Status.Message)
		if err := r.Status().Update(ctx, &restore); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
	}

	util.SetConnectedCondition(&restore.Status.Conditions, metav1.ConditionTrue, "Connected", "Successfully connected to database")

	// Record restore start time for metrics
	restoreStartTime := time.Now()
	engine := string(targetInstance.Spec.Engine)

	// Update status to Running
	now := metav1.Now()
	restore.Status.Phase = dbopsv1alpha1.PhaseRunning
	restore.Status.Message = "Restore in progress"
	restore.Status.StartedAt = &now
	restore.Status.Restore = &dbopsv1alpha1.RestoreInfo{
		TargetInstance: targetInstance.Name,
		TargetDatabase: targetDatabaseName,
	}
	if sourceBackup != nil {
		restore.Status.Restore.SourceBackup = sourceBackup.Name
	}
	restore.Status.Progress = &dbopsv1alpha1.RestoreProgress{
		Percentage:   0,
		CurrentPhase: "Starting",
	}
	if err := r.Status().Update(ctx, &restore); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Create restore reader from storage backend with decompression and decryption
	var restoreReader io.ReadCloser
	var storageConfig *dbopsv1alpha1.StorageConfig
	var compressionConfig *dbopsv1alpha1.CompressionConfig
	var encryptionConfig *dbopsv1alpha1.EncryptionConfig
	var backupPath string

	if sourceBackup != nil {
		// Use storage config from the source backup
		storageConfig = &sourceBackup.Spec.Storage
		compressionConfig = sourceBackup.Spec.Compression
		encryptionConfig = sourceBackup.Spec.Encryption
		if sourceBackup.Status.Backup != nil {
			backupPath = sourceBackup.Status.Backup.Path
		}
	} else if restore.Spec.FromPath != nil {
		// Use storage config from FromPath
		storageConfig = &restore.Spec.FromPath.Storage
		compressionConfig = restore.Spec.FromPath.Compression
		encryptionConfig = restore.Spec.FromPath.Encryption
		backupPath = restore.Spec.FromPath.BackupPath
	}

	if backupPath == "" {
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = "Backup path not found"
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "BackupPathMissing", restore.Status.Message)
		if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	reader, err := storage.NewRestoreReader(ctx, &storage.RestoreReaderConfig{
		StorageConfig:     storageConfig,
		CompressionConfig: compressionConfig,
		EncryptionConfig:  encryptionConfig,
		SecretManager:     r.SecretManager,
		Namespace:         restore.Namespace,
		BackupPath:        backupPath,
	})
	if err != nil {
		log.Error(err, "Failed to create restore reader")
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = fmt.Sprintf("Failed to read backup from storage: %v", err)
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "StorageFailed", restore.Status.Message)
		if statusErr := r.Status().Update(ctx, &restore); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	restoreReader = reader
	defer func() { _ = restoreReader.Close() }()

	// Build restore options
	restoreOpts := r.buildRestoreOptions(&restore, sourceBackup, targetDatabaseName, targetInstance.Spec.Engine, restoreReader)

	// Execute restore with retry
	restoreRetryConfig := util.BackupRetryConfig()
	var restoreResult *adapterpkg.RestoreResult
	result = util.RetryWithBackoff(ctx, restoreRetryConfig, func() error {
		var err error
		restoreResult, err = dbAdapter.Restore(ctx, restoreOpts)
		return err
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Restore failed after retries",
			"attempts", result.Attempts,
			"duration", result.TotalTime)
		restore.Status.Phase = dbopsv1alpha1.PhaseFailed
		restore.Status.Message = fmt.Sprintf("Restore failed: %v", result.LastError)
		util.SetSyncedCondition(&restore.Status.Conditions, metav1.ConditionFalse, "RestoreFailed", restore.Status.Message)
		util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "RestoreFailed", restore.Status.Message)
		if err := r.Status().Update(ctx, &restore); err != nil {
			log.Error(err, "Failed to update status")
		}
		// Record failure metric
		metrics.RecordRestoreOperation(engine, restore.Namespace, metrics.StatusFailure)
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(restoreRetryConfig, result)}, nil
	}

	// Update status to Completed
	completedAt := metav1.Now()
	restore.Status.Phase = dbopsv1alpha1.PhaseCompleted
	restore.Status.Message = "Restore completed successfully"
	restore.Status.CompletedAt = &completedAt
	restore.Status.Restore = &dbopsv1alpha1.RestoreInfo{
		TargetInstance: targetInstance.Name,
		TargetDatabase: restoreResult.TargetDatabase,
	}
	if sourceBackup != nil {
		restore.Status.Restore.SourceBackup = sourceBackup.Name
	}
	restore.Status.Progress = &dbopsv1alpha1.RestoreProgress{
		Percentage:     100,
		CurrentPhase:   "Completed",
		TablesRestored: restoreResult.TablesRestored,
		TablesTotal:    restoreResult.TablesRestored,
	}
	restore.Status.Warnings = restoreResult.Warnings
	util.SetSyncedCondition(&restore.Status.Conditions, metav1.ConditionTrue, "RestoreCompleted", "Restore completed successfully")
	util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionTrue, "Ready", "DatabaseRestore is ready")

	if err := r.Status().Update(ctx, &restore); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Record success metrics
	duration := time.Since(restoreStartTime).Seconds()
	metrics.RecordRestoreOperation(engine, restore.Namespace, metrics.StatusSuccess)
	metrics.RecordRestoreDuration(engine, restore.Namespace, duration)

	log.Info("DatabaseRestore completed successfully",
		"restore", restore.Name,
		"targetDatabase", restoreResult.TargetDatabase,
		"tablesRestored", restoreResult.TablesRestored,
		"duration", time.Since(now.Time))

	return ctrl.Result{}, nil
}

// buildRestoreOptions builds restore options from the spec
func (r *DatabaseRestoreReconciler) buildRestoreOptions(restore *dbopsv1alpha1.DatabaseRestore, sourceBackup *dbopsv1alpha1.DatabaseBackup, targetDatabase string, engine dbopsv1alpha1.EngineType, reader io.Reader) adapterpkg.RestoreOptions {
	opts := adapterpkg.RestoreOptions{
		Database:  targetDatabase,
		RestoreID: string(restore.UID),
		Reader:    reader,
	}

	// Set drop existing based on in-place setting
	if restore.Spec.Target.InPlace {
		opts.DropExisting = true
	}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if restore.Spec.Postgres != nil {
			opts.DropExisting = restore.Spec.Postgres.DropExisting || opts.DropExisting
			opts.CreateDatabase = restore.Spec.Postgres.CreateDatabase
			opts.DataOnly = restore.Spec.Postgres.DataOnly
			opts.SchemaOnly = restore.Spec.Postgres.SchemaOnly
			opts.NoOwner = restore.Spec.Postgres.NoOwner
			opts.NoPrivileges = restore.Spec.Postgres.NoPrivileges
			opts.RoleMapping = restore.Spec.Postgres.RoleMapping
			opts.Schemas = restore.Spec.Postgres.Schemas
			opts.Tables = restore.Spec.Postgres.Tables
			opts.Jobs = restore.Spec.Postgres.Jobs
			opts.DisableTriggers = restore.Spec.Postgres.DisableTriggers
			opts.Analyze = restore.Spec.Postgres.Analyze
		} else {
			// Set defaults for PostgreSQL
			opts.CreateDatabase = true
			opts.NoOwner = true
			opts.Jobs = 1
			opts.Analyze = true
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if restore.Spec.MySQL != nil {
			opts.DropExisting = restore.Spec.MySQL.DropExisting || opts.DropExisting
			opts.CreateDatabase = restore.Spec.MySQL.CreateDatabase
			opts.Routines = restore.Spec.MySQL.Routines
			opts.Triggers = restore.Spec.MySQL.Triggers
			opts.Events = restore.Spec.MySQL.Events
			opts.DisableForeignKeyChecks = restore.Spec.MySQL.DisableForeignKeyChecks
			opts.DisableBinlog = restore.Spec.MySQL.DisableBinlog
		} else {
			// Set defaults for MySQL
			opts.CreateDatabase = true
			opts.Routines = true
			opts.Triggers = true
			opts.Events = true
			opts.DisableForeignKeyChecks = true
			opts.DisableBinlog = true
		}
	}

	return opts
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseRestore{}).
		Named("databaserestore").
		Complete(r)
}
