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

package v1alpha1

// PostgresSSLMode defines PostgreSQL SSL modes
// +kubebuilder:validation:Enum=disable;allow;prefer;require;verify-ca;verify-full
type PostgresSSLMode string

const (
	PostgresSSLModeDisable    PostgresSSLMode = "disable"
	PostgresSSLModeAllow      PostgresSSLMode = "allow"
	PostgresSSLModePrefer     PostgresSSLMode = "prefer"
	PostgresSSLModeRequire    PostgresSSLMode = "require"
	PostgresSSLModeVerifyCA   PostgresSSLMode = "verify-ca"
	PostgresSSLModeVerifyFull PostgresSSLMode = "verify-full"
)

// PostgresInstanceConfig defines PostgreSQL-specific instance configuration
type PostgresInstanceConfig struct {
	// SSLMode specifies the SSL mode for connections
	// +kubebuilder:validation:Enum=disable;allow;prefer;require;verify-ca;verify-full
	// +kubebuilder:default=prefer
	SSLMode PostgresSSLMode `json:"sslMode,omitempty"`

	// ConnectTimeout is the connection timeout in seconds
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	ConnectTimeout int32 `json:"connectTimeout,omitempty"`

	// StatementTimeout is the statement timeout (e.g., "30s")
	// +optional
	StatementTimeout string `json:"statementTimeout,omitempty"`

	// ApplicationName is the application name for connections
	// +kubebuilder:default=db-provision-operator
	ApplicationName string `json:"applicationName,omitempty"`
}

// PostgresDatabaseConfig defines PostgreSQL-specific database configuration
type PostgresDatabaseConfig struct {
	// Encoding sets the database encoding (default: UTF8)
	// +kubebuilder:default=UTF8
	Encoding string `json:"encoding,omitempty"`

	// LCCollate sets the collation order
	// +optional
	LCCollate string `json:"lcCollate,omitempty"`

	// LCCtype sets the character classification
	// +optional
	LCCtype string `json:"lcCtype,omitempty"`

	// Tablespace sets the default tablespace
	// +kubebuilder:default=pg_default
	Tablespace string `json:"tablespace,omitempty"`

	// Template is the template database to use
	// +kubebuilder:default=template0
	Template string `json:"template,omitempty"`

	// ConnectionLimit sets the maximum concurrent connections (-1 = unlimited)
	// +kubebuilder:validation:Minimum=-1
	// +kubebuilder:default=-1
	ConnectionLimit int32 `json:"connectionLimit,omitempty"`

	// IsTemplate marks this as a template database
	// +optional
	IsTemplate bool `json:"isTemplate,omitempty"`

	// AllowConnections allows/disallows connections to this database
	// +kubebuilder:default=true
	AllowConnections bool `json:"allowConnections,omitempty"`

	// Extensions to install in the database
	// +optional
	Extensions []PostgresExtension `json:"extensions,omitempty"`

	// Schemas to create in the database
	// +optional
	Schemas []PostgresSchema `json:"schemas,omitempty"`

	// DefaultPrivileges sets default privileges for new objects
	// +optional
	DefaultPrivileges []PostgresDefaultPrivilege `json:"defaultPrivileges,omitempty"`
}

// PostgresExtension defines a PostgreSQL extension to install
type PostgresExtension struct {
	// Name of the extension
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Schema to install the extension in (default: public)
	// +kubebuilder:default=public
	Schema string `json:"schema,omitempty"`

	// Version of the extension (optional, uses default if not specified)
	// +optional
	Version string `json:"version,omitempty"`
}

// PostgresSchema defines a schema to create
type PostgresSchema struct {
	// Name of the schema
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Owner of the schema (optional)
	// +optional
	Owner string `json:"owner,omitempty"`
}

// PostgresDefaultPrivilege defines default privileges for new objects
type PostgresDefaultPrivilege struct {
	// Role to grant privileges to
	// +kubebuilder:validation:Required
	Role string `json:"role"`

	// Schema where the default applies
	// +kubebuilder:validation:Required
	Schema string `json:"schema"`

	// ObjectType is the type of objects (tables, sequences, functions, types)
	// +kubebuilder:validation:Enum=tables;sequences;functions;types
	ObjectType string `json:"objectType"`

	// Privileges to grant
	// +kubebuilder:validation:MinItems=1
	Privileges []string `json:"privileges"`
}

