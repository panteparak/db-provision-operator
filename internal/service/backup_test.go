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

func TestBackupService_Backup(t *testing.T) {
	tests := []struct {
		name        string
		opts        BackupOptions
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantSuccess bool
	}{
		{
			name: "empty database validation error",
			opts: BackupOptions{
				Database: "",
				Writer:   &bytes.Buffer{},
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name: "nil writer validation error",
			opts: BackupOptions{
				Database: "testdb",
				Writer:   nil,
			},
			setupMock:   func(m *testutil.MockAdapter) {},
			wantErr:     true,
			errContains: "writer is required",
		},
		{
			name: "successful backup",
			opts: BackupOptions{
				Database: "testdb",
				BackupID: "backup-001",
				Writer:   &bytes.Buffer{},
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.BackupFunc = func(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
					return &types.BackupResult{
						Path:      "test.dump",
						SizeBytes: 1024,
					}, nil
				}
			},
			wantErr:     false,
			wantSuccess: true,
		},
		{
			name: "adapter error",
			opts: BackupOptions{
				Database: "testdb",
				BackupID: "backup-002",
				Writer:   &bytes.Buffer{},
			},
			setupMock: func(m *testutil.MockAdapter) {
				m.BackupFunc = func(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
					return nil, errors.New("disk full")
				}
			},
			wantErr:     true,
			errContains: "backup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewBackupServiceWithAdapter(mock, cfg)

			result, err := svc.Backup(context.Background(), tt.opts)

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
				assert.Equal(t, "test.dump", result.Path)
				assert.Equal(t, int64(1024), result.SizeBytes)
				assert.True(t, result.Duration > 0)
			}
		})
	}
}

func TestBackupService_GetBackupExtension(t *testing.T) {
	tests := []struct {
		name    string
		engine  string
		spec    *dbopsv1alpha1.DatabaseBackupSpec
		wantExt string
	}{
		{
			name:   "postgres plain format",
			engine: "postgres",
			spec: &dbopsv1alpha1.DatabaseBackupSpec{
				Postgres: &dbopsv1alpha1.PostgresBackupConfig{
					Format: "plain",
				},
			},
			wantExt: ".sql",
		},
		{
			name:   "postgres custom format",
			engine: "postgres",
			spec: &dbopsv1alpha1.DatabaseBackupSpec{
				Postgres: &dbopsv1alpha1.PostgresBackupConfig{
					Format: "custom",
				},
			},
			wantExt: ".dump",
		},
		{
			name:   "postgres directory format",
			engine: "postgres",
			spec: &dbopsv1alpha1.DatabaseBackupSpec{
				Postgres: &dbopsv1alpha1.PostgresBackupConfig{
					Format: "directory",
				},
			},
			wantExt: ".dir",
		},
		{
			name:   "postgres tar format",
			engine: "postgres",
			spec: &dbopsv1alpha1.DatabaseBackupSpec{
				Postgres: &dbopsv1alpha1.PostgresBackupConfig{
					Format: "tar",
				},
			},
			wantExt: ".tar",
		},
		{
			name:    "postgres default (nil spec)",
			engine:  "postgres",
			spec:    nil,
			wantExt: ".dump",
		},
		{
			name:    "postgres default (nil postgres config)",
			engine:  "postgres",
			spec:    &dbopsv1alpha1.DatabaseBackupSpec{},
			wantExt: ".dump",
		},
		{
			name:    "mysql",
			engine:  "mysql",
			spec:    nil,
			wantExt: ".sql",
		},
		{
			name:   "mysql with spec",
			engine: "mysql",
			spec: &dbopsv1alpha1.DatabaseBackupSpec{
				MySQL: &dbopsv1alpha1.MySQLBackupConfig{
					SingleTransaction: true,
				},
			},
			wantExt: ".sql",
		},
		{
			name:    "unknown engine",
			engine:  "cockroachdb",
			spec:    nil,
			wantExt: ".bak",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()

			cfg := &Config{Engine: tt.engine, Host: "localhost", Port: 5432}
			svc := NewBackupServiceWithAdapter(mock, cfg)

			ext := svc.GetBackupExtension(tt.spec)
			assert.Equal(t, tt.wantExt, ext)
		})
	}
}

