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
	adapter adapter.DatabaseAdapter
	config  *Config
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
		adapter: dbAdapter,
		config:  cfg,
	}, nil
}

// NewRoleServiceWithAdapter creates a RoleService with a pre-created adapter.
func NewRoleServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *RoleService {
	return &RoleService{
		adapter: adp,
		config:  cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *RoleService) Connect(ctx context.Context) error {
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
func (s *RoleService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
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

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if role already exists
	exists, err := s.adapter.RoleExists(ctx, spec.RoleName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", spec.RoleName, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", spec.RoleName, err)
	}

	if exists {
		// Update existing role
		updateOpts := s.buildUpdateOptions(spec)
		if err := s.adapter.UpdateRole(ctx, spec.RoleName, updateOpts); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, NewTimeoutError("update", spec.RoleName, s.config.Timeouts.OperationTimeout.String(), err)
			}
			return nil, NewDatabaseError("update", spec.RoleName, err)
		}
		return NewUpdatedResult(fmt.Sprintf("Role '%s' updated successfully", spec.RoleName)), nil
	}

	// Build create options
	createOpts := s.buildCreateOptions(spec)

	// Create the role
	if err := s.adapter.CreateRole(ctx, createOpts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("create", spec.RoleName, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("create", spec.RoleName, err)
	}

	return NewCreatedResult(fmt.Sprintf("Role '%s' created successfully", spec.RoleName)), nil
}

// Get retrieves information about a role.
func (s *RoleService) Get(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	if roleName == "" {
		return nil, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Check if role exists
	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", roleName, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", roleName, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Get role info
	info, err := s.adapter.GetRoleInfo(ctx, roleName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("get info", roleName, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return nil, NewDatabaseError("get info", roleName, err)
	}

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

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if role exists
	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", roleName, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", roleName, err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	// Build update options
	opts := s.buildUpdateOptions(spec)

	// Update the role
	if err := s.adapter.UpdateRole(ctx, roleName, opts); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("update", roleName, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("update", roleName, err)
	}

	return NewUpdatedResult(fmt.Sprintf("Role '%s' updated successfully", roleName)), nil
}

// Delete drops a database role.
func (s *RoleService) Delete(ctx context.Context, roleName string) (*Result, error) {
	if roleName == "" {
		return nil, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	// Check if role exists
	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("check existence", roleName, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("check existence", roleName, err)
	}
	if !exists {
		return NewSuccessResult(fmt.Sprintf("Role '%s' does not exist", roleName)), nil
	}

	// Drop the role
	if err := s.adapter.DropRole(ctx, roleName); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, NewTimeoutError("delete", roleName, s.config.Timeouts.OperationTimeout.String(), err)
		}
		return nil, NewDatabaseError("delete", roleName, err)
	}

	return NewSuccessResult(fmt.Sprintf("Role '%s' deleted successfully", roleName)), nil
}

// Exists checks if a role exists.
func (s *RoleService) Exists(ctx context.Context, roleName string) (bool, error) {
	if roleName == "" {
		return false, &ValidationError{Field: "roleName", Message: "role name is required"}
	}

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return false, NewTimeoutError("check existence", roleName, s.config.Timeouts.QueryTimeout.String(), err)
		}
		return false, NewDatabaseError("check existence", roleName, err)
	}

	return exists, nil
}

// buildCreateOptions builds adapter.CreateRoleOptions from the spec.
// This logic was extracted from databaserole_controller.go
func (s *RoleService) buildCreateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.CreateRoleOptions {
	opts := types.CreateRoleOptions{
		RoleName: spec.RoleName,
		Inherit:  true, // Default to inheriting privileges
	}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec.Postgres != nil {
			opts.Login = spec.Postgres.Login
			opts.Inherit = spec.Postgres.Inherit
			opts.CreateDB = spec.Postgres.CreateDB
			opts.CreateRole = spec.Postgres.CreateRole
			opts.Superuser = spec.Postgres.Superuser
			opts.Replication = spec.Postgres.Replication
			opts.BypassRLS = spec.Postgres.BypassRLS
			opts.InRoles = spec.Postgres.InRoles

			// Convert grants
			if len(spec.Postgres.Grants) > 0 {
				opts.Grants = make([]types.GrantOptions, 0, len(spec.Postgres.Grants))
				for _, g := range spec.Postgres.Grants {
					opts.Grants = append(opts.Grants, types.GrantOptions{
						Database:        g.Database,
						Schema:          g.Schema,
						Tables:          g.Tables,
						Sequences:       g.Sequences,
						Functions:       g.Functions,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if spec.MySQL != nil {
			opts.UseNativeRoles = spec.MySQL.UseNativeRoles

			// Convert grants
			if len(spec.MySQL.Grants) > 0 {
				opts.Grants = make([]types.GrantOptions, 0, len(spec.MySQL.Grants))
				for _, g := range spec.MySQL.Grants {
					opts.Grants = append(opts.Grants, types.GrantOptions{
						Level:           string(g.Level),
						Database:        g.Database,
						Table:           g.Table,
						Columns:         g.Columns,
						Procedure:       g.Procedure,
						Function:        g.Function,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	}

	return opts
}

// buildUpdateOptions builds adapter.UpdateRoleOptions from the spec.
// This logic was extracted from databaserole_controller.go
func (s *RoleService) buildUpdateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.UpdateRoleOptions {
	opts := types.UpdateRoleOptions{}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres:
		if spec.Postgres != nil {
			login := spec.Postgres.Login
			opts.Login = &login
			inherit := spec.Postgres.Inherit
			opts.Inherit = &inherit
			createDB := spec.Postgres.CreateDB
			opts.CreateDB = &createDB
			createRole := spec.Postgres.CreateRole
			opts.CreateRole = &createRole
			superuser := spec.Postgres.Superuser
			opts.Superuser = &superuser
			replication := spec.Postgres.Replication
			opts.Replication = &replication
			bypassRLS := spec.Postgres.BypassRLS
			opts.BypassRLS = &bypassRLS
			opts.InRoles = spec.Postgres.InRoles

			// Convert grants
			if len(spec.Postgres.Grants) > 0 {
				opts.Grants = make([]types.GrantOptions, 0, len(spec.Postgres.Grants))
				for _, g := range spec.Postgres.Grants {
					opts.Grants = append(opts.Grants, types.GrantOptions{
						Database:        g.Database,
						Schema:          g.Schema,
						Tables:          g.Tables,
						Sequences:       g.Sequences,
						Functions:       g.Functions,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if spec.MySQL != nil {
			// Convert grants for add
			if len(spec.MySQL.Grants) > 0 {
				opts.AddGrants = make([]types.GrantOptions, 0, len(spec.MySQL.Grants))
				for _, g := range spec.MySQL.Grants {
					opts.AddGrants = append(opts.AddGrants, types.GrantOptions{
						Level:           string(g.Level),
						Database:        g.Database,
						Table:           g.Table,
						Columns:         g.Columns,
						Procedure:       g.Procedure,
						Function:        g.Function,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	}

	return opts
}
