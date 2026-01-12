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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/adapter/types"
)

// DatabaseService handles database operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type DatabaseService struct {
	baseService
	adapter     adapter.DatabaseAdapter
	config      *Config
	specBuilder SpecBuilder
}

// NewDatabaseService creates a new DatabaseService with the given configuration.
// It creates the appropriate database adapter based on the engine type.
func NewDatabaseService(cfg *Config) (*DatabaseService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &DatabaseService{
		baseService: newBaseService(cfg, "DatabaseService"),
		adapter:     dbAdapter,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}, nil
}

// NewDatabaseServiceWithAdapter creates a DatabaseService with a pre-created adapter.
// This is useful for testing or when the adapter is already available.
func NewDatabaseServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *DatabaseService {
	return &DatabaseService{
		baseService: newBaseService(cfg, "DatabaseService"),
		adapter:     adp,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}
}

// Connect establishes a connection to the database server.
func (s *DatabaseService) Connect(ctx context.Context) error {
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
func (s *DatabaseService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// Create creates a new database with the given spec.
// If the database already exists, it returns success without error.
// This method includes verification that the database accepts connections.
// For controllers that need more granular control, use CreateOnly + VerifyAccess.
func (s *DatabaseService) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec) (*Result, error) {
	if spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}
	if spec.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}

	op := s.startOp("Create", spec.Name)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database already exists
	exists, err := s.adapter.DatabaseExists(ctx, spec.Name)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", spec.Name, err)
	}
	if exists {
		op.Success("database already exists")
		return NewExistsResult(fmt.Sprintf("Database '%s' already exists", spec.Name)), nil
	}

	// Build create options using SpecBuilder
	opts := s.specBuilder.BuildDatabaseCreateOptions(spec)
	op.Debug("creating database", "encoding", opts.Encoding, "owner", opts.Owner)

	// Create the database
	if err := s.adapter.CreateDatabase(ctx, opts); err != nil {
		op.Error(err, "failed to create database")
		return nil, s.wrapError(ctx, s.config, "create", spec.Name, err)
	}

	// Verify database is accepting connections
	if err := s.adapter.VerifyDatabaseAccess(ctx, spec.Name); err != nil {
		op.Error(err, "database not accepting connections")
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("verify access", spec.Name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, &DatabaseError{
			Operation: "verify access",
			Resource:  spec.Name,
			Err:       fmt.Errorf("database created but not accepting connections: %w", err),
		}
	}

	op.Success("database created successfully")
	return NewCreatedResult(fmt.Sprintf("Database '%s' created successfully", spec.Name)), nil
}

// CreateOnly creates a new database without verifying connections.
// This is useful for controllers that want to handle the verification step separately
// to provide more granular status updates.
// If the database already exists, it returns success without error.
func (s *DatabaseService) CreateOnly(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec) (*Result, error) {
	if spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}
	if spec.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}

	op := s.startOp("CreateOnly", spec.Name)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database already exists
	exists, err := s.adapter.DatabaseExists(ctx, spec.Name)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", spec.Name, err)
	}
	if exists {
		op.Success("database already exists")
		return NewExistsResult(fmt.Sprintf("Database '%s' already exists", spec.Name)), nil
	}

	// Build create options using SpecBuilder
	opts := s.specBuilder.BuildDatabaseCreateOptions(spec)
	op.Debug("creating database (no verify)", "encoding", opts.Encoding)

	// Create the database
	if err := s.adapter.CreateDatabase(ctx, opts); err != nil {
		op.Error(err, "failed to create database")
		return nil, s.wrapError(ctx, s.config, "create", spec.Name, err)
	}

	op.Success("database created (verification pending)")
	return NewCreatedResult(fmt.Sprintf("Database '%s' created successfully", spec.Name)), nil
}

// Get retrieves information about a database.
func (s *DatabaseService) Get(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}

	op := s.startOp("Get", name)

	// Apply query timeout for read operations
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Check if database exists
	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", name, err)
	}
	if !exists {
		op.Debug("database not found")
		return nil, ErrNotFound
	}

	// Get database info
	info, err := s.adapter.GetDatabaseInfo(ctx, name)
	if err != nil {
		op.Error(err, "failed to get info")
		return nil, s.wrapError(ctx, s.config, "get info", name, err)
	}

	op.Success("retrieved database info")
	return info, nil
}

// Update updates database settings (extensions, schemas, charset, etc.).
func (s *DatabaseService) Update(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec) (*Result, error) {
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}
	if spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	op := s.startOp("Update", name)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database exists
	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", name, err)
	}
	if !exists {
		op.Debug("database not found")
		return nil, ErrNotFound
	}

	// Build update options using SpecBuilder
	opts := s.specBuilder.BuildDatabaseUpdateOptions(spec)
	op.Debug("updating database", "extensions", len(opts.Extensions), "schemas", len(opts.Schemas))

	// Update the database
	if err := s.adapter.UpdateDatabase(ctx, name, opts); err != nil {
		op.Error(err, "failed to update database")
		return nil, s.wrapError(ctx, s.config, "update", name, err)
	}

	op.Success("database updated successfully")
	return NewUpdatedResult(fmt.Sprintf("Database '%s' updated successfully", name)), nil
}

// Delete drops a database.
func (s *DatabaseService) Delete(ctx context.Context, name string, force bool) (*Result, error) {
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}

	op := s.startOp("Delete", name)
	op.Debug("delete requested", "force", force)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database exists
	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", name, err)
	}
	if !exists {
		op.Success("database does not exist (no-op)")
		return NewSuccessResult(fmt.Sprintf("Database '%s' does not exist", name)), nil
	}

	// Drop the database
	opts := types.DropDatabaseOptions{
		Force: force,
	}
	if err := s.adapter.DropDatabase(ctx, name, opts); err != nil {
		op.Error(err, "failed to drop database")
		return nil, s.wrapError(ctx, s.config, "delete", name, err)
	}

	op.Success("database deleted successfully")
	return NewSuccessResult(fmt.Sprintf("Database '%s' deleted successfully", name)), nil
}

// Exists checks if a database exists.
func (s *DatabaseService) Exists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, &ValidationError{Field: "name", Message: "database name is required"}
	}

	op := s.startOp("Exists", name)

	// Apply query timeout for read operations
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		op.Error(err, "failed to check existence")
		return false, s.wrapError(ctx, s.config, "check existence", name, err)
	}

	op.Debug("existence check complete", "exists", exists)
	return exists, nil
}

// VerifyAccess verifies that a database is accepting connections.
func (s *DatabaseService) VerifyAccess(ctx context.Context, name string) error {
	if name == "" {
		return &ValidationError{Field: "name", Message: "database name is required"}
	}

	op := s.startOp("VerifyAccess", name)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	if err := s.adapter.VerifyDatabaseAccess(ctx, name); err != nil {
		op.Error(err, "database not accepting connections")
		return s.wrapError(ctx, s.config, "verify access", name, err)
	}

	op.Success("database is accepting connections")
	return nil
}
