//go:build integration

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
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mariadb"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// AdminAccountConfig holds configuration for the operator admin account
type AdminAccountConfig struct {
	Username string
	Password string
}

// DatabaseContainer wraps testcontainers for different database types
type DatabaseContainer struct {
	container testcontainers.Container
	engine    string
	host      string
	port      int
	user      string
	password  string
	database  string
	mu        sync.Mutex
}

// DatabaseContainerConfig holds configuration for starting a database container
type DatabaseContainerConfig struct {
	Engine   string // "postgresql", "mysql", "mariadb"
	Image    string // optional, uses default if empty
	User     string
	Password string
	Database string
}

// StartDatabaseContainer starts a database container based on engine type
func StartDatabaseContainer(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	dc := &DatabaseContainer{
		engine:   cfg.Engine,
		user:     cfg.User,
		password: cfg.Password,
		database: cfg.Database,
	}

	switch cfg.Engine {
	case "postgresql", "postgres":
		return dc.startPostgres(ctx, cfg)
	case "mysql":
		return dc.startMySQL(ctx, cfg)
	case "mariadb":
		return dc.startMariaDB(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported engine: %s", cfg.Engine)
	}
}

func (dc *DatabaseContainer) startPostgres(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	image := cfg.Image
	if image == "" {
		image = "postgres:16-alpine"
	}

	container, err := postgres.Run(ctx,
		image,
		postgres.WithUsername(cfg.User),
		postgres.WithPassword(cfg.Password),
		postgres.WithDatabase(cfg.Database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres: %w", err)
	}

	dc.container = container
	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get postgres host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get postgres port: %w", err)
	}

	dc.host = host
	dc.port = port.Int()
	return dc, nil
}

func (dc *DatabaseContainer) startMySQL(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	image := cfg.Image
	if image == "" {
		image = "mysql:8"
	}

	container, err := mysql.Run(ctx,
		image,
		mysql.WithUsername(cfg.User),
		mysql.WithPassword(cfg.Password),
		mysql.WithDatabase(cfg.Database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start mysql: %w", err)
	}

	dc.container = container
	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get mysql host: %w", err)
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get mysql port: %w", err)
	}

	dc.host = host
	dc.port = port.Int()
	return dc, nil
}

func (dc *DatabaseContainer) startMariaDB(ctx context.Context, cfg DatabaseContainerConfig) (*DatabaseContainer, error) {
	image := cfg.Image
	if image == "" {
		image = "mariadb:11.2"
	}

	container, err := mariadb.Run(ctx,
		image,
		mariadb.WithUsername(cfg.User),
		mariadb.WithPassword(cfg.Password),
		mariadb.WithDatabase(cfg.Database),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start mariadb: %w", err)
	}

	dc.container = container
	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get mariadb host: %w", err)
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return nil, fmt.Errorf("failed to get mariadb port: %w", err)
	}

	dc.host = host
	dc.port = port.Int()
	return dc, nil
}

// Stop terminates the container
func (dc *DatabaseContainer) Stop(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.container != nil {
		return dc.container.Terminate(ctx)
	}
	return nil
}

// ConnectionInfo returns connection details
func (dc *DatabaseContainer) ConnectionInfo() (host string, port int, user, password, database string) {
	return dc.host, dc.port, dc.user, dc.password, dc.database
}

// Host returns the container host
func (dc *DatabaseContainer) Host() string {
	return dc.host
}

// Port returns the mapped container port
func (dc *DatabaseContainer) Port() int {
	return dc.port
}

// Engine returns the database engine type
func (dc *DatabaseContainer) Engine() string {
	return dc.engine
}

// waitForDatabaseReadyUnlocked waits for the database to be ready to accept connections
// with exponential backoff retry logic. This is an unlocked version for internal use
// when the caller already holds dc.mu.
func (dc *DatabaseContainer) waitForDatabaseReadyUnlocked(ctx context.Context, maxRetries int, initialDelay time.Duration) (*sql.DB, error) {
	var db *sql.DB
	var err error
	var dsn string

	switch dc.engine {
	case "postgresql", "postgres":
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			dc.host, dc.port, dc.user, dc.password, dc.database)
	case "mysql", "mariadb":
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			dc.user, dc.password, dc.host, dc.port, dc.database)
	default:
		return nil, fmt.Errorf("unsupported engine: %s", dc.engine)
	}

	delay := initialDelay
	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		driver := "pgx"
		if dc.engine == "mysql" || dc.engine == "mariadb" {
			driver = "mysql"
		}

		db, err = sql.Open(driver, dsn)
		if err != nil {
			time.Sleep(delay)
			delay = delay * 2
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			continue
		}

		// Try to ping the database
		if err = db.PingContext(ctx); err == nil {
			return db, nil
		}

		db.Close()
		time.Sleep(delay)
		delay = delay * 2
		if delay > 5*time.Second {
			delay = 5 * time.Second
		}
	}

	return nil, fmt.Errorf("database not ready after %d retries: %w", maxRetries, err)
}

