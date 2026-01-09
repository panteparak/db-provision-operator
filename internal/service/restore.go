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

// RestoreService handles database restore operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type RestoreService struct {
	adapter adapter.DatabaseAdapter
	config  *Config
}

// NewRestoreService creates a new RestoreService with the given configuration.
func NewRestoreService(cfg *Config) (*RestoreService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &RestoreService{
		adapter: dbAdapter,
		config:  cfg,
	}, nil
}

// NewRestoreServiceWithAdapter creates a RestoreService with a pre-created adapter.
func NewRestoreServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *RestoreService {
	return &RestoreService{
		adapter: adp,
		config:  cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *RestoreService) Connect(ctx context.Context) error {
	if err := s.adapter.Connect(ctx); err != nil {
		return NewConnectionError(s.config.Host, s.config.Port, err)
	}
	return nil
}

// Close closes the database connection.
func (s *RestoreService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// RestoreOptions contains options for performing a restore.
type RestoreOptions struct {
	// Database name to restore to
	Database string
	// RestoreID is a unique identifier for this restore operation
	RestoreID string
	// Reader to read backup data from (for CLI local file usage)
	Reader io.Reader
	// DropExisting drops the existing database before restore
	DropExisting bool
	// CreateDatabase creates the database if it doesn't exist
	CreateDatabase bool
	// Spec contains engine-specific restore options from CRD
	Spec *dbopsv1alpha1.DatabaseRestoreSpec
}

// RestoreResult contains the result of a restore operation.
type RestoreResult struct {
	// Success indicates if the restore completed successfully
	Success bool
	// Message contains a human-readable description
	Message string
	// TargetDatabase is the name of the database restored to
	TargetDatabase string
	// TablesRestored is the number of tables restored
	TablesRestored int32
	// Warnings contains any warnings from the restore operation
	Warnings []string
	// Duration is how long the restore took
	Duration time.Duration
}

// Restore performs a database restore.
// For CLI usage, provide a Reader in options to read backup from a local file.
// For controller usage, the storage configuration is handled separately.
func (s *RestoreService) Restore(ctx context.Context, opts RestoreOptions) (*RestoreResult, error) {
	if opts.Database == "" {
		return nil, &ValidationError{Field: "database", Message: "database name is required"}
	}
	if opts.Reader == nil {
		return nil, &ValidationError{Field: "reader", Message: "reader is required for backup input"}
	}

	startTime := time.Now()

	// Build adapter restore options
	adapterOpts := s.buildRestoreOptions(opts)

	// Execute restore
	result, err := s.adapter.Restore(ctx, adapterOpts)
	if err != nil {
		return nil, NewDatabaseError("restore", opts.Database, err)
	}

	duration := time.Since(startTime)

	return &RestoreResult{
		Success:        true,
		Message:        fmt.Sprintf("Restore to database '%s' completed successfully", opts.Database),
		TargetDatabase: result.TargetDatabase,
		TablesRestored: result.TablesRestored,
		Warnings:       result.Warnings,
		Duration:       duration,
	}, nil
}

// RestoreFromFile performs a restore from a local file.
// This is a convenience method for CLI usage.
func (s *RestoreService) RestoreFromFile(ctx context.Context, database, filePath string, dropExisting bool, spec *dbopsv1alpha1.DatabaseRestoreSpec) (*RestoreResult, error) {
	// Open input file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	// Perform restore
	return s.Restore(ctx, RestoreOptions{
		Database:       database,
		RestoreID:      fmt.Sprintf("cli-%d", time.Now().Unix()),
		Reader:         file,
		DropExisting:   dropExisting,
		CreateDatabase: true,
		Spec:           spec,
	})
}

// buildRestoreOptions builds adapter.RestoreOptions from the service options.
// This logic was extracted from databaserestore_controller.go
func (s *RestoreService) buildRestoreOptions(opts RestoreOptions) types.RestoreOptions {
	adapterOpts := types.RestoreOptions{
		Database:       opts.Database,
		RestoreID:      opts.RestoreID,
		Reader:         opts.Reader,
		DropExisting:   opts.DropExisting,
		CreateDatabase: opts.CreateDatabase,
	}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if opts.Spec != nil && opts.Spec.Postgres != nil {
			pg := opts.Spec.Postgres
			adapterOpts.DropExisting = pg.DropExisting || opts.DropExisting
			adapterOpts.CreateDatabase = pg.CreateDatabase
			adapterOpts.DataOnly = pg.DataOnly
			adapterOpts.SchemaOnly = pg.SchemaOnly
			adapterOpts.NoOwner = pg.NoOwner
			adapterOpts.NoPrivileges = pg.NoPrivileges
			adapterOpts.RoleMapping = pg.RoleMapping
			adapterOpts.Schemas = pg.Schemas
			adapterOpts.Tables = pg.Tables
			adapterOpts.Jobs = pg.Jobs
			adapterOpts.DisableTriggers = pg.DisableTriggers
			adapterOpts.Analyze = pg.Analyze
		} else {
			// Set defaults for PostgreSQL
			adapterOpts.CreateDatabase = true
			adapterOpts.NoOwner = true
			adapterOpts.Jobs = 1
			adapterOpts.Analyze = true
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if opts.Spec != nil && opts.Spec.MySQL != nil {
			mysql := opts.Spec.MySQL
			adapterOpts.DropExisting = mysql.DropExisting || opts.DropExisting
			adapterOpts.CreateDatabase = mysql.CreateDatabase
			adapterOpts.Routines = mysql.Routines
			adapterOpts.Triggers = mysql.Triggers
			adapterOpts.Events = mysql.Events
			adapterOpts.DisableForeignKeyChecks = mysql.DisableForeignKeyChecks
			adapterOpts.DisableBinlog = mysql.DisableBinlog
		} else {
			// Set defaults for MySQL
			adapterOpts.CreateDatabase = true
			adapterOpts.Routines = true
			adapterOpts.Triggers = true
			adapterOpts.Events = true
			adapterOpts.DisableForeignKeyChecks = true
			adapterOpts.DisableBinlog = true
		}
	}

	return adapterOpts
}
