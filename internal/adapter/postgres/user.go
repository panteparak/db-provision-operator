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

// CreateUser creates a new PostgreSQL user (role with login)
func (a *Adapter) CreateUser(ctx context.Context, opts types.CreateUserOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	b := sqlbuilder.PgCreateRole(opts.Username).
		Login(true). // Users always get LOGIN
		Superuser(opts.Superuser).
		CreateDB(opts.CreateDB).
		CreateRoleOpt(opts.CreateRole).
		Inherit(opts.Inherit).
		Replication(opts.Replication).
		BypassRLS(opts.BypassRLS)

	if opts.Password != "" {
		b.WithPassword(opts.Password)
	}
	if opts.ConnectionLimit != 0 {
		b.ConnectionLimit(int(opts.ConnectionLimit))
	}
	if opts.ValidUntil != "" {
		b.ValidUntil(opts.ValidUntil)
	}
	if len(opts.InRoles) > 0 {
		b.InRoles(opts.InRoles...)
	}

	query := b.Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create user %s: %w", opts.Username, err)
	}

	// Set configuration parameters
	for param, value := range opts.ConfigParams {
		q := sqlbuilder.PgAlterRole(opts.Username).Set(param, value).Build()
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to set config param %s for user %s: %w", param, opts.Username, err)
		}
	}

	return nil
}

// DropUser drops an existing PostgreSQL user
func (a *Adapter) DropUser(ctx context.Context, username string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// First revoke all privileges and reassign owned objects
	// This is necessary before dropping a role
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(username)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(username)))

	query := sqlbuilder.PgDropRole(username).IfExists().Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user %s: %w", username, err)
	}

	return nil
}

// UserExists checks if a PostgreSQL user exists
func (a *Adapter) UserExists(ctx context.Context, username string) (bool, error) {
	pool, err := a.getPool()
	if err != nil {
		return false, err
	}

	var exists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// UpdateUser updates an existing PostgreSQL user
func (a *Adapter) UpdateUser(ctx context.Context, username string, opts types.UpdateUserOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	b := sqlbuilder.PgAlterRole(username)
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
	if opts.ConnectionLimit != nil {
		b.ConnectionLimit(int(*opts.ConnectionLimit))
		hasOpts = true
	}
	if opts.ValidUntil != nil {
		b.ValidUntil(*opts.ValidUntil)
		hasOpts = true
	}

	if hasOpts {
		query := b.Build()
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to update user %s: %w", username, err)
		}
	}

	// Handle role membership changes
	for _, role := range opts.InRoles {
		q, buildErr := sqlbuilder.NewPg().GrantRole(role).To(username).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build grant role query: %w", buildErr)
		}
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to grant role %s to user %s: %w", role, username, err)
		}
	}

	// Set configuration parameters
	for param, value := range opts.ConfigParams {
		q := sqlbuilder.PgAlterRole(username).Set(param, value).Build()
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to set config param %s for user %s: %w", param, username, err)
		}
	}

	return nil
}

// UpdatePassword updates a user's password
func (a *Adapter) UpdatePassword(ctx context.Context, username, password string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	query := sqlbuilder.PgAlterRole(username).WithPassword(password).Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update password for user %s: %w", username, err)
	}

	return nil
}

// GetUserInfo retrieves information about a PostgreSQL user
func (a *Adapter) GetUserInfo(ctx context.Context, username string) (*types.UserInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var info types.UserInfo
	var validUntil *string
	err = pool.QueryRow(ctx, `
		SELECT rolname, rolconnlimit, rolvaliduntil,
		       rolsuper, rolcreatedb, rolcreaterole,
		       rolinherit, rolcanlogin, rolreplication, rolbypassrls
		FROM pg_roles
		WHERE rolname = $1`,
		username).Scan(
		&info.Username, &info.ConnectionLimit, &validUntil,
		&info.Superuser, &info.CreateDB, &info.CreateRole,
		&info.Inherit, &info.Login, &info.Replication, &info.BypassRLS)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	if validUntil != nil {
		info.ValidUntil = *validUntil
	}

	// Get role memberships
	rows, err := pool.Query(ctx, `
		SELECT r.rolname
		FROM pg_roles r
		JOIN pg_auth_members m ON r.oid = m.roleid
		JOIN pg_roles u ON m.member = u.oid
		WHERE u.rolname = $1`,
		username)
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

// GetOwnedObjects returns all database objects owned by the specified user.
// This queries pg_class (for relations: tables, views, sequences, etc.) and
// pg_proc (for functions) to find objects owned by the user.
func (a *Adapter) GetOwnedObjects(ctx context.Context, username string) ([]types.OwnedObject, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var objects []types.OwnedObject

	// Query relations (tables, views, sequences, indexes, etc.)
	relQuery := `
		SELECT n.nspname, c.relname,
		       CASE c.relkind
		         WHEN 'r' THEN 'table'
		         WHEN 'S' THEN 'sequence'
		         WHEN 'v' THEN 'view'
		         WHEN 'm' THEN 'materialized view'
		         WHEN 'i' THEN 'index'
		         WHEN 'f' THEN 'foreign table'
		         WHEN 'p' THEN 'partitioned table'
		         WHEN 'c' THEN 'composite type'
		         ELSE c.relkind::text
		       END as reltype
		FROM pg_class c
		JOIN pg_roles r ON c.relowner = r.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE r.rolname = $1
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		  AND c.relkind IN ('r', 'S', 'v', 'm', 'f', 'p')
		ORDER BY n.nspname, c.relname`

	rows, err := pool.Query(ctx, relQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to query owned relations: %w", err)
	}

	for rows.Next() {
		var obj types.OwnedObject
		if err := rows.Scan(&obj.Schema, &obj.Name, &obj.Type); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan owned relation: %w", err)
		}
		objects = append(objects, obj)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating owned relations: %w", err)
	}

	// Query functions
	funcQuery := `
		SELECT n.nspname, p.proname, 'function'
		FROM pg_proc p
		JOIN pg_roles r ON p.proowner = r.oid
		JOIN pg_namespace n ON p.pronamespace = n.oid
		WHERE r.rolname = $1
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		ORDER BY n.nspname, p.proname`

	rows, err = pool.Query(ctx, funcQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to query owned functions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var obj types.OwnedObject
		if err := rows.Scan(&obj.Schema, &obj.Name, &obj.Type); err != nil {
			return nil, fmt.Errorf("failed to scan owned function: %w", err)
		}
		objects = append(objects, obj)
	}

	return objects, rows.Err()
}
