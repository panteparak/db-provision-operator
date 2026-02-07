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

// RoleService handles database role operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type RoleService struct {
	baseService
	adapter     adapter.DatabaseAdapter
	config      *Config
	specBuilder SpecBuilder
}

// NewRoleService creates a new RoleService with the given configuration.
func NewRoleService(cfg *Config) (*RoleService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &RoleService{
		baseService: newBaseService(cfg, "RoleService"),
		adapter:     dbAdapter,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}, nil
}

// NewRoleServiceWithAdapter creates a RoleService with a pre-created adapter.
func NewRoleServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *RoleService {
	return &RoleService{
		baseService: newBaseService(cfg, "RoleService"),
		adapter:     adp,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}
}

// Connect establishes a connection to the database server.
func (s *RoleService) Connect(ctx context.Context) error {
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
func (s *RoleService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// Adapter returns the underlying database adapter.
// This is useful for drift detection and other operations that need
// direct adapter access.
func (s *RoleService) Adapter() adapter.DatabaseAdapter {
	return s.adapter
}

// Create creates a new database role with the given spec.
// If the role already exists, it updates the role instead.
func (s *RoleService) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec) (*Result, error) {
	if spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}
	if spec.RoleName == "" {
		return nil, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	op := s.startOp("Create", spec.RoleName)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if role already exists
	exists, err := s.adapter.RoleExists(ctx, spec.RoleName)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", spec.RoleName, err)
	}

	if exists {
		op.Debug("role exists, updating instead")
		// Update existing role using SpecBuilder
		updateOpts := s.specBuilder.BuildRoleUpdateOptions(spec)
		if err := s.adapter.UpdateRole(ctx, spec.RoleName, updateOpts); err != nil {
			op.Error(err, "failed to update role")
			return nil, s.wrapError(ctx, s.config, "update", spec.RoleName, err)
		}
		op.Success("role updated successfully")
		return NewUpdatedResult(fmt.Sprintf("Role '%s' updated successfully", spec.RoleName)), nil
	}

	// Build create options using SpecBuilder
	createOpts := s.specBuilder.BuildRoleCreateOptions(spec)
	op.Debug("creating role", "inherit", createOpts.Inherit, "grants", len(createOpts.Grants))

	// Create the role
	if err := s.adapter.CreateRole(ctx, createOpts); err != nil {
		op.Error(err, "failed to create role")
		return nil, s.wrapError(ctx, s.config, "create", spec.RoleName, err)
	}

	op.Success("role created successfully")
	return NewCreatedResult(fmt.Sprintf("Role '%s' created successfully", spec.RoleName)), nil
}

// Get retrieves information about a role.
func (s *RoleService) Get(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	if roleName == "" {
		return nil, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	op := s.startOp("Get", roleName)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Check if role exists
	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", roleName, err)
	}
	if !exists {
		op.Debug("role not found")
		return nil, ErrNotFound
	}

	// Get role info
	info, err := s.adapter.GetRoleInfo(ctx, roleName)
	if err != nil {
		op.Error(err, "failed to get info")
		return nil, s.wrapError(ctx, s.config, "get info", roleName, err)
	}

	op.Success("retrieved role info")
	return info, nil
}

// Update updates role settings.
func (s *RoleService) Update(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec) (*Result, error) {
	if roleName == "" {
		return nil, &ValidationError{Field: "roleName", Message: "role name is required"}
	}
	if spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	op := s.startOp("Update", roleName)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if role exists
	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", roleName, err)
	}
	if !exists {
		op.Debug("role not found")
		return nil, ErrNotFound
	}

	// Build update options using SpecBuilder
	opts := s.specBuilder.BuildRoleUpdateOptions(spec)
	op.Debug("updating role settings")

	// Update the role
	if err := s.adapter.UpdateRole(ctx, roleName, opts); err != nil {
		op.Error(err, "failed to update role")
		return nil, s.wrapError(ctx, s.config, "update", roleName, err)
	}

	op.Success("role updated successfully")
	return NewUpdatedResult(fmt.Sprintf("Role '%s' updated successfully", roleName)), nil
}

// Delete drops a database role.
func (s *RoleService) Delete(ctx context.Context, roleName string) (*Result, error) {
	if roleName == "" {
		return nil, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	op := s.startOp("Delete", roleName)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if role exists
	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		op.Error(err, "failed to check existence")
		return nil, s.wrapError(ctx, s.config, "check existence", roleName, err)
	}
	if !exists {
		op.Success("role does not exist (no-op)")
		return NewSuccessResult(fmt.Sprintf("Role '%s' does not exist", roleName)), nil
	}

	// Drop the role
	if err := s.adapter.DropRole(ctx, roleName); err != nil {
		op.Error(err, "failed to drop role")
		return nil, s.wrapError(ctx, s.config, "delete", roleName, err)
	}

	op.Success("role deleted successfully")
	return NewSuccessResult(fmt.Sprintf("Role '%s' deleted successfully", roleName)), nil
}

// Exists checks if a role exists.
func (s *RoleService) Exists(ctx context.Context, roleName string) (bool, error) {
	if roleName == "" {
		return false, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	op := s.startOp("Exists", roleName)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		op.Error(err, "failed to check existence")
		return false, s.wrapError(ctx, s.config, "check existence", roleName, err)
	}

	op.Debug("existence check complete", "exists", exists)
	return exists, nil
}
