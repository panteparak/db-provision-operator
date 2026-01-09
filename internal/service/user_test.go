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

func TestUserService_Create(t *testing.T) {
	tests := []struct {
		name        string
		opts        CreateUserServiceOptions
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantCreated bool
		wantUpdated bool
	}{
		{
			name: "successful user creation",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "testuser",
				},
				Password: "secret123",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
				m.CreateUserFunc = func(ctx context.Context, opts types.CreateUserOptions) error {
					return nil
				}
			},
			wantCreated: true,
		},
		{
			name: "update existing user",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "existinguser",
				},
				Password: "newpassword",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error {
					return nil
				}
				m.UpdatePasswordFunc = func(ctx context.Context, username, password string) error {
					return nil
				}
			},
			wantUpdated: true,
		},
		{
			name: "nil spec validation error",
			opts: CreateUserServiceOptions{
				Spec:     nil,
				Password: "secret",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name: "empty username validation error",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "",
				},
				Password: "secret",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name: "empty password validation error",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "testuser",
				},
				Password: "",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "password is required",
		},
		{
			name: "adapter error on user exists check",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "testuser",
				},
				Password: "secret",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, errors.New("database unreachable")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
		{
			name: "adapter error on create",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "testuser",
				},
				Password: "secret",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
				m.CreateUserFunc = func(ctx context.Context, opts types.CreateUserOptions) error {
					return errors.New("create failed")
				}
			},
			wantErr:     true,
			errContains: "create",
		},
		{
			name: "adapter error on update",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "existinguser",
				},
				Password: "newpassword",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error {
					return errors.New("update failed")
				}
			},
			wantErr:     true,
			errContains: "update",
		},
		{
			name: "adapter error on update password for existing user",
			opts: CreateUserServiceOptions{
				Spec: &dbopsv1alpha1.DatabaseUserSpec{
					Username: "existinguser",
				},
				Password: "newpassword",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error {
					return nil
				}
				m.UpdatePasswordFunc = func(ctx context.Context, username, password string) error {
					return errors.New("password update failed")
				}
			},
			wantErr:     true,
			errContains: "update password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewUserServiceWithAdapter(mock, cfg)

			result, err := svc.Create(context.Background(), tt.opts)

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

func TestUserService_CreateWithPostgresOptions(t *testing.T) {
	t.Run("postgres user with full options", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts types.CreateUserOptions
		mock.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
			return false, nil
		}
		mock.CreateUserFunc = func(ctx context.Context, opts types.CreateUserOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewUserServiceWithAdapter(mock, cfg)

		opts := CreateUserServiceOptions{
			Spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "pguser",
				Postgres: &dbopsv1alpha1.PostgresUserConfig{
					ConnectionLimit: 10,
					ValidUntil:      "2025-12-31T23:59:59Z",
					Superuser:       false,
					CreateDB:        true,
					CreateRole:      true,
					Login:           true,
					Inherit:         true,
					Replication:     false,
					BypassRLS:       false,
					InRoles:         []string{"role1", "role2"},
				},
			},
			Password: "secret",
		}

		result, err := svc.Create(context.Background(), opts)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Created)

		// Verify the options passed to adapter
		assert.Equal(t, "pguser", capturedOpts.Username)
		assert.Equal(t, "secret", capturedOpts.Password)
		assert.Equal(t, int32(10), capturedOpts.ConnectionLimit)
		assert.Equal(t, "2025-12-31T23:59:59Z", capturedOpts.ValidUntil)
		assert.True(t, capturedOpts.CreateDB)
		assert.True(t, capturedOpts.CreateRole)
		assert.True(t, capturedOpts.Login)
		assert.True(t, capturedOpts.Inherit)
		assert.Equal(t, []string{"role1", "role2"}, capturedOpts.InRoles)
	})
}

func TestUserService_CreateWithMySQLOptions(t *testing.T) {
	t.Run("mysql user with full options", func(t *testing.T) {
		mock := testutil.NewMockAdapter()

		var capturedOpts types.CreateUserOptions
		mock.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
			return false, nil
		}
		mock.CreateUserFunc = func(ctx context.Context, opts types.CreateUserOptions) error {
			capturedOpts = opts
			return nil
		}

		cfg := &Config{Engine: "mysql", Host: "localhost", Port: 3306}
		svc := NewUserServiceWithAdapter(mock, cfg)

		opts := CreateUserServiceOptions{
			Spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "mysqluser",
				MySQL: &dbopsv1alpha1.MySQLUserConfig{
					MaxQueriesPerHour:     100,
					MaxUpdatesPerHour:     50,
					MaxConnectionsPerHour: 20,
					MaxUserConnections:    5,
					AuthPlugin:            "mysql_native_password",
					RequireSSL:            true,
					AllowedHosts:          []string{"%", "localhost"},
					AccountLocked:         false,
				},
			},
			Password: "secret",
		}

		result, err := svc.Create(context.Background(), opts)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.Created)

		// Verify the options passed to adapter
		assert.Equal(t, "mysqluser", capturedOpts.Username)
		assert.Equal(t, "secret", capturedOpts.Password)
		assert.Equal(t, int32(100), capturedOpts.MaxQueriesPerHour)
		assert.Equal(t, int32(50), capturedOpts.MaxUpdatesPerHour)
		assert.Equal(t, int32(20), capturedOpts.MaxConnectionsPerHour)
		assert.Equal(t, int32(5), capturedOpts.MaxUserConnections)
		assert.Equal(t, "mysql_native_password", capturedOpts.AuthPlugin)
		assert.True(t, capturedOpts.RequireSSL)
		assert.Equal(t, []string{"%", "localhost"}, capturedOpts.AllowedHosts)
	})
}

