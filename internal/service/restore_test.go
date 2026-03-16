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
	"bytes"
	"context"
	"errors"
	"testing"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreService_Restore(t *testing.T) {
	tests := []struct {
		name        string
		opts        RestoreOptions
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantSuccess bool
	}{
		{
			name: "empty database validation error",
			opts: RestoreOptions{
				Database: "",
				Reader:   bytes.NewReader([]byte("data")),
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name: "nil reader validation error",
			opts: RestoreOptions{
				Database: "testdb",
				Reader:   nil,
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "reader is required",
		},
		{
			name: "successful restore",
			opts: RestoreOptions{
				Database:       "testdb",
				RestoreID:      "restore-001",
				Reader:         bytes.NewReader([]byte("backup data")),
				DropExisting:   false,
				CreateDatabase: true,
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RestoreFunc = func(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
					return &types.RestoreResult{
						RestoreID:      "restore-001",
						TargetDatabase: "testdb",
						TablesRestored: 10,
						Warnings:       []string{},
					}, nil
				}
			},
			wantErr:     false,
			wantSuccess: true,
		},
		{
			name: "adapter error",
			opts: RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-002",
				Reader:    bytes.NewReader([]byte("backup data")),
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.RestoreFunc = func(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
					return nil, errors.New("corrupt backup file")
				}
			},
			wantErr:     true,
			errContains: "restore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewRestoreServiceWithAdapter(mock, cfg)

			result, err := svc.Restore(context.Background(), tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantSuccess, result.Success)
			if tt.wantSuccess {
				assert.Contains(t, result.Message, "testdb")
				assert.Equal(t, "testdb", result.TargetDatabase)
				assert.Equal(t, int32(10), result.TablesRestored)
				assert.True(t, result.Duration > 0)
			}
		})
	}
}

func TestRestoreService_BuildRestoreOptions(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		opts   RestoreOptions
		verify func(t *testing.T, result types.RestoreOptions)
	}{
		{
			name:   "postgres with spec",
			engine: "postgres",
			opts: RestoreOptions{
				Database:       "testdb",
				RestoreID:      "restore-001",
				Reader:         bytes.NewReader([]byte("data")),
				DropExisting:   false,
				CreateDatabase: false,
				Spec: &dbopsv1alpha1.DatabaseRestoreSpec{
					Postgres: &dbopsv1alpha1.PostgresRestoreConfig{
						DropExisting:    true,
						CreateDatabase:  true,
						DataOnly:        true,
						SchemaOnly:      false,
						NoOwner:         true,
						NoPrivileges:    true,
						RoleMapping:     map[string]string{"old_user": "new_user"},
						Schemas:         []string{"public"},
						Tables:          []string{"users"},
						Jobs:            4,
						DisableTriggers: true,
						Analyze:         true,
					},
				},
			},
			verify: func(t *testing.T, result types.RestoreOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.Equal(t, "restore-001", result.RestoreID)
				assert.True(t, result.DropExisting)
				assert.True(t, result.CreateDatabase)
				assert.True(t, result.DataOnly)
				assert.False(t, result.SchemaOnly)
				assert.True(t, result.NoOwner)
				assert.True(t, result.NoPrivileges)
				assert.Equal(t, map[string]string{"old_user": "new_user"}, result.RoleMapping)
				assert.Equal(t, []string{"public"}, result.Schemas)
				assert.Equal(t, []string{"users"}, result.Tables)
				assert.Equal(t, int32(4), result.Jobs)
				assert.True(t, result.DisableTriggers)
				assert.True(t, result.Analyze)
			},
		},
		{
			name:   "postgres defaults (nil spec)",
			engine: "postgres",
			opts: RestoreOptions{
				Database:       "testdb",
				RestoreID:      "restore-002",
				Reader:         bytes.NewReader([]byte("data")),
				DropExisting:   false,
				CreateDatabase: false,
				Spec:           nil,
			},
			verify: func(t *testing.T, result types.RestoreOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.False(t, result.DropExisting)
				assert.True(t, result.CreateDatabase)
				assert.True(t, result.NoOwner)
				assert.Equal(t, int32(1), result.Jobs)
				assert.True(t, result.Analyze)
			},
		},
		{
			name:   "postgres spec merges DropExisting with opts",
			engine: "postgres",
			opts: RestoreOptions{
				Database:     "testdb",
				RestoreID:    "restore-003",
				Reader:       bytes.NewReader([]byte("data")),
				DropExisting: true,
				Spec: &dbopsv1alpha1.DatabaseRestoreSpec{
					Postgres: &dbopsv1alpha1.PostgresRestoreConfig{
						DropExisting: false,
					},
				},
			},
			verify: func(t *testing.T, result types.RestoreOptions) {
				// DropExisting should be true because opts.DropExisting is true (OR logic)
				assert.True(t, result.DropExisting)
			},
		},
		{
			name:   "mysql with spec",
			engine: "mysql",
			opts: RestoreOptions{
				Database:       "testdb",
				RestoreID:      "restore-004",
				Reader:         bytes.NewReader([]byte("data")),
				DropExisting:   false,
				CreateDatabase: false,
				Spec: &dbopsv1alpha1.DatabaseRestoreSpec{
					MySQL: &dbopsv1alpha1.MySQLRestoreConfig{
						DropExisting:            true,
						CreateDatabase:          true,
						Routines:                true,
						Triggers:                true,
						Events:                  true,
						DisableForeignKeyChecks: true,
						DisableBinlog:           true,
					},
				},
			},
			verify: func(t *testing.T, result types.RestoreOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.True(t, result.DropExisting)
				assert.True(t, result.CreateDatabase)
				assert.True(t, result.Routines)
				assert.True(t, result.Triggers)
				assert.True(t, result.Events)
				assert.True(t, result.DisableForeignKeyChecks)
				assert.True(t, result.DisableBinlog)
			},
		},
		{
			name:   "mysql defaults (nil spec)",
			engine: "mysql",
			opts: RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-005",
				Reader:    bytes.NewReader([]byte("data")),
				Spec:      nil,
			},
			verify: func(t *testing.T, result types.RestoreOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.True(t, result.CreateDatabase)
				assert.True(t, result.Routines)
				assert.True(t, result.Triggers)
				assert.True(t, result.Events)
				assert.True(t, result.DisableForeignKeyChecks)
				assert.True(t, result.DisableBinlog)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()

			cfg := &Config{Engine: tt.engine, Host: "localhost", Port: 5432}
			svc := NewRestoreServiceWithAdapter(mock, cfg)

			result := svc.buildRestoreOptions(tt.opts)
			tt.verify(t, result)
		})
	}
}
