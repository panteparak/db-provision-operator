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

func TestRoleService_Create(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseRoleSpec
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantCreated bool
		wantUpdated bool
	}{
		{
			name: "successful role creation",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, nil
				}
				m.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
					return nil
				}
			},
			wantCreated: true,
		},
		{
			name: "update existing role",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "existingrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
					return nil
				}
			},
			wantUpdated: true,
		},
		{
			name:        "nil spec validation error",
			spec:        nil,
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name: "empty role name validation error",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "role name is required",
		},
		{
			name: "adapter error on role exists check",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, errors.New("database unreachable")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
		{
			name: "adapter error on create",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, nil
				}
				m.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
					return errors.New("create failed")
				}
			},
			wantErr:     true,
			errContains: "create",
		},
		{
			name: "adapter error on update",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "existingrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
					return errors.New("update failed")
				}
			},
			wantErr:     true,
			errContains: "update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewRoleServiceWithAdapter(mock, cfg)

			result, err := svc.Create(context.Background(), tt.spec)

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
			assert.Equal(t, tt.wantCreated, result.Created)
			assert.Equal(t, tt.wantUpdated, result.Updated)
		})
	}
}

func TestRoleService_CreateWithPostgresOptions(t *testing.T) {
	t.Run("postgres role with full options", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts types.CreateRoleOptions
		mock.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
			return false, nil
		}
		mock.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewRoleServiceWithAdapter(mock, cfg)

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "pgrole",
			Postgres: &dbopsv1alpha1.PostgresRoleConfig{
				Login:       false,
				Inherit:     true,
				CreateDB:    true,
				CreateRole:  false,
				Superuser:   false,
				Replication: false,
				BypassRLS:   false,
				InRoles:     []string{"parent_role"},
				Grants: []dbopsv1alpha1.PostgresGrant{
					{
						Database:        "testdb",
						Schema:          "public",
						Tables:          []string{"*"},
						Privileges:      []string{"SELECT", "INSERT"},
						WithGrantOption: false,
					},
				},
			},
		}

		result, err := svc.Create(context.Background(), spec)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Created)

		// Verify the options passed to adapter
		assert.Equal(t, "pgrole", capturedOpts.RoleName)
		assert.False(t, capturedOpts.Login)
		assert.True(t, capturedOpts.Inherit)
		assert.True(t, capturedOpts.CreateDB)
		assert.False(t, capturedOpts.CreateRole)
		assert.Equal(t, []string{"parent_role"}, capturedOpts.InRoles)
		assert.Len(t, capturedOpts.Grants, 1)
		assert.Equal(t, "testdb", capturedOpts.Grants[0].Database)
		assert.Equal(t, "public", capturedOpts.Grants[0].Schema)
		assert.Equal(t, []string{"*"}, capturedOpts.Grants[0].Tables)
		assert.Equal(t, []string{"SELECT", "INSERT"}, capturedOpts.Grants[0].Privileges)
	})
}

func TestRoleService_CreateWithMySQLOptions(t *testing.T) {
	t.Run("mysql role with grants", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts types.CreateRoleOptions
		mock.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
			return false, nil
		}
		mock.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "mysql", Host: "localhost", Port: 3306}
		svc := NewRoleServiceWithAdapter(mock, cfg)

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "mysqlrole",
			MySQL: &dbopsv1alpha1.MySQLRoleConfig{
				UseNativeRoles: true,
				Grants: []dbopsv1alpha1.MySQLGrant{
					{
						Level:           dbopsv1alpha1.MySQLGrantLevelDatabase,
						Database:        "testdb",
						Privileges:      []string{"SELECT", "INSERT", "UPDATE"},
						WithGrantOption: false,
					},
				},
			},
		}

		result, err := svc.Create(context.Background(), spec)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Created)

		// Verify the options passed to adapter
		assert.Equal(t, "mysqlrole", capturedOpts.RoleName)
		assert.True(t, capturedOpts.UseNativeRoles)
		assert.Len(t, capturedOpts.Grants, 1)
		assert.Equal(t, "database", capturedOpts.Grants[0].Level)
		assert.Equal(t, "testdb", capturedOpts.Grants[0].Database)
		assert.Equal(t, []string{"SELECT", "INSERT", "UPDATE"}, capturedOpts.Grants[0].Privileges)
	})
}

