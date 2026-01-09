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
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service/testutil"
)

func TestGrantService_Apply(t *testing.T) {
	tests := []struct {
		name                     string
		opts                     ApplyGrantServiceOptions
		engine                   string
		setupMock                func(*testutil.MockAdapter)
		wantErr                  bool
		errContains              string
		wantAppliedRoles         int
		wantAppliedDirectGrants  int
		wantAppliedDefaultPrivs  int
	}{
		{
			name: "successful postgres apply with roles",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Roles: []string{"reader", "writer"},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
			},
			wantAppliedRoles: 2,
		},
		{
			name: "successful postgres apply with direct grants",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								Tables:     []string{"*"},
								Privileges: []string{"SELECT", "INSERT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return nil
				}
			},
			wantAppliedDirectGrants: 1,
		},
		{
			name: "successful postgres apply with default privileges",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilegeGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								ObjectType: "TABLES",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
					return nil
				}
			},
			wantAppliedDefaultPrivs: 1,
		},
		{
			name: "successful postgres apply with all types",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Roles: []string{"admin"},
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								Privileges: []string{"USAGE"},
							},
						},
						DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilegeGrant{
							{
								Database:   "testdb",
								ObjectType: "TABLES",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
				m.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return nil
				}
				m.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
					return nil
				}
			},
			wantAppliedRoles:        1,
			wantAppliedDirectGrants: 1,
			wantAppliedDefaultPrivs: 1,
		},
		{
			name: "successful mysql apply with roles",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					MySQL: &dbopsv1alpha1.MySQLGrantConfig{
						Roles: []string{"app_reader"},
					},
				},
			},
			engine: "mysql",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
			},
			wantAppliedRoles: 1,
		},
		{
			name: "successful mysql apply with direct grants",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					MySQL: &dbopsv1alpha1.MySQLGrantConfig{
						Grants: []dbopsv1alpha1.MySQLGrant{
							{
								Level:      dbopsv1alpha1.MySQLGrantLevelDatabase,
								Database:   "testdb",
								Privileges: []string{"SELECT", "INSERT"},
							},
						},
					},
				},
			},
			engine: "mysql",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return nil
				}
			},
			wantAppliedDirectGrants: 1,
		},
		{
			name: "empty username validation error",
			opts: ApplyGrantServiceOptions{
				Username: "",
				Spec:     &dbopsv1alpha1.DatabaseGrantSpec{},
			},
			engine:      "postgres",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name: "nil spec validation error",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec:     nil,
			},
			engine:      "postgres",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name: "adapter error on grant role",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Roles: []string{"admin"},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return errors.New("grant role failed")
				}
			},
			wantErr:     true,
			errContains: "grant roles",
		},
		{
			name: "adapter error on direct grant",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return errors.New("grant failed")
				}
			},
			wantErr:     true,
			errContains: "apply grants",
		},
		{
			name: "adapter error on set default privileges",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilegeGrant{
							{
								Database:   "testdb",
								ObjectType: "TABLES",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
					return errors.New("default privileges failed")
				}
			},
			wantErr:     true,
			errContains: "set default privileges",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			port := int32(5432)
			if tt.engine == "mysql" {
				port = 3306
			}
			cfg := &Config{Engine: tt.engine, Host: "localhost", Port: port}
			svc := NewGrantServiceWithAdapter(mock, cfg)

			result, err := svc.Apply(context.Background(), tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantAppliedRoles, len(result.AppliedRoles))
			assert.Equal(t, tt.wantAppliedDirectGrants, result.AppliedDirectGrants)
			assert.Equal(t, tt.wantAppliedDefaultPrivs, result.AppliedDefaultPrivileges)
		})
	}
}

