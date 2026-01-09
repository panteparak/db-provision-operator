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

// UserService handles database user operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type UserService struct {
	adapter adapter.DatabaseAdapter
	config  *Config
}

// NewUserService creates a new UserService with the given configuration.
func NewUserService(cfg *Config) (*UserService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &UserService{
		adapter: dbAdapter,
		config:  cfg,
	}, nil
}

// NewUserServiceWithAdapter creates a UserService with a pre-created adapter.
func NewUserServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *UserService {
	return &UserService{
		adapter: adp,
		config:  cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *UserService) Connect(ctx context.Context) error {
	ctx, cancel := s.config.Timeouts.WithConnectTimeout(ctx)
	defer cancel()

	if err := s.adapter.Connect(ctx); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError("connect", s.config.Host, s.config.Timeouts.ConnectTimeout.String(), err)
		}
		return NewConnectionError(s.config.Host, s.config.Port, err)
	}
	return nil
}

// Close closes the database connection.
func (s *UserService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// CreateOptions contains options for creating a user via the service.
type CreateUserServiceOptions struct {
	Spec     *dbopsv1alpha1.DatabaseUserSpec
	Password string // The password to use (service doesn't generate passwords)
}

// Create creates a new database user with the given spec.
// The password must be provided - password generation is the caller's responsibility.
// If the user already exists, it updates the user instead.
func (s *UserService) Create(ctx context.Context, opts CreateUserServiceOptions) (*Result, error) {
	if opts.Spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}
	if opts.Spec.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if opts.Password == "" {
		return nil, &ValidationError{Field: "password", Message: "password is required"}
	}

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user already exists
	exists, err := s.adapter.UserExists(ctx, opts.Spec.Username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", opts.Spec.Username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", opts.Spec.Username, err)
	}

	if exists {
		// Update existing user
		updateOpts := s.buildUpdateOptions(opts.Spec)
		if err := s.adapter.UpdateUser(ctx, opts.Spec.Username, updateOpts); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, NewTimeoutError("update", opts.Spec.Username, s.config.Timeouts.OperationTimeout.String(), err)
			}
			return nil, NewDatabaseError("update", opts.Spec.Username, err)
		}

		// Also update password if user exists
		if err := s.adapter.UpdatePassword(ctx, opts.Spec.Username, opts.Password); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, NewTimeoutError("update password", opts.Spec.Username, s.config.Timeouts.OperationTimeout.String(), err)
			}
			return nil, NewDatabaseError("update password", opts.Spec.Username, err)
		}

		return NewUpdatedResult(fmt.Sprintf("User '%s' updated successfully", opts.Spec.Username)), nil
	}

	// Build create options
	createOpts := s.buildCreateOptions(opts.Spec, opts.Password)

	// Create the user
	if err := s.adapter.CreateUser(ctx, createOpts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("create", opts.Spec.Username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("create", opts.Spec.Username, err)
	}

	return NewCreatedResult(fmt.Sprintf("User '%s' created successfully", opts.Spec.Username)), nil
}

// Get retrieves information about a user.
func (s *UserService) Get(ctx context.Context, username string) (*types.UserInfo, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", username, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", username, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Get user info
	info, err := s.adapter.GetUserInfo(ctx, username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("get info", username, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return nil, NewDatabaseError("get info", username, err)
	}

	return info, nil
}

// Update updates user settings.
func (s *UserService) Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", username, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Build update options
	opts := s.buildUpdateOptions(spec)

	// Update the user
	if err := s.adapter.UpdateUser(ctx, username, opts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("update", username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("update", username, err)
	}

	return NewUpdatedResult(fmt.Sprintf("User '%s' updated successfully", username)), nil
}

// UpdatePassword updates a user's password.
func (s *UserService) UpdatePassword(ctx context.Context, username, password string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if password == "" {
		return nil, &ValidationError{Field: "password", Message: "password is required"}
	}

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", username, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Update password
	if err := s.adapter.UpdatePassword(ctx, username, password); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("update password", username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("update password", username, err)
	}

	return NewSuccessResult(fmt.Sprintf("Password updated for user '%s'", username)), nil
}

// Delete drops a database user.
func (s *UserService) Delete(ctx context.Context, username string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", username, err)
	}
	if !exists {
		return NewSuccessResult(fmt.Sprintf("User '%s' does not exist", username)), nil
	}

	// Drop the user
	if err := s.adapter.DropUser(ctx, username); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("delete", username, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("delete", username, err)
	}

	return NewSuccessResult(fmt.Sprintf("User '%s' deleted successfully", username)), nil
}

