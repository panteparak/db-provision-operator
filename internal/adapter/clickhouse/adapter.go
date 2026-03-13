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
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/db-provision-operator/internal/adapter/types"
)

// Adapter implements the DatabaseAdapter interface for ClickHouse
type Adapter struct {
	config types.ConnectionConfig
	db     *sql.DB
	mu     sync.RWMutex
}

// NewAdapter creates a new ClickHouse adapter
func NewAdapter(config types.ConnectionConfig) *Adapter {
	return &Adapter{
		config: config,
	}
}

// Connect establishes a connection to the ClickHouse server
func (a *Adapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.db != nil {
		return nil // Already connected
	}

	opts := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", a.config.Host, a.config.Port)},
		Auth: clickhouse.Auth{
			Database: a.config.Database,
			Username: a.config.Username,
			Password: a.config.Password,
		},
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	}

	// TLS configuration
	if a.config.TLSEnabled {
		tlsConfig, err := a.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to build TLS config: %w", err)
		}
		if tlsConfig != nil {
			opts.TLS = tlsConfig
		}
	}

	db := clickhouse.OpenDB(opts)

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
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

// GetVersion returns the ClickHouse server version
func (a *Adapter) GetVersion(ctx context.Context) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.db == nil {
		return "", fmt.Errorf("not connected")
	}

	var version string
	err := a.db.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("failed to get version: %w", err)
	}
	return version, nil
}

// buildTLSConfig builds TLS configuration for ClickHouse
func (a *Adapter) buildTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: a.config.Host,
	}

	switch a.config.TLSMode {
	case "disable":
		return nil, nil
	case "preferred", "require":
		tlsConfig.InsecureSkipVerify = true
	case "verify-ca", "verify-full":
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

// escapeIdentifier escapes a ClickHouse identifier (database, table, column names)
func escapeIdentifier(s string) string {
	escaped := strings.ReplaceAll(s, "`", "``")
	return "`" + escaped + "`"
}

// escapeLiteral escapes a ClickHouse string literal
func escapeLiteral(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	return "'" + escaped + "'"
}

// SetResourceComment sets tracking metadata on a database resource.
// ClickHouse uses a metadata table for resource tracking.
func (a *Adapter) SetResourceComment(ctx context.Context, resourceType, resourceName, comment string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}
	return a.setMetadataTableEntry(ctx, db, resourceType, resourceName, comment)
}

// GetResourceComment retrieves the tracking metadata from a database resource.
func (a *Adapter) GetResourceComment(ctx context.Context, resourceType, resourceName string) (string, error) {
	db, err := a.getDB()
	if err != nil {
		return "", err
	}
	return a.getMetadataTableEntry(ctx, db, resourceType, resourceName)
}

// SetUserAttribute is a no-op for ClickHouse (no user attribute support like MySQL).
func (a *Adapter) SetUserAttribute(_ context.Context, _, _, _ string) error {
	return nil
}

// GetUserAttribute is a no-op for ClickHouse.
func (a *Adapter) GetUserAttribute(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// setMetadataTableEntry sets a tracking entry in the metadata table.
func (a *Adapter) setMetadataTableEntry(ctx context.Context, db *sql.DB, resourceType, resourceName, metadata string) error {
	createTableQuery := `CREATE TABLE IF NOT EXISTS _dbops_metadata (
		resource_type String,
		resource_name String,
		metadata String,
		managed_since DateTime DEFAULT now()
	) ENGINE = ReplacingMergeTree()
	ORDER BY (resource_type, resource_name)`

	_, err := db.ExecContext(ctx, createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	query := fmt.Sprintf("INSERT INTO _dbops_metadata (resource_type, resource_name, metadata) VALUES (%s, %s, %s)",
		escapeLiteral(resourceType), escapeLiteral(resourceName), escapeLiteral(metadata))

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to set metadata entry: %w", err)
	}

	return nil
}

// getMetadataTableEntry retrieves a tracking entry from the metadata table.
func (a *Adapter) getMetadataTableEntry(ctx context.Context, db *sql.DB, resourceType, resourceName string) (string, error) {
	// Check if table exists
	var count uint64
	err := db.QueryRowContext(ctx,
		"SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = '_dbops_metadata'").Scan(&count)
	if err != nil || count == 0 {
		return "", nil
	}

	var metadata string
	err = db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT metadata FROM _dbops_metadata FINAL WHERE resource_type = %s AND resource_name = %s",
			escapeLiteral(resourceType), escapeLiteral(resourceName))).Scan(&metadata)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", nil
		}
		return "", fmt.Errorf("failed to get metadata entry: %w", err)
	}

	return metadata, nil
}