// PostgresUserConfig defines PostgreSQL-specific user configuration
type PostgresUserConfig struct {
	// ConnectionLimit sets the maximum concurrent connections (-1 = unlimited)
	// +kubebuilder:validation:Minimum=-1
	// +kubebuilder:default=-1
	ConnectionLimit int32 `json:"connectionLimit,omitempty"`

	// ValidUntil sets the password expiration time (RFC3339 format)
	// +optional
	ValidUntil string `json:"validUntil,omitempty"`

	// Superuser grants superuser privileges
	// +optional
	Superuser bool `json:"superuser,omitempty"`

	// CreateDB allows the user to create databases
	// +optional
	CreateDB bool `json:"createDB,omitempty"`

	// CreateRole allows the user to create roles
	// +optional
	CreateRole bool `json:"createRole,omitempty"`

	// Inherit enables privilege inheritance
	// +kubebuilder:default=true
	Inherit bool `json:"inherit,omitempty"`

	// Login enables login capability
	// +kubebuilder:default=true
	Login bool `json:"login,omitempty"`

	// Replication enables replication privileges
	// +optional
	Replication bool `json:"replication,omitempty"`

	// BypassRLS allows bypassing row-level security
	// +optional
	BypassRLS bool `json:"bypassRLS,omitempty"`

	// InRoles lists roles this user should be a member of
	// +optional
	InRoles []string `json:"inRoles,omitempty"`

	// DefaultRole sets the role to assume on connection (SET ROLE)
	// This is used for object ownership during rotation - objects created
	// by the user will be owned by this role instead of the user
	// +optional
	DefaultRole string `json:"defaultRole,omitempty"`

	// ConfigParameters sets session parameters for this user
	// +optional
	ConfigParameters map[string]string `json:"configParameters,omitempty"`
}

// PostgresRoleConfig defines PostgreSQL-specific role configuration
type PostgresRoleConfig struct {
	// Login enables login capability (usually false for group roles)
	// +optional
	Login bool `json:"login,omitempty"`

	// Inherit enables privilege inheritance
	// +kubebuilder:default=true
	Inherit bool `json:"inherit,omitempty"`

	// CreateDB allows the role to create databases
	// +optional
	CreateDB bool `json:"createDB,omitempty"`

	// CreateRole allows the role to create other roles
	// +optional
	CreateRole bool `json:"createRole,omitempty"`

	// Superuser grants superuser privileges
	// +optional
	Superuser bool `json:"superuser,omitempty"`

	// Replication enables replication privileges
	// +optional
	Replication bool `json:"replication,omitempty"`

	// BypassRLS allows bypassing row-level security
	// +optional
	BypassRLS bool `json:"bypassRLS,omitempty"`

	// InRoles lists roles this role should inherit from
	// +optional
	InRoles []string `json:"inRoles,omitempty"`

	// Grants defines the permissions this role grants
	// +optional
	Grants []PostgresGrant `json:"grants,omitempty"`
}

