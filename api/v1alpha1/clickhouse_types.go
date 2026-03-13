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
	// HTTPPort is the ClickHouse HTTP interface port (optional, for HTTP protocol alongside native)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	HTTPPort *int32 `json:"httpPort,omitempty"`

	// Secure enables TLS for native protocol connections
	// +optional
	Secure bool `json:"secure,omitempty"`
}

// ClickHouseDatabaseConfig defines ClickHouse-specific database configuration
type ClickHouseDatabaseConfig struct {
	// Engine specifies the database engine (e.g., "Atomic", "Lazy", "Replicated")
	// +kubebuilder:default=Atomic
	// +optional
	Engine string `json:"engine,omitempty"`
}

// ClickHouseUserConfig defines ClickHouse-specific user configuration
type ClickHouseUserConfig struct {
	// AllowedHosts restricts where the user can connect from.
	// Supports IP addresses, hostnames, and LIKE patterns.
	// If empty, the user can connect from anywhere.
	// +optional
	AllowedHosts []string `json:"allowedHosts,omitempty"`

	// DefaultDatabase sets the default database for the user
	// +optional
	DefaultDatabase string `json:"defaultDatabase,omitempty"`
}

// ClickHouseRoleConfig defines ClickHouse-specific role configuration
type ClickHouseRoleConfig struct {
	// Settings defines ClickHouse settings to apply to the role
	// +optional
	Settings map[string]string `json:"settings,omitempty"`
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

// ClickHouseGrantLevel defines the level of a ClickHouse grant
// +kubebuilder:validation:Enum=global;database;table
type ClickHouseGrantLevel string

const (
	ClickHouseGrantLevelGlobal   ClickHouseGrantLevel = "global"
	ClickHouseGrantLevelDatabase ClickHouseGrantLevel = "database"
	ClickHouseGrantLevelTable    ClickHouseGrantLevel = "table"
)

// ClickHouseGrant defines a ClickHouse privilege grant
type ClickHouseGrant struct {
	// Level is the grant level
	// +kubebuilder:validation:Enum=global;database;table
	Level ClickHouseGrantLevel `json:"level"`

	// Database is the target database (for database/table levels)
	// +optional
	Database string `json:"database,omitempty"`

	// Table is the target table (for table level)
	// +optional
	Table string `json:"table,omitempty"`

	// Privileges to grant (SELECT, INSERT, ALTER, CREATE, DROP, TRUNCATE, SHOW, etc.)
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=20
	// +kubebuilder:validation:items:Pattern=`^[A-Z][A-Z ]*$`
	Privileges []string `json:"privileges"`

	// WithGrantOption allows the grantee to grant these privileges to others
	// +optional
	WithGrantOption bool `json:"withGrantOption,omitempty"`
}

// ClickHouseBackupConfig defines ClickHouse-specific backup configuration
type ClickHouseBackupConfig struct {
	// Method specifies the backup method
	// +kubebuilder:default=sql
	// +optional
	Method string `json:"method,omitempty"`
}

// ClickHouseRestoreConfig defines ClickHouse-specific restore configuration
type ClickHouseRestoreConfig struct {
	// DropExisting drops existing database before restore
	// +optional
	DropExisting bool `json:"dropExisting,omitempty"`
}
