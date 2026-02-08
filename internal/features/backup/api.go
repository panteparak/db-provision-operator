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

// Package backup provides the DatabaseBackup feature module for managing database backups.
package backup

import (
	"context"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// API defines the public interface for the backup module.
type API interface {
	// Execute performs a backup operation.
	Execute(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*Result, error)

	// Delete removes a backup from storage.
	Delete(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error

	// GetStatus retrieves the current status of a backup.
	GetStatus(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*Status, error)

	// IsExpired checks if a backup has expired based on its TTL.
	IsExpired(backup *dbopsv1alpha1.DatabaseBackup) bool
}

// Result represents the result of a backup operation.
type Result struct {
	// Success indicates whether the backup was successful.
	Success bool

	// Path is the storage path where the backup is stored.
	Path string

	// SizeBytes is the uncompressed size of the backup.
	SizeBytes int64

	// CompressedSizeBytes is the compressed size of the backup (if compression enabled).
	CompressedSizeBytes int64

	// Checksum is the checksum of the backup file.
	Checksum string

	// Format is the format of the backup (e.g., custom, plain, tar).
	Format string

	// Duration is how long the backup took.
	Duration time.Duration

	// Message contains additional information about the backup.
	Message string
}

// Status represents the current status of a backup.
type Status struct {
	// Phase is the current phase of the backup.
	Phase dbopsv1alpha1.Phase

	// Message contains status information.
	Message string

	// Progress is the backup progress (0-100).
	Progress int

	// StartedAt is when the backup started.
	StartedAt *time.Time

	// CompletedAt is when the backup completed.
	CompletedAt *time.Time
}

// RepositoryInterface defines the interface for backup repository operations.
// This interface enables dependency injection and testing with mocks.
type RepositoryInterface interface {
	// ExecuteBackup performs the actual backup operation.
	ExecuteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error)

	// DeleteBackup removes a backup file from storage.
	DeleteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error

	// GetDatabase retrieves the Database referenced by the backup.
	GetDatabase(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error)

	// GetInstance retrieves the DatabaseInstance for a database.
	GetInstance(ctx context.Context, database *dbopsv1alpha1.Database) (*dbopsv1alpha1.DatabaseInstance, error)

	// GetEngine returns the database engine type for a backup.
	GetEngine(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
