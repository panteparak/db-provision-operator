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

// MySQLTLSMode defines MySQL TLS modes
// +kubebuilder:validation:Enum=disabled;preferred;required;skip-verify
type MySQLTLSMode string

const (
	MySQLTLSModeDisabled   MySQLTLSMode = "disabled"
	MySQLTLSModePreferred  MySQLTLSMode = "preferred"
	MySQLTLSModeRequired   MySQLTLSMode = "required"
	MySQLTLSModeSkipVerify MySQLTLSMode = "skip-verify"
)

// MySQLInstanceConfig defines MySQL-specific instance configuration
type MySQLInstanceConfig struct {
	// Charset sets the default character set
	// +kubebuilder:default=utf8mb4
	Charset string `json:"charset,omitempty"`

	// Collation sets the default collation
	// +kubebuilder:default=utf8mb4_unicode_ci
	Collation string `json:"collation,omitempty"`

	// ParseTime enables parsing of DATE and DATETIME to time.Time
	// +kubebuilder:default=true
	ParseTime bool `json:"parseTime,omitempty"`

	// Timeout is the connection timeout (e.g., "10s")
	// +kubebuilder:default="10s"
	Timeout string `json:"timeout,omitempty"`

	// ReadTimeout is the read timeout (e.g., "30s")
	// +kubebuilder:default="30s"
	ReadTimeout string `json:"readTimeout,omitempty"`

	// WriteTimeout is the write timeout (e.g., "30s")
	// +kubebuilder:default="30s"
	WriteTimeout string `json:"writeTimeout,omitempty"`

	// TLS specifies the TLS mode
	// +kubebuilder:validation:Enum=disabled;preferred;required;skip-verify
	// +kubebuilder:default=preferred
	TLS MySQLTLSMode `json:"tls,omitempty"`
}

// MySQLDatabaseConfig defines MySQL-specific database configuration
type MySQLDatabaseConfig struct {
	// Charset sets the database character set
	// +kubebuilder:default=utf8mb4
	Charset string `json:"charset,omitempty"`

	// Collation sets the database collation
	// +kubebuilder:default=utf8mb4_unicode_ci
	Collation string `json:"collation,omitempty"`

	// SQLMode sets the SQL mode for the database
	// +optional
	SQLMode string `json:"sqlMode,omitempty"`

	// DefaultStorageEngine sets the default storage engine
	// +kubebuilder:default=InnoDB
	DefaultStorageEngine string `json:"defaultStorageEngine,omitempty"`
}

// MySQLAuthPlugin defines MySQL authentication plugins
// +kubebuilder:validation:Enum=mysql_native_password;caching_sha2_password;sha256_password
type MySQLAuthPlugin string

const (
	MySQLAuthPluginNative       MySQLAuthPlugin = "mysql_native_password"
	MySQLAuthPluginCachingSHA2  MySQLAuthPlugin = "caching_sha2_password"
	MySQLAuthPluginSHA256       MySQLAuthPlugin = "sha256_password"
)

// MySQLUserConfig defines MySQL-specific user configuration
type MySQLUserConfig struct {
	// MaxQueriesPerHour limits queries per hour (0 = unlimited)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxQueriesPerHour int32 `json:"maxQueriesPerHour,omitempty"`

	// MaxUpdatesPerHour limits updates per hour (0 = unlimited)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxUpdatesPerHour int32 `json:"maxUpdatesPerHour,omitempty"`

	// MaxConnectionsPerHour limits connections per hour (0 = unlimited)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxConnectionsPerHour int32 `json:"maxConnectionsPerHour,omitempty"`

	// MaxUserConnections limits concurrent connections (0 = unlimited)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxUserConnections int32 `json:"maxUserConnections,omitempty"`

	// AuthPlugin specifies the authentication plugin
	// +kubebuilder:validation:Enum=mysql_native_password;caching_sha2_password;sha256_password
	// +kubebuilder:default=caching_sha2_password
	AuthPlugin MySQLAuthPlugin `json:"authPlugin,omitempty"`

	// RequireSSL requires SSL for connections
	// +optional
	RequireSSL bool `json:"requireSSL,omitempty"`

	// RequireX509 requires X509 certificate for connections
	// +optional
	RequireX509 bool `json:"requireX509,omitempty"`

	// AllowedHosts lists allowed host patterns for the user (e.g., "%", "localhost", "192.168.1.%")
	// +kubebuilder:default={"%"}
	AllowedHosts []string `json:"allowedHosts,omitempty"`

	// AccountLocked locks the account
	// +optional
	AccountLocked bool `json:"accountLocked,omitempty"`

	// FailedLoginAttempts sets failed login attempts before locking (0 = disabled)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	FailedLoginAttempts int32 `json:"failedLoginAttempts,omitempty"`

	// PasswordLockTime sets lock time in days after failed attempts (0 = permanent)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	PasswordLockTime int32 `json:"passwordLockTime,omitempty"`
}

// MySQLRoleConfig defines MySQL-specific role configuration
type MySQLRoleConfig struct {
	// UseNativeRoles enables MySQL 8.0+ native roles
	// +kubebuilder:default=true
	UseNativeRoles bool `json:"useNativeRoles,omitempty"`

	// Grants defines the permissions this role grants
	// +optional
	Grants []MySQLGrant `json:"grants,omitempty"`
}

// MySQLGrantLevel defines the level of a MySQL grant
// +kubebuilder:validation:Enum=global;database;table;column;procedure;function
type MySQLGrantLevel string

const (
	MySQLGrantLevelGlobal    MySQLGrantLevel = "global"
	MySQLGrantLevelDatabase  MySQLGrantLevel = "database"
	MySQLGrantLevelTable     MySQLGrantLevel = "table"
	MySQLGrantLevelColumn    MySQLGrantLevel = "column"
	MySQLGrantLevelProcedure MySQLGrantLevel = "procedure"
	MySQLGrantLevelFunction  MySQLGrantLevel = "function"
)