func TestGrantService_Revoke(t *testing.T) {
	tests := []struct {
		name        string
		opts        ApplyGrantServiceOptions
		engine      string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful postgres revoke roles",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Roles: []string{"reader", "writer"},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "successful postgres revoke direct grants",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Schema:     "public",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "successful mysql revoke roles",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					MySQL: &dbopsv1alpha1.MySQLGrantConfig{
						Roles: []string{"app_reader"},
					},
				},
			},
			engine: "mysql",
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "successful mysql revoke direct grants",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					MySQL: &dbopsv1alpha1.MySQLGrantConfig{
						Grants: []dbopsv1alpha1.MySQLGrant{
							{
								Level:      dbopsv1alpha1.MySQLGrantLevelDatabase,
								Database:   "testdb",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "mysql",
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "empty username validation error",
			opts: ApplyGrantServiceOptions{
				Username: "",
				Spec:     &dbopsv1alpha1.DatabaseGrantSpec{},
			},
			engine:      "postgres",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name: "nil spec validation error",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec:     nil,
			},
			engine:      "postgres",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name: "adapter error on revoke role",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Roles: []string{"admin"},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return errors.New("revoke role failed")
				}
			},
			wantErr:     true,
			errContains: "revoke roles",
		},
		{
			name: "adapter error on revoke direct grants",
			opts: ApplyGrantServiceOptions{
				Username: "testuser",
				Spec: &dbopsv1alpha1.DatabaseGrantSpec{
					Postgres: &dbopsv1alpha1.PostgresGrantConfig{
						Grants: []dbopsv1alpha1.PostgresGrant{
							{
								Database:   "testdb",
								Privileges: []string{"SELECT"},
							},
						},
					},
				},
			},
			engine: "postgres",
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
					return errors.New("revoke failed")
				}
			},
			wantErr:     true,
			errContains: "revoke grants",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			port := int32(5432)
			if tt.engine == "mysql" {
				port = 3306
			}
			cfg := &Config{Engine: tt.engine, Host: "localhost", Port: port}
			svc := NewGrantServiceWithAdapter(mock, cfg)

			result, err := svc.Revoke(context.Background(), tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
		})
	}
}

func TestGrantService_GrantRoles(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		roles       []string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful grant roles",
			username: "testuser",
			roles:    []string{"reader", "writer"},
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:        "empty username validation error",
			username:    "",
			roles:       []string{"reader"},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:        "empty roles validation error",
			username:    "testuser",
			roles:       []string{},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "at least one role is required",
		},
		{
			name:     "adapter error",
			username: "testuser",
			roles:    []string{"reader"},
			setupMock: func(m *testutil.MockAdapter) {
				m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return errors.New("grant role failed")
				}
			},
			wantErr:     true,
			errContains: "grant roles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewGrantServiceWithAdapter(mock, cfg)

			result, err := svc.GrantRoles(context.Background(), tt.username, tt.roles)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
		})
	}
}

func TestGrantService_RevokeRoles(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		roles       []string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful revoke roles",
			username: "testuser",
			roles:    []string{"reader", "writer"},
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:        "empty username validation error",
			username:    "",
			roles:       []string{"reader"},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:        "empty roles validation error",
			username:    "testuser",
			roles:       []string{},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "at least one role is required",
		},
		{
			name:     "adapter error",
			username: "testuser",
			roles:    []string{"reader"},
			setupMock: func(m *testutil.MockAdapter) {
				m.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
					return errors.New("revoke role failed")
				}
			},
			wantErr:     true,
			errContains: "revoke roles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewGrantServiceWithAdapter(mock, cfg)

			result, err := svc.RevokeRoles(context.Background(), tt.username, tt.roles)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Success)
		})
	}
}

func TestGrantService_GetGrants(t *testing.T) {
	tests := []struct {
		name        string
		grantee     string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantGrants  int
	}{
		{
			name:    "successful get grants",
			grantee: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
					return []types.GrantInfo{
						{Database: "testdb", Schema: "public", Privileges: []string{"SELECT"}},
						{Database: "testdb", Schema: "api", Privileges: []string{"SELECT", "INSERT"}},
					}, nil
				}
			},
			wantErr:    false,
			wantGrants: 2,
		},
		{
			name:    "empty grants",
			grantee: "newuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
					return []types.GrantInfo{}, nil
				}
			},
			wantErr:    false,
			wantGrants: 0,
		},
		{
			name:        "empty grantee validation error",
			grantee:     "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "grantee is required",
		},
		{
			name:    "adapter error",
			grantee: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
					return nil, errors.New("get grants failed")
				}
			},
			wantErr:     true,
			errContains: "get grants",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewGrantServiceWithAdapter(mock, cfg)

			grants, err := svc.GetGrants(context.Background(), tt.grantee)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, grants, tt.wantGrants)
		})
	}
}

func TestGrantService_Connect(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful connection",
			setupMock: func(m *testutil.MockAdapter) {
				m.ConnectFunc = func(ctx context.Context) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "connection failed",
			setupMock: func(m *testutil.MockAdapter) {
				m.ConnectFunc = func(ctx context.Context) error {
					return errors.New("connection refused")
				}
			},
			wantErr:     true,
			errContains: "connection to localhost:5432 failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewGrantServiceWithAdapter(mock, cfg)

			err := svc.Connect(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGrantService_Close(t *testing.T) {
	t.Run("successful close", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return nil }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewGrantServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.NoError(t, err)
		assert.Equal(t, 1, mock.GetCallCount("Close"))
	})

	t.Run("close with error", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return errors.New("close failed") }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewGrantServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "close failed")
	})
}

