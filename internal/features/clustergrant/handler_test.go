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

package clustergrant

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
)

func TestHandler_Apply(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.ClusterDatabaseGrantSpec
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
		wantApplied bool
	}{
		{
			name: "successful apply to user",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Roles: []string{"readonly"},
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:         "user",
						Name:         spec.UserRef.Name,
						Namespace:    spec.UserRef.Namespace,
						DatabaseName: "testuser",
					}, nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
					return &Result{Applied: true, Roles: []string{"readonly"}, DirectGrants: 1}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantApplied: true,
		},
		{
			name: "successful apply to cluster role",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				RoleRef: &dbopsv1alpha1.NamespacedRoleReference{
					Name:      "service-role",
					Namespace: "", // Cluster-scoped role
				},
				Postgres: &dbopsv1alpha1.PostgresGrantConfig{
					Grants: []dbopsv1alpha1.PostgresGrant{
						{Database: "testdb", Schema: "public", Privileges: []string{"SELECT"}},
					},
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:          "role",
						Name:          spec.RoleRef.Name,
						Namespace:     "",
						DatabaseName:  "service-role",
						IsClusterRole: true,
					}, nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
					return &Result{Applied: true, DirectGrants: 1}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantApplied: true,
		},
		{
			name: "successful apply to namespace-scoped role",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				RoleRef: &dbopsv1alpha1.NamespacedRoleReference{
					Name:      "app-role",
					Namespace: "team-b", // Namespace-scoped role
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:          "role",
						Name:          spec.RoleRef.Name,
						Namespace:     spec.RoleRef.Namespace,
						DatabaseName:  "app-role",
						IsClusterRole: false,
					}, nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
					return &Result{Applied: true, DirectGrants: 1}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantApplied: true,
		},
		{
			name: "repository error on resolve target",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return nil, errors.New("user not found")
				}
			},
			wantErr:     true,
			errContains: "resolve target",
		},
		{
			name: "repository error on apply",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:         "user",
						Name:         "testuser",
						Namespace:    "team-a",
						DatabaseName: "testuser",
					}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
					return "postgres", nil
				}
				m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
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

			result, err := handler.Apply(context.Background(), tt.spec)

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
		spec        *dbopsv1alpha1.ClusterDatabaseGrantSpec
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful revoke",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:         "user",
						Name:         "testuser",
						Namespace:    "team-a",
						DatabaseName: "testuser",
					}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
					return "postgres", nil
				}
				m.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "repository error on revoke",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:         "user",
						Name:         "testuser",
						Namespace:    "team-a",
						DatabaseName: "testuser",
					}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
					return "postgres", nil
				}
				m.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
					return errors.New("failed to revoke")
				}
			},
			wantErr:     true,
			errContains: "revoke",
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

			err := handler.Revoke(context.Background(), tt.spec)

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
		spec       *dbopsv1alpha1.ClusterDatabaseGrantSpec
		setupMock  func(*MockRepository)
		wantExists bool
		wantErr    bool
	}{
		{
			name: "grants exist",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name: "grants do not exist",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name: "repository error",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
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

			exists, err := handler.Exists(context.Background(), tt.spec)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestHandler_ResolveTarget(t *testing.T) {
	tests := []struct {
		name           string
		spec           *dbopsv1alpha1.ClusterDatabaseGrantSpec
		setupMock      func(*MockRepository)
		wantTargetType string
		wantErr        bool
	}{
		{
			name: "resolve user target",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:         "user",
						Name:         "testuser",
						Namespace:    "team-a",
						DatabaseName: "testuser",
					}, nil
				}
			},
			wantTargetType: "user",
			wantErr:        false,
		},
		{
			name: "resolve cluster role target",
			spec: &dbopsv1alpha1.ClusterDatabaseGrantSpec{
				ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
					Name: "test-cluster-instance",
				},
				RoleRef: &dbopsv1alpha1.NamespacedRoleReference{
					Name:      "service-role",
					Namespace: "",
				},
			},
			setupMock: func(m *MockRepository) {
				m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
					return &TargetInfo{
						Type:          "role",
						Name:          "service-role",
						Namespace:     "",
						DatabaseName:  "service-role",
						IsClusterRole: true,
					}, nil
				}
			},
			wantTargetType: "role",
			wantErr:        false,
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

			target, err := handler.ResolveTarget(context.Background(), tt.spec)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, target)
			assert.Equal(t, tt.wantTargetType, target.Type)
		})
	}
}

