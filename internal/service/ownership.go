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
	"strings"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
)

// OwnershipService handles per-database role and user provisioning.
// It creates a group role that owns the database and a login user
// that inherits from the group role.
type OwnershipService struct {
	*ResourceService
}

// NewOwnershipService creates an OwnershipService from an existing ResourceService.
func NewOwnershipService(rs *ResourceService) *OwnershipService {
	return &OwnershipService{ResourceService: rs}
}

// OwnershipResult contains the result of ownership provisioning.
type OwnershipResult struct {
	RoleName string
	UserName string
}

// DeriveRoleName returns the group role name for a database.
// Uses the configured name if set, otherwise derives db_<dbname>_owner.
// Names longer than 63 characters are truncated.
func DeriveRoleName(cfg *dbopsv1alpha1.PostgresOwnershipConfig, dbName string) string {
	if cfg != nil && cfg.RoleName != "" {
		return cfg.RoleName
	}
	return truncateIdentifier(fmt.Sprintf("db_%s_owner", dbName))
}

// DeriveUserName returns the login user name for a database.
// Uses the configured name if set, otherwise derives db_<dbname>_app.
// Names longer than 63 characters are truncated.
func DeriveUserName(cfg *dbopsv1alpha1.PostgresOwnershipConfig, dbName string) string {
	if cfg != nil && cfg.UserName != "" {
		return cfg.UserName
	}
	return truncateIdentifier(fmt.Sprintf("db_%s_app", dbName))
}

// truncateIdentifier truncates a PostgreSQL identifier to 63 characters (NAMEDATALEN-1).
func truncateIdentifier(name string) string {
	if len(name) > 63 {
		return name[:63]
	}
	return name
}

// EnsureOwnerRole creates the group role if it does not already exist.
// The role is created with NOLOGIN and INHERIT (group role pattern).
func (s *OwnershipService) EnsureOwnerRole(ctx context.Context, roleName string) error {
	op := s.startOp("EnsureOwnerRole", roleName)

	exists, err := s.adapter.RoleExists(ctx, roleName)
	if err != nil {
		op.Error(err, "failed to check role existence")
		return fmt.Errorf("check role existence: %w", err)
	}
	if exists {
		op.Success("role already exists")
		return nil
	}

	err = s.adapter.CreateRole(ctx, types.CreateRoleOptions{
		RoleName: roleName,
		Login:    false,
		Inherit:  true,
	})
	if err != nil {
		op.Error(err, "failed to create owner role")
		return fmt.Errorf("create owner role: %w", err)
	}

	op.Success("owner role created")
	return nil
}

// EnsureOwnerUser creates the login user and grants it membership in the owner role.
// The user is created with LOGIN and INHERIT, and is granted the owner role.
// On CockroachDB, role membership is granted via separate GRANT since IN ROLE is not supported.
func (s *OwnershipService) EnsureOwnerUser(ctx context.Context, userName, roleName string) error {
	op := s.startOp("EnsureOwnerUser", userName)

	exists, err := s.adapter.RoleExists(ctx, userName)
	if err != nil {
		op.Error(err, "failed to check user existence")
		return fmt.Errorf("check user existence: %w", err)
	}

	if !exists {
		// Create user without IN ROLE (CockroachDB compat)
		err = s.adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName: userName,
			Login:    true,
			Inherit:  true,
		})
		if err != nil {
			op.Error(err, "failed to create owner user")
			return fmt.Errorf("create owner user: %w", err)
		}
	}

	// Grant role membership (idempotent on both PG and CRDB)
	if err := s.adapter.GrantRole(ctx, userName, []string{roleName}); err != nil {
		op.Error(err, "failed to grant role to user")
		return fmt.Errorf("grant role to user: %w", err)
	}

	op.Success("owner user ensured")
	return nil
}

// TransferOwnership transfers database ownership to the specified role.
func (s *OwnershipService) TransferOwnership(ctx context.Context, dbName, roleName string) error {
	op := s.startOp("TransferOwnership", dbName)

	if err := s.adapter.TransferDatabaseOwnership(ctx, dbName, roleName); err != nil {
		op.Error(err, "failed to transfer ownership")
		return fmt.Errorf("transfer ownership: %w", err)
	}

	op.Success("ownership transferred")
	return nil
}

// SetDefaultPrivileges sets ALTER DEFAULT PRIVILEGES so objects created by the owner role
// are accessible to the app user. Applied to the public schema plus all specified schemas.
func (s *OwnershipService) SetDefaultPrivileges(ctx context.Context, dbName, ownerRole, appUser string, schemas []string) error {
	op := s.startOp("SetDefaultPrivileges", dbName)

	// Collect unique schema names: always include public
	schemaSet := map[string]bool{"public": true}
	for _, schema := range schemas {
		schemaSet[schema] = true
	}

	// Default privilege definitions per object type
	type defaultPriv struct {
		objectType string
		privileges []string
	}
	privDefs := []defaultPriv{
		{"tables", []string{"SELECT", "INSERT", "UPDATE", "DELETE"}},
		{"sequences", []string{"USAGE", "SELECT"}},
		{"functions", []string{"EXECUTE"}},
	}

	for schemaName := range schemaSet {
		for _, dp := range privDefs {
			opts := []types.DefaultPrivilegeGrantOptions{
				{
					Database:   dbName,
					Schema:     schemaName,
					GrantedBy:  ownerRole,
					ObjectType: dp.objectType,
					Privileges: dp.privileges,
				},
			}
			if err := s.adapter.SetDefaultPrivileges(ctx, appUser, opts); err != nil {
				op.Error(err, fmt.Sprintf("failed to set default privileges for %s in schema %s", dp.objectType, schemaName))
				return fmt.Errorf("set default privileges for %s in %s: %w",
					dp.objectType, schemaName, err)
			}
		}
	}

	op.Success("default privileges set")
	return nil
}

// DropOwnershipResources drops the auto-created user and role.
// Follows the safe deletion pattern: REASSIGN OWNED BY → DROP OWNED BY → DROP ROLE.
func (s *OwnershipService) DropOwnershipResources(ctx context.Context, roleName, userName string) error {
	op := s.startOp("DropOwnershipResources", roleName)

	// Drop the user first (it depends on the role)
	if userName != "" {
		if err := s.adapter.DropRole(ctx, userName); err != nil {
			// Log but continue — best effort cleanup
			op.Error(err, "failed to drop owner user")
		}
	}

	// Drop the group role
	if roleName != "" {
		if err := s.adapter.DropRole(ctx, roleName); err != nil {
			op.Error(err, "failed to drop owner role")
			return fmt.Errorf("drop owner role: %w", err)
		}
	}

	op.Success("ownership resources dropped")
	return nil
}

// HasAutoOwnership returns true if the spec has auto-ownership enabled.
func HasAutoOwnership(spec *dbopsv1alpha1.DatabaseSpec) bool {
	return spec.Postgres != nil &&
		spec.Postgres.Ownership != nil &&
		spec.Postgres.Ownership.AutoOwnership
}

// IsOwnershipSupported returns true if the engine supports database ownership.
func IsOwnershipSupported(engine string) bool {
	e := strings.ToLower(engine)
	return e == "postgres" || e == "cockroachdb"
}
