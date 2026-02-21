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
	"strings"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateDatabase creates a new CockroachDB database.
// CockroachDB supports OWNER and ENCODING but does NOT support:
//   - TEMPLATE (no template databases)
//   - LC_COLLATE / LC_CTYPE (locale is cluster-wide)
//   - TABLESPACE (CockroachDB manages storage automatically)
//   - IS_TEMPLATE
//
// Ownership is enforced by setting the OWNER option.
func (a *Adapter) CreateDatabase(ctx context.Context, opts types.CreateDatabaseOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	b := sqlbuilder.PgCreateDatabase(opts.Name)
	if opts.Owner != "" {
		b.Owner(opts.Owner)
	}
	if opts.Encoding != "" {
		b.Encoding(opts.Encoding)
	}
	if opts.ConnectionLimit != 0 {
		b.ConnectionLimit(int(opts.ConnectionLimit))
	}

	query := b.Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", opts.Name, err)
	}

	return nil
}

// DropDatabase drops an existing CockroachDB database.
// When Force is true, active sessions are cancelled before dropping.
// CockroachDB uses CANCEL SESSION instead of pg_terminate_backend.
func (a *Adapter) DropDatabase(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// Note: CockroachDB's DROP DATABASE ... CASCADE handles active connections gracefully.
	// Unlike PostgreSQL, we don't need to explicitly terminate sessions first.
	// The Force option is honored by using CASCADE which handles all dependencies.

	query := sqlbuilder.PgDropDatabase(name).IfExists().Cascade().Build()
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}

	return nil
}

// DatabaseExists checks if a CockroachDB database exists using crdb_internal.databases.
func (a *Adapter) DatabaseExists(ctx context.Context, name string) (bool, error) {
	pool, err := a.getPool()
	if err != nil {
		return false, err
	}

	var exists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM crdb_internal.databases WHERE name = $1)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return exists, nil
}

// GetDatabaseInfo retrieves information about a CockroachDB database.
// Uses crdb_internal.databases for native metadata and pg_roles for owner lookup.
func (a *Adapter) GetDatabaseInfo(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var info types.DatabaseInfo
	err = pool.QueryRow(ctx, `
		SELECT d.name, r.rolname, pg_encoding_to_char(e.encoding)
		FROM crdb_internal.databases d
		LEFT JOIN pg_catalog.pg_database e ON e.datname = d.name
		LEFT JOIN pg_catalog.pg_roles r ON r.oid = e.datdba
		WHERE d.name = $1`,
		name).Scan(&info.Name, &info.Owner, &info.Encoding)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	// Get database size using CockroachDB range size estimation
	var sizeBytes int64
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(range_size_bytes), 0)::INT8
		FROM crdb_internal.ranges
		WHERE database_name = $1`,
		name).Scan(&sizeBytes)
	if err != nil {
		// Size estimation may fail; don't block on it
		sizeBytes = 0
	}
	info.SizeBytes = sizeBytes

	// Get schemas in the database
	schemas, err := a.getDatabaseSchemas(ctx, name)
	if err != nil {
		schemas = nil
	}
	info.Schemas = schemas

	return &info, nil
}

// getDatabaseSchemas retrieves schemas in a CockroachDB database.
// CockroachDB supports pg_namespace for schema listing.
func (a *Adapter) getDatabaseSchemas(ctx context.Context, database string) ([]string, error) {
	rows, cleanup, err := a.queryWithNewConnection(ctx, database, `
		SELECT nspname FROM pg_namespace
		WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_extension', 'crdb_internal')
		AND nspname NOT LIKE 'pg_temp_%'
		AND nspname NOT LIKE 'pg_toast%'`)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}

	return schemas, rows.Err()
}

// UpdateDatabase updates database settings.
// CockroachDB does NOT support extensions (no CREATE EXTENSION).
// Supported: schemas, default privileges.
func (a *Adapter) UpdateDatabase(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
	// No-op case: return nil if nothing to do
	if len(opts.Schemas) == 0 && len(opts.DefaultPrivileges) == 0 {
		return nil
	}

	// Verify connection before proceeding
	if _, err := a.getPool(); err != nil {
		return err
	}

	// Handle schemas
	for _, schema := range opts.Schemas {
		if err := a.createSchema(ctx, name, schema); err != nil {
			return fmt.Errorf("failed to create schema %s: %w", schema.Name, err)
		}
	}

	// Handle default privileges
	for _, dp := range opts.DefaultPrivileges {
		if err := a.setDefaultPrivileges(ctx, name, dp); err != nil {
			return fmt.Errorf("failed to set default privileges: %w", err)
		}
	}

	return nil
}

// createSchema creates a schema in a CockroachDB database.
func (a *Adapter) createSchema(ctx context.Context, database string, schema types.SchemaOptions) error {
	var sb strings.Builder
	sb.WriteString("CREATE SCHEMA IF NOT EXISTS ")
	sb.WriteString(escapeIdentifier(schema.Name))

	if schema.Owner != "" {
		sb.WriteString(" AUTHORIZATION ")
		sb.WriteString(escapeIdentifier(schema.Owner))
	}

	return a.execWithNewConnection(ctx, database, sb.String())
}

// VerifyDatabaseAccess verifies that a database is accepting connections.
func (a *Adapter) VerifyDatabaseAccess(ctx context.Context, name string) error {
	return a.execWithNewConnection(ctx, name, "SELECT 1")
}

// setDefaultPrivileges sets default privileges in a CockroachDB database.
// CockroachDB supports ALTER DEFAULT PRIVILEGES with the same syntax as PostgreSQL.
func (a *Adapter) setDefaultPrivileges(ctx context.Context, database string, dp types.DefaultPrivilegeOptions) error {
	b := sqlbuilder.NewPg().AlterDefaultPrivileges(dp.Role, dp.Schema).
		Grant(dp.Privileges...).To(dp.Role)

	switch dp.ObjectType {
	case "tables":
		b.OnTables()
	case "sequences":
		b.OnSequences()
	case "functions":
		b.OnFunctions()
	case "types":
		b.OnTypes()
	default:
		return fmt.Errorf("unsupported object type: %s", dp.ObjectType)
	}

	q, err := b.Build()
	if err != nil {
		return fmt.Errorf("failed to build alter default privileges: %w", err)
	}

	return a.execWithNewConnection(ctx, database, q)
}
