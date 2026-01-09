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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseService_Create(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseSpec
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantCreated bool
	}{
		{
			name: "successful creation",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
				m.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
					return nil
				}
				m.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
					return nil
				}
			},
			wantErr:     false,
			wantCreated: true,
		},
		{
			name: "database already exists",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
			},
			wantErr:     false,
			wantCreated: false, // Exists, not created
		},
		{
			name:        "nil spec returns validation error",
			spec:        nil,
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
		{
			name: "empty name returns validation error",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "",
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name: "adapter error on exists check",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, errors.New("connection failed")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
		{
			name: "adapter error on create",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
				m.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
					return errors.New("permission denied")
				}
			},
			wantErr:     true,
			errContains: "create",
		},
		{
			name: "adapter error on verify access",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
				m.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
					return nil
				}
				m.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
					return errors.New("not accepting connections")
				}
			},
			wantErr:     true,
			errContains: "not accepting connections",
		},
		{
			name: "postgres spec with all options",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Encoding:         "UTF8",
					LCCollate:        "en_US.UTF-8",
					LCCtype:          "en_US.UTF-8",
					Tablespace:       "pg_default",
					Template:         "template0",
					ConnectionLimit:  100,
					IsTemplate:       false,
					AllowConnections: true,
				},
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
				m.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
					// Verify options were passed correctly
					assert.Equal(t, "testdb", opts.Name)
					assert.Equal(t, "UTF8", opts.Encoding)
					assert.Equal(t, "en_US.UTF-8", opts.LCCollate)
					assert.Equal(t, int32(100), opts.ConnectionLimit)
					return nil
				}
				m.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
					return nil
				}
			},
			wantErr:     false,
			wantCreated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{
				Engine: "postgres",
				Host:   "localhost",
				Port:   5432,
			}
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

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
			assert.Equal(t, tt.wantCreated, result.Created)
		})
	}
}

func TestDatabaseService_CreateOnly(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseSpec
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		wantCreated bool
	}{
		{
			name: "successful creation without verify",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
				m.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
					return nil
				}
			},
			wantErr:     false,
			wantCreated: true,
		},
		{
			name: "database already exists",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
			},
			wantErr:     false,
			wantCreated: false, // Exists, not created
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

			result, err := svc.CreateOnly(context.Background(), tt.spec)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantCreated, result.Created)

			// Verify VerifyDatabaseAccess was NOT called
			assert.Equal(t, 0, mock.GetCallCount("VerifyDatabaseAccess"))
		})
	}
}

func TestDatabaseService_Get(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantInfo    *types.DatabaseInfo
	}{
		{
			name:   "successful get",
			dbName: "testdb",
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
				m.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
					return &types.DatabaseInfo{
						Name:      "testdb",
						Owner:     "postgres",
						SizeBytes: 8388608,
						Encoding:  "UTF8",
					}, nil
				}
			},
			wantErr: false,
			wantInfo: &types.DatabaseInfo{
				Name:      "testdb",
				Owner:     "postgres",
				SizeBytes: 8388608,
				Encoding:  "UTF8",
			},
		},
		{
			name:   "database not found",
			dbName: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty name validation error",
			dbName:      "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name:   "adapter error on get info",
			dbName: "testdb",
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
				m.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
					return nil, errors.New("query failed")
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
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

			info, err := svc.Get(context.Background(), tt.dbName)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantInfo.Name, info.Name)
			assert.Equal(t, tt.wantInfo.Owner, info.Owner)
		})
	}
}

func TestDatabaseService_Update(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		spec        *dbopsv1alpha1.DatabaseSpec
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantUpdated bool
	}{
		{
			name:   "successful update",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
					Extensions: []dbopsv1alpha1.PostgresExtension{
						{Name: "uuid-ossp"},
					},
				},
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
				m.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
					assert.Len(t, opts.Extensions, 1)
					assert.Equal(t, "uuid-ossp", opts.Extensions[0].Name)
					return nil
				}
			},
			wantErr:     false,
			wantUpdated: true,
		},
		{
			name:   "database not found",
			dbName: "nonexistent",
			spec:   &dbopsv1alpha1.DatabaseSpec{Name: "nonexistent"},
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
			},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "empty name validation error",
			dbName:      "",
			spec:        &dbopsv1alpha1.DatabaseSpec{Name: ""},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name:        "nil spec validation error",
			dbName:      "testdb",
			spec:        nil,
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "spec is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

			result, err := svc.Update(context.Background(), tt.dbName, tt.spec)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantUpdated, result.Updated)
		})
	}
}

