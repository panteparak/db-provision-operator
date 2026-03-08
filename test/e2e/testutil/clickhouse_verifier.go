//go:build e2e

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

package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// ClickHouseVerifier implements DatabaseVerifier for ClickHouse.
// ClickHouse uses the clickhouse-go/v2 driver via database/sql interface.
type ClickHouseVerifier struct {
	config EngineConfig
	db     *sql.DB
}

// NewClickHouseVerifier creates a new ClickHouse database verifier
func NewClickHouseVerifier(cfg EngineConfig) *ClickHouseVerifier {
	return &ClickHouseVerifier{
		config: cfg,
	}
}

// Connect establishes a connection to the ClickHouse database
func (v *ClickHouseVerifier) Connect(ctx context.Context) error {
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s",
		v.config.Username, v.config.Password,
		v.config.Host, v.config.Port, v.config.AdminDatabase)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	v.db = db
	return nil
}

// Close closes the database connection
func (v *ClickHouseVerifier) Close() error {
	if v.db != nil {
		err := v.db.Close()
		v.db = nil
		return err
	}
	return nil
}

// DatabaseExists checks if a ClickHouse database exists
func (v *ClickHouseVerifier) DatabaseExists(ctx context.Context, name string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var count uint64
	err := v.db.QueryRowContext(ctx,
		"SELECT count() FROM system.databases WHERE name = ?", name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return count > 0, nil
}

// UserExists checks if a ClickHouse user exists
func (v *ClickHouseVerifier) UserExists(ctx context.Context, username string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var count uint64
	err := v.db.QueryRowContext(ctx,
		"SELECT count() FROM system.users WHERE name = ?", username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return count > 0, nil
}

// RoleExists checks if a ClickHouse role exists
func (v *ClickHouseVerifier) RoleExists(ctx context.Context, roleName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var count uint64
	err := v.db.QueryRowContext(ctx,
		"SELECT count() FROM system.roles WHERE name = ?", roleName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check role existence: %w", err)
	}

	return count > 0, nil
}

// HasPrivilege checks if a grantee has a specific privilege on an object.
// ClickHouse stores grants in system.grants table.
func (v *ClickHouseVerifier) HasPrivilege(ctx context.Context, grantee, privilege, objectType, objectName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	priv := strings.ToUpper(privilege)

	switch strings.ToLower(objectType) {
	case "database":
		return v.hasDatabasePrivilege(ctx, grantee, priv, objectName)
	case "table":
		return v.hasTablePrivilege(ctx, grantee, priv, objectName)
	case "global":
		return v.hasGlobalPrivilege(ctx, grantee, priv)
	default:
		return false, fmt.Errorf("unsupported object type: %s", objectType)
	}
}

func (v *ClickHouseVerifier) hasDatabasePrivilege(ctx context.Context, grantee, privilege, database string) (bool, error) {
	var count uint64
	err := v.db.QueryRowContext(ctx, `
		SELECT count() FROM system.grants
		WHERE (user_name = ? OR role_name = ?)
		AND access_type = ?
		AND database = ?`,
		grantee, grantee, privilege, database).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check database privilege: %w", err)
	}
	return count > 0, nil
}

func (v *ClickHouseVerifier) hasTablePrivilege(ctx context.Context, grantee, privilege, table string) (bool, error) {
	parts := strings.Split(table, ".")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid table format, expected database.table, got: %s", table)
	}
	database, tableName := parts[0], parts[1]

	var count uint64
	err := v.db.QueryRowContext(ctx, `
		SELECT count() FROM system.grants
		WHERE (user_name = ? OR role_name = ?)
		AND access_type = ?
		AND database = ?
		AND table = ?`,
		grantee, grantee, privilege, database, tableName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check table privilege: %w", err)
	}
	return count > 0, nil
}

func (v *ClickHouseVerifier) hasGlobalPrivilege(ctx context.Context, grantee, privilege string) (bool, error) {
	var count uint64
	err := v.db.QueryRowContext(ctx, `
		SELECT count() FROM system.grants
		WHERE (user_name = ? OR role_name = ?)
		AND access_type = ?
		AND database IS NULL`,
		grantee, grantee, privilege).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check global privilege: %w", err)
	}
	return count > 0, nil
}