func TestHandler_EventPublishing(t *testing.T) {
	t.Run("publishes ClusterGrantApplied event on apply", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
			return &TargetInfo{
				Type:         "user",
				Name:         "testuser",
				Namespace:    "team-a",
				DatabaseName: "testuser",
			}, nil
		}
		mockRepo.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
			return &Result{Applied: true, DirectGrants: 1}, nil
		}
		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
			return "postgres", nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Roles: []string{"readonly"},
			},
		}

		_, err := handler.Apply(context.Background(), spec)
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})

	t.Run("publishes ClusterGrantRevoked event on revoke", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
			return &TargetInfo{
				Type:         "user",
				Name:         "testuser",
				Namespace:    "team-a",
				DatabaseName: "testuser",
			}, nil
		}
		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
			return "postgres", nil
		}
		mockRepo.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
			return nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		err := handler.Revoke(context.Background(), spec)
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})
}

func TestHandler_GetInstance(t *testing.T) {
	t.Run("returns cluster instance from repository", func(t *testing.T) {
		mockRepo := NewMockRepository()
		expectedInstance := &dbopsv1alpha1.ClusterDatabaseInstance{
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
			},
		}
		mockRepo.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
			return expectedInstance, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
		}

		instance, err := handler.GetInstance(context.Background(), spec)
		require.NoError(t, err)
		assert.Equal(t, expectedInstance, instance)
	})
}

func TestHandler_DetectDrift(t *testing.T) {
	t.Run("detects drift successfully", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
			return &drift.Result{
				Diffs: []drift.Diff{
					{Field: "privileges", Expected: "SELECT,INSERT", Actual: "SELECT"},
				},
			}, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		result, err := handler.DetectDrift(context.Background(), spec, false)
		require.NoError(t, err)
		assert.True(t, result.HasDrift())
		assert.Len(t, result.Diffs, 1)
	})

	t.Run("no drift detected", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
			return drift.NewResult("clustergrant", "test-cluster-instance"), nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		result, err := handler.DetectDrift(context.Background(), spec, false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.HasDrift())
		assert.Empty(t, result.Diffs)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
			return nil, errors.New("connection refused")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		result, err := handler.DetectDrift(context.Background(), spec, false)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "detect drift: connection refused")
	})
}

