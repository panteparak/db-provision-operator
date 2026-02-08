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
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
	"github.com/db-provision-operator/internal/storage"
)

// Handler contains the business logic for restore operations.
type Handler struct {
	repo          RepositoryInterface
	secretManager *secret.Manager
	eventBus      eventbus.Bus
	logger        logr.Logger
	paused        bool // Track if restore operations are paused due to disconnection
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository    RepositoryInterface
	SecretManager *secret.Manager
	EventBus      eventbus.Bus
	Logger        logr.Logger
}

// NewHandler creates a new restore handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:          cfg.Repository,
		secretManager: cfg.SecretManager,
		eventBus:      cfg.EventBus,
		logger:        cfg.Logger,
	}
}

// Execute performs a database restore operation.
func (h *Handler) Execute(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("restore", restore.Name, "namespace", restore.Namespace)

	// Check if operations are paused
	if h.paused {
		return nil, fmt.Errorf("restore operations are paused due to instance disconnection")
	}

	startTime := time.Now()

	// Resolve target instance and database
	targetInstance, targetDatabaseName, err := h.resolveTarget(ctx, restore)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}

	// Get engine for metrics
	engine := string(targetInstance.Spec.Engine)

	// Check if instance is ready
	if targetInstance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return nil, fmt.Errorf("instance %s is not ready (phase: %s)", targetInstance.Name, targetInstance.Status.Phase)
	}

	// Resolve backup source
	sourceBackup, backupPath, storageConfig, compressionConfig, encryptionConfig, err := h.resolveSource(ctx, restore)
	if err != nil {
		return nil, fmt.Errorf("resolve source: %w", err)
	}

	if backupPath == "" {
		return nil, fmt.Errorf("backup path not found")
	}

	// Publish restore started event
	sourceBackupName := ""
	if sourceBackup != nil {
		sourceBackupName = sourceBackup.Name
	}
	h.publishRestoreStarted(ctx, restore, sourceBackupName, targetDatabaseName)

	log.Info("Starting restore",
		"targetInstance", targetInstance.Name,
		"targetDatabase", targetDatabaseName,
		"backupPath", backupPath)

	// Create restore reader
	restoreReader, err := h.repo.CreateRestoreReader(ctx, &storage.RestoreReaderConfig{
		StorageConfig:     storageConfig,
		CompressionConfig: compressionConfig,
		EncryptionConfig:  encryptionConfig,
		SecretManager:     h.secretManager,
		Namespace:         restore.Namespace,
		BackupPath:        backupPath,
	})
	if err != nil {
		metrics.RecordRestoreOperation(engine, restore.Namespace, metrics.StatusFailure)
		h.publishRestoreFailed(ctx, restore, sourceBackupName, targetDatabaseName, err.Error())
		return nil, fmt.Errorf("create restore reader: %w", err)
	}
	defer func() { _ = restoreReader.Close() }()

	// Build restore options
	restoreOpts := h.buildRestoreOptions(restore, sourceBackup, targetDatabaseName, targetInstance.Spec.Engine, restoreReader)

	// Execute restore
	restoreResult, err := h.repo.ExecuteRestore(ctx, targetInstance, restoreOpts)
	if err != nil {
		metrics.RecordRestoreOperation(engine, restore.Namespace, metrics.StatusFailure)
		h.publishRestoreFailed(ctx, restore, sourceBackupName, targetDatabaseName, err.Error())
		return nil, fmt.Errorf("execute restore: %w", err)
	}

	duration := time.Since(startTime)

	// Record success metrics
	metrics.RecordRestoreOperation(engine, restore.Namespace, metrics.StatusSuccess)
	metrics.RecordRestoreDuration(engine, restore.Namespace, duration.Seconds())

	// Publish restore completed event
	h.publishRestoreCompleted(ctx, restore, sourceBackupName, targetDatabaseName, duration)

	log.Info("Restore completed successfully",
		"targetDatabase", restoreResult.TargetDatabase,
		"tablesRestored", restoreResult.TablesRestored,
		"duration", duration)

	return &Result{
		TargetDatabase: restoreResult.TargetDatabase,
		TargetInstance: targetInstance.Name,
		SourceBackup:   sourceBackupName,
		TablesRestored: restoreResult.TablesRestored,
		Duration:       duration,
		Warnings:       restoreResult.Warnings,
	}, nil
}

