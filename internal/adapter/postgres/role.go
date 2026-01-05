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
	"strings"

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateRole creates a new PostgreSQL role
func (a *Adapter) CreateRole(ctx context.Context, opts types.CreateRoleOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("CREATE ROLE ")
	sb.WriteString(escapeIdentifier(opts.RoleName))

	var roleOpts []string

	if opts.Login {
		roleOpts = append(roleOpts, "LOGIN")
	} else {
		roleOpts = append(roleOpts, "NOLOGIN")
	}

	if opts.Superuser {
		roleOpts = append(roleOpts, "SUPERUSER")
	} else {
		roleOpts = append(roleOpts, "NOSUPERUSER")
	}

	if opts.CreateDB {
		roleOpts = append(roleOpts, "CREATEDB")
	} else {
		roleOpts = append(roleOpts, "NOCREATEDB")
	}

	if opts.CreateRole {
		roleOpts = append(roleOpts, "CREATEROLE")
	} else {
		roleOpts = append(roleOpts, "NOCREATEROLE")
	}

	if opts.Inherit {
		roleOpts = append(roleOpts, "INHERIT")
	} else {
		roleOpts = append(roleOpts, "NOINHERIT")
	}

	if opts.Replication {
		roleOpts = append(roleOpts, "REPLICATION")
	} else {
		roleOpts = append(roleOpts, "NOREPLICATION")
	}

	if opts.BypassRLS {
		roleOpts = append(roleOpts, "BYPASSRLS")
	} else {
		roleOpts = append(roleOpts, "NOBYPASSRLS")
	}

	if len(opts.InRoles) > 0 {
		var roles []string
		for _, r := range opts.InRoles {
			roles = append(roles, escapeIdentifier(r))
		}
		roleOpts = append(roleOpts, fmt.Sprintf("IN ROLE %s", strings.Join(roles, ", ")))
	}

	if len(roleOpts) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(roleOpts, " "))
	}

	_, err = pool.Exec(ctx, sb.String())
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

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(roleName))
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

	var alterOpts []string

	if opts.Login != nil {
		if *opts.Login {
			alterOpts = append(alterOpts, "LOGIN")
		} else {
			alterOpts = append(alterOpts, "NOLOGIN")
		}
	}

	if opts.Superuser != nil {
		if *opts.Superuser {
			alterOpts = append(alterOpts, "SUPERUSER")
		} else {
			alterOpts = append(alterOpts, "NOSUPERUSER")
		}
	}

	if opts.CreateDB != nil {
		if *opts.CreateDB {
			alterOpts = append(alterOpts, "CREATEDB")
		} else {
			alterOpts = append(alterOpts, "NOCREATEDB")
		}
	}

	if opts.CreateRole != nil {
		if *opts.CreateRole {
			alterOpts = append(alterOpts, "CREATEROLE")
		} else {
			alterOpts = append(alterOpts, "NOCREATEROLE")
		}
	}

	if opts.Inherit != nil {
		if *opts.Inherit {
			alterOpts = append(alterOpts, "INHERIT")
		} else {
			alterOpts = append(alterOpts, "NOINHERIT")
		}
	}

	if opts.Replication != nil {
		if *opts.Replication {
			alterOpts = append(alterOpts, "REPLICATION")
		} else {
			alterOpts = append(alterOpts, "NOREPLICATION")
		}
	}

	if opts.BypassRLS != nil {
		if *opts.BypassRLS {
			alterOpts = append(alterOpts, "BYPASSRLS")
		} else {
			alterOpts = append(alterOpts, "NOBYPASSRLS")
		}
	}

	if len(alterOpts) > 0 {
		query := fmt.Sprintf("ALTER ROLE %s WITH %s",
			escapeIdentifier(roleName),
			strings.Join(alterOpts, " "))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to update role %s: %w", roleName, err)
		}
	}

	// Handle role membership changes
	for _, role := range opts.InRoles {
		query := fmt.Sprintf("GRANT %s TO %s", escapeIdentifier(role), escapeIdentifier(roleName))
		if _, err := pool.Exec(ctx, query); err != nil {
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
		queries = append(queries, fmt.Sprintf(
			"GRANT %s ON DATABASE %s TO %s",
			strings.Join(grant.Privileges, ", "),
			escapeIdentifier(grant.Database),
			escapeIdentifier(grantee)))
	}

	// Schema-level privileges
	if grant.Schema != "" && len(grant.Tables) == 0 {
		queries = append(queries, fmt.Sprintf(
			"GRANT %s ON SCHEMA %s TO %s",
			strings.Join(grant.Privileges, ", "),
			escapeIdentifier(grant.Schema),
			escapeIdentifier(grantee)))
	}

	// Table-level privileges
	for _, table := range grant.Tables {
		tableName := table
		if grant.Schema != "" {
			tableName = fmt.Sprintf("%s.%s", escapeIdentifier(grant.Schema), escapeIdentifier(table))
		} else {
			tableName = escapeIdentifier(table)
		}

		q := fmt.Sprintf("GRANT %s ON TABLE %s TO %s",
			strings.Join(grant.Privileges, ", "),
			tableName,
			escapeIdentifier(grantee))
		if grant.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Sequence-level privileges
	for _, seq := range grant.Sequences {
		seqName := seq
		if grant.Schema != "" {
			seqName = fmt.Sprintf("%s.%s", escapeIdentifier(grant.Schema), escapeIdentifier(seq))
		} else {
			seqName = escapeIdentifier(seq)
		}

		q := fmt.Sprintf("GRANT %s ON SEQUENCE %s TO %s",
			strings.Join(grant.Privileges, ", "),
			seqName,
			escapeIdentifier(grantee))
		if grant.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Function-level privileges
	for _, fn := range grant.Functions {
		fnName := fn
		if grant.Schema != "" {
			fnName = fmt.Sprintf("%s.%s", escapeIdentifier(grant.Schema), fn)
		}

		q := fmt.Sprintf("GRANT %s ON FUNCTION %s TO %s",
			strings.Join(grant.Privileges, ", "),
			fnName,
			escapeIdentifier(grantee))
		if grant.WithGrantOption {
			q += " WITH GRANT OPTION"
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