func TestHandler_CorrectDrift(t *testing.T) {
	t.Run("successful correction", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
			result := drift.NewCorrectionResult("test-cluster-instance")
			result.AddCorrected(drift.Diff{Field: "privileges", Expected: "SELECT,INSERT", Actual: "SELECT"})
			return result, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		driftResult := drift.NewResult("clustergrant", "test-cluster-instance")
		driftResult.AddDiff(drift.Diff{Field: "privileges", Expected: "SELECT,INSERT", Actual: "SELECT"})

		result, err := handler.CorrectDrift(context.Background(), spec, driftResult, false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Corrected, 1)
		assert.True(t, result.HasCorrections())
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
			return nil, errors.New("correction failed")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		driftResult := drift.NewResult("clustergrant", "test-cluster-instance")
		driftResult.AddDiff(drift.Diff{Field: "privileges", Expected: "SELECT,INSERT", Actual: "SELECT"})

		result, err := handler.CorrectDrift(context.Background(), spec, driftResult, false)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "correct drift: correction failed")
	})

	t.Run("correction with mixed results", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
			result := drift.NewCorrectionResult("test-cluster-instance")
			result.AddCorrected(drift.Diff{Field: "privileges", Expected: "SELECT,INSERT", Actual: "SELECT"})
			result.AddSkipped(drift.Diff{Field: "owner", Expected: "admin", Actual: "postgres", Immutable: true}, "immutable field")
			result.AddFailed(drift.Diff{Field: "schema", Expected: "public", Actual: "private"}, errors.New("permission denied"))
			return result, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			ClusterInstanceRef: dbopsv1alpha1.ClusterInstanceReference{
				Name: "test-cluster-instance",
			},
			UserRef: &dbopsv1alpha1.NamespacedUserReference{
				Name:      "testuser",
				Namespace: "team-a",
			},
		}

		driftResult := drift.NewResult("clustergrant", "test-cluster-instance")
		driftResult.AddDiff(drift.Diff{Field: "privileges", Expected: "SELECT,INSERT", Actual: "SELECT"})
		driftResult.AddDiff(drift.Diff{Field: "owner", Expected: "admin", Actual: "postgres", Immutable: true})
		driftResult.AddDiff(drift.Diff{Field: "schema", Expected: "public", Actual: "private"})

		result, err := handler.CorrectDrift(context.Background(), spec, driftResult, false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.Corrected, 1)
		assert.Len(t, result.Skipped, 1)
		assert.Len(t, result.Failed, 1)
		assert.True(t, result.HasCorrections())
		assert.True(t, result.HasFailures())
	})
}

func TestHandler_CollectPrivileges(t *testing.T) {
	handler := &Handler{logger: logr.Discard()}

	t.Run("collects postgres privileges", func(t *testing.T) {
		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Roles: []string{"readonly", "writer"},
				Grants: []dbopsv1alpha1.PostgresGrant{
					{Privileges: []string{"SELECT", "INSERT"}},
				},
			},
		}

		privs := handler.collectPrivileges(spec)
		assert.Contains(t, privs, "readonly")
		assert.Contains(t, privs, "writer")
		assert.Contains(t, privs, "SELECT")
		assert.Contains(t, privs, "INSERT")
	})

	t.Run("collects mysql privileges", func(t *testing.T) {
		spec := &dbopsv1alpha1.ClusterDatabaseGrantSpec{
			MySQL: &dbopsv1alpha1.MySQLGrantConfig{
				Roles: []string{"reader"},
				Grants: []dbopsv1alpha1.MySQLGrant{
					{Privileges: []string{"SELECT"}},
				},
			},
		}

		privs := handler.collectPrivileges(spec)
		assert.Contains(t, privs, "reader")
		assert.Contains(t, privs, "SELECT")
	})
}

func TestHandler_UpdateInfoMetric(t *testing.T) {
	handler := &Handler{logger: logr.Discard()}

	t.Run("updates metric with user ref", func(t *testing.T) {
		grant := &dbopsv1alpha1.ClusterDatabaseGrant{
			Spec: dbopsv1alpha1.ClusterDatabaseGrantSpec{
				UserRef: &dbopsv1alpha1.NamespacedUserReference{
					Name:      "testuser",
					Namespace: "team-a",
				},
			},
			Status: dbopsv1alpha1.ClusterDatabaseGrantStatus{
				Phase: dbopsv1alpha1.PhaseReady,
			},
		}
		grant.Name = "test-grant"

		// Just ensure it doesn't panic
		handler.UpdateInfoMetric(grant)
	})

	t.Run("updates metric with cluster role ref", func(t *testing.T) {
		grant := &dbopsv1alpha1.ClusterDatabaseGrant{
			Spec: dbopsv1alpha1.ClusterDatabaseGrantSpec{
				RoleRef: &dbopsv1alpha1.NamespacedRoleReference{
					Name:      "service-role",
					Namespace: "", // Cluster-scoped
				},
			},
			Status: dbopsv1alpha1.ClusterDatabaseGrantStatus{
				Phase: dbopsv1alpha1.PhaseReady,
			},
		}
		grant.Name = "test-grant"

		// Just ensure it doesn't panic
		handler.UpdateInfoMetric(grant)
	})
}