func TestDatabaseService_Delete(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		force       bool
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:   "successful deletion",
			dbName: "testdb",
			force:  false,
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
				m.DropDatabaseFunc = func(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "force deletion",
			dbName: "testdb",
			force:  true,
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
				m.DropDatabaseFunc = func(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
					assert.True(t, opts.Force)
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "database does not exist - no error",
			dbName: "nonexistent",
			force:  false,
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
			},
			wantErr: false, // Delete is idempotent
		},
		{
			name:        "empty name validation error",
			dbName:      "",
			force:       false,
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name:   "adapter error on drop",
			dbName: "testdb",
			force:  false,
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
				m.DropDatabaseFunc = func(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
					return errors.New("active connections")
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
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

			result, err := svc.Delete(context.Background(), tt.dbName, tt.force)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
		})
	}
}

func TestDatabaseService_Exists(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		setupMock   func(*testutil.MockAdapter)
		wantExists  bool
		wantErr     bool
		errContains string
	}{
		{
			name:   "database exists",
			dbName: "testdb",
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:   "database does not exist",
			dbName: "nonexistent",
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:        "empty name validation error",
			dbName:      "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantExists:  false,
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name:   "adapter error",
			dbName: "testdb",
			setupMock: func(m *testutil.MockAdapter) {
				m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
					return false, errors.New("connection failed")
				}
			},
			wantExists:  false,
			wantErr:     true,
			errContains: "check existence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

			exists, err := svc.Exists(context.Background(), tt.dbName)

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

func TestDatabaseService_VerifyAccess(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name:   "successful verification",
			dbName: "testdb",
			setupMock: func(m *testutil.MockAdapter) {
				m.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "database not accepting connections",
			dbName: "testdb",
			setupMock: func(m *testutil.MockAdapter) {
				m.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
					return errors.New("database is not accepting connections")
				}
			},
			wantErr:     true,
			errContains: "verify access",
		},
		{
			name:        "empty name validation error",
			dbName:      "",
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

			err := svc.VerifyAccess(context.Background(), tt.dbName)

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

func TestDatabaseService_Connect(t *testing.T) {
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
			svc := NewDatabaseServiceWithAdapter(mock, cfg)

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

func TestDatabaseService_Close(t *testing.T) {
	t.Run("successful close", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return nil }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewDatabaseServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.NoError(t, err)
		assert.Equal(t, 1, mock.GetCallCount("Close"))
	})

	t.Run("close with error", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.CloseFunc = func() error { return errors.New("close failed") }

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewDatabaseServiceWithAdapter(mock, cfg)

		err := svc.Close()
		require.Error(t, err)
	})
}

func TestDatabaseService_MySQL(t *testing.T) {
	t.Run("mysql spec with all options", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
			return false, nil
		}
		mock.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
			assert.Equal(t, "testdb", opts.Name)
			assert.Equal(t, "utf8mb4", opts.Charset)
			assert.Equal(t, "utf8mb4_unicode_ci", opts.Collation)
			return nil
		}
		mock.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
			return nil
		}

		cfg := &Config{Engine: "mysql", Host: "localhost", Port: 3306}
		svc := NewDatabaseServiceWithAdapter(mock, cfg)

		spec := &dbopsv1alpha1.DatabaseSpec{
			Name: "testdb",
			MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{
				Charset:   "utf8mb4",
				Collation: "utf8mb4_unicode_ci",
			},
		}

		result, err := svc.Create(context.Background(), spec)
		require.NoError(t, err)
		assert.True(t, result.Created)
	})
}

func TestDatabaseService_CallTracking(t *testing.T) {
	t.Run("verifies adapter calls are tracked", func(t *testing.T) {
		mock := testutil.NewMockAdapter()
		mock.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) {
			return false, nil
		}
		mock.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error {
			return nil
		}
		mock.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error {
			return nil
		}

		cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
		svc := NewDatabaseServiceWithAdapter(mock, cfg)

		spec := &dbopsv1alpha1.DatabaseSpec{Name: "testdb"}
		_, err := svc.Create(context.Background(), spec)
		require.NoError(t, err)

		// Verify call sequence
		assert.Equal(t, 1, mock.GetCallCount("DatabaseExists"))
		assert.Equal(t, 1, mock.GetCallCount("CreateDatabase"))
		assert.Equal(t, 1, mock.GetCallCount("VerifyDatabaseAccess"))

		// Verify specific calls
		assert.True(t, mock.WasCalledWith("DatabaseExists", "testdb"))
		assert.True(t, mock.WasCalledWith("CreateDatabase", "testdb"))
		assert.True(t, mock.WasCalledWith("VerifyDatabaseAccess", "testdb"))
	})
}
