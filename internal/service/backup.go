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

package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/adapter/types"
)

// BackupService handles database backup operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type BackupService struct {
	baseService
	adapter adapter.DatabaseAdapter
	config  *Config
}

// NewBackupService creates a new BackupService with the given configuration.
func NewBackupService(cfg *Config) (*BackupService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &BackupService{
		baseService: newBaseService(cfg, "BackupService"),
		adapter:     dbAdapter,
		config:      cfg,
	}, nil
}

// NewBackupServiceWithAdapter creates a BackupService with a pre-created adapter.
func NewBackupServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *BackupService {
	return &BackupService{
		baseService: newBaseService(cfg, "BackupService"),
		adapter:     adp,
		config:      cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *BackupService) Connect(ctx context.Context) error {
	op := s.startOp("Connect", s.config.Host)

	ctx, cancel := s.config.Timeouts.WithConnectTimeout(ctx)
	defer cancel()

	if err := s.adapter.Connect(ctx); err != nil {
		op.Error(err, "failed to connect")
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError("connect", s.config.Host, s.config.Timeouts.ConnectTimeout.String(), err)
		}
		return NewConnectionError(s.config.Host, s.config.Port, err)
	}

	op.Success("connected successfully")
	return nil
}

// Close closes the database connection.
func (s *BackupService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// BackupOptions contains options for performing a backup.
type BackupOptions struct {
	// Database name to backup
	Database string
	// BackupID is a unique identifier for this backup
	BackupID string
	// Writer to write backup data to (for CLI local file usage)
	Writer io.Writer
	// Spec contains engine-specific backup options from CRD
	Spec *dbopsv1alpha1.DatabaseBackupSpec
}

// BackupResult contains the result of a backup operation.
type BackupResult struct {
	// Success indicates if the backup completed successfully
	Success bool
	// Message contains a human-readable description
	Message string
	// Path is the backup file path (if using storage backend)
	Path string
	// SizeBytes is the uncompressed size of the backup
	SizeBytes int64
	// CompressedSizeBytes is the compressed size (if compression enabled)
	CompressedSizeBytes int64
	// Checksum is the backup checksum
	Checksum string
	// Format is the backup format (e.g., "custom", "plain", "tar")
	Format string
	// Duration is how long the backup took
	Duration time.Duration
}

// Backup performs a database backup.
// For CLI usage, provide a Writer in options to write backup to a local file.
// For controller usage, the storage configuration is handled separately.
func (s *BackupService) Backup(ctx context.Context, opts BackupOptions) (*BackupResult, error) {
	if opts.Database == "" {
		return nil, &ValidationError{Field: "database", Message: "database name is required"}
	}
	if opts.Writer == nil {
		return nil, &ValidationError{Field: "writer", Message: "writer is required for backup output"}
	}

	op := s.startOp("Backup", opts.Database)
	op.Debug("starting backup", "backupID", opts.BackupID)

	startTime := time.Now()

	// Build adapter backup options
	adapterOpts := s.buildBackupOptions(opts)
	op.Debug("backup options built", "format", adapterOpts.Format, "method", adapterOpts.Method)

	// Execute backup
	result, err := s.adapter.Backup(ctx, adapterOpts)
	if err != nil {
		op.Error(err, "backup failed")
		return nil, s.wrapError(ctx, s.config, "backup", opts.Database, err)
	}

	duration := time.Since(startTime)

	op.Success("backup completed successfully")
	return &BackupResult{
		Success:             true,
		Message:             fmt.Sprintf("Backup of database '%s' completed successfully", opts.Database),
		Path:                result.Path,
		SizeBytes:           result.SizeBytes,
		CompressedSizeBytes: result.CompressedSizeBytes,
		Checksum:            result.Checksum,
		Format:              result.Format,
		Duration:            duration,
	}, nil
}

// BackupToFile performs a backup and writes it to a file.
// This is a convenience method for CLI usage.
func (s *BackupService) BackupToFile(ctx context.Context, database, filePath string, spec *dbopsv1alpha1.DatabaseBackupSpec) (*BackupResult, error) {
	op := s.startOp("BackupToFile", database)
	op.Debug("backing up to file", "path", filePath)

	// Create output file
	file, err := os.Create(filePath)
	if err != nil {
		op.Error(err, "failed to create output file")
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Perform backup
	result, err := s.Backup(ctx, BackupOptions{
		Database: database,
		BackupID: fmt.Sprintf("cli-%d", time.Now().Unix()),
		Writer:   file,
		Spec:     spec,
	})
	if err != nil {
		return nil, err
	}

	// Update path to local file path
	result.Path = filePath

	op.Success("backup to file completed")
	return result, nil
}

// GetBackupExtension returns the appropriate file extension based on engine and backup config.
func (s *BackupService) GetBackupExtension(spec *dbopsv1alpha1.DatabaseBackupSpec) string {
	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec != nil && spec.Postgres != nil {
			switch spec.Postgres.Format {
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

// buildBackupOptions builds adapter.BackupOptions from the service options.
// This logic was extracted from databasebackup_controller.go
func (s *BackupService) buildBackupOptions(opts BackupOptions) types.BackupOptions {
	adapterOpts := types.BackupOptions{
		Database: opts.Database,
		BackupID: opts.BackupID,
		Writer:   opts.Writer,
	}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if opts.Spec != nil && opts.Spec.Postgres != nil {
			pg := opts.Spec.Postgres
			adapterOpts.Method = string(pg.Method)
			adapterOpts.Format = string(pg.Format)
			adapterOpts.Jobs = pg.Jobs
			adapterOpts.DataOnly = pg.DataOnly
			adapterOpts.SchemaOnly = pg.SchemaOnly
			adapterOpts.Blobs = pg.Blobs
			adapterOpts.NoOwner = pg.NoOwner
			adapterOpts.NoPrivileges = pg.NoPrivileges
			adapterOpts.Schemas = pg.Schemas
			adapterOpts.ExcludeSchemas = pg.ExcludeSchemas
			adapterOpts.Tables = pg.Tables
			adapterOpts.ExcludeTables = pg.ExcludeTables
			adapterOpts.LockWaitTimeout = pg.LockWaitTimeout
		} else {
			// Set defaults for PostgreSQL
			adapterOpts.Method = "pg_dump"
			adapterOpts.Format = "custom"
			adapterOpts.Jobs = 1
			adapterOpts.Blobs = true
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if opts.Spec != nil && opts.Spec.MySQL != nil {
			mysql := opts.Spec.MySQL
			adapterOpts.SingleTransaction = mysql.SingleTransaction
			adapterOpts.Quick = mysql.Quick
			adapterOpts.LockTables = mysql.LockTables
			adapterOpts.Routines = mysql.Routines
			adapterOpts.Triggers = mysql.Triggers
			adapterOpts.Events = mysql.Events
			adapterOpts.ExtendedInsert = mysql.ExtendedInsert
			adapterOpts.SetGtidPurged = string(mysql.SetGtidPurged)
		} else {
			// Set defaults for MySQL
			adapterOpts.SingleTransaction = true
			adapterOpts.Quick = true
			adapterOpts.Routines = true
			adapterOpts.Triggers = true
		}
	}

	return adapterOpts
}
