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
	"fmt"
	"time"

	"github.com/go-logr/logr"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for backup operations.
type Handler struct {
	repo              *Repository
	eventBus          eventbus.Bus
	logger            logr.Logger
	instanceConnected map[string]bool // tracks instance connectivity status
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository *Repository
	EventBus   eventbus.Bus
	Logger     logr.Logger
}

// NewHandler creates a new backup handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:              cfg.Repository,
		eventBus:          cfg.EventBus,
		logger:            cfg.Logger,
		instanceConnected: make(map[string]bool),
	}
}

// Execute performs a backup operation.
func (h *Handler) Execute(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*Result, error) {
	log := h.logger.WithValues("backup", backup.Name, "namespace", backup.Namespace)

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, backup)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// Get storage type for event
	storageType := string(backup.Spec.Storage.Type)

	// Publish BackupStarted event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewBackupStarted(
			backup.Name,
			backup.Spec.DatabaseRef.Name,
			backup.Namespace,
			storageType,
		))
	}

	log.Info("Starting backup execution")
	startTime := time.Now()

	// Execute the backup via repository
	execResult, err := h.repo.ExecuteBackup(ctx, backup)
	if err != nil {
		// Record failure metric
		metrics.RecordBackupOperation(engine, backup.Namespace, metrics.StatusFailure)

		// Publish BackupFailed event
		if h.eventBus != nil {
			h.eventBus.PublishAsync(ctx, eventbus.NewBackupFailed(
				backup.Name,
				backup.Spec.DatabaseRef.Name,
				backup.Namespace,
				err.Error(),
			))
		}

		return nil, fmt.Errorf("execute backup: %w", err)
	}

	duration := time.Since(startTime)

	// Record success metrics
	metrics.RecordBackupOperation(engine, backup.Namespace, metrics.StatusSuccess)
	metrics.RecordBackupDuration(engine, backup.Namespace, duration.Seconds())

	// Use compressed size if available, otherwise use uncompressed size
	backupSize := float64(execResult.SizeBytes)
	if execResult.CompressedSizeBytes > 0 {
		backupSize = float64(execResult.CompressedSizeBytes)
	}
	metrics.SetBackupSize(execResult.Database, engine, backup.Namespace, backupSize)
	metrics.SetBackupLastSuccess(execResult.Database, engine, backup.Namespace, float64(time.Now().Unix()))

	// Publish BackupCompleted event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewBackupCompleted(
			backup.Name,
			backup.Spec.DatabaseRef.Name,
			backup.Namespace,
			execResult.Path,
			execResult.Checksum,
			execResult.SizeBytes,
			duration,
		))
	}

	log.Info("Backup completed successfully",
		"path", execResult.Path,
		"size", execResult.SizeBytes,
		"duration", duration)

	return &Result{
		Success:             true,
		Path:                execResult.Path,
		SizeBytes:           execResult.SizeBytes,
		CompressedSizeBytes: execResult.CompressedSizeBytes,
		Checksum:            execResult.Checksum,
		Format:              execResult.Format,
		Duration:            duration,
		Message:             "Backup completed successfully",
	}, nil
}

// Delete removes a backup from storage.
func (h *Handler) Delete(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
	log := h.logger.WithValues("backup", backup.Name, "namespace", backup.Namespace)

	log.Info("Deleting backup")

	if err := h.repo.DeleteBackup(ctx, backup); err != nil {
		log.Error(err, "Failed to delete backup file")
		// Continue with deletion even if file deletion fails
	} else {
		log.Info("Backup file deleted successfully")
	}

	// Clean up backup metrics
	if backup.Status.Source != nil {
		databaseName := backup.Status.Source.Database
		engine := backup.Status.Source.Engine
		metrics.DeleteBackupMetrics(databaseName, engine, backup.Namespace)
	}

	return nil
}

// GetStatus retrieves the current status of a backup.
func (h *Handler) GetStatus(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*Status, error) {
	status := &Status{
		Phase:   backup.Status.Phase,
		Message: backup.Status.Message,
	}

	if backup.Status.StartedAt != nil {
		t := backup.Status.StartedAt.Time
		status.StartedAt = &t
	}

	if backup.Status.CompletedAt != nil {
		t := backup.Status.CompletedAt.Time
		status.CompletedAt = &t
	}

	return status, nil
}

// IsExpired checks if a backup has expired based on its TTL.
func (h *Handler) IsExpired(backup *dbopsv1alpha1.DatabaseBackup) bool {
	if backup.Status.ExpiresAt == nil {
		return false
	}
	return time.Now().After(backup.Status.ExpiresAt.Time)
}

// OnDatabaseDeleted handles the DatabaseDeleted event.
// This may be used for cleanup or notifications related to backups of deleted databases.
func (h *Handler) OnDatabaseDeleted(ctx context.Context, event *eventbus.DatabaseDeleted) error {
	h.logger.V(1).Info("Database deleted, backups may need attention",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	// In a full implementation, this could:
	// - Mark related backups as orphaned
	// - Trigger backup cleanup based on policy
	// - Send notifications
	return nil
}

// OnInstanceDisconnected handles the InstanceDisconnected event.
// This pauses backup operations for the disconnected instance.
func (h *Handler) OnInstanceDisconnected(ctx context.Context, event *eventbus.InstanceDisconnected) error {
	h.logger.V(1).Info("Instance disconnected, backup operations may fail",
		"instance", event.InstanceName,
		"namespace", event.Namespace,
		"reason", event.Reason)

	// Track instance disconnection
	key := fmt.Sprintf("%s/%s", event.Namespace, event.InstanceName)
	h.instanceConnected[key] = false

	return nil
}

// OnInstanceConnected handles the InstanceConnected event.
// This resumes backup operations for the reconnected instance.
func (h *Handler) OnInstanceConnected(ctx context.Context, event *eventbus.InstanceConnected) error {
	h.logger.V(1).Info("Instance connected, backup operations can resume",
		"instance", event.InstanceName,
		"namespace", event.Namespace)

	// Track instance connection
	key := fmt.Sprintf("%s/%s", event.Namespace, event.InstanceName)
	h.instanceConnected[key] = true

	return nil
}

// IsInstanceConnected checks if an instance is connected.
func (h *Handler) IsInstanceConnected(namespace, instanceName string) bool {
	key := fmt.Sprintf("%s/%s", namespace, instanceName)
	connected, exists := h.instanceConnected[key]
	if !exists {
		// If we haven't received any events, assume connected
		return true
	}
	return connected
}

// UpdateInfoMetric updates the info metric for a backup.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(backup *dbopsv1alpha1.DatabaseBackup) {
	phase := string(backup.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	storageType := string(backup.Spec.Storage.Type)

	metrics.SetBackupInfo(
		backup.Name,
		backup.Namespace,
		backup.Spec.DatabaseRef.Name,
		storageType,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted backup.
func (h *Handler) CleanupInfoMetric(backup *dbopsv1alpha1.DatabaseBackup) {
	metrics.DeleteBackupInfo(backup.Name, backup.Namespace)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