func TestBackupService_BuildBackupOptions(t *testing.T) {
	tests := []struct {
		name   string
		engine string
		opts   BackupOptions
		verify func(t *testing.T, result types.BackupOptions)
	}{
		{
			name:   "postgres with spec",
			engine: "postgres",
			opts: BackupOptions{
				Database: "testdb",
				BackupID: "backup-001",
				Writer:   &bytes.Buffer{},
				Spec: &dbopsv1alpha1.DatabaseBackupSpec{
					Postgres: &dbopsv1alpha1.PostgresBackupConfig{
						Method:          "pg_dump",
						Format:          "custom",
						Jobs:            4,
						DataOnly:        true,
						SchemaOnly:      false,
						Blobs:           true,
						NoOwner:         true,
						NoPrivileges:    true,
						Schemas:         []string{"public"},
						ExcludeSchemas:  []string{"internal"},
						Tables:          []string{"users"},
						ExcludeTables:   []string{"logs"},
						LockWaitTimeout: "120s",
					},
				},
			},
			verify: func(t *testing.T, result types.BackupOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.Equal(t, "backup-001", result.BackupID)
				assert.Equal(t, "pg_dump", result.Method)
				assert.Equal(t, "custom", result.Format)
				assert.Equal(t, int32(4), result.Jobs)
				assert.True(t, result.DataOnly)
				assert.False(t, result.SchemaOnly)
				assert.True(t, result.Blobs)
				assert.True(t, result.NoOwner)
				assert.True(t, result.NoPrivileges)
				assert.Equal(t, []string{"public"}, result.Schemas)
				assert.Equal(t, []string{"internal"}, result.ExcludeSchemas)
				assert.Equal(t, []string{"users"}, result.Tables)
				assert.Equal(t, []string{"logs"}, result.ExcludeTables)
				assert.Equal(t, "120s", result.LockWaitTimeout)
			},
		},
		{
			name:   "postgres defaults (nil spec)",
			engine: "postgres",
			opts: BackupOptions{
				Database: "testdb",
				BackupID: "backup-002",
				Writer:   &bytes.Buffer{},
				Spec:     nil,
			},
			verify: func(t *testing.T, result types.BackupOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.Equal(t, "pg_dump", result.Method)
				assert.Equal(t, "custom", result.Format)
				assert.Equal(t, int32(1), result.Jobs)
				assert.True(t, result.Blobs)
			},
		},
		{
			name:   "mysql with spec",
			engine: "mysql",
			opts: BackupOptions{
				Database: "testdb",
				BackupID: "backup-003",
				Writer:   &bytes.Buffer{},
				Spec: &dbopsv1alpha1.DatabaseBackupSpec{
					MySQL: &dbopsv1alpha1.MySQLBackupConfig{
						SingleTransaction: true,
						Quick:             true,
						LockTables:        true,
						Routines:          true,
						Triggers:          true,
						Events:            true,
						ExtendedInsert:    true,
						SetGtidPurged:     "AUTO",
					},
				},
			},
			verify: func(t *testing.T, result types.BackupOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.True(t, result.SingleTransaction)
				assert.True(t, result.Quick)
				assert.True(t, result.LockTables)
				assert.True(t, result.Routines)
				assert.True(t, result.Triggers)
				assert.True(t, result.Events)
				assert.True(t, result.ExtendedInsert)
				assert.Equal(t, "AUTO", result.SetGtidPurged)
			},
		},
		{
			name:   "mysql defaults (nil spec)",
			engine: "mysql",
			opts: BackupOptions{
				Database: "testdb",
				BackupID: "backup-004",
				Writer:   &bytes.Buffer{},
				Spec:     nil,
			},
			verify: func(t *testing.T, result types.BackupOptions) {
				assert.Equal(t, "testdb", result.Database)
				assert.True(t, result.SingleTransaction)
				assert.True(t, result.Quick)
				assert.False(t, result.LockTables)
				assert.True(t, result.Routines)
				assert.True(t, result.Triggers)
				assert.False(t, result.Events)
				assert.False(t, result.ExtendedInsert)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()

			cfg := &Config{Engine: tt.engine, Host: "localhost", Port: 5432}
			svc := NewBackupServiceWithAdapter(mock, cfg)

			result := svc.buildBackupOptions(tt.opts)
			tt.verify(t, result)
		})
	}
}
