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

// ClickHouseInstanceConfig defines ClickHouse-specific instance configuration
type ClickHouseInstanceConfig struct {
	// DialTimeout is the connection dial timeout (e.g., "10s")
	// +kubebuilder:validation:Optional
	DialTimeout string `json:"dialTimeout,omitempty"`

	// ReadTimeout is the read timeout (e.g., "30s")
	// +kubebuilder:validation:Optional
	ReadTimeout string `json:"readTimeout,omitempty"`

	// MaxOpenConns sets the maximum number of open connections
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=25
	MaxOpenConns int32 `json:"maxOpenConns,omitempty"`

	// Debug enables debug logging for the ClickHouse driver
	// +kubebuilder:validation:Optional
	Debug bool `json:"debug,omitempty"`
}

// ClickHouseDatabaseConfig defines ClickHouse-specific database configuration
type ClickHouseDatabaseConfig struct {
	// Engine is the database engine (Atomic, Lazy, Replicated)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Atomic;Lazy;Replicated
	// +kubebuilder:default=Atomic
	Engine string `json:"engine,omitempty"`

	// Comment is the database comment
	// +kubebuilder:validation:Optional
	Comment string `json:"comment,omitempty"`
}

// ClickHouseHostRestrictionType defines the type of HOST clause
// +kubebuilder:validation:Enum=IP;LIKE;REGEXP;NAME;ANY;LOCAL;NONE
type ClickHouseHostRestrictionType string

const (
	ClickHouseHostIP     ClickHouseHostRestrictionType = "IP"
	ClickHouseHostLike   ClickHouseHostRestrictionType = "LIKE"
	ClickHouseHostRegexp ClickHouseHostRestrictionType = "REGEXP"
	ClickHouseHostName   ClickHouseHostRestrictionType = "NAME"
	ClickHouseHostAny    ClickHouseHostRestrictionType = "ANY"
	ClickHouseHostLocal  ClickHouseHostRestrictionType = "LOCAL"
	ClickHouseHostNone   ClickHouseHostRestrictionType = "NONE"
)

// ClickHouseHostRestriction specifies a HOST clause for user access control
type ClickHouseHostRestriction struct {
	// Type is the host restriction type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=IP;LIKE;REGEXP;NAME;ANY;LOCAL;NONE
	Type ClickHouseHostRestrictionType `json:"type"`

	// Value is the host restriction value (not required for ANY, LOCAL, NONE)
	// +kubebuilder:validation:Optional
	Value string `json:"value,omitempty"`
}

// ClickHouseUserConfig defines ClickHouse-specific user configuration
type ClickHouseUserConfig struct {
	// HostRestrictions specifies HOST clauses for user access control
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MaxItems=10
	HostRestrictions []ClickHouseHostRestriction `json:"hostRestrictions,omitempty"`

	// DefaultDatabase is the default database for the user
	// +kubebuilder:validation:Optional
	DefaultDatabase string `json:"defaultDatabase,omitempty"`

	// DefaultRole is the default role for the user
	// +kubebuilder:validation:Optional
	DefaultRole string `json:"defaultRole,omitempty"`
}

// ClickHouseRoleConfig defines ClickHouse-specific role configuration.
// ClickHouse roles are pure privilege containers with no attributes.
type ClickHouseRoleConfig struct{}

// ClickHouseGrant defines a ClickHouse privilege grant
type ClickHouseGrant struct {
	// Privileges to grant (SELECT, INSERT, ALTER, CREATE, DROP, etc.)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:items:Pattern=`^[A-Z][A-Z ]*$`
	Privileges []string `json:"privileges"`

	// Database is the target database
	// +optional
	Database string `json:"database,omitempty"`

	// Table is the target table
	// +optional
	Table string `json:"table,omitempty"`

	// Columns lists target columns for column-level grants
	// +optional
	Columns []string `json:"columns,omitempty"`

	// WithGrantOption allows the grantee to grant these privileges to others
	// +optional
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// ClickHouseGrantConfig defines ClickHouse-specific grant configuration
type ClickHouseGrantConfig struct {
	// Roles to assign to the user
	// +optional
	Roles []string `json:"roles,omitempty"`

	// Grants defines direct privilege grants
	// +optional
	Grants []ClickHouseGrant `json:"grants,omitempty"`
}

// ClickHouseBackupConfig defines ClickHouse-specific backup configuration.
// Uses native BACKUP DATABASE ... TO Disk(...) SQL.
type ClickHouseBackupConfig struct {
	// DiskName is the ClickHouse disk name for BACKUP TO Disk(...)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="backups"
	DiskName string `json:"diskName,omitempty"`

	// BaseBackup is the base backup path for incremental backups
	// +kubebuilder:validation:Optional
	BaseBackup string `json:"baseBackup,omitempty"`
}

// ClickHouseRestoreConfig defines ClickHouse-specific restore configuration.
// Uses native RESTORE DATABASE ... FROM Disk(...) SQL.
type ClickHouseRestoreConfig struct {
	// DiskName is the ClickHouse disk name for RESTORE FROM Disk(...)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="backups"
	DiskName string `json:"diskName,omitempty"`
}

// ClickHouseDatabaseStatus contains ClickHouse-specific database status
type ClickHouseDatabaseStatus struct {
	// Engine is the database engine
	Engine string `json:"engine,omitempty"`
}
