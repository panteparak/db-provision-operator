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

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateDatabase creates a new MySQL database
func (a *Adapter) CreateDatabase(ctx context.Context, opts types.CreateDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	b := sqlbuilder.MySQLCreateDatabase(opts.Name)
	if opts.Charset != "" {
		b.Charset(opts.Charset)
	}
	if opts.Collation != "" {
		b.Collation(opts.Collation)
	}

	query := b.Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", opts.Name, err)
	}

	return nil
}

// DropDatabase drops an existing MySQL database
func (a *Adapter) DropDatabase(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Force drop: kill active connections first
	if opts.Force {
		killQuery := `
			SELECT CONCAT('KILL ', id, ';')
			FROM information_schema.processlist
			WHERE db = ?`
		rows, err := db.QueryContext(ctx, killQuery, name)
		if err == nil {
			defer func() { _ = rows.Close() }()
			for rows.Next() {
				var killCmd string
				if err := rows.Scan(&killCmd); err == nil {
					_, _ = db.ExecContext(ctx, killCmd)
				}
			}
		}
	}

	query := sqlbuilder.MySQLDropDatabase(name).IfExists().Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}

	return nil
}

// DatabaseExists checks if a MySQL database exists
func (a *Adapter) DatabaseExists(ctx context.Context, name string) (bool, error) {
	db, err := a.getDB()
	if err != nil {
		return false, err
	}

	var exists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = ?)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return exists, nil
}

// GetDatabaseInfo retrieves information about a MySQL database
func (a *Adapter) GetDatabaseInfo(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var info types.DatabaseInfo
	err = db.QueryRowContext(ctx, `
		SELECT schema_name, default_character_set_name, default_collation_name
		FROM information_schema.schemata
		WHERE schema_name = ?`,
		name).Scan(&info.Name, &info.Charset, &info.Collation)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	// Get database size
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(data_length + index_length), 0)
		FROM information_schema.tables
		WHERE table_schema = ?`,
		name).Scan(&info.SizeBytes)
	if err != nil {
		// Ignore error, size is optional
		info.SizeBytes = 0
	}

	return &info, nil
}

// VerifyDatabaseAccess verifies that a database is accepting connections.
// For MySQL, this simply verifies we can select from the database.
// Unlike PostgreSQL, MySQL databases are immediately available after creation.
func (a *Adapter) VerifyDatabaseAccess(ctx context.Context, name string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Use the database and execute a simple query
	_, err = db.ExecContext(ctx, fmt.Sprintf("USE %s", escapeIdentifier(name)))
	if err != nil {
		return fmt.Errorf("failed to access database %s: %w", name, err)
	}

	// Execute a simple query to verify the database is accessible
	_, err = db.ExecContext(ctx, "SELECT 1")
	if err != nil {
		return fmt.Errorf("failed to query database %s: %w", name, err)
	}

	// Switch back to the admin database
	_, _ = db.ExecContext(ctx, fmt.Sprintf("USE %s", escapeIdentifier(a.config.Database)))

	return nil
}

// UpdateDatabase updates MySQL database settings
func (a *Adapter) UpdateDatabase(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Update charset and collation if specified
	if opts.Charset != "" || opts.Collation != "" {
		b := sqlbuilder.MySQLAlterDatabase(name)
		if opts.Charset != "" {
			b.Charset(opts.Charset)
		}
		if opts.Collation != "" {
			b.Collation(opts.Collation)
		}

		query := b.Build()
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to update database %s: %w", name, err)
		}
	}

	return nil
}