func TestRoleService_Get(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful get",
			roleName: "testrole",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
					return &types.RoleInfo{
						Name:    roleName,
						Inherit: true,
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:     "role not found",
			roleName: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty role name validation error",
			roleName:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "role name is required",
		},
		{
			name:     "adapter error on get info",
			roleName: "testrole",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
					return nil, errors.New("get info failed")
				}
			},
			wantErr:     true,
			errContains: "get info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewRoleServiceWithAdapter(mock, cfg)

			info, err := svc.Get(context.Background(), tt.roleName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.roleName, info.Name)
		})
	}
}

func TestRoleService_Update(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		spec        *dbopsv1alpha1.DatabaseRoleSpec
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful update",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "role not found",
			roleName: "nonexistent",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "nonexistent",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:     "empty role name validation error",
			roleName: "",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "role name is required",
		},
		{
			name:        "nil spec validation error",
			roleName:    "testrole",
			spec:        nil,
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name:     "adapter error on update",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
					return errors.New("update failed")
				}
			},
			wantErr:     true,
			errContains: "update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewRoleServiceWithAdapter(mock, cfg)

			result, err := svc.Update(context.Background(), tt.roleName, tt.spec)

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
			assert.True(t, result.Updated)
		})
	}
}

func TestRoleService_Delete(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful deletion",
			roleName: "testrole",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.DropRoleFunc = func(ctx context.Context, roleName string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "role does not exist - no error",
			roleName: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, nil
				}
			},
			wantErr: false,
		},
		{
			name:        "empty role name validation error",
			roleName:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "role name is required",
		},
		{
			name:     "adapter error on delete",
			roleName: "testrole",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
				m.DropRoleFunc = func(ctx context.Context, roleName string) error {
					return errors.New("delete failed")
				}
			},
			wantErr:     true,
			errContains: "delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewRoleServiceWithAdapter(mock, cfg)

			result, err := svc.Delete(context.Background(), tt.roleName)

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

func TestRoleService_Exists(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		setupMock   func(*testutil.MockAdapter)
		wantExists  bool
		wantErr     bool
		errContains string
	}{
		{
			name:     "role exists",
			roleName: "testrole",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:     "role does not exist",
			roleName: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:        "empty role name validation error",
			roleName:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "role name is required",
		},
		{
			name:     "adapter error",
			roleName: "testrole",
			setupMock: func(m *testutil.MockAdapter) {
				m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
					return false, errors.New("database error")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewRoleServiceWithAdapter(mock, cfg)

			exists, err := svc.Exists(context.Background(), tt.roleName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestRoleService_Connect(t *testing.T) {
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
			svc := NewRoleServiceWithAdapter(mock, cfg)

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

func TestRoleService_Close(t *testing.T) {
	t.Run("successful close", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return nil }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewRoleServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.NoError(t, err)
		assert.Equal(t, 1, mock.GetCallCount("Close"))
	})

	t.Run("close with error", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return errors.New("close failed") }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewRoleServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "close failed")
	})
}

func TestRoleService_CallTracking(t *testing.T) {
	t.Run("verifies adapter calls are tracked", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
			return false, nil
		}
		mock.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewRoleServiceWithAdapter(mock, cfg)

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
		}

		_, _ = svc.Create(context.Background(), spec)

		assert.Equal(t, 1, mock.GetCallCount("RoleExists"))
		assert.Equal(t, 1, mock.GetCallCount("CreateRole"))
		assert.True(t, mock.WasCalledWith("RoleExists", "testrole"))
		assert.True(t, mock.WasCalledWith("CreateRole", "testrole"))
	})
}
