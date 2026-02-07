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

// CockroachDBVerifier implements DatabaseVerifier for CockroachDB.
// CockroachDB is PostgreSQL wire-protocol compatible, so we use the pgx driver.
// However, some queries differ from standard PostgreSQL (e.g., system tables).
type CockroachDBVerifier struct {
	config EngineConfig
	pool   *pgxpool.Pool
}

// NewCockroachDBVerifier creates a new CockroachDB database verifier
func NewCockroachDBVerifier(cfg EngineConfig) *CockroachDBVerifier {
	return &CockroachDBVerifier{
		config: cfg,
	}
}

// Connect establishes a connection to the CockroachDB database
func (v *CockroachDBVerifier) Connect(ctx context.Context) error {
	// CockroachDB uses PostgreSQL wire protocol
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(v.config.Username), url.QueryEscape(v.config.Password),
		v.config.Host, v.config.Port, v.config.AdminDatabase)

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
func (v *CockroachDBVerifier) Close() error {
	if v.pool != nil {
		v.pool.Close()
		v.pool = nil
	}
	return nil
}

// DatabaseExists checks if a CockroachDB database exists
func (v *CockroachDBVerifier) DatabaseExists(ctx context.Context, name string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// CockroachDB uses crdb_internal.databases or pg_database
	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return exists, nil
}

// UserExists checks if a CockroachDB user exists
func (v *CockroachDBVerifier) UserExists(ctx context.Context, username string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// CockroachDB stores users in system.users
	var exists bool
	err := v.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM system.users WHERE username = $1)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// RoleExists checks if a CockroachDB role exists
// In CockroachDB, users and roles are the same concept
func (v *CockroachDBVerifier) RoleExists(ctx context.Context, roleName string) (bool, error) {
	return v.UserExists(ctx, roleName)
}

// HasPrivilege checks if a grantee has a specific privilege on an object
func (v *CockroachDBVerifier) HasPrivilege(ctx context.Context, grantee, privilege, objectType, objectName string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var hasPriv bool
	var err error

	// CockroachDB requires explicit type casts for has_*_privilege functions
	// because it cannot infer parameter types for these built-in functions (SQLSTATE 42P18)
	switch strings.ToLower(objectType) {
	case "database":
		err = v.pool.QueryRow(ctx,
			"SELECT has_database_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "schema":
		err = v.pool.QueryRow(ctx,
			"SELECT has_schema_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "table":
		err = v.pool.QueryRow(ctx,
			"SELECT has_table_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "sequence":
		err = v.pool.QueryRow(ctx,
			"SELECT has_sequence_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
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
func (v *CockroachDBVerifier) HasSchemaPrivilege(ctx context.Context, grantee, schema string, privileges []string) (bool, error) {
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
func (v *CockroachDBVerifier) HasPrivilegeOnDatabase(ctx context.Context, grantee, privilege, objectType, objectName, database string) (bool, error) {
	// Create a new connection to the specific database
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(v.config.Username), url.QueryEscape(v.config.Password),
		v.config.Host, v.config.Port, database)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return false, fmt.Errorf("failed to connect to database %s: %w", database, err)
	}
	defer pool.Close()

	var hasPriv bool

	// CockroachDB requires explicit type casts for has_*_privilege functions
	// because it cannot infer parameter types for these built-in functions (SQLSTATE 42P18)
	switch strings.ToLower(objectType) {
	case "database":
		err = pool.QueryRow(ctx,
			"SELECT has_database_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "schema":
		err = pool.QueryRow(ctx,
			"SELECT has_schema_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "table":
		err = pool.QueryRow(ctx,
			"SELECT has_table_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
			grantee, objectName, privilege).Scan(&hasPriv)
	case "sequence":
		err = pool.QueryRow(ctx,
			"SELECT has_sequence_privilege($1::TEXT, $2::TEXT, $3::TEXT)",
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
func (v *CockroachDBVerifier) HasRoleMembership(ctx context.Context, username, roleName string) (bool, error) {
	if v.pool == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// CockroachDB uses system.role_members
	var exists bool
	err := v.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM system.role_members
			WHERE role = $1 AND member = $2
		)`, roleName, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role membership: %w", err)
	}

	return exists, nil
}

// ConnectAsUser creates a new database connection as a specific user
func (v *CockroachDBVerifier) ConnectAsUser(ctx context.Context, username, password, database string) (UserConnection, error) {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(username), url.QueryEscape(password),
		v.config.Host, v.config.Port, database)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect as user %s: %w", username, err)
	}

	// Verify connection works
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database as user %s: %w", username, err)
	}

	return &CockroachDBUserConnection{pool: pool}, nil
}

// GetDatabaseOwner returns the owner of a CockroachDB database
func (v *CockroachDBVerifier) GetDatabaseOwner(ctx context.Context, database string) (string, error) {
	if v.pool == nil {
		return "", fmt.Errorf("not connected to database")
	}

	// CockroachDB stores database ownership in crdb_internal.databases
	var owner string
	err := v.pool.QueryRow(ctx,
		"SELECT owner FROM crdb_internal.databases WHERE name = $1",
		database).Scan(&owner)
	if err != nil {
		return "", fmt.Errorf("failed to get database owner: %w", err)
	}

	return owner, nil
}

// CockroachDBUserConnection implements UserConnection for CockroachDB
type CockroachDBUserConnection struct {
	pool *pgxpool.Pool
}

// Close closes the user connection
func (c *CockroachDBUserConnection) Close() error {
	if c.pool != nil {
		c.pool.Close()
		c.pool = nil
	}
	return nil
}

// Ping verifies the connection is alive
func (c *CockroachDBUserConnection) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// Exec executes a statement
func (c *CockroachDBUserConnection) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := c.pool.Exec(ctx, query, args...)
	return err
}

// Query executes a query
func (c *CockroachDBUserConnection) Query(ctx context.Context, query string, args ...interface{}) error {
	rows, err := c.pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	rows.Close()
	return nil
}

// CanCreateTable attempts to create a test table
func (c *CockroachDBUserConnection) CanCreateTable(ctx context.Context, tableName string) error {
	// CockroachDB uses SERIAL which is INT with auto-generation
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100),
		created_at TIMESTAMP DEFAULT current_timestamp()
	)`, tableName)
	return c.Exec(ctx, query)
}

// CanInsertData attempts to insert data into a table
func (c *CockroachDBUserConnection) CanInsertData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("INSERT INTO %s (name) VALUES ($1)", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanSelectData attempts to select data from a table
func (c *CockroachDBUserConnection) CanSelectData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("SELECT id, name FROM %s LIMIT 1", tableName)
	return c.Query(ctx, query)
}

// CanDeleteData attempts to delete data from a table
func (c *CockroachDBUserConnection) CanDeleteData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE name = $1", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanDropTable attempts to drop a table
func (c *CockroachDBUserConnection) CanDropTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	return c.Exec(ctx, query)
}

// Ensure CockroachDBVerifier implements DatabaseVerifier
var _ DatabaseVerifier = (*CockroachDBVerifier)(nil)

// Ensure CockroachDBUserConnection implements UserConnection
var _ UserConnection = (*CockroachDBUserConnection)(nil)
