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

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateDatabase creates a new ClickHouse database
func (a *Adapter) CreateDatabase(ctx context.Context, opts types.CreateDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	b := sqlbuilder.ClickHouseCreateDatabase(opts.Name)
	if opts.Engine != "" {
		b.Engine(opts.Engine)
	}
	if opts.Comment != "" {
		b.Comment(opts.Comment)
	}

	query := b.Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", opts.Name, err)
	}

	return nil
}

// DropDatabase drops an existing ClickHouse database
func (a *Adapter) DropDatabase(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Force drop: kill active queries on this database first
	if opts.Force {
		killQuery := fmt.Sprintf(
			"KILL QUERY WHERE current_database = %s",
			escapeLiteral(name))
		_, _ = db.ExecContext(ctx, killQuery)
	}

	query := sqlbuilder.ClickHouseDropDatabase(name).IfExists().Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}

	return nil
}

// DatabaseExists checks if a ClickHouse database exists
func (a *Adapter) DatabaseExists(ctx context.Context, name string) (bool, error) {
	db, err := a.getDB()
	if err != nil {
		return false, err
	}

	var count uint64
	err = db.QueryRowContext(ctx,
		"SELECT count() FROM system.databases WHERE name = ?",
		name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return count > 0, nil
}

// GetDatabaseInfo retrieves information about a ClickHouse database
func (a *Adapter) GetDatabaseInfo(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var info types.DatabaseInfo
	err = db.QueryRowContext(ctx, `
		SELECT name, engine, comment
		FROM system.databases
		WHERE name = ?`,
		name).Scan(&info.Name, &info.Engine, &info.Comment)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	// Get database size from system.parts
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(sum(bytes_on_disk), 0)
		FROM system.parts
		WHERE database = ?`,
		name).Scan(&info.SizeBytes)
	if err != nil {
		// Ignore error, size is optional
		info.SizeBytes = 0
	}

	return &info, nil
}

// VerifyDatabaseAccess verifies that a database is accessible.
// ClickHouse databases are immediately available after creation.
func (a *Adapter) VerifyDatabaseAccess(ctx context.Context, name string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Check that the database exists and is queryable
	query := fmt.Sprintf("SELECT 1 FROM %s.system FORMAT Null", escapeIdentifier(name))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		// Fallback: just check that the database exists in system.databases
		var count uint64
		err2 := db.QueryRowContext(ctx,
			"SELECT count() FROM system.databases WHERE name = ?",
			name).Scan(&count)
		if err2 != nil || count == 0 {
			return fmt.Errorf("failed to access database %s: %w", name, err)
		}
	}

	return nil
}

// TransferDatabaseOwnership is a no-op for ClickHouse as it does not support database ownership.
func (a *Adapter) TransferDatabaseOwnership(_ context.Context, _, _ string) error {
	return nil
}

// UpdateDatabase updates ClickHouse database settings
func (a *Adapter) UpdateDatabase(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// ClickHouse only supports modifying the comment via ALTER DATABASE
	if opts.Comment != "" {
		b := sqlbuilder.ClickHouseAlterDatabase(name)
		b.Comment(opts.Comment)
		query := b.Build()
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to update database %s: %w", name, err)
		}
	}

	return nil
}
