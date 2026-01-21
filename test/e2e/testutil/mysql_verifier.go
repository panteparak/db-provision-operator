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

	"github.com/go-sql-driver/mysql"
)

// init registers the MySQL driver - the blank import is handled by the driver itself
func init() {
	// Driver registered via mysql.init()
}

// MySQLVerifier implements DatabaseVerifier for MySQL
type MySQLVerifier struct {
	config EngineConfig
	db     *sql.DB
}

// NewMySQLVerifier creates a new MySQL database verifier
func NewMySQLVerifier(cfg EngineConfig) *MySQLVerifier {
	return &MySQLVerifier{
		config: cfg,
	}
}

// Connect establishes a connection to the MySQL database
func (v *MySQLVerifier) Connect(ctx context.Context) error {
	cfg := mysql.Config{
		User:                 v.config.Username,
		Passwd:               v.config.Password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", v.config.Host, v.config.Port),
		DBName:               v.config.AdminDatabase,
		AllowNativePasswords: true,
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	v.db = db
	return nil
}

// Close closes the database connection
func (v *MySQLVerifier) Close() error {
	if v.db != nil {
		err := v.db.Close()
		v.db = nil
		return err
	}
	return nil
}

// DatabaseExists checks if a MySQL database/schema exists
func (v *MySQLVerifier) DatabaseExists(ctx context.Context, name string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	var exists bool
	err := v.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}

	return exists, nil
}

// UserExists checks if a MySQL user exists
func (v *MySQLVerifier) UserExists(ctx context.Context, username string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// Extract just the username part (handle user@host format)
	user := username
	if idx := strings.Index(username, "@"); idx > 0 {
		user = username[:idx]
	}

	var exists bool
	err := v.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM mysql.user WHERE User = ?)",
		user).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// RoleExists checks if a MySQL role exists (MySQL 8.0+ treats roles as users with locked accounts)
func (v *MySQLVerifier) RoleExists(ctx context.Context, roleName string) (bool, error) {
	return v.UserExists(ctx, roleName)
}

// HasPrivilege checks if a grantee has a specific privilege on an object
func (v *MySQLVerifier) HasPrivilege(ctx context.Context, grantee, privilege, objectType, objectName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// Extract just the username part
	user := grantee
	if idx := strings.Index(grantee, "@"); idx > 0 {
		user = grantee[:idx]
	}

	priv := strings.ToUpper(privilege)

	switch strings.ToLower(objectType) {
	case "database":
		return v.hasDatabasePrivilege(ctx, user, priv, objectName)
	case "table":
		return v.hasTablePrivilege(ctx, user, priv, objectName)
	case "global":
		return v.hasGlobalPrivilege(ctx, user, priv)
	default:
		return false, fmt.Errorf("unsupported object type: %s", objectType)
	}
}

// hasDatabasePrivilege checks if user has a privilege on a database
func (v *MySQLVerifier) hasDatabasePrivilege(ctx context.Context, user, privilege, database string) (bool, error) {
	// Map common privilege names to MySQL column names
	privColumn := v.privilegeToColumn(privilege)
	if privColumn == "" {
		return false, fmt.Errorf("unsupported privilege: %s", privilege)
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM mysql.db WHERE User = ? AND Db = ? AND %s = 'Y')", privColumn)
	var exists bool
	err := v.db.QueryRowContext(ctx, query, user, database).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database privilege: %w", err)
	}

	return exists, nil
}

// hasTablePrivilege checks if user has a privilege on a table
func (v *MySQLVerifier) hasTablePrivilege(ctx context.Context, user, privilege, table string) (bool, error) {
	// Parse table name (format: database.table)
	parts := strings.Split(table, ".")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid table format, expected database.table, got: %s", table)
	}
	database, tableName := parts[0], parts[1]

	privColumn := v.privilegeToColumn(privilege)
	if privColumn == "" {
		return false, fmt.Errorf("unsupported privilege: %s", privilege)
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM mysql.tables_priv WHERE User = ? AND Db = ? AND Table_name = ? AND FIND_IN_SET(?, Table_priv) > 0)")
	var exists bool
	err := v.db.QueryRowContext(ctx, query, user, database, tableName, privilege).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check table privilege: %w", err)
	}

	return exists, nil
}

