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
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2" // ClickHouse driver

	"github.com/db-provision-operator/internal/adapter/types"
)

// Adapter implements the DatabaseAdapter interface for ClickHouse
type Adapter struct {
	config types.ConnectionConfig
	db     *sql.DB
	mu     sync.RWMutex

	// For tracking backup/restore operations
	activeOps sync.Map
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

	dsn := a.buildDSN()

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	maxOpen := int(a.config.ClickHouseMaxOpenConns)
	if maxOpen <= 0 {
		maxOpen = 25
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

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

// buildDSN builds a ClickHouse Data Source Name
// Format: clickhouse://user:password@host:port/database?param=value
func (a *Adapter) buildDSN() string {
	var params []string

	// Timeouts
	if a.config.ClickHouseDialTimeout != "" {
		params = append(params, "dial_timeout="+a.config.ClickHouseDialTimeout)
	}
	if a.config.ClickHouseReadTimeout != "" {
		params = append(params, "read_timeout="+a.config.ClickHouseReadTimeout)
	}

	// Debug mode
	if a.config.ClickHouseDebug {
		params = append(params, "debug=true")
	}

	// TLS configuration
	if a.config.TLSEnabled {
		params = append(params, "secure=true")
		switch a.config.TLSMode {
		case "disable":
			params = append(params, "secure=false")
		case "preferred", "require":
			params = append(params, "skip_verify=true")
		case "verify-ca", "verify-full":
			params = append(params, "skip_verify=false")
		default:
			params = append(params, "skip_verify=true")
		}
	}

	paramStr := ""
	if len(params) > 0 {
		paramStr = "?" + strings.Join(params, "&")
	}

	database := a.config.Database
	if database == "" {
		database = "default"
	}

	return fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s%s",
		a.config.Username,
		a.config.Password,
		a.config.Host,
		a.config.Port,
		database,
		paramStr,
	)
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
// ClickHouse does not support native resource comments on users/roles,
// so we use a metadata table for all resource types.
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

// SetUserAttribute is a no-op for ClickHouse as it doesn't support user attributes.
func (a *Adapter) SetUserAttribute(_ context.Context, _, _, _ string) error {
	return nil
}

// GetUserAttribute is a no-op for ClickHouse as it doesn't support user attributes.
func (a *Adapter) GetUserAttribute(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// setMetadataTableEntry sets a tracking entry in the metadata table.
// Creates the table if it doesn't exist using MergeTree engine.
func (a *Adapter) setMetadataTableEntry(ctx context.Context, db *sql.DB, resourceType, resourceName, metadata string) error {
	// Ensure metadata table exists in the default database
	createTableQuery := `CREATE TABLE IF NOT EXISTS _dbops_metadata (
		resource_type String,
		resource_name String,
		metadata String,
		managed_since DateTime DEFAULT now()
	) ENGINE = MergeTree()
	ORDER BY (resource_type, resource_name)`

	_, err := db.ExecContext(ctx, createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// ClickHouse doesn't support UPSERT directly; use ALTER TABLE DELETE + INSERT
	deleteQuery := fmt.Sprintf(
		"ALTER TABLE _dbops_metadata DELETE WHERE resource_type = %s AND resource_name = %s",
		escapeLiteral(resourceType), escapeLiteral(resourceName))
	_, _ = db.ExecContext(ctx, deleteQuery)

	// Insert the new entry
	insertQuery := fmt.Sprintf(
		"INSERT INTO _dbops_metadata (resource_type, resource_name, metadata) VALUES (%s, %s, %s)",
		escapeLiteral(resourceType), escapeLiteral(resourceName), escapeLiteral(metadata))

	_, err = db.ExecContext(ctx, insertQuery)
	if err != nil {
		return fmt.Errorf("failed to set metadata entry: %w", err)
	}

	return nil
}

// getMetadataTableEntry retrieves a tracking entry from the metadata table.
func (a *Adapter) getMetadataTableEntry(ctx context.Context, db *sql.DB, resourceType, resourceName string) (string, error) {
	// Check if table exists first
	var count uint64
	checkQuery := "SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = '_dbops_metadata'"
	err := db.QueryRowContext(ctx, checkQuery).Scan(&count)
	if err != nil || count == 0 {
		return "", nil // Table doesn't exist, no metadata
	}

	// Get metadata entry (use FINAL to see latest after mutations)
	query := fmt.Sprintf(
		"SELECT metadata FROM _dbops_metadata FINAL WHERE resource_type = %s AND resource_name = %s LIMIT 1",
		escapeLiteral(resourceType), escapeLiteral(resourceName))

	var metadata string
	err = db.QueryRowContext(ctx, query).Scan(&metadata)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", nil
		}
		return "", fmt.Errorf("failed to get metadata entry: %w", err)
	}

	return metadata, nil
}
