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
	adapter adapter.DatabaseAdapter
	config  *Config
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
		adapter: dbAdapter,
		config:  cfg,
	}, nil
}

// NewDatabaseServiceWithAdapter creates a DatabaseService with a pre-created adapter.
// This is useful for testing or when the adapter is already available.
func NewDatabaseServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *DatabaseService {
	return &DatabaseService{
		adapter: adp,
		config:  cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *DatabaseService) Connect(ctx context.Context) error {
	ctx, cancel := s.config.Timeouts.WithConnectTimeout(ctx)
	defer cancel()

	if err := s.adapter.Connect(ctx); err != nil {
		// Wrap timeout errors with more context
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError("connect", s.config.Host, s.config.Timeouts.ConnectTimeout.String(), err)
		}
		return NewConnectionError(s.config.Host, s.config.Port, err)
	}
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

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database already exists
	exists, err := s.adapter.DatabaseExists(ctx, spec.Name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", spec.Name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", spec.Name, err)
	}
	if exists {
		return NewExistsResult(fmt.Sprintf("Database '%s' already exists", spec.Name)), nil
	}

	// Build create options based on engine type
	opts := s.buildCreateOptions(spec)

	// Create the database
	if err := s.adapter.CreateDatabase(ctx, opts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("create", spec.Name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("create", spec.Name, err)
	}

	// Verify database is accepting connections
	if err := s.adapter.VerifyDatabaseAccess(ctx, spec.Name); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("verify access", spec.Name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		// Database was created but not yet ready - this is a transient state
		return nil, &DatabaseError{
			Operation: "verify access",
			Resource:  spec.Name,
			Err:       fmt.Errorf("database created but not accepting connections: %w", err),
		}
	}

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

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database already exists
	exists, err := s.adapter.DatabaseExists(ctx, spec.Name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", spec.Name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", spec.Name, err)
	}
	if exists {
		return NewExistsResult(fmt.Sprintf("Database '%s' already exists", spec.Name)), nil
	}

	// Build create options based on engine type
	opts := s.buildCreateOptions(spec)

	// Create the database
	if err := s.adapter.CreateDatabase(ctx, opts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("create", spec.Name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("create", spec.Name, err)
	}

	return NewCreatedResult(fmt.Sprintf("Database '%s' created successfully", spec.Name)), nil
}

// Get retrieves information about a database.
func (s *DatabaseService) Get(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}

	// Apply query timeout for read operations
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Check if database exists
	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", name, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", name, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Get database info
	info, err := s.adapter.GetDatabaseInfo(ctx, name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("get info", name, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return nil, NewDatabaseError("get info", name, err)
	}

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

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database exists
	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", name, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Build update options
	opts := s.buildUpdateOptions(spec)

	// Update the database
	if err := s.adapter.UpdateDatabase(ctx, name, opts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("update", name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("update", name, err)
	}

	return NewUpdatedResult(fmt.Sprintf("Database '%s' updated successfully", name)), nil
}

// Delete drops a database.
func (s *DatabaseService) Delete(ctx context.Context, name string, force bool) (*Result, error) {
	if name == "" {
		return nil, &ValidationError{Field: "name", Message: "database name is required"}
	}

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if database exists
	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", name, err)
	}
	if !exists {
		return NewSuccessResult(fmt.Sprintf("Database '%s' does not exist", name)), nil
	}

	// Drop the database
	opts := types.DropDatabaseOptions{
		Force: force,
	}
	if err := s.adapter.DropDatabase(ctx, name, opts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("delete", name, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("delete", name, err)
	}

	return NewSuccessResult(fmt.Sprintf("Database '%s' deleted successfully", name)), nil
}

// Exists checks if a database exists.
func (s *DatabaseService) Exists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, &ValidationError{Field: "name", Message: "database name is required"}
	}

	// Apply query timeout for read operations
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	exists, err := s.adapter.DatabaseExists(ctx, name)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, NewTimeoutError("check existence", name, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return false, NewDatabaseError("check existence", name, err)
	}

	return exists, nil
}

// VerifyAccess verifies that a database is accepting connections.
func (s *DatabaseService) VerifyAccess(ctx context.Context, name string) error {
	if name == "" {
		return &ValidationError{Field: "name", Message: "database name is required"}
	}

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	if err := s.adapter.VerifyDatabaseAccess(ctx, name); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError("verify access", name, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return NewDatabaseError("verify access", name, err)
	}

	return nil
}

// buildCreateOptions builds adapter.CreateDatabaseOptions from the spec.
// This logic was extracted from database_controller.go
func (s *DatabaseService) buildCreateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.CreateDatabaseOptions {
	opts := types.CreateDatabaseOptions{
		Name: spec.Name,
	}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec.Postgres != nil {
			opts.Encoding = spec.Postgres.Encoding
			opts.LCCollate = spec.Postgres.LCCollate
			opts.LCCtype = spec.Postgres.LCCtype
			opts.Tablespace = spec.Postgres.Tablespace
			opts.Template = spec.Postgres.Template
			opts.ConnectionLimit = spec.Postgres.ConnectionLimit
			opts.IsTemplate = spec.Postgres.IsTemplate
			opts.AllowConnections = spec.Postgres.AllowConnections
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if spec.MySQL != nil {
			opts.Charset = spec.MySQL.Charset
			opts.Collation = spec.MySQL.Collation
		}
	}

	return opts
}

// buildUpdateOptions builds adapter.UpdateDatabaseOptions from the spec.
// This logic was extracted from database_controller.go
func (s *DatabaseService) buildUpdateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.UpdateDatabaseOptions {
	opts := types.UpdateDatabaseOptions{}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec.Postgres != nil {
			// Extensions
			for _, ext := range spec.Postgres.Extensions {
				opts.Extensions = append(opts.Extensions, types.ExtensionOptions{
					Name:    ext.Name,
					Schema:  ext.Schema,
					Version: ext.Version,
				})
			}
			// Schemas
			for _, schema := range spec.Postgres.Schemas {
				opts.Schemas = append(opts.Schemas, types.SchemaOptions{
					Name:  schema.Name,
					Owner: schema.Owner,
				})
			}
			// Default privileges
			for _, dp := range spec.Postgres.DefaultPrivileges {
				opts.DefaultPrivileges = append(opts.DefaultPrivileges, types.DefaultPrivilegeOptions{
					Role:       dp.Role,
					Schema:     dp.Schema,
					ObjectType: dp.ObjectType,
					Privileges: dp.Privileges,
				})
			}
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if spec.MySQL != nil {
			opts.Charset = spec.MySQL.Charset
			opts.Collation = spec.MySQL.Collation
		}
	}

	return opts
}
