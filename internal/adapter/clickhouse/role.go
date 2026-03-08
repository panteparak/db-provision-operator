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

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// roleLog is the logger for role operations
var roleLog = ctrl.Log.WithName("clickhouse-adapter").WithName("role")

// CreateRole creates a new ClickHouse role.
// ClickHouse roles are pure privilege containers — they have no attributes like LOGIN or SUPERUSER.
func (a *Adapter) CreateRole(ctx context.Context, opts types.CreateRoleOptions) error {
	log := roleLog.WithValues("roleName", opts.RoleName, "grantCount", len(opts.Grants))
	log.Info("Creating role")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	query := sqlbuilder.ClickHouseCreateRole(opts.RoleName).Build()
	log.V(1).Info("Creating native role", "query", query)
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		log.Error(err, "Failed to create role", "query", query)
		return fmt.Errorf("failed to create role %s: %w", opts.RoleName, err)
	}
	log.Info("Successfully created role")

	// Apply grants to the role
	for i, grant := range opts.Grants {
		log.V(1).Info("Applying grant to role", "grantIndex", i, "database", grant.Database)
		if err := a.applyGrantToRole(ctx, opts.RoleName, grant); err != nil {
			log.Error(err, "Failed to apply grant to role", "grantIndex", i)
			return err
		}
	}

	log.Info("Successfully created role with all grants")
	return nil
}

// DropRole drops an existing ClickHouse role
func (a *Adapter) DropRole(ctx context.Context, roleName string) error {
	log := roleLog.WithValues("roleName", roleName)
	log.Info("Dropping role")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	query := sqlbuilder.ClickHouseDropRole(roleName).IfExists().Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		log.Error(err, "Failed to drop role")
		return fmt.Errorf("failed to drop role %s: %w", roleName, err)
	}

	log.Info("Successfully dropped role")
	return nil
}

// RoleExists checks if a ClickHouse role exists
func (a *Adapter) RoleExists(ctx context.Context, roleName string) (bool, error) {
	log := roleLog.WithValues("roleName", roleName)
	log.V(1).Info("Checking if role exists")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return false, err
	}

	var count uint64
	err = db.QueryRowContext(ctx,
		"SELECT count() FROM system.roles WHERE name = ?",
		roleName).Scan(&count)
	if err != nil {
		log.Error(err, "Failed to check role existence")
		return false, fmt.Errorf("failed to check role existence: %w", err)
	}

	log.V(1).Info("Role existence check completed", "exists", count > 0)
	return count > 0, nil
}

// UpdateRole updates an existing ClickHouse role.
// ClickHouse roles have no attributes — only grants can be changed.
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

// GetRoleInfo retrieves information about a ClickHouse role
func (a *Adapter) GetRoleInfo(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var info types.RoleInfo
	info.Name = roleName

	var count uint64
	err = db.QueryRowContext(ctx,
		"SELECT count() FROM system.roles WHERE name = ?",
		roleName).Scan(&count)
	if err != nil || count == 0 {
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

	b := sqlbuilder.NewClickHouse().Grant(grant.Privileges...)

	switch grant.Level {
	case "global":
		b.OnGlobal()
	case "database":
		b.OnDatabase(grant.Database)
	case "table":
		b.OnMySQLTable(grant.Database, grant.Table)
	default:
		if grant.Database != "" {
			b.OnDatabase(grant.Database)
		} else {
			return fmt.Errorf("database is required for grant")
		}
	}

	b.To(roleName)
	if grant.WithGrantOption {
		b.WithGrantOption()
	}

	query, buildErr := b.Build()
	if buildErr != nil {
		return fmt.Errorf("failed to build grant query: %w", buildErr)
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

	b := sqlbuilder.NewClickHouse().Revoke(grant.Privileges...)

	switch grant.Level {
	case "global":
		b.OnGlobal()
	case "database":
		b.OnDatabase(grant.Database)
	case "table":
		b.OnMySQLTable(grant.Database, grant.Table)
	default:
		if grant.Database != "" {
			b.OnDatabase(grant.Database)
		} else {
			return fmt.Errorf("database is required for revoke")
		}
	}

	b.From(roleName)

	query, buildErr := b.Build()
	if buildErr != nil {
		return fmt.Errorf("failed to build revoke query: %w", buildErr)
	}

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to revoke from role %s: %w", roleName, err)
	}

	return nil
}
