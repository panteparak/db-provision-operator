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

package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/db-provision-operator/internal/adapter/types"
)

// Adapter implements the DatabaseAdapter interface for MySQL
type Adapter struct {
	config types.ConnectionConfig
	db     *sql.DB
	mu     sync.RWMutex

	// For tracking backup/restore operations
	activeOps sync.Map
}

// NewAdapter creates a new MySQL adapter
func NewAdapter(config types.ConnectionConfig) *Adapter {
	return &Adapter{
		config: config,
	}
}

// Connect establishes a connection to the MySQL server
func (a *Adapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.db != nil {
		return nil // Already connected
	}

	dsn, err := a.buildDSN()
	if err != nil {
		return fmt.Errorf("failed to build DSN: %w", err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	a.db = db
	return nil
}

// Close closes the database connection
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.db != nil {
		err := a.db.Close()
		a.db = nil
		return err
	}
	return nil
}

// Ping checks if the database connection is alive
func (a *Adapter) Ping(ctx context.Context) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.db == nil {
		return fmt.Errorf("not connected")
	}
	return a.db.PingContext(ctx)
}

// GetVersion returns the MySQL server version
func (a *Adapter) GetVersion(ctx context.Context) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.db == nil {
		return "", fmt.Errorf("not connected")
	}

	var version string
	err := a.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}
	return version, nil
}

// buildDSN builds a MySQL Data Source Name
func (a *Adapter) buildDSN() (string, error) {
	cfg := mysql.NewConfig()
	cfg.User = a.config.Username
	cfg.Passwd = a.config.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", a.config.Host, a.config.Port)
	cfg.DBName = a.config.Database
	cfg.ParseTime = a.config.ParseTime
	cfg.MultiStatements = true

	// Timeouts
	if a.config.Timeout != "" {
		if d, err := time.ParseDuration(a.config.Timeout); err == nil {
			cfg.Timeout = d
		}
	}
	if a.config.ReadTimeout != "" {
		if d, err := time.ParseDuration(a.config.ReadTimeout); err == nil {
			cfg.ReadTimeout = d
		}
	}
	if a.config.WriteTimeout != "" {
		if d, err := time.ParseDuration(a.config.WriteTimeout); err == nil {
			cfg.WriteTimeout = d
		}
	}

	// Charset and collation
	if a.config.Charset != "" {
		cfg.Params = map[string]string{
			"charset": a.config.Charset,
		}
	}
	if a.config.Collation != "" {
		cfg.Collation = a.config.Collation
	}

	// TLS configuration
	if a.config.TLSEnabled {
		tlsConfig, err := a.buildTLSConfig()
		if err != nil {
			return "", err
		}
		if tlsConfig != nil {
			tlsConfigName := "custom"
			mysql.RegisterTLSConfig(tlsConfigName, tlsConfig)
			cfg.TLSConfig = tlsConfigName
		}
	}

	return cfg.FormatDSN(), nil
}

// buildTLSConfig builds TLS configuration for MySQL
func (a *Adapter) buildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: a.config.Host,
	}

	// Configure verification mode based on TLSMode
	switch a.config.TLSMode {
	case "disable":
		return nil, nil
	case "preferred", "require":
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

// getDB returns the database connection, ensuring thread safety
func (a *Adapter) getDB() (*sql.DB, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	return a.db, nil
}

// execWithDatabase executes a query on a specific database
func (a *Adapter) execWithDatabase(ctx context.Context, database, query string, args ...interface{}) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	if database != "" && database != a.config.Database {
		// Use the specified database
		_, err = db.ExecContext(ctx, fmt.Sprintf("USE %s", escapeIdentifier(database)))
		if err != nil {
			return fmt.Errorf("failed to use database %s: %w", database, err)
		}
	}

	_, err = db.ExecContext(ctx, query, args...)
	return err
}

// queryWithDatabase executes a query and returns rows on a specific database
func (a *Adapter) queryWithDatabase(ctx context.Context, database, query string, args ...interface{}) (*sql.Rows, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	if database != "" && database != a.config.Database {
		_, err = db.ExecContext(ctx, fmt.Sprintf("USE %s", escapeIdentifier(database)))
		if err != nil {
			return nil, fmt.Errorf("failed to use database %s: %w", database, err)
		}
	}

	return db.QueryContext(ctx, query, args...)
}

// escapeIdentifier escapes a MySQL identifier (database, table, column names)
func escapeIdentifier(s string) string {
	// Escape backticks by doubling them
	escaped := strings.ReplaceAll(s, "`", "``")
	return "`" + escaped + "`"
}

// escapeLiteral escapes a MySQL string literal
func escapeLiteral(s string) string {
	// Escape single quotes by doubling them
	escaped := strings.ReplaceAll(s, "'", "''")
	// Also escape backslashes
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	return "'" + escaped + "'"
}
