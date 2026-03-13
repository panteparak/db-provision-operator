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

package clickhouse

import (
	"context"
	"fmt"

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateRole creates a new ClickHouse role
func (a *Adapter) CreateRole(ctx context.Context, opts types.CreateRoleOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("CREATE ROLE IF NOT EXISTS %s", escapeIdentifier(opts.RoleName))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create role %s: %w", opts.RoleName, err)
	}

	// Apply initial grants if provided
	if len(opts.Grants) > 0 {
		if err := a.Grant(ctx, opts.RoleName, opts.Grants); err != nil {
			return fmt.Errorf("failed to apply grants to role %s: %w", opts.RoleName, err)
		}
	}

	return nil
}

// DropRole drops an existing ClickHouse role
func (a *Adapter) DropRole(ctx context.Context, roleName string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(roleName))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop role %s: %w", roleName, err)
	}

	return nil
}

// RoleExists checks if a ClickHouse role exists
func (a *Adapter) RoleExists(ctx context.Context, roleName string) (bool, error) {
	db, err := a.getDB()
	if err != nil {
		return false, err
	}

	var count uint64
	err = db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT count() FROM system.roles WHERE name = %s", escapeLiteral(roleName)),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check role existence: %w", err)
	}

	return count > 0, nil
}

// UpdateRole updates an existing ClickHouse role by applying grant changes
func (a *Adapter) UpdateRole(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
	// Apply full grant set
	if len(opts.Grants) > 0 {
		if err := a.Grant(ctx, roleName, opts.Grants); err != nil {
			return fmt.Errorf("failed to apply grants to role %s: %w", roleName, err)
		}
	}

	// Apply additional grants
	if len(opts.AddGrants) > 0 {
		if err := a.Grant(ctx, roleName, opts.AddGrants); err != nil {
			return fmt.Errorf("failed to add grants to role %s: %w", roleName, err)
		}
	}

	// Revoke grants
	if len(opts.RemoveGrants) > 0 {
		if err := a.Revoke(ctx, roleName, opts.RemoveGrants); err != nil {
			return fmt.Errorf("failed to remove grants from role %s: %w", roleName, err)
		}
	}

	return nil
}

// GetRoleInfo retrieves information about a ClickHouse role
func (a *Adapter) GetRoleInfo(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var name string
	err = db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT name FROM system.roles WHERE name = %s", escapeLiteral(roleName)),
	).Scan(&name)
	if err != nil {
		return nil, fmt.Errorf("failed to get role info for %s: %w", roleName, err)
	}

	return &types.RoleInfo{Name: name}, nil
}
