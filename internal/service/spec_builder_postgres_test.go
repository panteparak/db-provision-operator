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

func TestPostgresSpecBuilder_BuildDatabaseCreateOptions(t *testing.T) {
	builder := NewPostgresSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseSpec
		expected types.CreateDatabaseOptions
	}{
		{
			name: "minimal spec with nil Postgres field",
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
			name: "full spec with all Postgres options",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:  "mydb",
				Owner: "dbowner",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Encoding:         "UTF8",
					LCCollate:        "en_US.UTF-8",
					LCCtype:          "en_US.UTF-8",
					Tablespace:       "pg_default",
					Template:         "template0",
					ConnectionLimit:  50,
					IsTemplate:       true,
					AllowConnections: true,
				},
			},
			expected: types.CreateDatabaseOptions{
				Name:             "mydb",
				Owner:            "dbowner",
				Encoding:         "UTF8",
				LCCollate:        "en_US.UTF-8",
				LCCtype:          "en_US.UTF-8",
				Tablespace:       "pg_default",
				Template:         "template0",
				ConnectionLimit:  50,
				IsTemplate:       true,
				AllowConnections: true,
			},
		},
		{
			name: "spec with empty Postgres config",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:     "emptydb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{},
			},
			expected: types.CreateDatabaseOptions{
				Name: "emptydb",
			},
		},
		{
			name: "spec with partial Postgres config",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "partialdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Encoding: "SQL_ASCII",
					Template: "template1",
				},
			},
			expected: types.CreateDatabaseOptions{
				Name:     "partialdb",
				Encoding: "SQL_ASCII",
				Template: "template1",
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

func TestPostgresSpecBuilder_BuildDatabaseUpdateOptions(t *testing.T) {
	builder := NewPostgresSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseSpec
		expected types.UpdateDatabaseOptions
	}{
		{
			name: "nil Postgres field returns empty options",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			expected: types.UpdateDatabaseOptions{},
		},
		{
			name: "full spec with extensions, schemas, and default privileges",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Extensions: []dbopsv1alpha1.PostgresExtension{
						{Name: "uuid-ossp", Schema: "public", Version: "1.1"},
						{Name: "pgcrypto", Schema: "extensions"},
					},
					Schemas: []dbopsv1alpha1.PostgresSchema{
						{Name: "app", Owner: "appuser"},
						{Name: "analytics"},
					},
					DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilege{
						{
							Role:       "appuser",
							Schema:     "app",
							ObjectType: "tables",
							Privileges: []string{"SELECT", "INSERT", "UPDATE"},
						},
						{
							Role:       "readonly",
							Schema:     "public",
							ObjectType: "sequences",
							Privileges: []string{"USAGE"},
						},
					},
				},
			},
			expected: types.UpdateDatabaseOptions{
				Extensions: []types.ExtensionOptions{
					{Name: "uuid-ossp", Schema: "public", Version: "1.1"},
					{Name: "pgcrypto", Schema: "extensions"},
				},
				Schemas: []types.SchemaOptions{
					{Name: "app", Owner: "appuser"},
					{Name: "analytics"},
				},
				DefaultPrivileges: []types.DefaultPrivilegeOptions{
					{
						Role:       "appuser",
						Schema:     "app",
						ObjectType: "tables",
						Privileges: []string{"SELECT", "INSERT", "UPDATE"},
					},
					{
						Role:       "readonly",
						Schema:     "public",
						ObjectType: "sequences",
						Privileges: []string{"USAGE"},
					},
				},
			},
		},
		{
			name: "empty Postgres config returns empty options",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:     "testdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{},
			},
			expected: types.UpdateDatabaseOptions{},
		},
		{
			name: "extensions only",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Extensions: []dbopsv1alpha1.PostgresExtension{
						{Name: "hstore"},
					},
				},
			},
			expected: types.UpdateDatabaseOptions{
				Extensions: []types.ExtensionOptions{
					{Name: "hstore"},
				},
			},
		},
		{
			name: "empty arrays are not appended",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Extensions:        []dbopsv1alpha1.PostgresExtension{},
					Schemas:           []dbopsv1alpha1.PostgresSchema{},
					DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilege{},
				},
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

