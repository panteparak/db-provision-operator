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

	"github.com/db-provision-operator/internal/service/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceService_HealthCheck(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantHealthy bool
		wantVersion string
	}{
		{
			name: "ping success with version",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return nil
				}
				m.GetVersionFunc = func(ctx context.Context) (string, error) {
					return "PostgreSQL 15.4", nil
				}
			},
			wantErr:     false,
			wantHealthy: true,
			wantVersion: "PostgreSQL 15.4",
		},
		{
			name: "ping failure",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return errors.New("connection refused")
				}
			},
			wantErr:     true,
			errContains: "ping",
		},
		{
			name: "version retrieval failure is non-critical",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return nil
				}
				m.GetVersionFunc = func(ctx context.Context) (string, error) {
					return "", errors.New("version query failed")
				}
			},
			wantErr:     false,
			wantHealthy: true,
			wantVersion: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewInstanceServiceWithAdapter(mock, cfg)

			result, err := svc.HealthCheck(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantHealthy, result.Healthy)
			assert.Equal(t, tt.wantVersion, result.Version)
			if tt.wantHealthy {
				assert.Contains(t, result.Message, "Connected to")
				assert.True(t, result.Latency > 0)
			}
		})
	}
}

func TestInstanceService_TestConnection(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantSuccess bool
	}{
		{
			name: "successful connection",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return nil
				}
				m.GetVersionFunc = func(ctx context.Context) (string, error) {
					return "PostgreSQL 15.4", nil
				}
			},
			wantErr:     false,
			wantSuccess: true,
		},
		{
			name: "unhealthy connection",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return errors.New("connection refused")
				}
			},
			wantErr:     true,
			errContains: "ping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewInstanceServiceWithAdapter(mock, cfg)

			result, err := svc.TestConnection(context.Background())

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
				assert.Contains(t, result.Message, "Connected to")
				assert.NotNil(t, result.Data)
				data, ok := result.Data.(map[string]interface{})
				require.True(t, ok)
				assert.NotEmpty(t, data["version"])
				assert.NotEmpty(t, data["latency"])
			}
		})
	}
}

func TestInstanceService_GetVersion(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
		wantVersion string
	}{
		{
			name: "success",
			setupMock: func(m *testutil.MockAdapter) {
				m.GetVersionFunc = func(ctx context.Context) (string, error) {
					return "PostgreSQL 15.4", nil
				}
			},
			wantErr:     false,
			wantVersion: "PostgreSQL 15.4",
		},
		{
			name: "error",
			setupMock: func(m *testutil.MockAdapter) {
				m.GetVersionFunc = func(ctx context.Context) (string, error) {
					return "", errors.New("query failed")
				}
			},
			wantErr:     true,
			errContains: "get version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewInstanceServiceWithAdapter(mock, cfg)

			version, err := svc.GetVersion(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantVersion, version)
		})
	}
}

func TestInstanceService_Ping(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*testutil.MockAdapter)
		wantErr     bool
		errContains string
	}{
		{
			name: "success",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "error",
			setupMock: func(m *testutil.MockAdapter) {
				m.PingFunc = func(ctx context.Context) error {
					return errors.New("connection lost")
				}
			},
			wantErr:     true,
			errContains: "ping",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := testutil.NewMockAdapter()
			tt.setupMock(mock)

			cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
			svc := NewInstanceServiceWithAdapter(mock, cfg)

			err := svc.Ping(context.Background())

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
