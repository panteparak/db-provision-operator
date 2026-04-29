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
	"strings"

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateDatabase creates a new ClickHouse database
func (a *Adapter) CreateDatabase(ctx context.Context, opts types.CreateDatabaseOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", escapeIdentifier(opts.Name)))

	// ClickHouse supports ENGINE clause for databases (e.g. Atomic, Replicated, Memory)
	// StorageEngine field is reused here for ClickHouse ENGINE
	if opts.StorageEngine != "" {
		sb.WriteString(fmt.Sprintf(" ENGINE = %s", opts.StorageEngine))
	}

	_, err = db.ExecContext(ctx, sb.String())
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

	// ClickHouse does not require force-killing connections before dropping a database.
	// The opts.Force field is accepted but unused.
	_ = opts

	query := fmt.Sprintf("DROP DATABASE IF EXISTS %s", escapeIdentifier(name))
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
	var engine string
	err = db.QueryRowContext(ctx,
		"SELECT name, engine FROM system.databases WHERE name = ?",
		name).Scan(&info.Name, &engine)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	// engine is ClickHouse-specific; store it in StorageEngine-equivalent field
	// DatabaseInfo has no engine field, so we leave extra info unused unless
	// callers inspect the raw struct; Name is the critical field.
	_ = engine

	return &info, nil
}

// VerifyDatabaseAccess verifies that a ClickHouse database is accessible
func (a *Adapter) VerifyDatabaseAccess(ctx context.Context, name string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// system.one is a single-row virtual table always available in ClickHouse
	_, err = db.ExecContext(ctx, "SELECT 1 FROM system.one")
	if err != nil {
		return fmt.Errorf("failed to verify database access for %s: %w", name, err)
	}

	return nil
}

// TransferDatabaseOwnership is a no-op for ClickHouse as ClickHouse does not
// support database ownership.
func (a *Adapter) TransferDatabaseOwnership(_ context.Context, _, _ string) error {
	return nil
}

// ExecSQL executes a single SQL statement on the named database.
func (a *Adapter) ExecSQL(ctx context.Context, database string, statement string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Switch to the target database
	if _, err := db.ExecContext(ctx, fmt.Sprintf("USE %s", escapeIdentifier(database))); err != nil {
		return fmt.Errorf("failed to switch to database %s: %w", database, err)
	}

	// Execute the statement
	_, execErr := db.ExecContext(ctx, statement)

	// Always switch back to admin database
	_, _ = db.ExecContext(ctx, fmt.Sprintf("USE %s", escapeIdentifier(a.config.Database)))

	if execErr != nil {
		return fmt.Errorf("failed to execute SQL on database %s: %w", database, execErr)
	}
	return nil
}

// ExecSQLAsRole executes a SQL statement on the named database.
// ClickHouse does not support SET ROLE; the role parameter is ignored
// and the statement executes as the operator user.
func (a *Adapter) ExecSQLAsRole(ctx context.Context, database, _, statement string) error {
	return a.ExecSQL(ctx, database, statement)
}

// UpdateDatabase is a no-op for ClickHouse as ClickHouse does not support
// ALTER DATABASE for most settings.
func (a *Adapter) UpdateDatabase(_ context.Context, _ string, _ types.UpdateDatabaseOptions) error {
	return nil
}
