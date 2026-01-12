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

package service

import (
	"strconv"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/util"
)

// Config holds connection configuration for services.
// This can be populated from environment variables (CLI) or from K8s resources (controller).
type Config struct {
	// Engine type: postgres, mysql
	Engine string

	// Connection details
	Host     string
	Port     int32
	Database string
	Username string
	Password string

	// TLS configuration
	TLSEnabled bool
	SSLMode    string
	TLSCA      []byte
	TLSCert    []byte
	TLSKey     []byte

	// Timeout configuration for service operations
	// These timeouts wrap context deadlines around adapter operations
	Timeouts util.TimeoutConfig

	// Engine-specific options
	// PostgreSQL
	ConnectTimeout   int32
	StatementTimeout string
	ApplicationName  string

	// MySQL
	Charset      string
	Collation    string
	ParseTime    bool
	Timeout      string
	ReadTimeout  string
	WriteTimeout string
}

// ConfigFromEnv creates a Config from environment variables.
// Environment variable names:
//   - DBCTL_ENGINE: postgres|mysql
//   - DBCTL_HOST: database host
//   - DBCTL_PORT: database port
//   - DBCTL_DATABASE: admin database name
//   - DBCTL_USERNAME: username
//   - DBCTL_PASSWORD: password
//   - DBCTL_SSL_MODE: disable|require|verify-ca|verify-full
//   - DBCTL_SSL_CA: path to CA certificate
//   - DBCTL_SSL_CERT: path to client certificate
//   - DBCTL_SSL_KEY: path to client key
//   - DBCTL_CONNECT_TIMEOUT: connection timeout (e.g., "30s")
//   - DBCTL_OPERATION_TIMEOUT: operation timeout (e.g., "60s")
//   - DBCTL_QUERY_TIMEOUT: query timeout (e.g., "30s")
//   - DBCTL_LONG_OPERATION_TIMEOUT: backup/restore timeout (e.g., "10m")
func ConfigFromEnv(getEnv func(string) string) (*Config, error) {
	cfg := &Config{
		Engine:   getEnv("DBCTL_ENGINE"),
		Host:     getEnv("DBCTL_HOST"),
		Database: getEnv("DBCTL_DATABASE"),
		Username: getEnv("DBCTL_USERNAME"),
		Password: getEnv("DBCTL_PASSWORD"),
		SSLMode:  getEnv("DBCTL_SSL_MODE"),
	}

	// Parse port
	portStr := getEnv("DBCTL_PORT")
	if portStr != "" {
		port, err := strconv.ParseInt(portStr, 10, 32)
		if err != nil {
			return nil, &ValidationError{Field: "DBCTL_PORT", Message: "invalid port number"}
		}
		cfg.Port = int32(port)
	} else {
		// Default ports
		switch cfg.Engine {
		case "postgres":
			cfg.Port = 5432
		case "mysql":
			cfg.Port = 3306
		}
	}

	// Set defaults
	if cfg.Database == "" {
		switch cfg.Engine {
		case "postgres":
			cfg.Database = "postgres"
		case "mysql":
			cfg.Database = "mysql"
		}
	}

	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}

	cfg.TLSEnabled = cfg.SSLMode != "disable"

	// Parse timeout configuration
	cfg.Timeouts = util.DefaultTimeoutConfig()
	if timeout := getEnv("DBCTL_CONNECT_TIMEOUT"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, &ValidationError{Field: "DBCTL_CONNECT_TIMEOUT", Message: "invalid duration format"}
		}
		cfg.Timeouts.ConnectTimeout = d
	}
	if timeout := getEnv("DBCTL_OPERATION_TIMEOUT"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, &ValidationError{Field: "DBCTL_OPERATION_TIMEOUT", Message: "invalid duration format"}
		}
		cfg.Timeouts.OperationTimeout = d
	}
	if timeout := getEnv("DBCTL_QUERY_TIMEOUT"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, &ValidationError{Field: "DBCTL_QUERY_TIMEOUT", Message: "invalid duration format"}
		}
		cfg.Timeouts.QueryTimeout = d
	}
	if timeout := getEnv("DBCTL_LONG_OPERATION_TIMEOUT"); timeout != "" {
		d, err := time.ParseDuration(timeout)
		if err != nil {
			return nil, &ValidationError{Field: "DBCTL_LONG_OPERATION_TIMEOUT", Message: "invalid duration format"}
		}
		cfg.Timeouts.LongOperationTimeout = d
	}

	// Validation
	if cfg.Engine == "" {
		return nil, &ValidationError{Field: "DBCTL_ENGINE", Message: "engine is required"}
	}
	if cfg.Host == "" {
		return nil, &ValidationError{Field: "DBCTL_HOST", Message: "host is required"}
	}
	if cfg.Username == "" {
		return nil, &ValidationError{Field: "DBCTL_USERNAME", Message: "username is required"}
	}

	return cfg, nil
}

