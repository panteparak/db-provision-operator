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

package cockroachdb

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/db-provision-operator/internal/adapter/types"
)

// Adapter implements the DatabaseAdapter interface for CockroachDB.
// CockroachDB is PostgreSQL wire-compatible, so we use the pgx driver,
// but adapt SQL statements for CockroachDB-specific behavior:
//   - No SUPERUSER, REPLICATION, or BYPASSRLS role attributes
//   - No extensions or tablespace support
//   - Native BACKUP/RESTORE instead of pg_dump/pg_restore
//   - Uses crdb_internal schema for metadata queries
type Adapter struct {
	config types.ConnectionConfig
	pool   *pgxpool.Pool
	mu     sync.RWMutex

	// For tracking backup/restore operations
	activeOps sync.Map
}

// NewAdapter creates a new CockroachDB adapter
func NewAdapter(config types.ConnectionConfig) *Adapter {
	return &Adapter{
		config: config,
	}
}

// Connect establishes a connection to the CockroachDB cluster
func (a *Adapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pool != nil {
		return nil // Already connected
	}

	connString, err := a.buildConnectionString()
	if err != nil {
		return fmt.Errorf("failed to build connection string: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Configure TLS if enabled
	if a.config.TLSEnabled {
		tlsConfig, err := a.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to build TLS config: %w", err)
		}
		poolConfig.ConnConfig.TLSConfig = tlsConfig
	}

	// Set connection timeout
	if a.config.ConnectTimeout > 0 {
		poolConfig.ConnConfig.ConnectTimeout = time.Duration(a.config.ConnectTimeout) * time.Second
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	a.pool = pool
	return nil
}

// Close closes the database connection
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pool != nil {
		a.pool.Close()
		a.pool = nil
	}
	return nil
}

// Ping checks if the database connection is alive
func (a *Adapter) Ping(ctx context.Context) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.pool == nil {
		return fmt.Errorf("not connected")
	}
	return a.pool.Ping(ctx)
}

// GetVersion returns the CockroachDB server version
func (a *Adapter) GetVersion(ctx context.Context) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.pool == nil {
		return "", fmt.Errorf("not connected")
	}

	var version string
	err := a.pool.QueryRow(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}
	return version, nil
}

// buildConnectionString builds a CockroachDB connection string using libpq format
func (a *Adapter) buildConnectionString() (string, error) {
	var parts []string

	parts = append(parts, fmt.Sprintf("host=%s", a.config.Host))
	parts = append(parts, fmt.Sprintf("port=%d", a.config.Port))
	parts = append(parts, fmt.Sprintf("dbname=%s", a.config.Database))
	parts = append(parts, fmt.Sprintf("user=%s", a.config.Username))
	parts = append(parts, fmt.Sprintf("password=%s", a.config.Password))

	// SSL mode
	if a.config.SSLMode != "" {
		parts = append(parts, fmt.Sprintf("sslmode=%s", a.config.SSLMode))
	} else if a.config.TLSEnabled {
		parts = append(parts, fmt.Sprintf("sslmode=%s", a.config.TLSMode))
	} else {
		parts = append(parts, "sslmode=disable")
	}

	// Application name
	if a.config.ApplicationName != "" {
		parts = append(parts, fmt.Sprintf("application_name=%s", a.config.ApplicationName))
	}

	// Connect timeout
	if a.config.ConnectTimeout > 0 {
		parts = append(parts, fmt.Sprintf("connect_timeout=%d", a.config.ConnectTimeout))
	}

	return strings.Join(parts, " "), nil
}

// buildTLSConfig builds TLS configuration
func (a *Adapter) buildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Set server name for verification
	tlsConfig.ServerName = a.config.Host

	// Configure verification mode
	switch a.config.TLSMode {
	case "disable":
		return nil, nil
	case "require":
		tlsConfig.InsecureSkipVerify = true
	case "verify-ca":
		tlsConfig.InsecureSkipVerify = false
	case "verify-full":
		tlsConfig.InsecureSkipVerify = false
	default:
		tlsConfig.InsecureSkipVerify = true
	}

	// Add CA certificate
	if len(a.config.TLSCA) > 0 {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(a.config.TLSCA) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Add client certificate for mTLS
	if len(a.config.TLSCert) > 0 && len(a.config.TLSKey) > 0 {
		cert, err := tls.X509KeyPair(a.config.TLSCert, a.config.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// getPool returns the connection pool, ensuring thread safety
func (a *Adapter) getPool() (*pgxpool.Pool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.pool == nil {
		return nil, fmt.Errorf("not connected")
	}
	return a.pool, nil
}

// execWithNewConnection executes a query on a new connection to a specific database.
// This is needed for operations that must run against a different database than the pool's default.
func (a *Adapter) execWithNewConnection(ctx context.Context, database, query string, args ...interface{}) error {
	connString := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		a.config.Host, a.config.Port, database, a.config.Username, a.config.Password,
		func() string {
			if a.config.SSLMode != "" {
				return a.config.SSLMode
			}
			return "disable"
		}(),
	)

	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return fmt.Errorf("failed to connect to database %s: %w", database, err)
	}
	defer func() { _ = conn.Close(ctx) }()

	_, err = conn.Exec(ctx, query, args...)
	return err
}

// queryWithNewConnection executes a query and returns rows on a new connection
func (a *Adapter) queryWithNewConnection(ctx context.Context, database, query string, args ...interface{}) (pgx.Rows, func(), error) {
	connString := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		a.config.Host, a.config.Port, database, a.config.Username, a.config.Password,
		func() string {
			if a.config.SSLMode != "" {
				return a.config.SSLMode
			}
			return "disable"
		}(),
	)

	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to database %s: %w", database, err)
	}

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		_ = conn.Close(ctx)
		return nil, nil, err
	}

	cleanup := func() {
		rows.Close()
		_ = conn.Close(ctx)
	}

	return rows, cleanup, nil
}

// escapeIdentifier escapes a SQL identifier (table, column, role, etc.)
func escapeIdentifier(s string) string {
	escaped := strings.ReplaceAll(s, `"`, `""`)
	return `"` + escaped + `"`
}

// escapeLiteral escapes a SQL string literal
func escapeLiteral(s string) string {
	escaped := strings.ReplaceAll(s, `'`, `''`)
	return `'` + escaped + `'`
}
