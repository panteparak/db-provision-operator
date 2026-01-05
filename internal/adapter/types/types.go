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

package types

import (
	"context"
	"io"
)

// DatabaseAdapter defines the interface for database engine adapters.
// Each database engine (PostgreSQL, MySQL, etc.) implements this interface
// to provide a unified way to manage databases, users, roles, and permissions.
type DatabaseAdapter interface {
	// Connection management
	Connect(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
	GetVersion(ctx context.Context) (string, error)

	// Database operations
	DatabaseManager

	// User operations
	UserManager

	// Role operations
	RoleManager

	// Grant operations
	GrantManager

	// Backup operations
	BackupManager

	// Restore operations
	RestoreManager
}

// DatabaseManager defines database CRUD operations.
type DatabaseManager interface {
	// CreateDatabase creates a new database with the given options
	CreateDatabase(ctx context.Context, opts CreateDatabaseOptions) error

	// DropDatabase drops an existing database
	DropDatabase(ctx context.Context, name string, opts DropDatabaseOptions) error

	// DatabaseExists checks if a database exists
	DatabaseExists(ctx context.Context, name string) (bool, error)

	// GetDatabaseInfo retrieves database information
	GetDatabaseInfo(ctx context.Context, name string) (*DatabaseInfo, error)

	// UpdateDatabase updates database settings (extensions, schemas, etc.)
	UpdateDatabase(ctx context.Context, name string, opts UpdateDatabaseOptions) error
}

// CreateDatabaseOptions contains options for creating a database
type CreateDatabaseOptions struct {
	Name string

	// PostgreSQL-specific
	Owner            string
	Encoding         string
	LCCollate        string
	LCCtype          string
	Tablespace       string
	Template         string
	ConnectionLimit  int32
	IsTemplate       bool
	AllowConnections bool

	// MySQL-specific
	Charset       string
	Collation     string
	SQLMode       string
	StorageEngine string
}

// DropDatabaseOptions contains options for dropping a database
type DropDatabaseOptions struct {
	Force bool // Force drop even with active connections
}

// UpdateDatabaseOptions contains options for updating a database
type UpdateDatabaseOptions struct {
	// PostgreSQL-specific
	Extensions        []ExtensionOptions
	Schemas           []SchemaOptions
	DefaultPrivileges []DefaultPrivilegeOptions

	// MySQL-specific
	Charset   string
	Collation string
}

// ExtensionOptions contains options for installing an extension
type ExtensionOptions struct {
	Name    string
	Schema  string
	Version string
}

// SchemaOptions contains options for creating a schema
type SchemaOptions struct {
	Name  string
	Owner string
}

// DefaultPrivilegeOptions contains options for default privileges
type DefaultPrivilegeOptions struct {
	Role       string
	Schema     string
	ObjectType string // tables, sequences, functions, types
	Privileges []string
}

// DatabaseInfo contains database status information
type DatabaseInfo struct {
	Name      string
	Owner     string
	SizeBytes int64
	Encoding  string
	Collation string

	// PostgreSQL-specific
	Extensions []ExtensionInfo
	Schemas    []string

	// MySQL-specific
	Charset string
}

// ExtensionInfo contains extension status information
type ExtensionInfo struct {
	Name    string
	Version string
}

// UserManager defines user CRUD operations.
type UserManager interface {
	// CreateUser creates a new database user
	CreateUser(ctx context.Context, opts CreateUserOptions) error

	// DropUser drops an existing user
	DropUser(ctx context.Context, username string) error

	// UserExists checks if a user exists
	UserExists(ctx context.Context, username string) (bool, error)

	// UpdateUser updates user settings
	UpdateUser(ctx context.Context, username string, opts UpdateUserOptions) error

	// UpdatePassword updates the user's password
	UpdatePassword(ctx context.Context, username, password string) error

	// GetUserInfo retrieves user information
	GetUserInfo(ctx context.Context, username string) (*UserInfo, error)
}

// CreateUserOptions contains options for creating a user
type CreateUserOptions struct {
	Username string
	Password string

	// PostgreSQL-specific
	ConnectionLimit int32
	ValidUntil      string
	Superuser       bool
	CreateDB        bool
	CreateRole      bool
	Inherit         bool
	Login           bool
	Replication     bool
	BypassRLS       bool
	InRoles         []string
	ConfigParams    map[string]string

	// MySQL-specific
	MaxQueriesPerHour     int32
	MaxUpdatesPerHour     int32
	MaxConnectionsPerHour int32
	MaxUserConnections    int32
	AuthPlugin            string
	RequireSSL            bool
	RequireX509           bool
	AllowedHosts          []string
	AccountLocked         bool
}

// UpdateUserOptions contains options for updating a user
type UpdateUserOptions struct {
	// PostgreSQL-specific
	ConnectionLimit *int32
	ValidUntil      *string
	Superuser       *bool
	CreateDB        *bool
	CreateRole      *bool
	Inherit         *bool
	Login           *bool
	Replication     *bool
	BypassRLS       *bool
	InRoles         []string
	ConfigParams    map[string]string

	// MySQL-specific
	MaxQueriesPerHour     *int32
	MaxUpdatesPerHour     *int32
	MaxConnectionsPerHour *int32
	MaxUserConnections    *int32
	RequireSSL            *bool
	RequireX509           *bool
	AccountLocked         *bool
}

// UserInfo contains user status information
type UserInfo struct {
	Username  string
	CreatedAt string

	// PostgreSQL-specific
	ConnectionLimit int32
	ValidUntil      string
	Superuser       bool
	CreateDB        bool
	CreateRole      bool
	Inherit         bool
	Login           bool
	Replication     bool
	BypassRLS       bool
	InRoles         []string

	// MySQL-specific
	AllowedHosts []string
}

// RoleManager defines role CRUD operations.
type RoleManager interface {
	// CreateRole creates a new role
	CreateRole(ctx context.Context, opts CreateRoleOptions) error

	// DropRole drops an existing role
	DropRole(ctx context.Context, roleName string) error

	// RoleExists checks if a role exists
	RoleExists(ctx context.Context, roleName string) (bool, error)

	// UpdateRole updates role settings
	UpdateRole(ctx context.Context, roleName string, opts UpdateRoleOptions) error

	// GetRoleInfo retrieves role information
	GetRoleInfo(ctx context.Context, roleName string) (*RoleInfo, error)
}

// CreateRoleOptions contains options for creating a role
type CreateRoleOptions struct {
	RoleName string

	// PostgreSQL-specific
	Login       bool
	Inherit     bool
	CreateDB    bool
	CreateRole  bool
	Superuser   bool
	Replication bool
	BypassRLS   bool
	InRoles     []string
	Grants      []GrantOptions

	// MySQL-specific
	UseNativeRoles bool
}

// UpdateRoleOptions contains options for updating a role
type UpdateRoleOptions struct {
	// PostgreSQL-specific
	Login       *bool
	Inherit     *bool
	CreateDB    *bool
	CreateRole  *bool
	Superuser   *bool
	Replication *bool
	BypassRLS   *bool
	InRoles     []string
	Grants      []GrantOptions

	// MySQL-specific
	AddGrants    []GrantOptions
	RemoveGrants []GrantOptions
}

// RoleInfo contains role status information
type RoleInfo struct {
	Name      string
	CreatedAt string

	// PostgreSQL-specific
	Login       bool
	Inherit     bool
	CreateDB    bool
	CreateRole  bool
	Superuser   bool
	Replication bool
	BypassRLS   bool
	InRoles     []string
}

// GrantManager defines grant CRUD operations.
type GrantManager interface {
	// Grant grants privileges
	Grant(ctx context.Context, grantee string, opts []GrantOptions) error

	// Revoke revokes privileges
	Revoke(ctx context.Context, grantee string, opts []GrantOptions) error

	// GrantRole grants role membership
	GrantRole(ctx context.Context, grantee string, roles []string) error

	// RevokeRole revokes role membership
	RevokeRole(ctx context.Context, grantee string, roles []string) error

	// SetDefaultPrivileges sets default privileges for new objects
	SetDefaultPrivileges(ctx context.Context, grantee string, opts []DefaultPrivilegeGrantOptions) error

	// GetGrants retrieves grants for a user/role
	GetGrants(ctx context.Context, grantee string) ([]GrantInfo, error)
}

// GrantOptions contains options for granting privileges
type GrantOptions struct {
	Database        string
	Schema          string
	Tables          []string
	Sequences       []string
	Functions       []string
	Privileges      []string
	WithGrantOption bool

	// MySQL-specific
	Level     string // global, database, table, column, procedure, function
	Table     string
	Columns   []string
	Procedure string
	Function  string
}

// DefaultPrivilegeGrantOptions contains options for default privilege grants
type DefaultPrivilegeGrantOptions struct {
	Database   string
	Schema     string
	GrantedBy  string
	ObjectType string
	Privileges []string
}

// GrantInfo contains grant status information
type GrantInfo struct {
	Grantor    string
	Grantee    string
	Database   string
	Schema     string
	ObjectType string
	ObjectName string
	Privileges []string
}

// BackupManager defines backup operations.
type BackupManager interface {
	// Backup performs a database backup
	Backup(ctx context.Context, opts BackupOptions) (*BackupResult, error)

	// GetBackupProgress returns the backup progress (0-100)
	GetBackupProgress(ctx context.Context, backupID string) (int, error)

	// CancelBackup cancels a running backup
	CancelBackup(ctx context.Context, backupID string) error
}

// BackupOptions contains options for backup
type BackupOptions struct {
	Database string
	BackupID string
	Writer   io.Writer

	// PostgreSQL-specific
	Method          string // pg_dump, pg_basebackup
	Format          string // plain, custom, directory, tar
	Jobs            int32
	DataOnly        bool
	SchemaOnly      bool
	Blobs           bool
	NoOwner         bool
	NoPrivileges    bool
	Schemas         []string
	ExcludeSchemas  []string
	Tables          []string
	ExcludeTables   []string
	LockWaitTimeout string

	// MySQL-specific
	SingleTransaction bool
	Quick             bool
	LockTables        bool
	Routines          bool
	Triggers          bool
	Events            bool
	ExtendedInsert    bool
	SetGtidPurged     string
	Databases         []string
}

// BackupResult contains backup result information
type BackupResult struct {
	BackupID            string
	Path                string
	SizeBytes           int64
	CompressedSizeBytes int64
	Checksum            string
	Format              string
	StartTime           string
	EndTime             string
}

// RestoreManager defines restore operations.
type RestoreManager interface {
	// Restore performs a database restore
	Restore(ctx context.Context, opts RestoreOptions) (*RestoreResult, error)

	// GetRestoreProgress returns the restore progress (0-100)
	GetRestoreProgress(ctx context.Context, restoreID string) (int, error)

	// CancelRestore cancels a running restore
	CancelRestore(ctx context.Context, restoreID string) error
}

// RestoreOptions contains options for restore
type RestoreOptions struct {
	Database  string
	RestoreID string
	Reader    io.Reader

	// PostgreSQL-specific
	DropExisting    bool
	CreateDatabase  bool
	DataOnly        bool
	SchemaOnly      bool
	NoOwner         bool
	NoPrivileges    bool
	RoleMapping     map[string]string
	Schemas         []string
	Tables          []string
	Jobs            int32
	DisableTriggers bool
	Analyze         bool

	// MySQL-specific
	Routines                bool
	Triggers                bool
	Events                  bool
	DisableForeignKeyChecks bool
	DisableBinlog           bool
}

// RestoreResult contains restore result information
type RestoreResult struct {
	RestoreID      string
	TargetDatabase string
	TablesRestored int32
	StartTime      string
	EndTime        string
	Warnings       []string
}

// ConnectionConfig contains database connection configuration
type ConnectionConfig struct {
	Host     string
	Port     int32
	Database string
	Username string
	Password string

	// TLS
	TLSEnabled bool
	TLSMode    string
	TLSCA      []byte
	TLSCert    []byte
	TLSKey     []byte

	// PostgreSQL-specific
	SSLMode          string
	ConnectTimeout   int32
	StatementTimeout string
	ApplicationName  string

	// MySQL-specific
	Charset      string
	Collation    string
	ParseTime    bool
	Timeout      string
	ReadTimeout  string
	WriteTimeout string
}
