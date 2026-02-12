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

// Package restore provides the DatabaseRestore feature module for managing database restores.
package restore

import (
	"context"
	"io"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/storage"
)

// API defines the public interface for the restore module.
type API interface {
	// Execute performs a database restore operation.
	Execute(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (*Result, error)

	// ValidateSpec validates the restore specification.
	ValidateSpec(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) error

	// CheckDeadline checks if the restore has exceeded its deadline.
	CheckDeadline(restore *dbopsv1alpha1.DatabaseRestore) (bool, string)

	// IsTerminal checks if the restore is in a terminal state (completed or failed).
	IsTerminal(restore *dbopsv1alpha1.DatabaseRestore) bool
}

// Result represents the result of a restore operation.
type Result struct {
	// TargetDatabase is the name of the restored database.
	TargetDatabase string

	// TargetInstance is the name of the target instance.
	TargetInstance string

	// SourceBackup is the name of the source backup (if using BackupRef).
	SourceBackup string

	// TablesRestored is the number of tables restored.
	TablesRestored int32

	// Duration is the time taken for the restore.
	Duration time.Duration

	// Warnings contains any warnings from the restore process.
	Warnings []string
}

// ValidationError represents a validation error for restore specification.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// RepositoryInterface defines the repository operations for restore.
// This interface allows for mock implementations in tests.
type RepositoryInterface interface {
	// GetBackup retrieves the referenced DatabaseBackup.
	GetBackup(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error)

	// GetDatabase retrieves the referenced Database.
	GetDatabase(ctx context.Context, namespace string, dbRef *dbopsv1alpha1.DatabaseReference) (*dbopsv1alpha1.Database, error)

	// GetInstance retrieves the referenced DatabaseInstance.
	// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
	GetInstance(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error)

	// ResolveInstance resolves the instance reference (supports both instanceRef and clusterInstanceRef).
	ResolveInstance(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error)

	// CreateRestoreReader creates a reader for the backup data.
	CreateRestoreReader(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error)

	// ExecuteRestore performs the actual database restore operation.
	// Deprecated: Use ExecuteRestoreWithResolved instead for cluster instance support.
	ExecuteRestore(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error)

	// ExecuteRestoreWithResolved performs the actual database restore operation using a resolved instance.
	ExecuteRestoreWithResolved(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error)

	// GetEngine returns the database engine type for a given instance.
	// Deprecated: Use GetEngineWithRefs instead for cluster instance support.
	GetEngine(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (string, error)

	// GetEngineWithRefs returns the database engine type, supporting both instanceRef and clusterInstanceRef.
	GetEngineWithRefs(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (string, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