// ToAdapterConfig converts service config to adapter connection config.
func (c *Config) ToAdapterConfig() adapter.ConnectionConfig {
	return adapter.ConnectionConfig{
		Host:     c.Host,
		Port:     c.Port,
		Database: c.Database,
		Username: c.Username,
		Password: c.Password,

		TLSEnabled: c.TLSEnabled,
		TLSMode:    c.SSLMode,
		TLSCA:      c.TLSCA,
		TLSCert:    c.TLSCert,
		TLSKey:     c.TLSKey,

		// PostgreSQL
		SSLMode:          c.SSLMode,
		ConnectTimeout:   c.ConnectTimeout,
		StatementTimeout: c.StatementTimeout,
		ApplicationName:  c.ApplicationName,

		// MySQL
		Charset:      c.Charset,
		Collation:    c.Collation,
		ParseTime:    c.ParseTime,
		Timeout:      c.Timeout,
		ReadTimeout:  c.ReadTimeout,
		WriteTimeout: c.WriteTimeout,
	}
}

// GetEngineType returns the engine type as a dbopsv1alpha1.EngineType.
func (c *Config) GetEngineType() dbopsv1alpha1.EngineType {
	switch c.Engine {
	case "postgres":
		return dbopsv1alpha1.EngineTypePostgres
	case "mysql":
		return dbopsv1alpha1.EngineTypeMySQL
	default:
		return dbopsv1alpha1.EngineType(c.Engine)
	}
}

// Validate validates the configuration and returns an error if invalid.
// This method should be called before using the config to ensure all required fields are set.
func (c *Config) Validate() error {
	if c == nil {
		return &ValidationError{Field: "config", Message: "config is nil"}
	}
	if c.Engine == "" {
		return &ValidationError{Field: "engine", Message: "engine is required"}
	}
	if c.Engine != "postgres" && c.Engine != "mysql" {
		return &ValidationError{Field: "engine", Message: "engine must be 'postgres' or 'mysql'"}
	}
	if c.Host == "" {
		return &ValidationError{Field: "host", Message: "host is required"}
	}
	if c.Port <= 0 || c.Port > 65535 {
		return &ValidationError{Field: "port", Message: "port must be between 1 and 65535"}
	}
	if c.Username == "" {
		return &ValidationError{Field: "username", Message: "username is required"}
	}
	// Password is not required for some auth methods (cert-based, IAM)
	// Database has sensible defaults set by the config builders

	// Validate SSL mode
	validSSLModes := map[string]bool{
		"":            true,
		"disable":     true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[c.SSLMode] {
		return &ValidationError{Field: "sslMode", Message: "invalid SSL mode"}
	}

	return nil
}

// WithTimeouts returns a copy of the config with the specified timeout configuration.
func (c *Config) WithTimeouts(timeouts util.TimeoutConfig) *Config {
	copy := *c
	copy.Timeouts = timeouts
	return &copy
}

// Clone returns a deep copy of the config.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	cpy := *c
	// Deep copy byte slices
	if c.TLSCA != nil {
		cpy.TLSCA = append([]byte(nil), c.TLSCA...)
	}
	if c.TLSCert != nil {
		cpy.TLSCert = append([]byte(nil), c.TLSCert...)
	}
	if c.TLSKey != nil {
		cpy.TLSKey = append([]byte(nil), c.TLSKey...)
	}
	return &cpy
}