// ValidateSpec validates the restore specification.
func (h *Handler) ValidateSpec(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) error {
	// Validate source
	if restore.Spec.BackupRef == nil && restore.Spec.FromPath == nil {
		return &ValidationError{
			Field:   "spec",
			Message: "either backupRef or fromPath must be specified",
		}
	}

	// Validate safety confirmation for in-place restore
	if restore.Spec.Target.InPlace {
		if restore.Spec.Confirmation == nil ||
			restore.Spec.Confirmation.AcknowledgeDataLoss != dbopsv1alpha1.RestoreConfirmDataLoss {
			return &ValidationError{
				Field:   "spec.confirmation.acknowledgeDataLoss",
				Message: fmt.Sprintf("in-place restore requires confirmation.acknowledgeDataLoss to be set to '%s'", dbopsv1alpha1.RestoreConfirmDataLoss),
			}
		}
	}

	// Validate target
	if !restore.Spec.Target.InPlace && restore.Spec.Target.InstanceRef == nil {
		if restore.Spec.Target.DatabaseRef == nil {
			return &ValidationError{
				Field:   "spec.target",
				Message: "either target.instanceRef or target.databaseRef (with inPlace: true) must be specified",
			}
		}
	}

	// If using BackupRef, validate backup exists and is completed
	if restore.Spec.BackupRef != nil {
		backup, err := h.repo.GetBackup(ctx, restore.Namespace, restore.Spec.BackupRef)
		if err != nil {
			if errors.IsNotFound(err) {
				return &ValidationError{
					Field:   "spec.backupRef",
					Message: fmt.Sprintf("referenced backup %s not found", restore.Spec.BackupRef.Name),
				}
			}
			return fmt.Errorf("get backup: %w", err)
		}

		if backup.Status.Phase != dbopsv1alpha1.PhaseCompleted {
			return &ValidationError{
				Field:   "spec.backupRef",
				Message: fmt.Sprintf("backup %s is not completed (phase: %s)", backup.Name, backup.Status.Phase),
			}
		}
	}

	return nil
}

// CheckDeadline checks if the restore has exceeded its deadline.
func (h *Handler) CheckDeadline(restore *dbopsv1alpha1.DatabaseRestore) (bool, string) {
	if restore.Status.StartedAt == nil || restore.Spec.ActiveDeadlineSeconds <= 0 {
		return false, ""
	}

	deadline := restore.Status.StartedAt.Add(time.Duration(restore.Spec.ActiveDeadlineSeconds) * time.Second)
	if time.Now().After(deadline) {
		return true, "Restore exceeded active deadline"
	}

	return false, ""
}

// IsTerminal checks if the restore is in a terminal state.
func (h *Handler) IsTerminal(restore *dbopsv1alpha1.DatabaseRestore) bool {
	return restore.Status.Phase == dbopsv1alpha1.PhaseCompleted ||
		restore.Status.Phase == dbopsv1alpha1.PhaseFailed
}

// OnBackupCompleted handles the BackupCompleted event.
// This could trigger pending restores that were waiting for this backup.
func (h *Handler) OnBackupCompleted(ctx context.Context, event *eventbus.BackupCompleted) error {
	logf.FromContext(ctx).V(1).Info("Backup completed, pending restores may proceed",
		"backup", event.BackupName,
		"namespace", event.Namespace,
		"storagePath", event.StoragePath)
	// In a full implementation, this would query for DatabaseRestore resources
	// that reference this backup and trigger reconciliation
	return nil
}

// OnInstanceDisconnected handles the InstanceDisconnected event.
// This pauses restore operations for the affected instance.
func (h *Handler) OnInstanceDisconnected(ctx context.Context, event *eventbus.InstanceDisconnected) error {
	logf.FromContext(ctx).V(1).Info("Instance disconnected, pausing restore operations",
		"instance", event.InstanceName,
		"namespace", event.Namespace,
		"reason", event.Reason)
	h.paused = true
	return nil
}

// OnInstanceConnected handles the InstanceConnected event.
// This resumes restore operations for the affected instance.
func (h *Handler) OnInstanceConnected(ctx context.Context, event *eventbus.InstanceConnected) error {
	logf.FromContext(ctx).V(1).Info("Instance connected, resuming restore operations",
		"instance", event.InstanceName,
		"namespace", event.Namespace)
	h.paused = false
	return nil
}

// resolveTarget resolves the target instance and database for the restore.
func (h *Handler) resolveTarget(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (*dbopsv1alpha1.DatabaseInstance, string, error) {
	var targetInstance *dbopsv1alpha1.DatabaseInstance
	var targetDatabaseName string

	if restore.Spec.Target.InPlace && restore.Spec.Target.DatabaseRef != nil {
		// In-place restore: get instance from the target database's instanceRef
		targetDB, err := h.repo.GetDatabase(ctx, restore.Namespace, restore.Spec.Target.DatabaseRef)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, "", fmt.Errorf("target database %s not found", restore.Spec.Target.DatabaseRef.Name)
			}
			return nil, "", fmt.Errorf("get target database: %w", err)
		}

		targetDatabaseName = targetDB.Spec.Name

		// Get the instance from the target database
		targetInstance, err = h.repo.GetInstance(ctx, targetDB.Namespace, &targetDB.Spec.InstanceRef)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, "", fmt.Errorf("instance %s for database %s not found", targetDB.Spec.InstanceRef.Name, targetDB.Name)
			}
			return nil, "", fmt.Errorf("get instance: %w", err)
		}
	} else if restore.Spec.Target.InstanceRef != nil {
		// Restore to new database on specified instance
		var err error
		targetInstance, err = h.repo.GetInstance(ctx, restore.Namespace, restore.Spec.Target.InstanceRef)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, "", fmt.Errorf("target instance %s not found", restore.Spec.Target.InstanceRef.Name)
			}
			return nil, "", fmt.Errorf("get instance: %w", err)
		}

		targetDatabaseName = restore.Spec.Target.DatabaseName
	} else {
		return nil, "", fmt.Errorf("either target.instanceRef or target.databaseRef (with inPlace: true) must be specified")
	}

	return targetInstance, targetDatabaseName, nil
}