// MySQLGrant defines a MySQL privilege grant
type MySQLGrant struct {
	// Level is the grant level
	// +kubebuilder:validation:Enum=global;database;table;column;procedure;function
	Level MySQLGrantLevel `json:"level"`

	// Database is the target database (for database/table/column/procedure/function levels)
	// +optional
	Database string `json:"database,omitempty"`

	// Table is the target table (for table/column levels)
	// +optional
	Table string `json:"table,omitempty"`

	// Columns lists target columns (for column level)
	// +optional
	Columns []string `json:"columns,omitempty"`

	// Procedure is the target procedure (for procedure level)
	// +optional
	Procedure string `json:"procedure,omitempty"`

	// Function is the target function (for function level)
	// +optional
	Function string `json:"function,omitempty"`

	// Privileges to grant (SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER, etc.)
	// +kubebuilder:validation:MinItems=1
	Privileges []string `json:"privileges"`

	// WithGrantOption allows the grantee to grant these privileges to others
	// +optional
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// MySQLGrantConfig defines MySQL-specific grant configuration
type MySQLGrantConfig struct {
	// Roles to assign to the user (MySQL 8.0+)
	// +optional
	Roles []string `json:"roles,omitempty"`

	// Grants defines direct privilege grants
	// +optional
	Grants []MySQLGrant `json:"grants,omitempty"`
}

// MySQLBackupMethod defines MySQL backup methods
// +kubebuilder:validation:Enum=mysqldump;xtrabackup;mysqlpump
type MySQLBackupMethod string

const (
	MySQLBackupMethodMysqldump  MySQLBackupMethod = "mysqldump"
	MySQLBackupMethodXtrabackup MySQLBackupMethod = "xtrabackup"
	MySQLBackupMethodMysqlpump  MySQLBackupMethod = "mysqlpump"
)

// MySQLGtidPurged defines GTID_PURGED setting
// +kubebuilder:validation:Enum=OFF;ON;AUTO
type MySQLGtidPurged string

const (
	MySQLGtidPurgedOff  MySQLGtidPurged = "OFF"
	MySQLGtidPurgedOn   MySQLGtidPurged = "ON"
	MySQLGtidPurgedAuto MySQLGtidPurged = "AUTO"
)

// MySQLBackupConfig defines MySQL-specific backup configuration
type MySQLBackupConfig struct {
	// Method specifies the backup method
	// +kubebuilder:validation:Enum=mysqldump;xtrabackup;mysqlpump
	// +kubebuilder:default=mysqldump
	Method MySQLBackupMethod `json:"method,omitempty"`

	// SingleTransaction uses a single transaction for InnoDB tables
	// +kubebuilder:default=true
	SingleTransaction bool `json:"singleTransaction,omitempty"`

	// Quick retrieves rows one at a time instead of buffering
	// +kubebuilder:default=true
	Quick bool `json:"quick,omitempty"`

	// LockTables locks all tables before backup
	// +optional
	LockTables bool `json:"lockTables,omitempty"`

	// Routines includes stored procedures and functions
	// +kubebuilder:default=true
	Routines bool `json:"routines,omitempty"`

	// Triggers includes triggers
	// +kubebuilder:default=true
	Triggers bool `json:"triggers,omitempty"`

	// Events includes events
	// +kubebuilder:default=true
	Events bool `json:"events,omitempty"`

	// ExtendedInsert uses extended INSERT statements
	// +kubebuilder:default=true
	ExtendedInsert bool `json:"extendedInsert,omitempty"`

	// SetGtidPurged controls SET @@GLOBAL.GTID_PURGED
	// +kubebuilder:validation:Enum=OFF;ON;AUTO
	// +kubebuilder:default=AUTO
	SetGtidPurged MySQLGtidPurged `json:"setGtidPurged,omitempty"`

	// Databases lists specific databases to backup (empty = all)
	// +optional
	Databases []string `json:"databases,omitempty"`

	// Tables lists specific tables to backup (empty = all)
	// +optional
	Tables []string `json:"tables,omitempty"`

	// ExcludeTables lists tables to exclude
	// +optional
	ExcludeTables []string `json:"excludeTables,omitempty"`
}

// MySQLRestoreConfig defines MySQL-specific restore configuration
type MySQLRestoreConfig struct {
	// DropExisting drops existing database before restore
	// +optional
	DropExisting bool `json:"dropExisting,omitempty"`

	// CreateDatabase creates the database if it doesn't exist
	// +kubebuilder:default=true
	CreateDatabase bool `json:"createDatabase,omitempty"`

	// Routines restores stored procedures and functions
	// +kubebuilder:default=true
	Routines bool `json:"routines,omitempty"`

	// Triggers restores triggers
	// +kubebuilder:default=true
	Triggers bool `json:"triggers,omitempty"`

	// Events restores events
	// +kubebuilder:default=true
	Events bool `json:"events,omitempty"`

	// DisableForeignKeyChecks disables foreign key checks during restore
	// +kubebuilder:default=true
	DisableForeignKeyChecks bool `json:"disableForeignKeyChecks,omitempty"`

	// DisableBinlog disables binary logging during restore
	// +kubebuilder:default=true
	DisableBinlog bool `json:"disableBinlog,omitempty"`
}

// MySQLDatabaseStatus contains MySQL-specific database status
type MySQLDatabaseStatus struct {
	// Charset is the database character set
	Charset string `json:"charset,omitempty"`

	// Collation is the database collation
	Collation string `json:"collation,omitempty"`
}