// PostgresGrant defines a PostgreSQL privilege grant
type PostgresGrant struct {
	// Database is the target database
	// +kubebuilder:validation:Required
	Database string `json:"database"`

	// Schema is the target schema (optional, for schema-level grants)
	// +optional
	Schema string `json:"schema,omitempty"`

	// Tables lists specific tables or "*" for all tables
	// +optional
	Tables []string `json:"tables,omitempty"`

	// Sequences lists specific sequences or "*" for all sequences
	// +optional
	Sequences []string `json:"sequences,omitempty"`

	// Functions lists specific functions or "*" for all functions
	// +optional
	Functions []string `json:"functions,omitempty"`

	// Privileges to grant (SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER, CREATE, CONNECT, TEMPORARY, EXECUTE, USAGE)
	// +kubebuilder:validation:MinItems=1
	Privileges []string `json:"privileges"`

	// WithGrantOption allows the grantee to grant these privileges to others
	// +optional
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// PostgresGrantConfig defines PostgreSQL-specific grant configuration
type PostgresGrantConfig struct {
	// Roles to assign to the user
	// +optional
	Roles []string `json:"roles,omitempty"`

	// Grants defines direct privilege grants
	// +optional
	Grants []PostgresGrant `json:"grants,omitempty"`

	// DefaultPrivileges sets default privileges for future objects
	// +optional
	DefaultPrivileges []PostgresDefaultPrivilegeGrant `json:"defaultPrivileges,omitempty"`
}

// PostgresDefaultPrivilegeGrant defines a default privilege grant
type PostgresDefaultPrivilegeGrant struct {
	// Database is the target database
	// +kubebuilder:validation:Required
	Database string `json:"database"`

	// Schema is the target schema
	// +kubebuilder:validation:Required
	Schema string `json:"schema"`

	// GrantedBy is the role that creates the objects
	// +kubebuilder:validation:Required
	GrantedBy string `json:"grantedBy"`

	// ObjectType is the type of objects (tables, sequences, functions, types)
	// +kubebuilder:validation:Enum=tables;sequences;functions;types
	ObjectType string `json:"objectType"`

	// Privileges to grant
	// +kubebuilder:validation:MinItems=1
	Privileges []string `json:"privileges"`
}

// PostgresBackupMethod defines PostgreSQL backup methods
// +kubebuilder:validation:Enum=pg_dump;pg_basebackup
type PostgresBackupMethod string

const (
	PostgresBackupMethodPgDump       PostgresBackupMethod = "pg_dump"
	PostgresBackupMethodPgBasebackup PostgresBackupMethod = "pg_basebackup"
)

// PostgresDumpFormat defines pg_dump output formats
// +kubebuilder:validation:Enum=plain;custom;directory;tar
type PostgresDumpFormat string

const (
	PostgresDumpFormatPlain     PostgresDumpFormat = "plain"
	PostgresDumpFormatCustom    PostgresDumpFormat = "custom"
	PostgresDumpFormatDirectory PostgresDumpFormat = "directory"
	PostgresDumpFormatTar       PostgresDumpFormat = "tar"
)

// PostgresBackupConfig defines PostgreSQL-specific backup configuration
type PostgresBackupConfig struct {
	// Method specifies the backup method
	// +kubebuilder:validation:Enum=pg_dump;pg_basebackup
	// +kubebuilder:default=pg_dump
	Method PostgresBackupMethod `json:"method,omitempty"`

	// Format specifies the output format (for pg_dump)
	// +kubebuilder:validation:Enum=plain;custom;directory;tar
	// +kubebuilder:default=custom
	Format PostgresDumpFormat `json:"format,omitempty"`

	// Jobs sets the number of parallel jobs (for directory format)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Jobs int32 `json:"jobs,omitempty"`

	// DataOnly backs up only data, not schema
	// +optional
	DataOnly bool `json:"dataOnly,omitempty"`

	// SchemaOnly backs up only schema, not data
	// +optional
	SchemaOnly bool `json:"schemaOnly,omitempty"`

	// Blobs includes large objects in the backup
	// +kubebuilder:default=true
	Blobs bool `json:"blobs,omitempty"`

	// NoOwner omits ownership information
	// +optional
	NoOwner bool `json:"noOwner,omitempty"`

	// NoPrivileges omits privilege (GRANT/REVOKE) information
	// +optional
	NoPrivileges bool `json:"noPrivileges,omitempty"`

	// Schemas lists specific schemas to include (empty = all)
	// +optional
	Schemas []string `json:"schemas,omitempty"`

	// ExcludeSchemas lists schemas to exclude
	// +optional
	ExcludeSchemas []string `json:"excludeSchemas,omitempty"`

	// Tables lists specific tables to include (empty = all)
	// +optional
	Tables []string `json:"tables,omitempty"`

	// ExcludeTables lists tables to exclude (format: schema.table)
	// +optional
	ExcludeTables []string `json:"excludeTables,omitempty"`

	// LockWaitTimeout sets the lock wait timeout (e.g., "60s")
	// +kubebuilder:default="60s"
	LockWaitTimeout string `json:"lockWaitTimeout,omitempty"`

	// NoSync disables fsync after backup
	// +optional
	NoSync bool `json:"noSync,omitempty"`
}

// PostgresRestoreConfig defines PostgreSQL-specific restore configuration
type PostgresRestoreConfig struct {
	// DropExisting drops existing database before restore
	// +optional
	DropExisting bool `json:"dropExisting,omitempty"`

	// CreateDatabase creates the database if it doesn't exist
	// +kubebuilder:default=true
	CreateDatabase bool `json:"createDatabase,omitempty"`

	// DataOnly restores only data, not schema
	// +optional
	DataOnly bool `json:"dataOnly,omitempty"`

	// SchemaOnly restores only schema, not data
	// +optional
	SchemaOnly bool `json:"schemaOnly,omitempty"`

	// NoOwner omits ownership restoration
	// +kubebuilder:default=true
	NoOwner bool `json:"noOwner,omitempty"`

	// NoPrivileges omits privilege restoration
	// +optional
	NoPrivileges bool `json:"noPrivileges,omitempty"`

	// RoleMapping maps old role names to new role names
	// +optional
	RoleMapping map[string]string `json:"roleMapping,omitempty"`

	// Schemas lists specific schemas to restore (empty = all)
	// +optional
	Schemas []string `json:"schemas,omitempty"`

	// Tables lists specific tables to restore (empty = all)
	// +optional
	Tables []string `json:"tables,omitempty"`

	// Jobs sets the number of parallel jobs for restore
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Jobs int32 `json:"jobs,omitempty"`

	// DisableTriggers disables triggers during restore
	// +optional
	DisableTriggers bool `json:"disableTriggers,omitempty"`

	// Analyze runs ANALYZE after restore
	// +kubebuilder:default=true
	Analyze bool `json:"analyze,omitempty"`
}

// PostgresDatabaseStatus contains PostgreSQL-specific database status
type PostgresDatabaseStatus struct {
	// Encoding is the database encoding
	Encoding string `json:"encoding,omitempty"`

	// Collation is the database collation
	Collation string `json:"collation,omitempty"`

	// InstalledExtensions lists installed extensions
	InstalledExtensions []PostgresExtensionStatus `json:"installedExtensions,omitempty"`

	// Schemas lists schemas in the database
	Schemas []string `json:"schemas,omitempty"`
}

// PostgresExtensionStatus contains extension status information
type PostgresExtensionStatus struct {
	// Name of the extension
	Name string `json:"name"`

	// Version of the extension
	Version string `json:"version"`
}