// ConfigBuilder provides a fluent interface for building Config objects.
// Example usage:
//
//	cfg := NewConfigBuilder().
//	    WithEngine("postgres").
//	    WithHost("localhost").
//	    WithPort(5432).
//	    WithCredentials("admin", "secret").
//	    WithDefaultTimeouts().
//	    Build()
type ConfigBuilder struct {
	config *Config
	errors []error
}

// NewConfigBuilder creates a new ConfigBuilder with default values.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: &Config{
			Timeouts: util.DefaultTimeoutConfig(),
		},
		errors: []error{},
	}
}

// WithEngine sets the database engine type.
func (b *ConfigBuilder) WithEngine(engine string) *ConfigBuilder {
	b.config.Engine = engine
	// Set engine-specific defaults
	switch engine {
	case "postgres":
		if b.config.Port == 0 {
			b.config.Port = 5432
		}
		if b.config.Database == "" {
			b.config.Database = "postgres"
		}
	case "mysql":
		if b.config.Port == 0 {
			b.config.Port = 3306
		}
		if b.config.Database == "" {
			b.config.Database = "mysql"
		}
	}
	return b
}

// WithHost sets the database host.
func (b *ConfigBuilder) WithHost(host string) *ConfigBuilder {
	b.config.Host = host
	return b
}

// WithPort sets the database port.
func (b *ConfigBuilder) WithPort(port int32) *ConfigBuilder {
	b.config.Port = port
	return b
}

// WithDatabase sets the database name.
func (b *ConfigBuilder) WithDatabase(database string) *ConfigBuilder {
	b.config.Database = database
	return b
}

// WithCredentials sets the username and password.
func (b *ConfigBuilder) WithCredentials(username, password string) *ConfigBuilder {
	b.config.Username = username
	b.config.Password = password
	return b
}

// WithSSL configures SSL/TLS settings.
func (b *ConfigBuilder) WithSSL(mode string, ca, cert, key []byte) *ConfigBuilder {
	b.config.SSLMode = mode
	b.config.TLSEnabled = mode != "" && mode != "disable"
	b.config.TLSCA = ca
	b.config.TLSCert = cert
	b.config.TLSKey = key
	return b
}

// WithTimeouts sets custom timeout configuration.
func (b *ConfigBuilder) WithTimeouts(timeouts util.TimeoutConfig) *ConfigBuilder {
	b.config.Timeouts = timeouts
	return b
}

// WithDefaultTimeouts sets the default timeout configuration.
func (b *ConfigBuilder) WithDefaultTimeouts() *ConfigBuilder {
	b.config.Timeouts = util.DefaultTimeoutConfig()
	return b
}

// WithFastTimeouts sets fast timeout configuration (useful for testing).
func (b *ConfigBuilder) WithFastTimeouts() *ConfigBuilder {
	b.config.Timeouts = util.FastTimeoutConfig()
	return b
}

// WithNoTimeouts disables timeouts (useful for long-running operations).
func (b *ConfigBuilder) WithNoTimeouts() *ConfigBuilder {
	b.config.Timeouts = util.NoTimeoutConfig()
	return b
}

// WithPostgresOptions sets PostgreSQL-specific options.
func (b *ConfigBuilder) WithPostgresOptions(connectTimeout int32, statementTimeout, appName string) *ConfigBuilder {
	b.config.ConnectTimeout = connectTimeout
	b.config.StatementTimeout = statementTimeout
	b.config.ApplicationName = appName
	return b
}

// WithMySQLOptions sets MySQL-specific options.
func (b *ConfigBuilder) WithMySQLOptions(charset, collation string, parseTime bool) *ConfigBuilder {
	b.config.Charset = charset
	b.config.Collation = collation
	b.config.ParseTime = parseTime
	return b
}

