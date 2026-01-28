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
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresVerifier implements DatabaseVerifier for PostgreSQL
type PostgresVerifier struct {
	config EngineConfig
	pool   *pgxpool.Pool
}

// NewPostgresVerifier creates a new PostgreSQL database verifier
func NewPostgresVerifier(cfg EngineConfig) *PostgresVerifier {
	return &PostgresVerifier{
		config: cfg,
	}
}

// Connect establishes a connection to the PostgreSQL database
func (v *PostgresVerifier) Connect(ctx context.Context) error {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(v.config.Username), url.QueryEscape(v.config.Password), v.config.Host, v.config.Port, v.config.AdminDatabase)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	v.pool = pool
	return nil
}

// Close closes the database connection pool
func (v *PostgresVerifier) Close() error {
	if v.pool != nil {
		v.pool.Close()
		v.pool = nil
	}
	return nil
}

// DatabaseExists checks if a PostgreSQL database exists
func (v *PostgresVerifier) DatabaseExists(ctx context.Context, name string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return exists, nil
}

// UserExists checks if a PostgreSQL user/role exists
func (v *PostgresVerifier) UserExists(ctx context.Context, username string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// RoleExists checks if a PostgreSQL role exists (alias for UserExists since PostgreSQL treats both the same)
func (v *PostgresVerifier) RoleExists(ctx context.Context, roleName string) (bool, error) {
	return v.UserExists(ctx, roleName)
}

// HasPrivilege checks if a grantee has a specific privilege on an object
func (v *PostgresVerifier) HasPrivilege(ctx context.Context, grantee, privilege, objectType, objectName string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var hasPriv bool
	var err error

	switch strings.ToLower(objectType) {
	case "database":
		err = v.pool.QueryRow(ctx,
			"SELECT has_database_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "schema":
		err = v.pool.QueryRow(ctx,
			"SELECT has_schema_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "table":
		err = v.pool.QueryRow(ctx,
			"SELECT has_table_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "sequence":
		err = v.pool.QueryRow(ctx,
			"SELECT has_sequence_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "function":
		err = v.pool.QueryRow(ctx,
			"SELECT has_function_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	default:
		return false, fmt.Errorf("unsupported object type: %s", objectType)
	}

	if err != nil {
		return false, fmt.Errorf("failed to check privilege: %w", err)
	}

	return hasPriv, nil
}

// HasSchemaPrivilege checks if a grantee has specific privileges on a schema
func (v *PostgresVerifier) HasSchemaPrivilege(ctx context.Context, grantee, schema string, privileges []string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// Check each privilege
	for _, priv := range privileges {
		hasPriv, err := v.HasPrivilege(ctx, grantee, priv, "schema", schema)
		if err != nil {
			return false, err
		}
		if !hasPriv {
			return false, nil
		}
	}

	return true, nil
}

// HasPrivilegeOnDatabase checks if a grantee has a privilege on an object in a specific database
// This is necessary because PostgreSQL schema privileges are per-database
func (v *PostgresVerifier) HasPrivilegeOnDatabase(ctx context.Context, grantee, privilege, objectType, objectName, database string) (bool, error) {
	// Create a new connection to the specific database
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(v.config.Username), url.QueryEscape(v.config.Password), v.config.Host, v.config.Port, database)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return false, fmt.Errorf("failed to connect to database %s: %w", database, err)
	}
	defer pool.Close()

	var hasPriv bool

	switch strings.ToLower(objectType) {
	case "database":
		err = pool.QueryRow(ctx,
			"SELECT has_database_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "schema":
		err = pool.QueryRow(ctx,
			"SELECT has_schema_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "table":
		err = pool.QueryRow(ctx,
			"SELECT has_table_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "sequence":
		err = pool.QueryRow(ctx,
			"SELECT has_sequence_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "function":
		err = pool.QueryRow(ctx,
			"SELECT has_function_privilege($1, $2, $3)",
			grantee, objectName, privilege).Scan(&hasPriv)
	default:
		return false, fmt.Errorf("unsupported object type: %s", objectType)
	}

	if err != nil {
		return false, fmt.Errorf("failed to check privilege on database %s: %w", database, err)
	}

	return hasPriv, nil
}

// HasRoleMembership checks if a user is a member of a role
func (v *PostgresVerifier) HasRoleMembership(ctx context.Context, username, roleName string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var exists bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pg_auth_members am
			JOIN pg_roles r ON am.roleid = r.oid
			JOIN pg_roles m ON am.member = m.oid
			WHERE r.rolname = $1 AND m.rolname = $2
		)`, roleName, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role membership: %w", err)
	}

	return exists, nil
}

// ConnectAsUser creates a new database connection as a specific user
// This allows testing that generated credentials actually work
func (v *PostgresVerifier) ConnectAsUser(ctx context.Context, username, password, database string) (UserConnection, error) {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(username), url.QueryEscape(password), v.config.Host, v.config.Port, database)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect as user %s: %w", username, err)
	}

	// Verify connection works
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database as user %s: %w", username, err)
	}

	return &PostgresUserConnection{pool: pool}, nil
}

// GetDatabaseOwner returns the owner of a PostgreSQL database
func (v *PostgresVerifier) GetDatabaseOwner(ctx context.Context, database string) (string, error) {
	if v.pool == nil {
		return "", fmt.Errorf("not connected to database")
	}

	var owner string
	err := v.pool.QueryRow(ctx,
		"SELECT pg_catalog.pg_get_userbyid(d.datdba) FROM pg_catalog.pg_database d WHERE d.datname = $1",
		database).Scan(&owner)
	if err != nil {
		return "", fmt.Errorf("failed to get database owner: %w", err)
	}

	return owner, nil
}

// PostgresUserConnection implements UserConnection for PostgreSQL
type PostgresUserConnection struct {
	pool *pgxpool.Pool
}

// Close closes the user connection
func (c *PostgresUserConnection) Close() error {
	if c.pool != nil {
		c.pool.Close()
		c.pool = nil
	}
	return nil
}

// Ping verifies the connection is alive
func (c *PostgresUserConnection) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// Exec executes a statement
func (c *PostgresUserConnection) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := c.pool.Exec(ctx, query, args...)
	return err
}

// Query executes a query
func (c *PostgresUserConnection) Query(ctx context.Context, query string, args ...interface{}) error {
	rows, err := c.pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	rows.Close()
	return nil
}

// CanCreateTable attempts to create a test table
func (c *PostgresUserConnection) CanCreateTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`, tableName)
	return c.Exec(ctx, query)
}

// CanInsertData attempts to insert data into a table
func (c *PostgresUserConnection) CanInsertData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("INSERT INTO %s (name) VALUES ($1)", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanSelectData attempts to select data from a table
func (c *PostgresUserConnection) CanSelectData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("SELECT id, name FROM %s LIMIT 1", tableName)
	return c.Query(ctx, query)
}

// CanDeleteData attempts to delete data from a table
func (c *PostgresUserConnection) CanDeleteData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE name = $1", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanDropTable attempts to drop a table
func (c *PostgresUserConnection) CanDropTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	return c.Exec(ctx, query)
}

// QueryRow executes a query that returns a single row on a specific database.
// The dest parameter should be a pointer to scan the result into.
func (v *PostgresVerifier) QueryRow(ctx context.Context, database string, query string, dest interface{}) error {
	// If database is empty or same as admin database, use existing pool
	if database == "" || database == v.config.AdminDatabase {
		if v.pool == nil {
			return fmt.Errorf("not connected to database")
		}
		return v.pool.QueryRow(ctx, query).Scan(dest)
	}

	// Create a new connection to the specific database
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(v.config.Username), url.QueryEscape(v.config.Password),
		v.config.Host, v.config.Port, database)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to database %s: %w", database, err)
	}
	defer pool.Close()

	return pool.QueryRow(ctx, query).Scan(dest)
}

// Ensure PostgresVerifier implements DatabaseVerifier
var _ DatabaseVerifier = (*PostgresVerifier)(nil)

// Ensure PostgresUserConnection implements UserConnection
var _ UserConnection = (*PostgresUserConnection)(nil)