// Exists checks if a user exists.
func (s *UserService) Exists(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return false, &ValidationError{Field: "username", Message: "username is required"}
	}

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, NewTimeoutError("check existence", username, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return false, NewDatabaseError("check existence", username, err)
	}

	return exists, nil
}

// buildCreateOptions builds adapter.CreateUserOptions from the spec.
// This logic was extracted from databaseuser_controller.go
func (s *UserService) buildCreateOptions(spec *dbopsv1alpha1.DatabaseUserSpec, password string) types.CreateUserOptions {
	opts := types.CreateUserOptions{
		Username: spec.Username,
		Password: password,
		Login:    true,  // Default to allowing login
		Inherit:  true,  // Default to inheriting privileges
	}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec.Postgres != nil {
			opts.ConnectionLimit = spec.Postgres.ConnectionLimit
			opts.ValidUntil = spec.Postgres.ValidUntil
			opts.Superuser = spec.Postgres.Superuser
			opts.CreateDB = spec.Postgres.CreateDB
			opts.CreateRole = spec.Postgres.CreateRole
			opts.Inherit = spec.Postgres.Inherit
			opts.Login = spec.Postgres.Login
			opts.Replication = spec.Postgres.Replication
			opts.BypassRLS = spec.Postgres.BypassRLS
			opts.InRoles = spec.Postgres.InRoles
			opts.ConfigParams = spec.Postgres.ConfigParameters
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if spec.MySQL != nil {
			opts.MaxQueriesPerHour = spec.MySQL.MaxQueriesPerHour
			opts.MaxUpdatesPerHour = spec.MySQL.MaxUpdatesPerHour
			opts.MaxConnectionsPerHour = spec.MySQL.MaxConnectionsPerHour
			opts.MaxUserConnections = spec.MySQL.MaxUserConnections
			opts.AuthPlugin = string(spec.MySQL.AuthPlugin)
			opts.RequireSSL = spec.MySQL.RequireSSL
			opts.RequireX509 = spec.MySQL.RequireX509
			opts.AllowedHosts = spec.MySQL.AllowedHosts
			opts.AccountLocked = spec.MySQL.AccountLocked
		}
	}

	return opts
}

// buildUpdateOptions builds adapter.UpdateUserOptions from the spec.
// This logic was extracted from databaseuser_controller.go
func (s *UserService) buildUpdateOptions(spec *dbopsv1alpha1.DatabaseUserSpec) types.UpdateUserOptions {
	opts := types.UpdateUserOptions{}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec.Postgres != nil {
			if spec.Postgres.ConnectionLimit != 0 {
				opts.ConnectionLimit = &spec.Postgres.ConnectionLimit
			}
			if spec.Postgres.ValidUntil != "" {
				opts.ValidUntil = &spec.Postgres.ValidUntil
			}
			opts.InRoles = spec.Postgres.InRoles
			opts.ConfigParams = spec.Postgres.ConfigParameters
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if spec.MySQL != nil {
			if spec.MySQL.MaxQueriesPerHour != 0 {
				opts.MaxQueriesPerHour = &spec.MySQL.MaxQueriesPerHour
			}
			if spec.MySQL.MaxUpdatesPerHour != 0 {
				opts.MaxUpdatesPerHour = &spec.MySQL.MaxUpdatesPerHour
			}
			if spec.MySQL.MaxConnectionsPerHour != 0 {
				opts.MaxConnectionsPerHour = &spec.MySQL.MaxConnectionsPerHour
			}
			if spec.MySQL.MaxUserConnections != 0 {
				opts.MaxUserConnections = &spec.MySQL.MaxUserConnections
			}
		}
	}

	return opts
}