func TestGrantService_BuildOptions(t *testing.T) {
	t.Run("postgres grant options are built correctly", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts []types.GrantOptions
		mock.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewGrantServiceWithAdapter(mock, cfg)

		opts := ApplyGrantServiceOptions{
			Username: "testuser",
			Spec: &dbopsv1alpha1.DatabaseGrantSpec{
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Grants: []dbopsv1alpha1.PostgresGrant{
						{
							Database:        "testdb",
							Schema:          "public",
							Tables:          []string{"users", "orders"},
							Sequences:       []string{"users_id_seq"},
							Functions:       []string{"get_user"},
							Privileges:      []string{"SELECT", "INSERT"},
							WithGrantOption: true,
						},
					},
				},
			},
		}

		_, _ = svc.Apply(context.Background(), opts)

		require.Len(t, capturedOpts, 1)
		assert.Equal(t, "testdb", capturedOpts[0].Database)
		assert.Equal(t, "public", capturedOpts[0].Schema)
		assert.Equal(t, []string{"users", "orders"}, capturedOpts[0].Tables)
		assert.Equal(t, []string{"users_id_seq"}, capturedOpts[0].Sequences)
		assert.Equal(t, []string{"get_user"}, capturedOpts[0].Functions)
		assert.Equal(t, []string{"SELECT", "INSERT"}, capturedOpts[0].Privileges)
		assert.True(t, capturedOpts[0].WithGrantOption)
	})

	t.Run("mysql grant options are built correctly", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts []types.GrantOptions
		mock.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "mysql", Host: "localhost", Port: 3306}
		svc := NewGrantServiceWithAdapter(mock, cfg)

		opts := ApplyGrantServiceOptions{
			Username: "testuser",
			Spec: &dbopsv1alpha1.DatabaseGrantSpec{
				MySQL: &dbopsv1alpha1.MySQLGrantConfig{
					Grants: []dbopsv1alpha1.MySQLGrant{
						{
							Level:           dbopsv1alpha1.MySQLGrantLevelTable,
							Database:        "testdb",
							Table:           "users",
							Columns:         []string{"id", "name"},
							Privileges:      []string{"SELECT", "UPDATE"},
							WithGrantOption: false,
						},
					},
				},
			},
		}

		_, _ = svc.Apply(context.Background(), opts)

		require.Len(t, capturedOpts, 1)
		assert.Equal(t, "table", capturedOpts[0].Level)
		assert.Equal(t, "testdb", capturedOpts[0].Database)
		assert.Equal(t, "users", capturedOpts[0].Table)
		assert.Equal(t, []string{"id", "name"}, capturedOpts[0].Columns)
		assert.Equal(t, []string{"SELECT", "UPDATE"}, capturedOpts[0].Privileges)
		assert.False(t, capturedOpts[0].WithGrantOption)
	})

	t.Run("default privilege options are built correctly", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts []types.DefaultPrivilegeGrantOptions
		mock.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewGrantServiceWithAdapter(mock, cfg)

		opts := ApplyGrantServiceOptions{
			Username: "testuser",
			Spec: &dbopsv1alpha1.DatabaseGrantSpec{
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					DefaultPrivileges: []dbopsv1alpha1.PostgresDefaultPrivilegeGrant{
						{
							Database:   "testdb",
							Schema:     "public",
							GrantedBy:  "admin",
							ObjectType: "TABLES",
							Privileges: []string{"SELECT", "INSERT"},
						},
					},
				},
			},
		}

		_, _ = svc.Apply(context.Background(), opts)

		require.Len(t, capturedOpts, 1)
		assert.Equal(t, "testdb", capturedOpts[0].Database)
		assert.Equal(t, "public", capturedOpts[0].Schema)
		assert.Equal(t, "admin", capturedOpts[0].GrantedBy)
		assert.Equal(t, "TABLES", capturedOpts[0].ObjectType)
		assert.Equal(t, []string{"SELECT", "INSERT"}, capturedOpts[0].Privileges)
	})
}

func TestGrantService_CallTracking(t *testing.T) {
	t.Run("verifies adapter calls are tracked", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
			return nil
		}
		mock.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewGrantServiceWithAdapter(mock, cfg)

		opts := ApplyGrantServiceOptions{
			Username: "testuser",
			Spec: &dbopsv1alpha1.DatabaseGrantSpec{
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Roles: []string{"admin"},
					Grants: []dbopsv1alpha1.PostgresGrant{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT"},
						},
					},
				},
			},
		}

		_, _ = svc.Apply(context.Background(), opts)

		// Verify method call counts
		assert.Equal(t, 1, mock.GetCallCount("GrantRole"))
		assert.Equal(t, 1, mock.GetCallCount("Grant"))

		// WasCalledWith only works for comparable types (not slices)
		// Just verify the grantee was passed correctly
		assert.True(t, mock.WasCalledWith("Grant", "testuser"))
	})
}
