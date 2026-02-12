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

package database

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

func TestHandler_Create(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
		wantCreated bool
	}{
		{
			name: "successful creation",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
					return false, nil
				}
				m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
					return &Result{Created: true, Message: "created"}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantCreated: true,
		},
		{
			name: "database already exists",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
					return true, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantCreated: false,
		},
		{
			name: "empty name returns error",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:   "default",
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "database name is required",
		},
		{
			name: "empty instance ref returns error",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name:        "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{},
			},
			namespace:   "default",
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "instanceRef.name is required",
		},
		{
			name: "repository error on exists check",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
					return false, errors.New("connection failed")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
		{
			name: "repository error on create",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
					return false, nil
				}
				m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
					return nil, errors.New("permission denied")
				}
			},
			wantErr:     true,
			errContains: "create",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			result, err := handler.Create(context.Background(), tt.spec, tt.namespace)

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

func TestHandler_Delete(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		spec        *dbopsv1alpha1.DatabaseSpec
		namespace   string
		force       bool
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name:   "successful deletion",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return &dbopsv1alpha1.DatabaseInstance{}, nil
				}
				m.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "force deletion",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     true,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return &dbopsv1alpha1.DatabaseInstance{}, nil
				}
				m.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
					assert.True(t, force)
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "repository error on delete",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return &dbopsv1alpha1.DatabaseInstance{}, nil
				}
				m.DeleteFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
					return errors.New("database in use")
				}
			},
			wantErr:     true,
			errContains: "delete",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			err := handler.Delete(context.Background(), tt.dbName, tt.spec, tt.namespace, tt.force)

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
		dbName     string
		spec       *dbopsv1alpha1.DatabaseSpec
		namespace  string
		setupMock  func(*MockRepository)
		wantExists bool
		wantErr    bool
	}{
		{
			name:   "database exists",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:   "database does not exist",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:   "repository error",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
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

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			exists, err := handler.Exists(context.Background(), tt.dbName, tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestHandler_DetectDrift(t *testing.T) {
	tests := []struct {
		name             string
		spec             *dbopsv1alpha1.DatabaseSpec
		namespace        string
		allowDestructive bool
		setupMock        func(*MockRepository)
		wantDrift        bool
		wantErr          bool
	}{
		{
			name: "no drift detected",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:        "default",
			allowDestructive: false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
					return drift.NewResult("database", spec.Name), nil
				}
			},
			wantDrift: false,
			wantErr:   false,
		},
		{
			name: "drift detected",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:        "default",
			allowDestructive: false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
					result := drift.NewResult("database", spec.Name)
					result.AddDiff(drift.Diff{
						Field:    "encoding",
						Expected: "UTF8",
						Actual:   "LATIN1",
					})
					return result, nil
				}
			},
			wantDrift: true,
			wantErr:   false,
		},
		{
			name: "repository error",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:        "default",
			allowDestructive: false,
			setupMock: func(m *MockRepository) {
				m.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
					return nil, errors.New("database not found")
				}
			},
			wantDrift: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			result, err := handler.DetectDrift(context.Background(), tt.spec, tt.namespace, tt.allowDestructive)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantDrift, result.HasDrift())
		})
	}
}

func TestHandler_CorrectDrift(t *testing.T) {
	tests := []struct {
		name             string
		spec             *dbopsv1alpha1.DatabaseSpec
		namespace        string
		driftResult      *drift.Result
		allowDestructive bool
		setupMock        func(*MockRepository)
		wantCorrected    int
		wantErr          bool
	}{
		{
			name: "successful correction",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			driftResult: func() *drift.Result {
				r := drift.NewResult("database", "testdb")
				r.AddDiff(drift.Diff{Field: "charset", Expected: "utf8mb4", Actual: "utf8"})
				return r
			}(),
			allowDestructive: false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "mysql", nil
				}
				m.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
					result := drift.NewCorrectionResult(spec.Name)
					result.AddCorrected(drift.Diff{Field: "charset", Expected: "utf8mb4", Actual: "utf8"})
					return result, nil
				}
			},
			wantCorrected: 1,
			wantErr:       false,
		},
		{
			name: "correction skipped for immutable field",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			driftResult: func() *drift.Result {
				r := drift.NewResult("database", "testdb")
				r.AddDiff(drift.Diff{Field: "encoding", Expected: "UTF8", Actual: "LATIN1", Immutable: true})
				return r
			}(),
			allowDestructive: false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
					result := drift.NewCorrectionResult(spec.Name)
					result.AddSkipped(drift.Diff{Field: "encoding", Expected: "UTF8", Actual: "LATIN1", Immutable: true}, "immutable field")
					return result, nil
				}
			},
			wantCorrected: 0,
			wantErr:       false,
		},
		{
			name: "repository error",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			driftResult: func() *drift.Result {
				r := drift.NewResult("database", "testdb")
				r.AddDiff(drift.Diff{Field: "charset", Expected: "utf8mb4", Actual: "utf8"})
				return r
			}(),
			allowDestructive: false,
			setupMock: func(m *MockRepository) {
				m.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
					return nil, errors.New("correction failed")
				}
			},
			wantCorrected: 0,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			result, err := handler.CorrectDrift(context.Background(), tt.spec, tt.namespace, tt.driftResult, tt.allowDestructive)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Len(t, result.Corrected, tt.wantCorrected)
		})
	}
}

func TestHandler_Update(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		spec        *dbopsv1alpha1.DatabaseSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantUpdated bool
		wantErr     bool
	}{
		{
			name:   "successful update",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
					return &Result{Updated: true, Message: "updated"}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantUpdated: true,
			wantErr:     false,
		},
		{
			name:   "no update needed",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
					return &Result{Updated: false, Message: "no changes"}, nil
				}
			},
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name:   "repository error",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
					return nil, errors.New("update failed")
				}
			},
			wantUpdated: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			result, err := handler.Update(context.Background(), tt.dbName, tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantUpdated, result.Updated)
		})
	}
}

func TestHandler_GetInfo(t *testing.T) {
	tests := []struct {
		name      string
		dbName    string
		spec      *dbopsv1alpha1.DatabaseSpec
		namespace string
		setupMock func(*MockRepository)
		wantInfo  *Info
		wantErr   bool
	}{
		{
			name:   "successful get info",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
					return &Info{
						Name:      "testdb",
						Owner:     "testuser",
						SizeBytes: 1024 * 1024,
						Encoding:  "UTF8",
					}, nil
				}
			},
			wantInfo: &Info{
				Name:      "testdb",
				Owner:     "testuser",
				SizeBytes: 1024 * 1024,
				Encoding:  "UTF8",
			},
			wantErr: false,
		},
		{
			name:   "database not found",
			dbName: "nonexistent",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "nonexistent",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetInfoFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
					return nil, errors.New("database not found")
				}
			},
			wantInfo: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			info, err := handler.GetInfo(context.Background(), tt.dbName, tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantInfo, info)
		})
	}
}

func TestHandler_VerifyAccess(t *testing.T) {
	tests := []struct {
		name      string
		dbName    string
		spec      *dbopsv1alpha1.DatabaseSpec
		namespace string
		setupMock func(*MockRepository)
		wantErr   bool
	}{
		{
			name:   "access verified",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "access denied",
			dbName: "testdb",
			spec: &dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.VerifyAccessFunc = func(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
					return errors.New("connection refused")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			err := handler.VerifyAccess(context.Background(), tt.dbName, tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
