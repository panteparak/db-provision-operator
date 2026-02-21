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

package postgres

import (
	"context"
	"fmt"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateRole creates a new PostgreSQL role
func (a *Adapter) CreateRole(ctx context.Context, opts types.CreateRoleOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	b := sqlbuilder.PgCreateRole(opts.RoleName).
		Login(opts.Login).
		Superuser(opts.Superuser).
		CreateDB(opts.CreateDB).
		CreateRoleOpt(opts.CreateRole).
		Inherit(opts.Inherit).
		Replication(opts.Replication).
		BypassRLS(opts.BypassRLS)

	if len(opts.InRoles) > 0 {
		b.InRoles(opts.InRoles...)
	}

	query := b.Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create role %s: %w", opts.RoleName, err)
	}

	// Apply grants if specified
	for _, grant := range opts.Grants {
		if err := a.applyGrant(ctx, opts.RoleName, grant); err != nil {
			return fmt.Errorf("failed to apply grant to role %s: %w", opts.RoleName, err)
		}
	}

	return nil
}

// DropRole drops an existing PostgreSQL role
func (a *Adapter) DropRole(ctx context.Context, roleName string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// First reassign owned objects and drop owned
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(roleName)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(roleName)))

	query := sqlbuilder.PgDropRole(roleName).IfExists().Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop role %s: %w", roleName, err)
	}

	return nil
}

// RoleExists checks if a PostgreSQL role exists
func (a *Adapter) RoleExists(ctx context.Context, roleName string) (bool, error) {
	pool, err := a.getPool()
	if err != nil {
		return false, err
	}

	var exists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)",
		roleName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role existence: %w", err)
	}

	return exists, nil
}

// UpdateRole updates an existing PostgreSQL role
func (a *Adapter) UpdateRole(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	b := sqlbuilder.PgAlterRole(roleName)
	hasOpts := false

	if opts.Login != nil {
		b.Login(*opts.Login)
		hasOpts = true
	}
	if opts.Superuser != nil {
		b.Superuser(*opts.Superuser)
		hasOpts = true
	}
	if opts.CreateDB != nil {
		b.CreateDB(*opts.CreateDB)
		hasOpts = true
	}
	if opts.CreateRole != nil {
		b.CreateRoleOpt(*opts.CreateRole)
		hasOpts = true
	}
	if opts.Inherit != nil {
		b.Inherit(*opts.Inherit)
		hasOpts = true
	}
	if opts.Replication != nil {
		b.Replication(*opts.Replication)
		hasOpts = true
	}
	if opts.BypassRLS != nil {
		b.BypassRLS(*opts.BypassRLS)
		hasOpts = true
	}

	if hasOpts {
		query := b.Build()
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to update role %s: %w", roleName, err)
		}
	}

	// Handle role membership changes
	for _, role := range opts.InRoles {
		q, err := sqlbuilder.NewPg().GrantRole(role).To(roleName).Build()
		if err != nil {
			return fmt.Errorf("failed to build grant role query: %w", err)
		}
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to grant role %s to %s: %w", role, roleName, err)
		}
	}

	// Apply new grants
	for _, grant := range opts.Grants {
		if err := a.applyGrant(ctx, roleName, grant); err != nil {
			return fmt.Errorf("failed to apply grant to role %s: %w", roleName, err)
		}
	}

	return nil
}

// GetRoleInfo retrieves information about a PostgreSQL role
func (a *Adapter) GetRoleInfo(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var info types.RoleInfo
	err = pool.QueryRow(ctx, `
		SELECT rolname, rolcanlogin, rolinherit, rolcreatedb,
		       rolcreaterole, rolsuper, rolreplication, rolbypassrls
		FROM pg_roles
		WHERE rolname = $1`,
		roleName).Scan(
		&info.Name, &info.Login, &info.Inherit, &info.CreateDB,
		&info.CreateRole, &info.Superuser, &info.Replication, &info.BypassRLS)
	if err != nil {
		return nil, fmt.Errorf("failed to get role info: %w", err)
	}

	// Get role memberships
	rows, err := pool.Query(ctx, `
		SELECT r.rolname
		FROM pg_roles r
		JOIN pg_auth_members m ON r.oid = m.roleid
		JOIN pg_roles u ON m.member = u.oid
		WHERE u.rolname = $1`,
		roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role memberships: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		info.InRoles = append(info.InRoles, role)
	}

	return &info, rows.Err()
}

// applyGrant applies a single grant to a role
func (a *Adapter) applyGrant(ctx context.Context, grantee string, grant types.GrantOptions) error {
	if grant.Database == "" {
		return fmt.Errorf("database is required for grant")
	}

	var queries []string

	// Database-level privileges
	if grant.Schema == "" && len(grant.Tables) == 0 {
		q, err := sqlbuilder.NewPg().Grant(grant.Privileges...).OnDatabase(grant.Database).To(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Schema-level privileges
	if grant.Schema != "" && len(grant.Tables) == 0 {
		q, err := sqlbuilder.NewPg().Grant(grant.Privileges...).OnSchema(grant.Schema).To(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Table-level privileges
	for _, table := range grant.Tables {
		b := sqlbuilder.NewPg().Grant(grant.Privileges...).OnTable(grant.Schema, table).To(grantee)
		if grant.WithGrantOption {
			b.WithGrantOption()
		}
		q, err := b.Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Sequence-level privileges
	for _, seq := range grant.Sequences {
		b := sqlbuilder.NewPg().Grant(grant.Privileges...).OnSequence(grant.Schema, seq).To(grantee)
		if grant.WithGrantOption {
			b.WithGrantOption()
		}
		q, err := b.Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Function-level privileges
	for _, fn := range grant.Functions {
		b := sqlbuilder.NewPg().Grant(grant.Privileges...).OnFunction(grant.Schema, fn).To(grantee)
		if grant.WithGrantOption {
			b.WithGrantOption()
		}
		q, err := b.Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Execute all queries on the target database
	for _, query := range queries {
		if err := a.execWithNewConnection(ctx, grant.Database, query); err != nil {
			return err
		}
	}

	return nil
}
