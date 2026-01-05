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

package mysql

import (
	"context"
	"fmt"

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateRole creates a new MySQL role
// Note: Roles are supported in MySQL 8.0+
func (a *Adapter) CreateRole(ctx context.Context, opts types.CreateRoleOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Check if MySQL supports roles (8.0+)
	if opts.UseNativeRoles {
		query := fmt.Sprintf("CREATE ROLE IF NOT EXISTS %s", escapeLiteral(opts.RoleName))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to create role %s: %w", opts.RoleName, err)
		}

		// Apply grants to the role
		for _, grant := range opts.Grants {
			if err := a.applyGrantToRole(ctx, opts.RoleName, grant); err != nil {
				return err
			}
		}
	} else {
		// For older MySQL versions, create a user without login capability
		// MySQL < 8.0 doesn't have native roles, so we emulate with users
		query := fmt.Sprintf("CREATE USER IF NOT EXISTS %s@'%%' ACCOUNT LOCK",
			escapeLiteral(opts.RoleName))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to create role (as user) %s: %w", opts.RoleName, err)
		}

		// Apply grants
		for _, grant := range opts.Grants {
			if err := a.applyGrantToRole(ctx, opts.RoleName, grant); err != nil {
				return err
			}
		}
	}

	return nil
}

// DropRole drops an existing MySQL role
func (a *Adapter) DropRole(ctx context.Context, roleName string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Try native role first
	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeLiteral(roleName))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		// Fall back to dropping as user
		query = fmt.Sprintf("DROP USER IF EXISTS %s@'%%'", escapeLiteral(roleName))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to drop role %s: %w", roleName, err)
		}
	}

	return nil
}

// RoleExists checks if a MySQL role exists
func (a *Adapter) RoleExists(ctx context.Context, roleName string) (bool, error) {
	db, err := a.getDB()
	if err != nil {
		return false, err
	}

	// Check if role exists (MySQL 8.0+)
	var exists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM mysql.user WHERE User = ? AND account_locked = 'Y')",
		roleName).Scan(&exists)
	if err != nil {
		// Try alternative query for older MySQL
		err = db.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM mysql.user WHERE User = ?)",
			roleName).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("failed to check role existence: %w", err)
		}
	}

	return exists, nil
}

// UpdateRole updates an existing MySQL role
func (a *Adapter) UpdateRole(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
	// Apply new grants
	for _, grant := range opts.Grants {
		if err := a.applyGrantToRole(ctx, roleName, grant); err != nil {
			return err
		}
	}

	// Add additional grants
	for _, grant := range opts.AddGrants {
		if err := a.applyGrantToRole(ctx, roleName, grant); err != nil {
			return err
		}
	}

	// Remove grants
	for _, grant := range opts.RemoveGrants {
		if err := a.revokeGrantFromRole(ctx, roleName, grant); err != nil {
			return err
		}
	}

	return nil
}

// GetRoleInfo retrieves information about a MySQL role
func (a *Adapter) GetRoleInfo(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var info types.RoleInfo
	info.Name = roleName

	// Check if the role/user exists
	var exists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM mysql.user WHERE User = ?)",
		roleName).Scan(&exists)
	if err != nil || !exists {
		return nil, fmt.Errorf("role %s not found", roleName)
	}

	return &info, nil
}

// applyGrantToRole applies a grant to a role
func (a *Adapter) applyGrantToRole(ctx context.Context, roleName string, grant types.GrantOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	var query string

	switch grant.Level {
	case "global":
		query = fmt.Sprintf("GRANT %s ON *.* TO %s",
			formatPrivileges(grant.Privileges),
			escapeLiteral(roleName))
	case "database":
		query = fmt.Sprintf("GRANT %s ON %s.* TO %s",
			formatPrivileges(grant.Privileges),
			escapeIdentifier(grant.Database),
			escapeLiteral(roleName))
	case "table":
		query = fmt.Sprintf("GRANT %s ON %s.%s TO %s",
			formatPrivileges(grant.Privileges),
			escapeIdentifier(grant.Database),
			escapeIdentifier(grant.Table),
			escapeLiteral(roleName))
	default:
		// Default to database level
		if grant.Database != "" {
			query = fmt.Sprintf("GRANT %s ON %s.* TO %s",
				formatPrivileges(grant.Privileges),
				escapeIdentifier(grant.Database),
				escapeLiteral(roleName))
		} else {
			return fmt.Errorf("database is required for grant")
		}
	}

	if grant.WithGrantOption {
		query += " WITH GRANT OPTION"
	}

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to grant to role %s: %w", roleName, err)
	}

	return nil
}

// revokeGrantFromRole revokes a grant from a role
func (a *Adapter) revokeGrantFromRole(ctx context.Context, roleName string, grant types.GrantOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	var query string

	switch grant.Level {
	case "global":
		query = fmt.Sprintf("REVOKE %s ON *.* FROM %s",
			formatPrivileges(grant.Privileges),
			escapeLiteral(roleName))
	case "database":
		query = fmt.Sprintf("REVOKE %s ON %s.* FROM %s",
			formatPrivileges(grant.Privileges),
			escapeIdentifier(grant.Database),
			escapeLiteral(roleName))
	case "table":
		query = fmt.Sprintf("REVOKE %s ON %s.%s FROM %s",
			formatPrivileges(grant.Privileges),
			escapeIdentifier(grant.Database),
			escapeIdentifier(grant.Table),
			escapeLiteral(roleName))
	default:
		if grant.Database != "" {
			query = fmt.Sprintf("REVOKE %s ON %s.* FROM %s",
				formatPrivileges(grant.Privileges),
				escapeIdentifier(grant.Database),
				escapeLiteral(roleName))
		} else {
			return fmt.Errorf("database is required for revoke")
		}
	}

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to revoke from role %s: %w", roleName, err)
	}

	return nil
}

// formatPrivileges formats a list of privileges for a GRANT/REVOKE statement
func formatPrivileges(privileges []string) string {
	if len(privileges) == 0 {
		return "USAGE"
	}
	return fmt.Sprintf("%s", joinPrivileges(privileges))
}

// joinPrivileges joins privileges with commas
func joinPrivileges(privileges []string) string {
	result := ""
	for i, p := range privileges {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
