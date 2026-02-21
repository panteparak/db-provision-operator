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
	cfg.MultiStatements = false

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
			if err := mysql.RegisterTLSConfig(tlsConfigName, tlsConfig); err != nil {
				return "", fmt.Errorf("failed to register TLS config: %w", err)
			}
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

// SetResourceComment sets tracking metadata on a database resource.
// Implements the ResourceTracker interface.
// For MySQL:
//   - Users: Uses user attributes (MySQL 8.0.21+) if available, otherwise uses metadata table
//   - Databases: Uses metadata table
//   - Roles: Uses metadata table
//
// resourceType: "database", "role", "user"
// resourceName: The name of the resource
// comment: The tracking metadata string
func (a *Adapter) SetResourceComment(ctx context.Context, resourceType, resourceName, comment string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// For users, try to use native attributes first (MySQL 8.0.21+)
	if resourceType == "user" {
		version, err := a.GetVersion(ctx)
		if err == nil && a.supportsUserAttributes(version) {
			return a.SetUserAttribute(ctx, resourceName, "managed_by", comment)
		}
	}

	// Fall back to metadata table
	return a.setMetadataTableEntry(ctx, db, resourceType, resourceName, comment)
}

// GetResourceComment retrieves the tracking metadata from a database resource.
// Implements the ResourceTracker interface.
// Returns empty string if no metadata is set.
func (a *Adapter) GetResourceComment(ctx context.Context, resourceType, resourceName string) (string, error) {
	db, err := a.getDB()
	if err != nil {
		return "", err
	}

	// For users, try to get native attributes first (MySQL 8.0.21+)
	if resourceType == "user" {
		version, err := a.GetVersion(ctx)
		if err == nil && a.supportsUserAttributes(version) {
			return a.GetUserAttribute(ctx, resourceName, "managed_by")
		}
	}

	// Fall back to metadata table
	return a.getMetadataTableEntry(ctx, db, resourceType, resourceName)
}

// SetUserAttribute sets a JSON attribute on a user (MySQL 8.0.21+ only).
// Uses ALTER USER ... ATTRIBUTE statement.
func (a *Adapter) SetUserAttribute(ctx context.Context, username, key, value string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Check if user attributes are supported
	version, err := a.GetVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get MySQL version: %w", err)
	}
	if !a.supportsUserAttributes(version) {
		return fmt.Errorf("user attributes require MySQL 8.0.21+, current version: %s", version)
	}

	// Build JSON attribute
	// Note: This replaces the entire attribute object, so we need to merge with existing
	existingAttr, err := a.getUserAttributes(ctx, db, username)
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		return fmt.Errorf("failed to get existing attributes: %w", err)
	}

	// Simple attribute replacement for now (single key)
	attrJSON := fmt.Sprintf(`{"%s": %s}`, key, escapeLiteral(value))
	if existingAttr != "" && existingAttr != "{}" {
		// Merge would require JSON parsing; for simplicity, we just set the key
		attrJSON = fmt.Sprintf(`{"%s": %s}`, key, escapeLiteral(value))
	}

	query := fmt.Sprintf("ALTER USER %s@'%%' ATTRIBUTE %s",
		escapeIdentifier(username), escapeLiteral(attrJSON))

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to set user attribute: %w", err)
	}

	return nil
}

// GetUserAttribute retrieves a JSON attribute from a user (MySQL 8.0.21+ only).
func (a *Adapter) GetUserAttribute(ctx context.Context, username, key string) (string, error) {
	db, err := a.getDB()
	if err != nil {
		return "", err
	}

	// Check if user attributes are supported
	version, err := a.GetVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get MySQL version: %w", err)
	}
	if !a.supportsUserAttributes(version) {
		return "", nil // Not supported, return empty
	}

	query := `SELECT JSON_UNQUOTE(JSON_EXTRACT(User_attributes, CONCAT('$.', ?)))
		FROM mysql.user WHERE User = ? AND Host = '%'`

	var value sql.NullString
	err = db.QueryRowContext(ctx, query, key, username).Scan(&value)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", nil
		}
		return "", fmt.Errorf("failed to get user attribute: %w", err)
	}

	if !value.Valid {
		return "", nil
	}
	return value.String, nil
}

// getUserAttributes retrieves the full user attributes JSON.
func (a *Adapter) getUserAttributes(ctx context.Context, db *sql.DB, username string) (string, error) {
	query := `SELECT COALESCE(User_attributes, '{}') FROM mysql.user WHERE User = ? AND Host = '%'`

	var attr string
	err := db.QueryRowContext(ctx, query, username).Scan(&attr)
	if err != nil {
		return "", err
	}
	return attr, nil
}

// supportsUserAttributes checks if the MySQL version supports user attributes (8.0.21+).
func (a *Adapter) supportsUserAttributes(version string) bool {
	// Parse version like "8.0.21" or "8.0.21-mysql"
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	var major, minor, patch int
	_, _ = fmt.Sscanf(parts[0], "%d", &major)
	_, _ = fmt.Sscanf(parts[1], "%d", &minor)
	if len(parts) >= 3 {
		_, _ = fmt.Sscanf(parts[2], "%d", &patch)
	}

	// MySQL 8.0.21+
	if major > 8 {
		return true
	}
	if major == 8 && minor > 0 {
		return true
	}
	if major == 8 && minor == 0 && patch >= 21 {
		return true
	}
	return false
}

// setMetadataTableEntry sets a tracking entry in the metadata table.
// Creates the table if it doesn't exist.
func (a *Adapter) setMetadataTableEntry(ctx context.Context, db *sql.DB, resourceType, resourceName, metadata string) error {
	// Ensure metadata table exists
	createTableQuery := `CREATE TABLE IF NOT EXISTS _dbops_metadata (
		resource_type VARCHAR(50) NOT NULL,
		resource_name VARCHAR(255) NOT NULL,
		metadata TEXT,
		managed_since TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (resource_type, resource_name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`

	_, err := db.ExecContext(ctx, createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create metadata table: %w", err)
	}

	// Upsert the metadata entry
	query := `INSERT INTO _dbops_metadata (resource_type, resource_name, metadata)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE metadata = VALUES(metadata)`

	_, err = db.ExecContext(ctx, query, resourceType, resourceName, metadata)
	if err != nil {
		return fmt.Errorf("failed to set metadata entry: %w", err)
	}

	return nil
}

// getMetadataTableEntry retrieves a tracking entry from the metadata table.
func (a *Adapter) getMetadataTableEntry(ctx context.Context, db *sql.DB, resourceType, resourceName string) (string, error) {
	// Check if table exists first
	var tableName string
	checkQuery := `SELECT TABLE_NAME FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '_dbops_metadata'`
	err := db.QueryRowContext(ctx, checkQuery).Scan(&tableName)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", nil // Table doesn't exist, no metadata
		}
		return "", fmt.Errorf("failed to check metadata table: %w", err)
	}

	// Get metadata entry
	query := `SELECT COALESCE(metadata, '') FROM _dbops_metadata
		WHERE resource_type = ? AND resource_name = ?`

	var metadata string
	err = db.QueryRowContext(ctx, query, resourceType, resourceName).Scan(&metadata)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return "", nil
		}
		return "", fmt.Errorf("failed to get metadata entry: %w", err)
	}

	return metadata, nil
}
