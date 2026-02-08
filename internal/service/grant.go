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

// GrantService handles database grant operations using the appropriate adapter.
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type GrantService struct {
	baseService
	adapter adapter.DatabaseAdapter
	config  *Config
}

// NewGrantService creates a new GrantService with the given configuration.
func NewGrantService(cfg *Config) (*GrantService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &GrantService{
		baseService: newBaseService(cfg, "GrantService"),
		adapter:     dbAdapter,
		config:      cfg,
	}, nil
}

// NewGrantServiceWithAdapter creates a GrantService with a pre-created adapter.
func NewGrantServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *GrantService {
	return &GrantService{
		baseService: newBaseService(cfg, "GrantService"),
		adapter:     adp,
		config:      cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *GrantService) Connect(ctx context.Context) error {
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
func (s *GrantService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// Adapter returns the underlying database adapter for drift detection.
func (s *GrantService) Adapter() adapter.DatabaseAdapter {
	return s.adapter
}

// ApplyGrantServiceOptions contains options for applying grants.
type ApplyGrantServiceOptions struct {
	Username string
	Spec     *dbopsv1alpha1.DatabaseGrantSpec
}

// GrantResult contains detailed information about grant operations.
// This is returned by Apply/Revoke methods and includes breakdown of applied grants.
type GrantResult struct {
	AppliedRoles             []string
	AppliedDirectGrants      int
	AppliedDefaultPrivileges int
}

// Apply applies all grants from the spec to the specified user.
// This is the main method for the CLI's "create" command.
func (s *GrantService) Apply(ctx context.Context, opts ApplyGrantServiceOptions) (*GrantResult, error) {
	if opts.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if opts.Spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	op := s.startOp("Apply", opts.Username)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	result := &GrantResult{
		AppliedRoles: []string{},
	}

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.EngineTypeCockroachDB:
		// CockroachDB uses PostgreSQL wire protocol and the same grant syntax
		if opts.Spec.Postgres != nil {
			// Grant roles
			if len(opts.Spec.Postgres.Roles) > 0 {
				op.Debug("granting roles", "roles", opts.Spec.Postgres.Roles)
				if err := s.adapter.GrantRole(ctx, opts.Username, opts.Spec.Postgres.Roles); err != nil {
					op.Error(err, "failed to grant roles")
					return nil, s.wrapError(ctx, s.config, "grant roles", opts.Username, err)
				}
				result.AppliedRoles = append(result.AppliedRoles, opts.Spec.Postgres.Roles...)
			}

			// Apply direct grants
			if len(opts.Spec.Postgres.Grants) > 0 {
				op.Debug("applying direct grants", "count", len(opts.Spec.Postgres.Grants))
				grantOpts := s.buildPostgresGrantOptions(opts.Spec.Postgres.Grants)
				if err := s.adapter.Grant(ctx, opts.Username, grantOpts); err != nil {
					op.Error(err, "failed to apply grants")
					return nil, s.wrapError(ctx, s.config, "apply grants", opts.Username, err)
				}
				result.AppliedDirectGrants = len(opts.Spec.Postgres.Grants)
			}

			// Apply default privileges (Note: CockroachDB has limited default privilege support)
			if len(opts.Spec.Postgres.DefaultPrivileges) > 0 {
				op.Debug("setting default privileges", "count", len(opts.Spec.Postgres.DefaultPrivileges))
				defPrivOpts := s.buildDefaultPrivilegeOptions(opts.Spec.Postgres.DefaultPrivileges)
				if err := s.adapter.SetDefaultPrivileges(ctx, opts.Username, defPrivOpts); err != nil {
					op.Error(err, "failed to set default privileges")
					return nil, s.wrapError(ctx, s.config, "set default privileges", opts.Username, err)
				}
				result.AppliedDefaultPrivileges = len(opts.Spec.Postgres.DefaultPrivileges)
			}
		}

	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		// MariaDB uses the same MySQL wire protocol and grant syntax
		if opts.Spec.MySQL != nil {
			// Grant roles
			if len(opts.Spec.MySQL.Roles) > 0 {
				op.Debug("granting roles", "roles", opts.Spec.MySQL.Roles)
				if err := s.adapter.GrantRole(ctx, opts.Username, opts.Spec.MySQL.Roles); err != nil {
					op.Error(err, "failed to grant roles")
					return nil, s.wrapError(ctx, s.config, "grant roles", opts.Username, err)
				}
				result.AppliedRoles = append(result.AppliedRoles, opts.Spec.MySQL.Roles...)
			}

			// Apply direct grants
			if len(opts.Spec.MySQL.Grants) > 0 {
				op.Debug("applying direct grants", "count", len(opts.Spec.MySQL.Grants))
				grantOpts := s.buildMySQLGrantOptions(opts.Spec.MySQL.Grants)
				if err := s.adapter.Grant(ctx, opts.Username, grantOpts); err != nil {
					op.Error(err, "failed to apply grants")
					return nil, s.wrapError(ctx, s.config, "apply grants", opts.Username, err)
				}
				result.AppliedDirectGrants = len(opts.Spec.MySQL.Grants)
			}
		}
	}

	op.Success("grants applied successfully")
	return result, nil
}

// Revoke revokes all grants from the spec from the specified user.
// This is the main method for the CLI's "delete" command.
func (s *GrantService) Revoke(ctx context.Context, opts ApplyGrantServiceOptions) (*Result, error) {
	if opts.Username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if opts.Spec == nil {
		return nil, &ValidationError{Field: "spec", Message: "spec is required"}
	}

	op := s.startOp("Revoke", opts.Username)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	var revokedCount int

	switch s.config.GetEngineType() {
	case dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.EngineTypeCockroachDB:
		// CockroachDB uses PostgreSQL wire protocol and the same grant syntax
		if opts.Spec.Postgres != nil {
			// Revoke roles
			if len(opts.Spec.Postgres.Roles) > 0 {
				op.Debug("revoking roles", "roles", opts.Spec.Postgres.Roles)
				if err := s.adapter.RevokeRole(ctx, opts.Username, opts.Spec.Postgres.Roles); err != nil {
					op.Error(err, "failed to revoke roles")
					return nil, s.wrapError(ctx, s.config, "revoke roles", opts.Username, err)
				}
				revokedCount += len(opts.Spec.Postgres.Roles)
			}

			// Revoke direct grants
			if len(opts.Spec.Postgres.Grants) > 0 {
				op.Debug("revoking direct grants", "count", len(opts.Spec.Postgres.Grants))
				grantOpts := s.buildPostgresGrantOptions(opts.Spec.Postgres.Grants)
				if err := s.adapter.Revoke(ctx, opts.Username, grantOpts); err != nil {
					op.Error(err, "failed to revoke grants")
					return nil, s.wrapError(ctx, s.config, "revoke grants", opts.Username, err)
				}
				revokedCount += len(opts.Spec.Postgres.Grants)
			}
		}

	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		// MariaDB uses the same MySQL wire protocol and grant syntax
		if opts.Spec.MySQL != nil {
			// Revoke roles
			if len(opts.Spec.MySQL.Roles) > 0 {
				op.Debug("revoking roles", "roles", opts.Spec.MySQL.Roles)
				if err := s.adapter.RevokeRole(ctx, opts.Username, opts.Spec.MySQL.Roles); err != nil {
					op.Error(err, "failed to revoke roles")
					return nil, s.wrapError(ctx, s.config, "revoke roles", opts.Username, err)
				}
				revokedCount += len(opts.Spec.MySQL.Roles)
			}

			// Revoke direct grants
			if len(opts.Spec.MySQL.Grants) > 0 {
				op.Debug("revoking direct grants", "count", len(opts.Spec.MySQL.Grants))
				grantOpts := s.buildMySQLGrantOptions(opts.Spec.MySQL.Grants)
				if err := s.adapter.Revoke(ctx, opts.Username, grantOpts); err != nil {
					op.Error(err, "failed to revoke grants")
					return nil, s.wrapError(ctx, s.config, "revoke grants", opts.Username, err)
				}
				revokedCount += len(opts.Spec.MySQL.Grants)
			}
		}
	}

	op.Success("grants revoked successfully")
	return NewSuccessResult(fmt.Sprintf("Revoked %d grants from user '%s'", revokedCount, opts.Username)), nil
}

// GrantRoles grants the specified roles to a user.
func (s *GrantService) GrantRoles(ctx context.Context, username string, roles []string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if len(roles) == 0 {
		return nil, &ValidationError{Field: "roles", Message: "at least one role is required"}
	}

	op := s.startOp("GrantRoles", username)
	op.Debug("granting roles", "roles", roles)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	if err := s.adapter.GrantRole(ctx, username, roles); err != nil {
		op.Error(err, "failed to grant roles")
		return nil, s.wrapError(ctx, s.config, "grant roles", username, err)
	}

	op.Success("roles granted successfully")
	return NewSuccessResult(fmt.Sprintf("Granted %d roles to user '%s'", len(roles), username)), nil
}

// RevokeRoles revokes the specified roles from a user.
func (s *GrantService) RevokeRoles(ctx context.Context, username string, roles []string) (*Result, error) {
	if username == "" {
		return nil, &ValidationError{Field: "username", Message: "username is required"}
	}
	if len(roles) == 0 {
		return nil, &ValidationError{Field: "roles", Message: "at least one role is required"}
	}

	op := s.startOp("RevokeRoles", username)
	op.Debug("revoking roles", "roles", roles)

	// Apply operation timeout
	ctx, cancel := s.config.Timeouts.WithOperationTimeout(ctx)
	defer cancel()

	if err := s.adapter.RevokeRole(ctx, username, roles); err != nil {
		op.Error(err, "failed to revoke roles")
		return nil, s.wrapError(ctx, s.config, "revoke roles", username, err)
	}

	op.Success("roles revoked successfully")
	return NewSuccessResult(fmt.Sprintf("Revoked %d roles from user '%s'", len(roles), username)), nil
}

// GetGrants retrieves the current grants for a user/role.
func (s *GrantService) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	if grantee == "" {
		return nil, &ValidationError{Field: "grantee", Message: "grantee is required"}
	}

	op := s.startOp("GetGrants", grantee)

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	grants, err := s.adapter.GetGrants(ctx, grantee)
	if err != nil {
		op.Error(err, "failed to get grants")
		return nil, s.wrapError(ctx, s.config, "get grants", grantee, err)
	}

	op.Success("retrieved grants")
	return grants, nil
}

// buildPostgresGrantOptions converts PostgresGrant specs to adapter GrantOptions.
// This logic was extracted from databasegrant_controller.go
func (s *GrantService) buildPostgresGrantOptions(grants []dbopsv1alpha1.PostgresGrant) []types.GrantOptions {
	opts := make([]types.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, types.GrantOptions{
			Database:        g.Database,
			Schema:          g.Schema,
			Tables:          g.Tables,
			Sequences:       g.Sequences,
			Functions:       g.Functions,
			Privileges:      g.Privileges,
			WithGrantOption: g.WithGrantOption,
		})
	}
	return opts
}

// buildMySQLGrantOptions converts MySQLGrant specs to adapter GrantOptions.
// This logic was extracted from databasegrant_controller.go
func (s *GrantService) buildMySQLGrantOptions(grants []dbopsv1alpha1.MySQLGrant) []types.GrantOptions {
	opts := make([]types.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, types.GrantOptions{
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
	return opts
}

// buildDefaultPrivilegeOptions converts PostgresDefaultPrivilegeGrant specs to adapter options.
// This logic was extracted from databasegrant_controller.go
func (s *GrantService) buildDefaultPrivilegeOptions(defPrivs []dbopsv1alpha1.PostgresDefaultPrivilegeGrant) []types.DefaultPrivilegeGrantOptions {
	opts := make([]types.DefaultPrivilegeGrantOptions, 0, len(defPrivs))
	for _, dp := range defPrivs {
		opts = append(opts, types.DefaultPrivilegeGrantOptions{
			Database:   dp.Database,
			Schema:     dp.Schema,
			GrantedBy:  dp.GrantedBy,
			ObjectType: dp.ObjectType,
			Privileges: dp.Privileges,
		})
	}
	return opts
}
