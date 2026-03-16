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
	"testing"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/stretchr/testify/assert"
)

func TestMySQLSpecBuilder_BuildDatabaseCreateOptions(t *testing.T) {
	builder := NewMySQLSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseSpec
		expected types.CreateDatabaseOptions
	}{
		{
			name: "minimal spec with nil MySQL field",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:  "testdb",
				Owner: "admin",
			},
			expected: types.CreateDatabaseOptions{
				Name:  "testdb",
				Owner: "admin",
			},
		},
		{
			name: "full spec with Charset and Collation",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:  "mydb",
				Owner: "dbowner",
				MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{
					Charset:   "utf8mb4",
					Collation: "utf8mb4_unicode_ci",
				},
			},
			expected: types.CreateDatabaseOptions{
				Name:      "mydb",
				Owner:     "dbowner",
				Charset:   "utf8mb4",
				Collation: "utf8mb4_unicode_ci",
			},
		},
		{
			name: "empty MySQL config returns base options only",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:  "emptydb",
				MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{},
			},
			expected: types.CreateDatabaseOptions{
				Name: "emptydb",
			},
		},
		{
			name: "charset only without collation",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "charsetdb",
				MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{
					Charset: "latin1",
				},
			},
			expected: types.CreateDatabaseOptions{
				Name:    "charsetdb",
				Charset: "latin1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildDatabaseCreateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLSpecBuilder_BuildDatabaseUpdateOptions(t *testing.T) {
	builder := NewMySQLSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseSpec
		expected types.UpdateDatabaseOptions
	}{
		{
			name: "nil MySQL field returns empty options",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			expected: types.UpdateDatabaseOptions{},
		},
		{
			name: "full spec with Charset and Collation",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{
					Charset:   "utf8mb4",
					Collation: "utf8mb4_general_ci",
				},
			},
			expected: types.UpdateDatabaseOptions{
				Charset:   "utf8mb4",
				Collation: "utf8mb4_general_ci",
			},
		},
		{
			name: "empty MySQL config returns empty options",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:  "testdb",
				MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{},
			},
			expected: types.UpdateDatabaseOptions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildDatabaseUpdateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLSpecBuilder_BuildUserCreateOptions(t *testing.T) {
	builder := NewMySQLSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseUserSpec
		password string
		expected types.CreateUserOptions
	}{
		{
			name: "minimal spec with nil MySQL field uses defaults",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
			},
			password: "secret123",
			expected: types.CreateUserOptions{
				Username: "testuser",
				Password: "secret123",
				Login:    true,
				Inherit:  true,
			},
		},
		{
			name: "full spec with all MySQL options",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "appuser",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					MaxQueriesPerHour:     100,
					MaxUpdatesPerHour:     50,
					MaxConnectionsPerHour: 20,
					MaxUserConnections:    5,
					AuthPlugin:            dbopsv1alpha1.MySQLAuthPluginCachingSHA2,
					RequireSSL:            true,
					RequireX509:           true,
					AllowedHosts:          []string{"192.168.1.%", "10.0.0.%"},
					AccountLocked:         true,
				},
			},
			password: "strongpass",
			expected: types.CreateUserOptions{
				Username:              "appuser",
				Password:              "strongpass",
				Login:                 true,
				Inherit:               true,
				MaxQueriesPerHour:     100,
				MaxUpdatesPerHour:     50,
				MaxConnectionsPerHour: 20,
				MaxUserConnections:    5,
				AuthPlugin:            "caching_sha2_password",
				RequireSSL:            true,
				RequireX509:           true,
				AllowedHosts:          []string{"192.168.1.%", "10.0.0.%"},
				AccountLocked:         true,
			},
		},
		{
			name: "empty MySQL config keeps defaults for Login and Inherit",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "minuser",
				MySQL:    &dbopsv1alpha1.MySQLUserConfig{},
			},
			password: "pass",
			expected: types.CreateUserOptions{
				Username: "minuser",
				Password: "pass",
				Login:    true,
				Inherit:  true,
			},
		},
		{
			name: "native password auth plugin",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "legacyuser",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					AuthPlugin: dbopsv1alpha1.MySQLAuthPluginNative,
				},
			},
			password: "pass",
			expected: types.CreateUserOptions{
				Username:   "legacyuser",
				Password:   "pass",
				Login:      true,
				Inherit:    true,
				AuthPlugin: "mysql_native_password",
			},
		},
		{
			name: "empty allowed hosts array",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "nohost",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					AllowedHosts: []string{},
				},
			},
			password: "pass",
			expected: types.CreateUserOptions{
				Username:     "nohost",
				Password:     "pass",
				Login:        true,
				Inherit:      true,
				AllowedHosts: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildUserCreateOptions(tt.spec, tt.password)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLSpecBuilder_BuildUserUpdateOptions(t *testing.T) {
	builder := NewMySQLSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseUserSpec
		expected types.UpdateUserOptions
	}{
		{
			name: "nil MySQL field returns empty options",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
			},
			expected: types.UpdateUserOptions{},
		},
		{
			name: "full spec with all rate limits",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					MaxQueriesPerHour:     200,
					MaxUpdatesPerHour:     100,
					MaxConnectionsPerHour: 50,
					MaxUserConnections:    10,
				},
			},
			expected: func() types.UpdateUserOptions {
				mqph := int32(200)
				muph := int32(100)
				mcph := int32(50)
				muc := int32(10)
				return types.UpdateUserOptions{
					MaxQueriesPerHour:     &mqph,
					MaxUpdatesPerHour:     &muph,
					MaxConnectionsPerHour: &mcph,
					MaxUserConnections:    &muc,
				}
			}(),
		},
		{
			name: "zero values are not set as pointers",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					MaxQueriesPerHour:     0,
					MaxUpdatesPerHour:     0,
					MaxConnectionsPerHour: 0,
					MaxUserConnections:    0,
				},
			},
			expected: types.UpdateUserOptions{},
		},
		{
			name: "partial rate limits - only non-zero values set",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					MaxQueriesPerHour:  500,
					MaxUserConnections: 3,
				},
			},
			expected: func() types.UpdateUserOptions {
				mqph := int32(500)
				muc := int32(3)
				return types.UpdateUserOptions{
					MaxQueriesPerHour:  &mqph,
					MaxUserConnections: &muc,
				}
			}(),
		},
		{
			name: "empty MySQL config returns empty options",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				MySQL:    &dbopsv1alpha1.MySQLUserConfig{},
			},
			expected: types.UpdateUserOptions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildUserUpdateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLSpecBuilder_BuildRoleCreateOptions(t *testing.T) {
	builder := NewMySQLSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseRoleSpec
		expected types.CreateRoleOptions
	}{
		{
			name: "minimal spec with nil MySQL field uses defaults",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "myrole",
			},
			expected: types.CreateRoleOptions{
				RoleName: "myrole",
				Inherit:  true,
			},
		},
		{
			name: "full spec with UseNativeRoles and grants",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "approle",
				MySQL: &dbopsv1alpha1.MySQLRoleConfig{
					UseNativeRoles: true,
					Grants: []dbopsv1alpha1.MySQLGrant{
						{
							Level:           dbopsv1alpha1.MySQLGrantLevelDatabase,
							Database:        "appdb",
							Privileges:      []string{"SELECT", "INSERT", "UPDATE"},
							WithGrantOption: true,
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelTable,
							Database:   "appdb",
							Table:      "users",
							Privileges: []string{"SELECT", "DELETE"},
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelColumn,
							Database:   "appdb",
							Table:      "orders",
							Columns:    []string{"id", "status"},
							Privileges: []string{"SELECT", "UPDATE"},
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelProcedure,
							Database:   "appdb",
							Procedure:  "process_order",
							Privileges: []string{"EXECUTE"},
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelFunction,
							Database:   "appdb",
							Function:   "calculate_total",
							Privileges: []string{"EXECUTE"},
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelGlobal,
							Privileges: []string{"RELOAD", "PROCESS"},
						},
					},
				},
			},
			expected: types.CreateRoleOptions{
				RoleName:       "approle",
				Inherit:        true,
				UseNativeRoles: true,
				Grants: []types.GrantOptions{
					{
						Level:           "database",
						Database:        "appdb",
						Privileges:      []string{"SELECT", "INSERT", "UPDATE"},
						WithGrantOption: true,
					},
					{
						Level:      "table",
						Database:   "appdb",
						Table:      "users",
						Privileges: []string{"SELECT", "DELETE"},
					},
					{
						Level:      "column",
						Database:   "appdb",
						Table:      "orders",
						Columns:    []string{"id", "status"},
						Privileges: []string{"SELECT", "UPDATE"},
					},
					{
						Level:      "procedure",
						Database:   "appdb",
						Procedure:  "process_order",
						Privileges: []string{"EXECUTE"},
					},
					{
						Level:      "function",
						Database:   "appdb",
						Function:   "calculate_total",
						Privileges: []string{"EXECUTE"},
					},
					{
						Level:      "global",
						Privileges: []string{"RELOAD", "PROCESS"},
					},
				},
			},
		},
		{
			name: "empty MySQL config keeps defaults",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "emptyrole",
				MySQL:    &dbopsv1alpha1.MySQLRoleConfig{},
			},
			expected: types.CreateRoleOptions{
				RoleName: "emptyrole",
				Inherit:  true,
			},
		},
		{
			name: "empty grants array does not populate Grants field",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "nograntrole",
				MySQL: &dbopsv1alpha1.MySQLRoleConfig{
					UseNativeRoles: true,
					Grants:         []dbopsv1alpha1.MySQLGrant{},
				},
			},
			expected: types.CreateRoleOptions{
				RoleName:       "nograntrole",
				Inherit:        true,
				UseNativeRoles: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildRoleCreateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMySQLSpecBuilder_BuildRoleUpdateOptions(t *testing.T) {
	builder := NewMySQLSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseRoleSpec
		expected types.UpdateRoleOptions
	}{
		{
			name: "nil MySQL field returns empty options",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "myrole",
			},
			expected: types.UpdateRoleOptions{},
		},
		{
			name: "grants are populated into AddGrants",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "approle",
				MySQL: &dbopsv1alpha1.MySQLRoleConfig{
					Grants: []dbopsv1alpha1.MySQLGrant{
						{
							Level:           dbopsv1alpha1.MySQLGrantLevelDatabase,
							Database:        "appdb",
							Privileges:      []string{"ALL PRIVILEGES"},
							WithGrantOption: true,
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelTable,
							Database:   "appdb",
							Table:      "logs",
							Privileges: []string{"SELECT"},
						},
					},
				},
			},
			expected: types.UpdateRoleOptions{
				AddGrants: []types.GrantOptions{
					{
						Level:           "database",
						Database:        "appdb",
						Privileges:      []string{"ALL PRIVILEGES"},
						WithGrantOption: true,
					},
					{
						Level:      "table",
						Database:   "appdb",
						Table:      "logs",
						Privileges: []string{"SELECT"},
					},
				},
			},
		},
		{
			name: "empty grants array does not populate AddGrants",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "emptyrole",
				MySQL: &dbopsv1alpha1.MySQLRoleConfig{
					Grants: []dbopsv1alpha1.MySQLGrant{},
				},
			},
			expected: types.UpdateRoleOptions{},
		},
		{
			name: "empty MySQL config returns empty options",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "emptyrole",
				MySQL:    &dbopsv1alpha1.MySQLRoleConfig{},
			},
			expected: types.UpdateRoleOptions{},
		},
		{
			name: "column-level grant with columns and procedure/function fields",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "colrole",
				MySQL: &dbopsv1alpha1.MySQLRoleConfig{
					Grants: []dbopsv1alpha1.MySQLGrant{
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelColumn,
							Database:   "mydb",
							Table:      "sensitive",
							Columns:    []string{"email", "phone"},
							Privileges: []string{"SELECT"},
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelProcedure,
							Database:   "mydb",
							Procedure:  "cleanup",
							Privileges: []string{"EXECUTE"},
						},
						{
							Level:      dbopsv1alpha1.MySQLGrantLevelFunction,
							Database:   "mydb",
							Function:   "hash_value",
							Privileges: []string{"EXECUTE"},
						},
					},
				},
			},
			expected: types.UpdateRoleOptions{
				AddGrants: []types.GrantOptions{
					{
						Level:      "column",
						Database:   "mydb",
						Table:      "sensitive",
						Columns:    []string{"email", "phone"},
						Privileges: []string{"SELECT"},
					},
					{
						Level:      "procedure",
						Database:   "mydb",
						Procedure:  "cleanup",
						Privileges: []string{"EXECUTE"},
					},
					{
						Level:      "function",
						Database:   "mydb",
						Function:   "hash_value",
						Privileges: []string{"EXECUTE"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildRoleUpdateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}