// hasGlobalPrivilege checks if user has a global privilege
func (v *MySQLVerifier) hasGlobalPrivilege(ctx context.Context, user, privilege string) (bool, error) {
	privColumn := v.privilegeToColumn(privilege)
	if privColumn == "" {
		return false, fmt.Errorf("unsupported privilege: %s", privilege)
	}

	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM mysql.user WHERE User = ? AND %s = 'Y')", privColumn)
	var exists bool
	err := v.db.QueryRowContext(ctx, query, user).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check global privilege: %w", err)
	}

	return exists, nil
}

// privilegeToColumn maps privilege names to MySQL system table column names
func (v *MySQLVerifier) privilegeToColumn(privilege string) string {
	mapping := map[string]string{
		"SELECT":           "Select_priv",
		"INSERT":           "Insert_priv",
		"UPDATE":           "Update_priv",
		"DELETE":           "Delete_priv",
		"CREATE":           "Create_priv",
		"DROP":             "Drop_priv",
		"RELOAD":           "Reload_priv",
		"SHUTDOWN":         "Shutdown_priv",
		"PROCESS":          "Process_priv",
		"FILE":             "File_priv",
		"REFERENCES":       "References_priv",
		"INDEX":            "Index_priv",
		"ALTER":            "Alter_priv",
		"SHOW DATABASES":   "Show_db_priv",
		"SUPER":            "Super_priv",
		"CREATE TEMPORARY": "Create_tmp_table_priv",
		"LOCK TABLES":      "Lock_tables_priv",
		"EXECUTE":          "Execute_priv",
		"REPLICATION SLAVE": "Repl_slave_priv",
		"REPLICATION CLIENT": "Repl_client_priv",
		"CREATE VIEW":      "Create_view_priv",
		"SHOW VIEW":        "Show_view_priv",
		"CREATE ROUTINE":   "Create_routine_priv",
		"ALTER ROUTINE":    "Alter_routine_priv",
		"CREATE USER":      "Create_user_priv",
		"EVENT":            "Event_priv",
		"TRIGGER":          "Trigger_priv",
		"GRANT":            "Grant_priv",
	}

	return mapping[strings.ToUpper(privilege)]
}

// HasSchemaPrivilege checks if a grantee has specific privileges on a schema (database in MySQL terms)
func (v *MySQLVerifier) HasSchemaPrivilege(ctx context.Context, grantee, schema string, privileges []string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// In MySQL, schemas and databases are the same thing
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

// HasRoleMembership checks if a user has been granted a role
// MySQL 8.0+ uses mysql.role_edges, MariaDB uses mysql.roles_mapping
func (v *MySQLVerifier) HasRoleMembership(ctx context.Context, username, roleName string) (bool, error) {
	if v.db == nil {
		return false, fmt.Errorf("not connected to database")
	}

	// Extract just the username part
	user := username
	if idx := strings.Index(username, "@"); idx > 0 {
		user = username[:idx]
	}
	role := roleName
	if idx := strings.Index(roleName, "@"); idx > 0 {
		role = roleName[:idx]
	}

	var exists bool
	var err error

	if v.config.Name == "mariadb" {
		// MariaDB uses mysql.roles_mapping table
		// Structure: User, Host, Role, Admin_option
		err = v.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM mysql.roles_mapping
				WHERE User = ? AND Role = ?
			)`, user, role).Scan(&exists)
	} else {
		// MySQL 8.0+ uses mysql.role_edges table
		// Structure: FROM_USER, FROM_HOST, TO_USER, TO_HOST
		// FROM_USER is the role, TO_USER is the grantee
		err = v.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM mysql.role_edges
				WHERE TO_USER = ? AND FROM_USER = ?
			)`, user, role).Scan(&exists)
	}

	if err != nil {
		return false, fmt.Errorf("failed to check role membership: %w", err)
	}

	return exists, nil
}