// SetupPostgresAdminAccount creates a least-privilege admin account for PostgreSQL
// and updates the container's credentials to use this account
func (dc *DatabaseContainer) SetupPostgresAdminAccount(ctx context.Context, cfg AdminAccountConfig) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Wait for database to be ready with retry logic
	db, err := dc.waitForDatabaseReadyUnlocked(ctx, 30, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer db.Close()

	// Create least-privilege admin role
	// Using $1, $2 placeholders is not supported for DDL, so we use fmt.Sprintf
	// but the values are controlled (test code), not user input
	statements := []string{
		fmt.Sprintf(`CREATE ROLE %s WITH LOGIN CREATEDB CREATEROLE PASSWORD '%s'`,
			cfg.Username, cfg.Password),
		fmt.Sprintf(`GRANT pg_signal_backend TO %s`, cfg.Username),
		fmt.Sprintf(`GRANT CONNECT ON DATABASE %s TO %s`, dc.database, cfg.Username),
	}

	// PostgreSQL 14+ has pg_read_all_data
	var version int
	if err := db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&version); err == nil && version >= 140000 {
		statements = append(statements, fmt.Sprintf(`GRANT pg_read_all_data TO %s`, cfg.Username))
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to execute statement %q: %w", stmt, err)
		}
	}

	// Update container credentials to use the new admin account
	dc.user = cfg.Username
	dc.password = cfg.Password

	return nil
}

// SetupMySQLAdminAccount creates a least-privilege admin account for MySQL/MariaDB
// and updates the container's credentials to use this account
func (dc *DatabaseContainer) SetupMySQLAdminAccount(ctx context.Context, cfg AdminAccountConfig) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Wait for database to be ready with retry logic
	db, err := dc.waitForDatabaseReadyUnlocked(ctx, 30, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to connect to mysql: %w", err)
	}
	defer db.Close()

	// Create least-privilege admin user
	// Note: information_schema doesn't need explicit GRANT - all users can query it based on
	// their privileges on actual database objects
	statements := []string{
		fmt.Sprintf(`CREATE USER '%s'@'%%' IDENTIFIED BY '%s'`, cfg.Username, cfg.Password),
		fmt.Sprintf(`GRANT CREATE, DROP, ALTER ON *.* TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT CREATE USER ON *.* TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT GRANT OPTION ON *.* TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT SELECT ON mysql.user TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT SELECT ON mysql.db TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT SELECT ON mysql.tables_priv TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT SELECT ON mysql.columns_priv TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT RELOAD ON *.* TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT PROCESS ON *.* TO '%s'@'%%'`, cfg.Username),
		fmt.Sprintf(`GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO '%s'@'%%'`, cfg.Username),
	}

	// Check MySQL version for version-specific privileges
	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err == nil {
		// MariaDB versions contain "MariaDB", MySQL 8.0+ has ROLE_ADMIN and CONNECTION_ADMIN
		if dc.engine == "mysql" {
			statements = append(statements,
				fmt.Sprintf(`GRANT ROLE_ADMIN ON *.* TO '%s'@'%%'`, cfg.Username),
				fmt.Sprintf(`GRANT CONNECTION_ADMIN ON *.* TO '%s'@'%%'`, cfg.Username),
			)
		} else {
			// MariaDB 10.5.2+ has CONNECTION ADMIN (with space)
			statements = append(statements,
				fmt.Sprintf(`GRANT CONNECTION ADMIN ON *.* TO '%s'@'%%'`, cfg.Username),
			)
		}
	}

	statements = append(statements, `FLUSH PRIVILEGES`)

	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to execute statement %q: %w", stmt, err)
		}
	}

	// Update container credentials to use the new admin account
	dc.user = cfg.Username
	dc.password = cfg.Password

	return nil
}

// IsSuperuser checks if the current user is a superuser (for verification tests)
func (dc *DatabaseContainer) IsSuperuser(ctx context.Context) (bool, error) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	switch dc.engine {
	case "postgresql", "postgres":
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			dc.host, dc.port, dc.user, dc.password, dc.database)
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return false, err
		}
		defer db.Close()

		var isSuperuser bool
		err = db.QueryRowContext(ctx, "SELECT rolsuper FROM pg_roles WHERE rolname = current_user").Scan(&isSuperuser)
		return isSuperuser, err

	case "mysql", "mariadb":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			dc.user, dc.password, dc.host, dc.port, dc.database)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return false, err
		}
		defer db.Close()

		// Check for ALL PRIVILEGES or SUPER privilege
		var grants string
		rows, err := db.QueryContext(ctx, fmt.Sprintf("SHOW GRANTS FOR '%s'@'%%'", dc.user))
		if err != nil {
			return false, err
		}
		defer rows.Close()

		for rows.Next() {
			if err := rows.Scan(&grants); err != nil {
				return false, err
			}
			// Root typically has "ALL PRIVILEGES" or "SUPER"
			if grants == fmt.Sprintf("GRANT ALL PRIVILEGES ON *.* TO `%s`@`%%` WITH GRANT OPTION", dc.user) {
				return true, nil
			}
		}
		return false, nil

	default:
		return false, fmt.Errorf("unsupported engine: %s", dc.engine)
	}
}

// WaitForReady waits for the database to be ready to accept connections
// This is useful for integration tests that need to ensure the database is fully initialized
func (dc *DatabaseContainer) WaitForReady(ctx context.Context) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	db, err := dc.waitForDatabaseReadyUnlocked(ctx, 30, 500*time.Millisecond)
	if err != nil {
		return err
	}
	db.Close()
	return nil
}
