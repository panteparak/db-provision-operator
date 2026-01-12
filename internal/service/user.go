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
	baseService
	adapter     adapter.DatabaseAdapter
	config      *Config
	specBuilder SpecBuilder
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
		baseService: newBaseService(cfg, "UserService"),
		adapter:     dbAdapter,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}, nil
}

// NewUserServiceWithAdapter creates a UserService with a pre-created adapter.
func NewUserServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *UserService {
	return &UserService{
		baseService: newBaseService(cfg, "UserService"),
		adapter:     adp,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}
}

// Connect establishes a connection to the database server.
func (s *UserService) Connect(ctx context.Context) error {
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

	op := s.startOp("Create", opts.Spec.Username)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user already exists
	exists, err := s.adapter.UserExists(ctx, opts.Spec.Username)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", opts.Spec.Username, err)
	}

	if exists {
		op.Debug("user exists, updating instead")
		// Update existing user using SpecBuilder
		updateOpts := s.specBuilder.BuildUserUpdateOptions(opts.Spec)
		if err := s.adapter.UpdateUser(ctx, opts.Spec.Username, updateOpts); err != nil {
			op.Error(err, "failed to update user")
			return nil, s.wrapError(ctx, s.config, "update", opts.Spec.Username, err)
		}

		// Also update password if user exists
		if err := s.adapter.UpdatePassword(ctx, opts.Spec.Username, opts.Password); err != nil {
			op.Error(err, "failed to update password")
			return nil, s.wrapError(ctx, s.config, "update password", opts.Spec.Username, err)
		}

		op.Success("user updated successfully")
		return NewUpdatedResult(fmt.Sprintf("User '%s' updated successfully", opts.Spec.Username)), nil
	}

	// Build create options using SpecBuilder
	createOpts := s.specBuilder.BuildUserCreateOptions(opts.Spec, opts.Password)
	op.Debug("creating user", "login", createOpts.Login, "inherit", createOpts.Inherit)

	// Create the user
	if err := s.adapter.CreateUser(ctx, createOpts); err != nil {
		op.Error(err, "failed to create user")
		return nil, s.wrapError(ctx, s.config, "create", opts.Spec.Username, err)
	}

	op.Success("user created successfully")
	return NewCreatedResult(fmt.Sprintf("User '%s' created successfully", opts.Spec.Username)), nil
}

// Get retrieves information about a user.
func (s *UserService) Get(ctx context.Context, username string) (*types.UserInfo, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}

	op := s.startOp("Get", username)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", username, err)
	}
	if !exists {
		op.Debug("user not found")
		return nil, ErrNotFound
	}

	// Get user info
	info, err := s.adapter.GetUserInfo(ctx, username)
	if err != nil {
		op.Error(err, "failed to get info")
		return nil, s.wrapError(ctx, s.config, "get info", username, err)
	}

	op.Success("retrieved user info")
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

	op := s.startOp("Update", username)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", username, err)
	}
	if !exists {
		op.Debug("user not found")
		return nil, ErrNotFound
	}

	// Build update options using SpecBuilder
	opts := s.specBuilder.BuildUserUpdateOptions(spec)
	op.Debug("updating user settings")

	// Update the user
	if err := s.adapter.UpdateUser(ctx, username, opts); err != nil {
		op.Error(err, "failed to update user")
		return nil, s.wrapError(ctx, s.config, "update", username, err)
	}

	op.Success("user updated successfully")
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

	op := s.startOp("UpdatePassword", username)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", username, err)
	}
	if !exists {
		op.Debug("user not found")
		return nil, ErrNotFound
	}

	// Update password
	if err := s.adapter.UpdatePassword(ctx, username, password); err != nil {
		op.Error(err, "failed to update password")
		return nil, s.wrapError(ctx, s.config, "update password", username, err)
	}

	op.Success("password updated successfully")
	return NewSuccessResult(fmt.Sprintf("Password updated for user '%s'", username)), nil
}

// Delete drops a database user.
func (s *UserService) Delete(ctx context.Context, username string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}

	op := s.startOp("Delete", username)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if user exists
	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", username, err)
	}
	if !exists {
		op.Success("user does not exist (no-op)")
		return NewSuccessResult(fmt.Sprintf("User '%s' does not exist", username)), nil
	}

	// Drop the user
	if err := s.adapter.DropUser(ctx, username); err != nil {
		op.Error(err, "failed to drop user")
		return nil, s.wrapError(ctx, s.config, "delete", username, err)
	}

	op.Success("user deleted successfully")
	return NewSuccessResult(fmt.Sprintf("User '%s' deleted successfully", username)), nil
}

// Exists checks if a user exists.
func (s *UserService) Exists(ctx context.Context, username string) (bool, error) {
	if username == "" {
		return false, &ValidationError{Field: "username", Message: "username is required"}
	}

	op := s.startOp("Exists", username)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	exists, err := s.adapter.UserExists(ctx, username)
	if err != nil {
		op.Error(err, "failed to check existence")
		return false, s.wrapError(ctx, s.config, "check existence", username, err)
	}

	op.Debug("existence check complete", "exists", exists)
	return exists, nil
}