// Build validates and returns the Config.
// Returns an error if validation fails.
func (b *ConfigBuilder) Build() (*Config, error) {
	if err := b.config.Validate(); err != nil {
		return nil, err
	}
	return b.config, nil
}

// MustBuild validates and returns the Config, panicking if validation fails.
// Useful for testing or when configuration is known to be valid.
func (b *ConfigBuilder) MustBuild() *Config {
	cfg, err := b.Build()
	if err != nil {
		panic(err)
	}
	return cfg
}

// Result holds the result of a service operation.
type Result struct {
	// Success indicates whether the operation was successful
	Success bool

	// Message provides a human-readable description of the result
	Message string

	// Created indicates whether a new resource was created (vs already existed)
	Created bool

	// Updated indicates whether an existing resource was updated
	Updated bool

	// Data holds operation-specific result data
	Data interface{}
}

// NewSuccessResult creates a successful result with a message.
func NewSuccessResult(message string) *Result {
	return &Result{
		Success: true,
		Message: message,
	}
}

// NewCreatedResult creates a result indicating a resource was created.
func NewCreatedResult(message string) *Result {
	return &Result{
		Success: true,
		Message: message,
		Created: true,
	}
}

// NewUpdatedResult creates a result indicating a resource was updated.
func NewUpdatedResult(message string) *Result {
	return &Result{
		Success: true,
		Message: message,
		Updated: true,
	}
}

// NewExistsResult creates a result indicating a resource already exists.
func NewExistsResult(message string) *Result {
	return &Result{
		Success: true,
		Message: message,
	}
}

// WithData adds data to a result and returns the result for chaining.
func (r *Result) WithData(data interface{}) *Result {
	r.Data = data
	return r
}

// ConfigFromInstance creates a Config from a DatabaseInstance spec and credentials.
// This is used by controllers to build service config from K8s resources.
func ConfigFromInstance(spec *dbopsv1alpha1.DatabaseInstanceSpec, username, password string, tlsCA, tlsCert, tlsKey []byte) *Config {
	cfg := &Config{
		Engine:   string(spec.Engine),
		Host:     spec.Connection.Host,
		Port:     spec.Connection.Port,
		Database: spec.Connection.Database,
		Username: username,
		Password: password,
		Timeouts: util.DefaultTimeoutConfig(),
	}

	// Set default database if not specified
	if cfg.Database == "" {
		switch spec.Engine {
		case dbopsv1alpha1.EngineTypePostgres:
			cfg.Database = "postgres"
		case dbopsv1alpha1.EngineTypeMySQL:
			cfg.Database = "mysql"
		}
	}

	// Set default port if not specified
	if cfg.Port == 0 {
		switch spec.Engine {
		case dbopsv1alpha1.EngineTypePostgres:
			cfg.Port = 5432
		case dbopsv1alpha1.EngineTypeMySQL:
			cfg.Port = 3306
		}
	}

	// PostgreSQL-specific options
	if spec.Postgres != nil {
		cfg.SSLMode = string(spec.Postgres.SSLMode)
		cfg.ConnectTimeout = spec.Postgres.ConnectTimeout
		cfg.StatementTimeout = spec.Postgres.StatementTimeout
		cfg.ApplicationName = spec.Postgres.ApplicationName
	}

	// MySQL-specific options
	if spec.MySQL != nil {
		cfg.Charset = spec.MySQL.Charset
		cfg.Collation = spec.MySQL.Collation
		cfg.Timeout = spec.MySQL.Timeout
		cfg.ReadTimeout = spec.MySQL.ReadTimeout
		cfg.WriteTimeout = spec.MySQL.WriteTimeout
		cfg.ParseTime = spec.MySQL.ParseTime
	}

	// TLS configuration
	if len(tlsCA) > 0 || len(tlsCert) > 0 || len(tlsKey) > 0 {
		cfg.TLSEnabled = true
		cfg.TLSCA = tlsCA
		cfg.TLSCert = tlsCert
		cfg.TLSKey = tlsKey
	}

	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}
	if cfg.SSLMode != "disable" {
		cfg.TLSEnabled = true
	}

	return cfg
}