func TestUserService_Get(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful get",
			username: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
					return &types.UserInfo{
						Username: username,
						Login:    true,
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:     "user not found",
			username: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty username validation error",
			username:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:     "adapter error on get info",
			username: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
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
			svc := NewUserServiceWithAdapter(mock, cfg)

			info, err := svc.Get(context.Background(), tt.username)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.username, info.Username)
		})
	}
}

func TestUserService_Update(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		spec        *dbopsv1alpha1.DatabaseUserSpec
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful update",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "user not found",
			username: "nonexistent",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "nonexistent",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:     "empty username validation error",
			username: "",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:        "nil spec validation error",
			username:    "testuser",
			spec:        nil,
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name:     "adapter error on update",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error {
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
			svc := NewUserServiceWithAdapter(mock, cfg)

			result, err := svc.Update(context.Background(), tt.username, tt.spec)

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

func TestUserService_UpdatePassword(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		password    string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful password update",
			username: "testuser",
			password: "newpassword",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdatePasswordFunc = func(ctx context.Context, username, password string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "user not found",
			username: "nonexistent",
			password: "newpassword",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty username validation error",
			username:    "",
			password:    "newpassword",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:        "empty password validation error",
			username:    "testuser",
			password:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "password is required",
		},
		{
			name:     "adapter error on password update",
			username: "testuser",
			password: "newpassword",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.UpdatePasswordFunc = func(ctx context.Context, username, password string) error {
					return errors.New("update password failed")
				}
			},
			wantErr:     true,
			errContains: "update password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewUserServiceWithAdapter(mock, cfg)

			result, err := svc.UpdatePassword(context.Background(), tt.username, tt.password)

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

func TestUserService_Delete(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful deletion",
			username: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.DropUserFunc = func(ctx context.Context, username string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "user does not exist - no error",
			username: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
			},
			wantErr: false,
		},
		{
			name:        "empty username validation error",
			username:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:     "adapter error on delete",
			username: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
				m.DropUserFunc = func(ctx context.Context, username string) error {
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
			svc := NewUserServiceWithAdapter(mock, cfg)

			result, err := svc.Delete(context.Background(), tt.username)

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

func TestUserService_Exists(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		setupMock   func(*testutil.MockAdapter)
		wantExists  bool
		wantErr     bool
		errContains string
	}{
		{
			name:     "user exists",
			username: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:     "user does not exist",
			username: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:        "empty username validation error",
			username:    "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name:     "adapter error",
			username: "testuser",
			setupMock: func(m *testutil.MockAdapter) {
				m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
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
			svc := NewUserServiceWithAdapter(mock, cfg)

			exists, err := svc.Exists(context.Background(), tt.username)

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

func TestUserService_Connect(t *testing.T) {
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
			svc := NewUserServiceWithAdapter(mock, cfg)

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

func TestUserService_Close(t *testing.T) {
	t.Run("successful close", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return nil }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewUserServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.NoError(t, err)
		assert.Equal(t, 1, mock.GetCallCount("Close"))
	})

	t.Run("close with error", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return errors.New("close failed") }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewUserServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "close failed")
	})
}

func TestUserService_CallTracking(t *testing.T) {
	t.Run("verifies adapter calls are tracked", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.UserExistsFunc = func(ctx context.Context, username string) (bool, error) {
			return false, nil
		}
		mock.CreateUserFunc = func(ctx context.Context, opts types.CreateUserOptions) error {
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewUserServiceWithAdapter(mock, cfg)

		opts := CreateUserServiceOptions{
			Spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
			},
			Password: "secret",
		}

		_, _ = svc.Create(context.Background(), opts)

		assert.Equal(t, 1, mock.GetCallCount("UserExists"))
		assert.Equal(t, 1, mock.GetCallCount("CreateUser"))
		assert.True(t, mock.WasCalledWith("UserExists", "testuser"))
		assert.True(t, mock.WasCalledWith("CreateUser", "testuser"))
	})
}
