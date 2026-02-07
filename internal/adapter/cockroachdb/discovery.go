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

package cockroachdb

import (
	"context"
	"fmt"
)

// SystemDatabases are CockroachDB system databases that should be excluded from discovery.
// These databases are essential for CockroachDB operation and should not be managed by CRs.
var SystemDatabases = []string{
	"system",    // CockroachDB internal system database
	"defaultdb", // Default database for connections
}

// SystemUserPrefixes are prefixes for CockroachDB internal users/roles.
// Users starting with these prefixes are system users and should be excluded.
var SystemUserPrefixes = []string{
	"crdb_internal_", // CockroachDB internal roles
}

// SystemUsers are specific CockroachDB system users that should be excluded from discovery.
var SystemUsers = []string{
	"root",   // Default admin user
	"node",   // Node-to-node communication user
	"admin",  // Admin role (if exists)
	"public", // Public pseudo-role
}

// ListDatabases returns all user-created databases, excluding system databases.
// This method queries crdb_internal.databases to list all databases and filters out:
// - system (CockroachDB internal database)
// - defaultdb (default connection database)
func (a *Adapter) ListDatabases(ctx context.Context) ([]string, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Query all databases, excluding system databases
	// CockroachDB uses crdb_internal.databases for database metadata
	rows, err := pool.Query(ctx, `
		SELECT name
		FROM crdb_internal.databases
		WHERE name NOT IN ('system', 'defaultdb')
		ORDER BY name`)
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
// In CockroachDB, users are roles with LOGIN capability (similar to PostgreSQL).
// This method queries pg_roles to list all login-capable roles and filters out:
// - root (default admin user)
// - node (internal node user)
// - admin (admin role)
func (a *Adapter) ListUsers(ctx context.Context) ([]string, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Query all roles with LOGIN capability, excluding system roles
	// CockroachDB uses pg_catalog.pg_roles for PostgreSQL compatibility
	rows, err := pool.Query(ctx, `
		SELECT rolname
		FROM pg_catalog.pg_roles
		WHERE rolcanlogin = true
		  AND rolname NOT IN ('root', 'node', 'admin')
		  AND rolname NOT LIKE 'crdb_internal_%'
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
// In CockroachDB, roles without LOGIN capability are "group roles".
// This method queries pg_roles to list all non-login roles and filters out:
// - admin (system admin role)
// - public (pseudo-role)
// - crdb_internal_* (CockroachDB internal roles)
func (a *Adapter) ListRoles(ctx context.Context) ([]string, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Query all roles without LOGIN capability, excluding system roles
	rows, err := pool.Query(ctx, `
		SELECT rolname
		FROM pg_catalog.pg_roles
		WHERE rolcanlogin = false
		  AND rolname NOT IN ('admin', 'public')
		  AND rolname NOT LIKE 'crdb_internal_%'
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