// HasSchemaPrivilege checks if a grantee has privileges on a schema.
// ClickHouse has no schema concept — schemas map to databases.
func (v *ClickHouseVerifier) HasSchemaPrivilege(ctx context.Context, grantee, schema string, privileges []string) (bool, error) {
	for _, priv := range privileges {
		hasPriv, err := v.HasPrivilege(ctx, grantee, priv, "database", schema)
		if err != nil {
			return false, err
		}
		if !hasPriv {
			return false, nil
		}
	}
	return true, nil
}

// HasPrivilegeOnDatabase checks if a grantee has a privilege on a specific database.
// For ClickHouse, system.grants is globally accessible so no reconnection needed.
func (v *ClickHouseVerifier) HasPrivilegeOnDatabase(ctx context.Context, grantee, privilege, objectType, objectName, database string) (bool, error) {
	return v.HasPrivilege(ctx, grantee, privilege, objectType, objectName)
}

// HasRoleMembership checks if a user has been granted a role
func (v *ClickHouseVerifier) HasRoleMembership(ctx context.Context, username, roleName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var count uint64
	err := v.db.QueryRowContext(ctx, `
		SELECT count() FROM system.role_grants
		WHERE user_name = ? AND granted_role_name = ?`,
		username, roleName).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check role membership: %w", err)
	}

	return count > 0, nil
}

// ConnectAsUser creates a new database connection as a specific user
func (v *ClickHouseVerifier) ConnectAsUser(ctx context.Context, username, password, database string) (UserConnection, error) {
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s",
		username, password,
		v.config.Host, v.config.Port, database)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection as user %s: %w", username, err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database as user %s: %w", username, err)
	}

	return &ClickHouseUserConnection{db: db}, nil
}

// GetDatabaseOwner returns the owner of a ClickHouse database.
// ClickHouse does not have database ownership — return empty string.
func (v *ClickHouseVerifier) GetDatabaseOwner(_ context.Context, _ string) (string, error) {
	return "", nil
}

// ClickHouseUserConnection implements UserConnection for ClickHouse
type ClickHouseUserConnection struct {
	db *sql.DB
}

// Close closes the user connection
func (c *ClickHouseUserConnection) Close() error {
	if c.db != nil {
		err := c.db.Close()
		c.db = nil
		return err
	}
	return nil
}

// Ping verifies the connection is alive
func (c *ClickHouseUserConnection) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Exec executes a statement
func (c *ClickHouseUserConnection) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

// Query executes a query
func (c *ClickHouseUserConnection) Query(ctx context.Context, query string, args ...interface{}) error {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}

// CanCreateTable attempts to create a test table using MergeTree engine
func (c *ClickHouseUserConnection) CanCreateTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id UInt64,
		name String,
		created_at DateTime DEFAULT now()
	) ENGINE = MergeTree() ORDER BY id`, tableName)
	return c.Exec(ctx, query)
}

// CanInsertData attempts to insert data into a table
func (c *ClickHouseUserConnection) CanInsertData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (1, 'test_value')", tableName)
	return c.Exec(ctx, query)
}

// CanSelectData attempts to select data from a table
func (c *ClickHouseUserConnection) CanSelectData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("SELECT id, name FROM %s LIMIT 1", tableName)
	return c.Query(ctx, query)
}

// CanDeleteData attempts to delete data from a table.
// ClickHouse uses ALTER TABLE ... DELETE (lightweight delete) or
// DELETE FROM (since v22.8+).
func (c *ClickHouseUserConnection) CanDeleteData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE name = 'test_value'", tableName)
	return c.Exec(ctx, query)
}

// CanDropTable attempts to drop a table
func (c *ClickHouseUserConnection) CanDropTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	return c.Exec(ctx, query)
}

// Ensure ClickHouseVerifier implements DatabaseVerifier
var _ DatabaseVerifier = (*ClickHouseVerifier)(nil)

// Ensure ClickHouseUserConnection implements UserConnection
var _ UserConnection = (*ClickHouseUserConnection)(nil)