// resolveSource resolves the backup source for the restore.
func (h *Handler) resolveSource(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (
	*dbopsv1alpha1.DatabaseBackup,
	string,
	*dbopsv1alpha1.StorageConfig,
	*dbopsv1alpha1.CompressionConfig,
	*dbopsv1alpha1.EncryptionConfig,
	error,
) {
	var sourceBackup *dbopsv1alpha1.DatabaseBackup
	var storageConfig *dbopsv1alpha1.StorageConfig
	var compressionConfig *dbopsv1alpha1.CompressionConfig
	var encryptionConfig *dbopsv1alpha1.EncryptionConfig
	var backupPath string

	if restore.Spec.BackupRef != nil {
		var err error
		sourceBackup, err = h.repo.GetBackup(ctx, restore.Namespace, restore.Spec.BackupRef)
		if err != nil {
			return nil, "", nil, nil, nil, fmt.Errorf("get backup: %w", err)
		}

		storageConfig = &sourceBackup.Spec.Storage
		compressionConfig = sourceBackup.Spec.Compression
		encryptionConfig = sourceBackup.Spec.Encryption
		if sourceBackup.Status.Backup != nil {
			backupPath = sourceBackup.Status.Backup.Path
		}

		// If target database name not specified, use source database name
		if restore.Spec.Target.DatabaseName == "" && sourceBackup.Status.Source != nil {
			// This will be handled in resolveTarget
		}
	} else if restore.Spec.FromPath != nil {
		storageConfig = &restore.Spec.FromPath.Storage
		compressionConfig = restore.Spec.FromPath.Compression
		encryptionConfig = restore.Spec.FromPath.Encryption
		backupPath = restore.Spec.FromPath.BackupPath
	}

	return sourceBackup, backupPath, storageConfig, compressionConfig, encryptionConfig, nil
}

// buildRestoreOptions builds restore options from the spec.
func (h *Handler) buildRestoreOptions(restore *dbopsv1alpha1.DatabaseRestore, sourceBackup *dbopsv1alpha1.DatabaseBackup, targetDatabase string, engine dbopsv1alpha1.EngineType, reader io.Reader) adapterpkg.RestoreOptions {
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

// publishRestoreStarted publishes a RestoreStarted event.
func (h *Handler) publishRestoreStarted(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, backupName, databaseName string) {
	if h.eventBus == nil {
		return
	}
	h.eventBus.PublishAsync(ctx, eventbus.NewRestoreStarted(
		restore.Name,
		backupName,
		databaseName,
		restore.Namespace,
	))
}

// publishRestoreCompleted publishes a RestoreCompleted event.
func (h *Handler) publishRestoreCompleted(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, backupName, databaseName string, duration time.Duration) {
	if h.eventBus == nil {
		return
	}
	h.eventBus.PublishAsync(ctx, eventbus.NewRestoreCompleted(
		restore.Name,
		backupName,
		databaseName,
		restore.Namespace,
		duration,
	))
}

// publishRestoreFailed publishes a RestoreFailed event.
func (h *Handler) publishRestoreFailed(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, backupName, databaseName, errMsg string) {
	if h.eventBus == nil {
		return
	}
	h.eventBus.PublishAsync(ctx, eventbus.NewRestoreFailed(
		restore.Name,
		backupName,
		databaseName,
		restore.Namespace,
		errMsg,
	))
}

// UpdateInfoMetric updates the info metric for a restore.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(restore *dbopsv1alpha1.DatabaseRestore) {
	phase := string(restore.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	backupRef := ""
	if restore.Spec.BackupRef != nil {
		backupRef = restore.Spec.BackupRef.Name
	}

	targetInstance := ""
	if restore.Spec.Target.InstanceRef != nil {
		targetInstance = restore.Spec.Target.InstanceRef.Name
	}

	metrics.SetRestoreInfo(
		restore.Name,
		restore.Namespace,
		backupRef,
		targetInstance,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted restore.
func (h *Handler) CleanupInfoMetric(restore *dbopsv1alpha1.DatabaseRestore) {
	metrics.DeleteRestoreInfo(restore.Name, restore.Namespace)
}

// GetBackup retrieves the referenced DatabaseBackup.
// This method is a wrapper around the repository method for use by the controller.
func (h *Handler) GetBackup(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
	return h.repo.GetBackup(ctx, namespace, backupRef)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
