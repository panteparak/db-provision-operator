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

// CreateDatabase creates a new PostgreSQL database
func (a *Adapter) CreateDatabase(ctx context.Context, opts types.CreateDatabaseOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// Build CREATE DATABASE statement
	var sb strings.Builder
	sb.WriteString("CREATE DATABASE ")
	sb.WriteString(escapeIdentifier(opts.Name))

	// Add options
	var options []string

	if opts.Owner != "" {
		options = append(options, fmt.Sprintf("OWNER = %s", escapeIdentifier(opts.Owner)))
	}
	if opts.Template != "" {
		options = append(options, fmt.Sprintf("TEMPLATE = %s", escapeIdentifier(opts.Template)))
	}
	if opts.Encoding != "" {
		options = append(options, fmt.Sprintf("ENCODING = %s", escapeLiteral(opts.Encoding)))
	}
	if opts.LCCollate != "" {
		options = append(options, fmt.Sprintf("LC_COLLATE = %s", escapeLiteral(opts.LCCollate)))
	}
	if opts.LCCtype != "" {
		options = append(options, fmt.Sprintf("LC_CTYPE = %s", escapeLiteral(opts.LCCtype)))
	}
	if opts.Tablespace != "" {
		options = append(options, fmt.Sprintf("TABLESPACE = %s", escapeIdentifier(opts.Tablespace)))
	}
	if opts.ConnectionLimit != 0 {
		options = append(options, fmt.Sprintf("CONNECTION LIMIT = %d", opts.ConnectionLimit))
	}
	if opts.IsTemplate {
		options = append(options, "IS_TEMPLATE = TRUE")
	}
	if !opts.AllowConnections {
		options = append(options, "ALLOW_CONNECTIONS = FALSE")
	}

	if len(options) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(options, " "))
	}

	_, err = pool.Exec(ctx, sb.String())
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", opts.Name, err)
	}

	return nil
}

// DropDatabase drops an existing PostgreSQL database
func (a *Adapter) DropDatabase(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// Force drop: terminate active connections first
	if opts.Force {
		terminateQuery := `
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()`
		_, err = pool.Exec(ctx, terminateQuery, name)
		if err != nil {
			return fmt.Errorf("failed to terminate connections to database %s: %w", name, err)
		}
	}

	query := fmt.Sprintf("DROP DATABASE IF EXISTS %s", escapeIdentifier(name))
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}

	return nil
}

// DatabaseExists checks if a PostgreSQL database exists
func (a *Adapter) DatabaseExists(ctx context.Context, name string) (bool, error) {
	pool, err := a.getPool()
	if err != nil {
		return false, err
	}

	var exists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return exists, nil
}

// GetDatabaseInfo retrieves information about a PostgreSQL database
func (a *Adapter) GetDatabaseInfo(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	// Get basic database info
	var info types.DatabaseInfo
	var ownerOid int64
	err = pool.QueryRow(ctx, `
		SELECT d.datname, d.datdba, pg_database_size(d.datname),
		       pg_encoding_to_char(d.encoding), d.datcollate
		FROM pg_database d
		WHERE d.datname = $1`,
		name).Scan(&info.Name, &ownerOid, &info.SizeBytes, &info.Encoding, &info.Collation)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	// Get owner name
	err = pool.QueryRow(ctx,
		"SELECT rolname FROM pg_roles WHERE oid = $1",
		ownerOid).Scan(&info.Owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get database owner: %w", err)
	}

	// Get extensions (requires connection to the specific database)
	extensions, err := a.getDatabaseExtensions(ctx, name)
	if err != nil {
		// Log but don't fail - extensions query requires connection to the database
		extensions = nil
	}
	info.Extensions = extensions

	// Get schemas (requires connection to the specific database)
	schemas, err := a.getDatabaseSchemas(ctx, name)
	if err != nil {
		// Log but don't fail
		schemas = nil
	}
	info.Schemas = schemas

	return &info, nil
}

// getDatabaseExtensions gets extensions installed in a database
func (a *Adapter) getDatabaseExtensions(ctx context.Context, database string) ([]types.ExtensionInfo, error) {
	rows, cleanup, err := a.queryWithNewConnection(ctx, database,
		"SELECT extname, extversion FROM pg_extension WHERE extname != 'plpgsql'")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var extensions []types.ExtensionInfo
	for rows.Next() {
		var ext types.ExtensionInfo
		if err := rows.Scan(&ext.Name, &ext.Version); err != nil {
			return nil, err
		}
		extensions = append(extensions, ext)
	}

	return extensions, rows.Err()
}

// getDatabaseSchemas gets schemas in a database
func (a *Adapter) getDatabaseSchemas(ctx context.Context, database string) ([]string, error) {
	rows, cleanup, err := a.queryWithNewConnection(ctx, database, `
		SELECT nspname FROM pg_namespace
		WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		AND nspname NOT LIKE 'pg_temp_%'
		AND nspname NOT LIKE 'pg_toast_temp_%'`)
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

// UpdateDatabase updates database settings
func (a *Adapter) UpdateDatabase(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
	// Handle extensions
	for _, ext := range opts.Extensions {
		if err := a.installExtension(ctx, name, ext); err != nil {
			return fmt.Errorf("failed to install extension %s: %w", ext.Name, err)
		}
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

// installExtension installs a PostgreSQL extension
func (a *Adapter) installExtension(ctx context.Context, database string, ext types.ExtensionOptions) error {
	var sb strings.Builder
	sb.WriteString("CREATE EXTENSION IF NOT EXISTS ")
	sb.WriteString(escapeIdentifier(ext.Name))

	if ext.Schema != "" {
		sb.WriteString(" SCHEMA ")
		sb.WriteString(escapeIdentifier(ext.Schema))
	}
	if ext.Version != "" {
		sb.WriteString(" VERSION ")
		sb.WriteString(escapeLiteral(ext.Version))
	}

	return a.execWithNewConnection(ctx, database, sb.String())
}

// createSchema creates a PostgreSQL schema
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

// setDefaultPrivileges sets default privileges in a database
func (a *Adapter) setDefaultPrivileges(ctx context.Context, database string, dp types.DefaultPrivilegeOptions) error {
	var sb strings.Builder
	sb.WriteString("ALTER DEFAULT PRIVILEGES")

	if dp.Role != "" {
		sb.WriteString(" FOR ROLE ")
		sb.WriteString(escapeIdentifier(dp.Role))
	}

	if dp.Schema != "" {
		sb.WriteString(" IN SCHEMA ")
		sb.WriteString(escapeIdentifier(dp.Schema))
	}

	sb.WriteString(" GRANT ")
	sb.WriteString(strings.Join(dp.Privileges, ", "))
	sb.WriteString(" ON ")

	switch dp.ObjectType {
	case "tables":
		sb.WriteString("TABLES")
	case "sequences":
		sb.WriteString("SEQUENCES")
	case "functions":
		sb.WriteString("FUNCTIONS")
	case "types":
		sb.WriteString("TYPES")
	default:
		return fmt.Errorf("unsupported object type: %s", dp.ObjectType)
	}

	sb.WriteString(" TO ")
	sb.WriteString(escapeIdentifier(dp.Role))

	return a.execWithNewConnection(ctx, database, sb.String())
}
