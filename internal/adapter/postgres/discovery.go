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
)

// SystemDatabases are PostgreSQL system databases that should be excluded from discovery.
// These databases are essential for PostgreSQL operation and should not be managed by CRs.
var SystemDatabases = []string{
	"postgres",  // Default admin database
	"template0", // Pristine template (cannot be modified)
	"template1", // Default template for new databases
}

// SystemUserPrefixes are prefixes for PostgreSQL internal users/roles.
// Roles starting with these prefixes are system roles and should be excluded.
var SystemUserPrefixes = []string{
	"pg_", // Internal PostgreSQL roles (pg_read_all_data, pg_write_all_data, etc.)
}

// SystemUsers are specific PostgreSQL system users that should be excluded from discovery.
var SystemUsers = []string{
	"postgres", // Default superuser
}

// ListDatabases returns all user-created databases, excluding system databases.
// This method queries pg_database to list all databases and filters out:
// - template databases (template0, template1)
// - the postgres admin database
func (a *Adapter) ListDatabases(ctx context.Context) ([]string, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Query all non-template databases, excluding system databases
	rows, err := pool.Query(ctx, `
		SELECT datname
		FROM pg_database
		WHERE datistemplate = false
		  AND datname NOT IN ('postgres', 'template0', 'template1')
		ORDER BY datname`)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan database name: %w", err)
		}
		databases = append(databases, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating database rows: %w", err)
	}

	return databases, nil
}

// ListUsers returns all user-created database users, excluding system users.
// In PostgreSQL, users and roles are the same (roles with LOGIN capability are users).
// This method queries pg_roles to list all roles with LOGIN and filters out:
// - The postgres superuser
// - Internal roles starting with pg_ prefix
func (a *Adapter) ListUsers(ctx context.Context) ([]string, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Query all roles with LOGIN capability, excluding system roles
	// rolcanlogin = true indicates the role can log in (i.e., it's a "user")
	rows, err := pool.Query(ctx, `
		SELECT rolname
		FROM pg_roles
		WHERE rolcanlogin = true
		  AND rolname != 'postgres'
		  AND rolname NOT LIKE 'pg_%'
		ORDER BY rolname`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan user name: %w", err)
		}
		users = append(users, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	return users, nil
}

// ListRoles returns all user-created roles, excluding system roles.
// This method queries pg_roles to list all roles WITHOUT LOGIN and filters out:
// - Internal roles starting with pg_ prefix
// - PUBLIC (special pseudo-role)
func (a *Adapter) ListRoles(ctx context.Context) ([]string, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Query all roles without LOGIN capability, excluding system roles
	// rolcanlogin = false indicates the role is a "group role" (not a user)
	rows, err := pool.Query(ctx, `
		SELECT rolname
		FROM pg_roles
		WHERE rolcanlogin = false
		  AND rolname != 'PUBLIC'
		  AND rolname NOT LIKE 'pg_%'
		ORDER BY rolname`)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan role name: %w", err)
		}
		roles = append(roles, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating role rows: %w", err)
	}

	return roles, nil
}
