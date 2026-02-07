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
)

// SystemDatabases are MySQL system databases that should be excluded from discovery.
// These databases are essential for MySQL operation and should not be managed by CRs.
var SystemDatabases = []string{
	"mysql",              // MySQL system database
	"information_schema", // SQL standard metadata
	"performance_schema", // Performance monitoring
	"sys",                // Performance Schema helper views
}

// SystemUserPrefixes are prefixes for MySQL internal users.
// Users starting with these prefixes are system users and should be excluded.
var SystemUserPrefixes = []string{
	"mysql.",      // MySQL internal users (mysql.session, mysql.sys, mysql.infoschema)
	"mariadb.",    // MariaDB internal users
	"debian-sys-", // Debian/Ubuntu system maintenance user
}

// SystemUsers are specific MySQL system users that should be excluded from discovery.
var SystemUsers = []string{
	"root",             // Default superuser
	"mysql.session",    // MySQL internal session user
	"mysql.sys",        // MySQL internal sys user
	"mysql.infoschema", // MySQL internal infoschema user
}

// ListDatabases returns all user-created databases, excluding system databases.
// This method queries INFORMATION_SCHEMA.SCHEMATA to list all databases and filters out:
// - mysql (system database)
// - information_schema (SQL standard metadata)
// - performance_schema (performance monitoring)
// - sys (performance schema helper views)
func (a *Adapter) ListDatabases(ctx context.Context) ([]string, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Query all databases, excluding system databases
	rows, err := db.QueryContext(ctx, `
		SELECT SCHEMA_NAME
		FROM INFORMATION_SCHEMA.SCHEMATA
		WHERE SCHEMA_NAME NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')
		ORDER BY SCHEMA_NAME`)
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
// This method queries mysql.user to list all users and filters out:
// - root (default superuser)
// - mysql.* (internal MySQL users)
// - debian-sys-maint (Debian/Ubuntu maintenance user)
func (a *Adapter) ListUsers(ctx context.Context) ([]string, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Query all users, excluding system users
	// We use DISTINCT to handle cases where the same user may exist for multiple hosts
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT User
		FROM mysql.user
		WHERE User != 'root'
		  AND User != ''
		  AND User NOT LIKE 'mysql.%'
		  AND User NOT LIKE 'mariadb.%'
		  AND User NOT LIKE 'debian-sys-%'
		ORDER BY User`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

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

// ListRoles returns all user-created roles.
// In MySQL 8.0+, roles are implemented as users without the ability to log in.
// For MySQL 5.7 and earlier, this returns an empty list as native roles don't exist.
// For MariaDB, roles are stored in mysql.user with is_role='Y'.
func (a *Adapter) ListRoles(ctx context.Context) ([]string, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Check if we're on MySQL 8.0+ or MariaDB by trying to query roles
	// First, try MariaDB style (is_role column)
	var roles []string

	// Try MariaDB-style role detection first
	mariaRows, mariaErr := db.QueryContext(ctx, `
		SELECT User
		FROM mysql.user
		WHERE is_role = 'Y'
		ORDER BY User`)

	if mariaErr == nil {
		defer func() { _ = mariaRows.Close() }()
		for mariaRows.Next() {
			var name string
			if err := mariaRows.Scan(&name); err != nil {
				return nil, fmt.Errorf("failed to scan role name: %w", err)
			}
			roles = append(roles, name)
		}
		if err := mariaRows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating role rows: %w", err)
		}
		return roles, nil
	}

	// Try MySQL 8.0+ style (users that are used as roles via role_edges)
	mysqlRows, mysqlErr := db.QueryContext(ctx, `
		SELECT DISTINCT FROM_USER as role_name
		FROM mysql.role_edges
		WHERE FROM_USER NOT LIKE 'mysql.%'
		ORDER BY role_name`)

	if mysqlErr == nil {
		defer func() { _ = mysqlRows.Close() }()
		for mysqlRows.Next() {
			var name string
			if err := mysqlRows.Scan(&name); err != nil {
				return nil, fmt.Errorf("failed to scan role name: %w", err)
			}
			roles = append(roles, name)
		}
		if err := mysqlRows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating role rows: %w", err)
		}
		return roles, nil
	}

	// If neither query worked, this is likely MySQL 5.7 or earlier without native roles
	// Return empty list as roles aren't supported
	return []string{}, nil
}
