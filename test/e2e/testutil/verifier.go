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
)

// DatabaseVerifier verifies actual database state (not just CR status)
// This interface allows E2E tests to validate that database operations
// actually succeeded by querying the database directly.
type DatabaseVerifier interface {
	// Connect establishes a connection to the database
	Connect(ctx context.Context) error

	// Close closes the database connection
	Close() error

	// DatabaseExists checks if a database with the given name exists
	DatabaseExists(ctx context.Context, name string) (bool, error)

	// UserExists checks if a user with the given username exists
	UserExists(ctx context.Context, username string) (bool, error)

	// RoleExists checks if a role with the given name exists
	RoleExists(ctx context.Context, roleName string) (bool, error)

	// HasPrivilege checks if a grantee has a specific privilege on an object
	// objectType can be: "database", "schema", "table", "sequence", "function"
	HasPrivilege(ctx context.Context, grantee, privilege, objectType, objectName string) (bool, error)

	// HasSchemaPrivilege checks if a grantee has privileges on a schema
	// This is a convenience method for schema-level grant verification
	HasSchemaPrivilege(ctx context.Context, grantee, schema string, privileges []string) (bool, error)

	// HasPrivilegeOnDatabase checks if a grantee has a privilege on a specific database
	// This is needed because PostgreSQL schema privileges are per-database
	HasPrivilegeOnDatabase(ctx context.Context, grantee, privilege, objectType, objectName, database string) (bool, error)

	// ===== Functionality Testing Methods =====

	// ConnectAsUser establishes a new connection as a specific user
	// Returns a UserConnection that can be used for testing operations
	ConnectAsUser(ctx context.Context, username, password, database string) (UserConnection, error)

	// GetDatabaseOwner returns the owner of a database
	GetDatabaseOwner(ctx context.Context, database string) (string, error)

	// HasRoleMembership checks if a user is a member of a role
	HasRoleMembership(ctx context.Context, username, roleName string) (bool, error)
}

// UserConnection represents a database connection as a specific user
// Used for testing that user credentials and permissions work correctly
type UserConnection interface {
	// Close closes the user connection
	Close() error

	// Ping verifies the connection is alive
	Ping(ctx context.Context) error

	// Exec executes a statement and returns error if it fails
	Exec(ctx context.Context, query string, args ...interface{}) error

	// Query executes a query and returns whether it succeeded
	Query(ctx context.Context, query string, args ...interface{}) error

	// CanCreateTable attempts to create a test table and returns success
	CanCreateTable(ctx context.Context, tableName string) error

	// CanInsertData attempts to insert into a table
	CanInsertData(ctx context.Context, tableName string) error

	// CanSelectData attempts to select from a table
	CanSelectData(ctx context.Context, tableName string) error

	// CanDeleteData attempts to delete from a table
	CanDeleteData(ctx context.Context, tableName string) error

	// CanDropTable attempts to drop a table
	CanDropTable(ctx context.Context, tableName string) error
}

// EngineConfig provides engine-specific configuration for database verification
type EngineConfig struct {
	Name          string // "postgres" or "mysql"
	Host          string // Database host
	Port          int32  // Database port
	AdminDatabase string // Admin database name ("postgres" or "mysql")
	Username      string // Admin username
	Password      string // Admin password
}

// PostgresEngineConfig creates configuration for PostgreSQL verification
func PostgresEngineConfig(host string, port int32, username, password string) EngineConfig {
	return EngineConfig{
		Name:          "postgres",
		Host:          host,
		Port:          port,
		AdminDatabase: "postgres",
		Username:      username,
		Password:      password,
	}
}

// MySQLEngineConfig creates configuration for MySQL verification
func MySQLEngineConfig(host string, port int32, username, password string) EngineConfig {
	return EngineConfig{
		Name:          "mysql",
		Host:          host,
		Port:          port,
		AdminDatabase: "mysql",
		Username:      username,
		Password:      password,
	}
}

// MariaDBEngineConfig creates configuration for MariaDB verification
// MariaDB is MySQL-compatible but uses different system tables for role management
func MariaDBEngineConfig(host string, port int32, username, password string) EngineConfig {
	return EngineConfig{
		Name:          "mariadb",
		Host:          host,
		Port:          port,
		AdminDatabase: "mysql",
		Username:      username,
		Password:      password,
	}
}
