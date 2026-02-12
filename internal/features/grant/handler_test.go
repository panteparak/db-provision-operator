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

package grant

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func TestHandler_Apply(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseGrantSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
		wantApplied bool
	}{
		{
			name: "successful apply",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Roles: []string{"app_read"},
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
					return &Result{Applied: true, Roles: []string{"app_read"}, DirectGrants: 1, Message: "applied"}, nil
				}
			},
			wantErr:     false,
			wantApplied: true,
		},
		{
			name: "apply with database ref",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
				DatabaseRef: &dbopsv1alpha1.DatabaseReference{
					Name: "testdb",
				},
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
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
					return &Result{Applied: true, DirectGrants: 2, Message: "applied"}, nil
				}
			},
			wantErr:     false,
			wantApplied: true,
		},
		{
			name: "get engine error",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "", errors.New("instance not found")
				}
			},
			wantErr:     true,
			errContains: "get engine",
		},
		{
			name: "apply error",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
					return nil, errors.New("permission denied")
				}
			},
			wantErr:     true,
			errContains: "apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := &Handler{
				repo:     mockRepo,
				eventBus: NewMockEventBus(),
				logger:   logr.Discard(),
			}

			result, err := handler.Apply(context.Background(), tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantApplied, result.Applied)
		})
	}
}

func TestHandler_Revoke(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseGrantSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful revoke",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Roles: []string{"app_read"},
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "revoke error",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
					return errors.New("user has dependent objects")
				}
			},
			wantErr:     true,
			errContains: "revoke",
		},
		{
			name: "revoke continues on engine error",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
					return "", errors.New("instance not found")
				}
				m.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
					return nil
				}
			},
			wantErr: false, // Revoke continues even if engine retrieval fails
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := &Handler{
				repo:     mockRepo,
				eventBus: NewMockEventBus(),
				logger:   logr.Discard(),
			}

			err := handler.Revoke(context.Background(), tt.spec, tt.namespace)

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

func TestHandler_Exists(t *testing.T) {
	tests := []struct {
		name       string
		spec       *dbopsv1alpha1.DatabaseGrantSpec
		namespace  string
		setupMock  func(*MockRepository)
		wantExists bool
		wantErr    bool
	}{
		{
			name: "grants exist",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name: "grants do not exist",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name: "repository error",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.UserReference{
					Name: "testuser",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error) {
					return false, errors.New("connection failed")
				}
			},
			wantExists: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := &Handler{
				repo:     mockRepo,
				eventBus: NewMockEventBus(),
				logger:   logr.Discard(),
			}

			exists, err := handler.Exists(context.Background(), tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestHandler_EventPublishing(t *testing.T) {
	t.Run("publishes GrantApplied event on apply", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
			return "postgres", nil
		}
		mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
			return &Result{Applied: true, Roles: []string{"app_read"}}, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: &dbopsv1alpha1.UserReference{
				Name: "testuser",
			},
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Roles: []string{"app_read"},
			},
		}

		_, err := handler.Apply(context.Background(), spec, "default")
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})

	t.Run("publishes GrantRevoked event on revoke", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
			return "postgres", nil
		}
		mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
			return nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseGrantSpec{
			UserRef: &dbopsv1alpha1.UserReference{
				Name: "testuser",
			},
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Roles: []string{"app_read"},
			},
		}

		err := handler.Revoke(context.Background(), spec, "default")
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})
}

func TestHandler_CollectPrivileges(t *testing.T) {
	handler := &Handler{
		logger: logr.Discard(),
	}

	tests := []struct {
		name string
		spec *dbopsv1alpha1.DatabaseGrantSpec
		want []string
	}{
		{
			name: "postgres roles and grants",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Roles: []string{"app_read", "app_write"},
					Grants: []dbopsv1alpha1.PostgresGrant{
						{Database: "testdb", Privileges: []string{"SELECT", "INSERT"}},
					},
				},
			},
			want: []string{"app_read", "app_write", "SELECT", "INSERT"},
		},
		{
			name: "mysql roles and grants",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{
				MySQL: &dbopsv1alpha1.MySQLGrantConfig{
					Roles: []string{"dba"},
					Grants: []dbopsv1alpha1.MySQLGrant{
						{Level: dbopsv1alpha1.MySQLGrantLevelGlobal, Privileges: []string{"ALL PRIVILEGES"}},
					},
				},
			},
			want: []string{"dba", "ALL PRIVILEGES"},
		},
		{
			name: "empty spec",
			spec: &dbopsv1alpha1.DatabaseGrantSpec{},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			privileges := handler.collectPrivileges(tt.spec)
			assert.Equal(t, tt.want, privileges)
		})
	}
}