// HasPrivilegeOnDatabase checks if a grantee has a privilege on a specific database
// For MySQL, this is the same as HasPrivilege since we can check from any connection
func (v *MySQLVerifier) HasPrivilegeOnDatabase(ctx context.Context, grantee, privilege, objectType, objectName, database string) (bool, error) {
	// For MySQL, we can check privileges from any connection since grants are stored in mysql.db
	// Just use the regular HasPrivilege method
	return v.HasPrivilege(ctx, grantee, privilege, objectType, objectName)
}

// ConnectAsUser creates a new database connection as a specific user
// This allows testing that generated credentials actually work
func (v *MySQLVerifier) ConnectAsUser(ctx context.Context, username, password, database string) (UserConnection, error) {
	cfg := mysql.Config{
		User:                 username,
		Passwd:               password,
		Net:                  "tcp",
		Addr:                 fmt.Sprintf("%s:%d", v.config.Host, v.config.Port),
		DBName:               database,
		AllowNativePasswords: true,
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open connection as user %s: %w", username, err)
	}

	// Verify connection works
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database as user %s: %w", username, err)
	}

	return &MySQLUserConnection{db: db}, nil
}

// GetDatabaseOwner returns the "owner" of a MySQL database
// MySQL doesn't have database ownership like PostgreSQL, so we return
// the user with the most privileges (ALL PRIVILEGES) on the database
func (v *MySQLVerifier) GetDatabaseOwner(ctx context.Context, database string) (string, error) {
	if v.db == nil {
		return "", fmt.Errorf("not connected to database")
	}

	// MySQL doesn't have a direct concept of database ownership
	// We look for users who have ALL PRIVILEGES on the database
	var user string
	err := v.db.QueryRowContext(ctx, `
		SELECT DISTINCT User FROM mysql.db
		WHERE Db = ?
		AND Select_priv = 'Y'
		AND Insert_priv = 'Y'
		AND Update_priv = 'Y'
		AND Delete_priv = 'Y'
		AND Create_priv = 'Y'
		AND Drop_priv = 'Y'
		LIMIT 1`,
		database).Scan(&user)
	if err != nil {
		if err == sql.ErrNoRows {
			// No user with ALL privileges, check for root or admin at global level
			return "root", nil
		}
		return "", fmt.Errorf("failed to get database owner: %w", err)
	}

	return user, nil
}

// MySQLUserConnection implements UserConnection for MySQL
type MySQLUserConnection struct {
	db *sql.DB
}

// Close closes the user connection
func (c *MySQLUserConnection) Close() error {
	if c.db != nil {
		err := c.db.Close()
		c.db = nil
		return err
	}
	return nil
}

// Ping verifies the connection is alive
func (c *MySQLUserConnection) Ping(ctx context.Context) error {
	return c.db.PingContext(ctx)
}

// Exec executes a statement
func (c *MySQLUserConnection) Exec(ctx context.Context, query string, args ...interface{}) error {
	_, err := c.db.ExecContext(ctx, query, args...)
	return err
}

// Query executes a query
func (c *MySQLUserConnection) Query(ctx context.Context, query string, args ...interface{}) error {
	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}

// CanCreateTable attempts to create a test table
func (c *MySQLUserConnection) CanCreateTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(100),
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`, tableName)
	return c.Exec(ctx, query)
}

// CanInsertData attempts to insert data into a table
func (c *MySQLUserConnection) CanInsertData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("INSERT INTO %s (name) VALUES (?)", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanSelectData attempts to select data from a table
func (c *MySQLUserConnection) CanSelectData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("SELECT id, name FROM %s LIMIT 1", tableName)
	return c.Query(ctx, query)
}

// CanDeleteData attempts to delete data from a table
func (c *MySQLUserConnection) CanDeleteData(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE name = ?", tableName)
	return c.Exec(ctx, query, "test_value")
}

// CanDropTable attempts to drop a table
func (c *MySQLUserConnection) CanDropTable(ctx context.Context, tableName string) error {
	query := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	return c.Exec(ctx, query)
}

// Ensure MySQLVerifier implements DatabaseVerifier
var _ DatabaseVerifier = (*MySQLVerifier)(nil)

// Ensure MySQLUserConnection implements UserConnection
var _ UserConnection = (*MySQLUserConnection)(nil)