func TestPostgresSpecBuilder_BuildUserCreateOptions(t *testing.T) {
	builder := NewPostgresSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseUserSpec
		password string
		expected types.CreateUserOptions
	}{
		{
			name: "minimal spec with nil Postgres field uses defaults",
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
			name: "full spec with all Postgres options",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "poweruser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{
					ConnectionLimit:  10,
					ValidUntil:       "2027-12-31",
					Superuser:        true,
					CreateDB:         true,
					CreateRole:       true,
					Inherit:          false,
					Login:            false,
					Replication:      true,
					BypassRLS:        true,
					InRoles:          []string{"admin", "developers"},
					ConfigParameters: map[string]string{"statement_timeout": "30s", "work_mem": "256MB"},
				},
			},
			password: "strongpass",
			expected: types.CreateUserOptions{
				Username:        "poweruser",
				Password:        "strongpass",
				ConnectionLimit: 10,
				ValidUntil:      "2027-12-31",
				Superuser:       true,
				CreateDB:        true,
				CreateRole:      true,
				Inherit:         false,
				Login:           false,
				Replication:     true,
				BypassRLS:       true,
				InRoles:         []string{"admin", "developers"},
				ConfigParams:    map[string]string{"statement_timeout": "30s", "work_mem": "256MB"},
			},
		},
		{
			name: "Postgres config overrides Login and Inherit defaults",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "svcuser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{
					Login:   true,
					Inherit: true,
				},
			},
			password: "pass",
			expected: types.CreateUserOptions{
				Username: "svcuser",
				Password: "pass",
				Login:    true,
				Inherit:  true,
			},
		},
		{
			name: "empty Postgres config zeroes Login and Inherit",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "minuser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{},
			},
			password: "pass",
			expected: types.CreateUserOptions{
				Username: "minuser",
				Password: "pass",
				Login:    false,
				Inherit:  false,
			},
		},
		{
			name: "empty password is passed through",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "nopassuser",
			},
			password: "",
			expected: types.CreateUserOptions{
				Username: "nopassuser",
				Password: "",
				Login:    true,
				Inherit:  true,
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

func TestPostgresSpecBuilder_BuildUserUpdateOptions(t *testing.T) {
	builder := NewPostgresSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseUserSpec
		expected types.UpdateUserOptions
	}{
		{
			name: "nil Postgres field returns empty options",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
			},
			expected: types.UpdateUserOptions{},
		},
		{
			name: "full spec with ConnectionLimit and ValidUntil",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{
					ConnectionLimit:  25,
					ValidUntil:       "2028-06-30",
					InRoles:          []string{"readers", "writers"},
					ConfigParameters: map[string]string{"search_path": "app,public"},
				},
			},
			expected: func() types.UpdateUserOptions {
				connLimit := int32(25)
				validUntil := "2028-06-30"
				return types.UpdateUserOptions{
					ConnectionLimit: &connLimit,
					ValidUntil:      &validUntil,
					InRoles:         []string{"readers", "writers"},
					ConfigParams:    map[string]string{"search_path": "app,public"},
				}
			}(),
		},
		{
			name: "zero ConnectionLimit is not set as pointer",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{
					ConnectionLimit: 0,
					ValidUntil:      "",
					InRoles:         []string{"role1"},
				},
			},
			expected: types.UpdateUserOptions{
				InRoles: []string{"role1"},
			},
		},
		{
			name: "empty Postgres config returns empty options",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{},
			},
			expected: types.UpdateUserOptions{},
		},
		{
			name: "empty ValidUntil is not set as pointer",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{
					ConnectionLimit: 5,
					ValidUntil:      "",
				},
			},
			expected: func() types.UpdateUserOptions {
				connLimit := int32(5)
				return types.UpdateUserOptions{
					ConnectionLimit: &connLimit,
				}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildUserUpdateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPostgresSpecBuilder_BuildRoleCreateOptions(t *testing.T) {
	builder := NewPostgresSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseRoleSpec
		expected types.CreateRoleOptions
	}{
		{
			name: "minimal spec with nil Postgres field uses defaults",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "myrole",
			},
			expected: types.CreateRoleOptions{
				RoleName: "myrole",
				Inherit:  true,
			},
		},
		{
			name: "full spec with all Postgres options and grants",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "approle",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{
					Login:       true,
					Inherit:     false,
					CreateDB:    true,
					CreateRole:  true,
					Superuser:   true,
					Replication: true,
					BypassRLS:   true,
					InRoles:     []string{"parent_role"},
					Grants: []dbopsv1alpha1.PostgresGrant{
						{
							Database:        "appdb",
							Schema:          "public",
							Tables:          []string{"users", "orders"},
							Sequences:       []string{"users_id_seq"},
							Functions:       []string{"get_user"},
							Privileges:      []string{"SELECT", "INSERT"},
							WithGrantOption: true,
						},
						{
							Database:   "appdb",
							Schema:     "analytics",
							Tables:     []string{"events"},
							Privileges: []string{"SELECT"},
						},
					},
				},
			},
			expected: types.CreateRoleOptions{
				RoleName:    "approle",
				Login:       true,
				Inherit:     false,
				CreateDB:    true,
				CreateRole:  true,
				Superuser:   true,
				Replication: true,
				BypassRLS:   true,
				InRoles:     []string{"parent_role"},
				Grants: []types.GrantOptions{
					{
						Database:        "appdb",
						Schema:          "public",
						Tables:          []string{"users", "orders"},
						Sequences:       []string{"users_id_seq"},
						Functions:       []string{"get_user"},
						Privileges:      []string{"SELECT", "INSERT"},
						WithGrantOption: true,
					},
					{
						Database:   "appdb",
						Schema:     "analytics",
						Tables:     []string{"events"},
						Privileges: []string{"SELECT"},
					},
				},
			},
		},
		{
			name: "empty Postgres config overrides Inherit default",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "emptyrole",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{},
			},
			expected: types.CreateRoleOptions{
				RoleName: "emptyrole",
				Inherit:  false,
			},
		},
		{
			name: "empty grants array does not populate Grants field",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "nograntrole",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{
					Login:  true,
					Grants: []dbopsv1alpha1.PostgresGrant{},
				},
			},
			expected: types.CreateRoleOptions{
				RoleName: "nograntrole",
				Login:    true,
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

func TestPostgresSpecBuilder_BuildRoleUpdateOptions(t *testing.T) {
	builder := NewPostgresSpecBuilder()

	tests := []struct {
		name     string
		spec     *dbopsv1alpha1.DatabaseRoleSpec
		expected types.UpdateRoleOptions
	}{
		{
			name: "nil Postgres field returns empty options",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "myrole",
			},
			expected: types.UpdateRoleOptions{},
		},
		{
			name: "full spec sets all pointer fields and grants",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "approle",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{
					Login:       true,
					Inherit:     true,
					CreateDB:    false,
					CreateRole:  false,
					Superuser:   false,
					Replication: false,
					BypassRLS:   false,
					InRoles:     []string{"editors"},
					Grants: []dbopsv1alpha1.PostgresGrant{
						{
							Database:        "mydb",
							Schema:          "public",
							Tables:          []string{"posts"},
							Privileges:      []string{"SELECT", "UPDATE"},
							WithGrantOption: false,
						},
					},
				},
			},
			expected: func() types.UpdateRoleOptions {
				loginT := true
				inheritT := true
				createDBF := false
				createRoleF := false
				superuserF := false
				replicationF := false
				bypassRLSF := false
				return types.UpdateRoleOptions{
					Login:       &loginT,
					Inherit:     &inheritT,
					CreateDB:    &createDBF,
					CreateRole:  &createRoleF,
					Superuser:   &superuserF,
					Replication: &replicationF,
					BypassRLS:   &bypassRLSF,
					InRoles:     []string{"editors"},
					Grants: []types.GrantOptions{
						{
							Database:   "mydb",
							Schema:     "public",
							Tables:     []string{"posts"},
							Privileges: []string{"SELECT", "UPDATE"},
						},
					},
				}
			}(),
		},
		{
			name: "empty Postgres config sets all bool pointers to false",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "emptyrole",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{},
			},
			expected: func() types.UpdateRoleOptions {
				f := false
				return types.UpdateRoleOptions{
					Login:       &f,
					Inherit:     &f,
					CreateDB:    &f,
					CreateRole:  &f,
					Superuser:   &f,
					Replication: &f,
					BypassRLS:   &f,
				}
			}(),
		},
		{
			name: "empty grants array does not populate Grants field",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "nograntrole",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{
					Login:  true,
					Grants: []dbopsv1alpha1.PostgresGrant{},
				},
			},
			expected: func() types.UpdateRoleOptions {
				loginT := true
				f := false
				return types.UpdateRoleOptions{
					Login:       &loginT,
					Inherit:     &f,
					CreateDB:    &f,
					CreateRole:  &f,
					Superuser:   &f,
					Replication: &f,
					BypassRLS:   &f,
				}
			}(),
		},
		{
			name: "grants with sequences and functions are converted",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "grantrole",
				Postgres: &dbopsv1alpha1.PostgresRoleConfig{
					Inherit: true,
					Grants: []dbopsv1alpha1.PostgresGrant{
						{
							Database:        "db1",
							Schema:          "app",
							Sequences:       []string{"seq1", "seq2"},
							Functions:       []string{"fn1"},
							Privileges:      []string{"USAGE", "EXECUTE"},
							WithGrantOption: true,
						},
					},
				},
			},
			expected: func() types.UpdateRoleOptions {
				f := false
				inheritT := true
				return types.UpdateRoleOptions{
					Login:       &f,
					Inherit:     &inheritT,
					CreateDB:    &f,
					CreateRole:  &f,
					Superuser:   &f,
					Replication: &f,
					BypassRLS:   &f,
					Grants: []types.GrantOptions{
						{
							Database:        "db1",
							Schema:          "app",
							Sequences:       []string{"seq1", "seq2"},
							Functions:       []string{"fn1"},
							Privileges:      []string{"USAGE", "EXECUTE"},
							WithGrantOption: true,
						},
					},
				}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.BuildRoleUpdateOptions(tt.spec)
			assert.Equal(t, tt.expected, result)
		})
	}
}
