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
// Uses the clickhouse-go/v2 driver via the database/sql interface.
type ClickHouseVerifier struct {
	config EngineConfig
	db     *sql.DB
}

// NewClickHouseVerifier creates a new ClickHouse database verifier.
func NewClickHouseVerifier(cfg EngineConfig) *ClickHouseVerifier {
	return &ClickHouseVerifier{config: cfg}
}

// ClickHouseEngineConfig creates configuration for ClickHouse verification.
func ClickHouseEngineConfig(host string, port int32, username, password string) EngineConfig {
	return EngineConfig{
		Name:          "clickhouse",
		Host:          host,
		Port:          port,
		AdminDatabase: "default",
		Username:      username,
		Password:      password,
	}
}

// Connect establishes a connection to the ClickHouse database.
func (v *ClickHouseVerifier) Connect(ctx context.Context) error {
	dsn := fmt.Sprintf("clickhouse://%s:%d/%s?username=%s&password=%s",
		v.config.Host, v.config.Port, v.config.AdminDatabase,
		v.config.Username, v.config.Password)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	v.db = db
	return nil
}

// Close closes the database connection.
func (v *ClickHouseVerifier) Close() error {
	if v.db != nil {
		err := v.db.Close()
		v.db = nil
		return err
	}
	return nil
}

// DatabaseExists checks if a ClickHouse database exists.
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

// UserExists checks if a ClickHouse user exists.
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

// RoleExists checks if a ClickHouse role exists.
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
// Uses SHOW GRANTS FOR <grantee> and inspects the output.
func (v *ClickHouseVerifier) HasPrivilege(ctx context.Context, grantee, privilege, objectType, objectName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	rows, err := v.db.QueryContext(ctx, fmt.Sprintf("SHOW GRANTS FOR %s", grantee))
	if err != nil {
		return false, fmt.Errorf("failed to show grants for %s: %w", grantee, err)
	}
	defer rows.Close()

	for rows.Next() {
		var grant string
		if err := rows.Scan(&grant); err != nil {
			return false, fmt.Errorf("failed to scan grant row: %w", err)
		}
		upperGrant := strings.ToUpper(grant)
		if strings.Contains(upperGrant, strings.ToUpper(privilege)) &&
			(objectName == "" || strings.Contains(upperGrant, strings.ToUpper(objectName))) {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("error iterating grant rows: %w", err)
	}

	return false, nil
}

// HasSchemaPrivilege returns false for ClickHouse, which has no schema concept.
func (v *ClickHouseVerifier) HasSchemaPrivilege(_ context.Context, _, _ string, _ []string) (bool, error) {
	// ClickHouse does not have schemas in the PostgreSQL sense.
	return false, nil
}

// HasPrivilegeOnDatabase checks if a grantee has a privilege on an object in a specific database.
// Uses SHOW GRANTS FOR <grantee> and looks for database-scoped privilege entries.
func (v *ClickHouseVerifier) HasPrivilegeOnDatabase(ctx context.Context, grantee, privilege, objectType, objectName, database string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	rows, err := v.db.QueryContext(ctx, fmt.Sprintf("SHOW GRANTS FOR %s", grantee))
	if err != nil {
		return false, fmt.Errorf("failed to show grants for %s: %w", grantee, err)
	}
	defer rows.Close()

	for rows.Next() {
		var grant string
		if err := rows.Scan(&grant); err != nil {
			return false, fmt.Errorf("failed to scan grant row: %w", err)
		}
		upperGrant := strings.ToUpper(grant)
		if strings.Contains(upperGrant, strings.ToUpper(privilege)) &&
			strings.Contains(upperGrant, strings.ToUpper(database)) &&
			(objectName == "" || strings.Contains(upperGrant, strings.ToUpper(objectName))) {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("error iterating grant rows: %w", err)
	}

	return false, nil
}

// HasRoleMembership checks if a user has been granted a role.
// Uses SHOW GRANTS FOR <username> and looks for the role name in the output.
func (v *ClickHouseVerifier) HasRoleMembership(ctx context.Context, username, roleName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	rows, err := v.db.QueryContext(ctx, fmt.Sprintf("SHOW GRANTS FOR %s", username))
	if err != nil {
		return false, fmt.Errorf("failed to show grants for %s: %w", username, err)
	}
	defer rows.Close()

	for rows.Next() {
		var grant string
		if err := rows.Scan(&grant); err != nil {
			return false, fmt.Errorf("failed to scan grant row: %w", err)
		}
		// ClickHouse role membership appears as: GRANT <roleName> TO <username>
		upperGrant := strings.ToUpper(grant)
		if strings.Contains(upperGrant, strings.ToUpper(roleName)) {
			return true, nil
		}
	}

	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("error iterating grant rows: %w", err)
	}

	return false, nil
}

// ConnectAsUser creates a new database connection as a specific user.
func (v *ClickHouseVerifier) ConnectAsUser(ctx context.Context, username, password, database string) (UserConnection, error) {
	dsn := fmt.Sprintf("clickhouse://%s:%d/%s?username=%s&password=%s",
		v.config.Host, v.config.Port, database, username, password)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection as user %s: %w", username, err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping clickhouse as user %s: %w", username, err)
	}

	return &ClickHouseUserConnection{db: db}, nil
}

// GetDatabaseOwner returns an empty string because ClickHouse has no database ownership concept.
func (v *ClickHouseVerifier) GetDatabaseOwner(_ context.Context, _ string) (string, error) {
	// ClickHouse does not have a database ownership concept.
	return "", nil
}

// ClickHouseUserConnection implements UserConnection for ClickHouse.
type ClickHouseUserConnection struct {
	db *sql.DB
}

// Close closes the user connection.
func (c *ClickHouseUserConnection) Close() error {
	if c.db != nil {
		err := c.db.Close()
		c.db = nil
		return err
	}
	return nil
}

// Ping verifies the connection is alive.
func (c *ClickHouseUserConnection) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Exec executes a statement.
func (c *ClickHouseUserConnection) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

// Query executes a query and discards the result rows.
func (c *ClickHouseUserConnection) Query(ctx context.Context, query string, args ...interface{}) error {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	return rows.Close()
}

// CanCreateTable attempts to create a test table using the MergeTree engine.
func (c *ClickHouseUserConnection) CanCreateTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id    UInt64,
		name  String,
		created_at DateTime DEFAULT now()
	) ENGINE = MergeTree()
	ORDER BY id`, tableName)
	return c.Exec(ctx, query)
}

// CanInsertData attempts to insert data into a table.
func (c *ClickHouseUserConnection) CanInsertData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("INSERT INTO %s (id, name) VALUES (?, ?)", tableName)
	return c.Exec(ctx, query, uint64(1), "test_value")
}

// CanSelectData attempts to select data from a table.
func (c *ClickHouseUserConnection) CanSelectData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("SELECT id, name FROM %s LIMIT 1", tableName)
	return c.Query(ctx, query)
}

// CanDeleteData attempts to delete data from a table.
// ClickHouse uses ALTER TABLE ... DELETE (lightweight mutations) instead of direct DELETE.
func (c *ClickHouseUserConnection) CanDeleteData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("ALTER TABLE %s DELETE WHERE name = ?", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanDropTable attempts to drop a table.
func (c *ClickHouseUserConnection) CanDropTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	return c.Exec(ctx, query)
}

// Ensure ClickHouseVerifier implements DatabaseVerifier.
var _ DatabaseVerifier = (*ClickHouseVerifier)(nil)

// Ensure ClickHouseUserConnection implements UserConnection.
var _ UserConnection = (*ClickHouseUserConnection)(nil)
