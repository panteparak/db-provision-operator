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

package role

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func TestHandler_Create(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseRoleSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
		wantCreated bool
	}{
		{
			name: "successful creation",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
					return false, nil
				}
				m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
					return &Result{Created: true, Message: "created"}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantCreated: true,
		},
		{
			name: "role already exists",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
					return true, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantCreated: false,
		},
		{
			name: "empty role name returns error",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:   "default",
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "role name is required",
		},
		{
			name: "repository error on exists check",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
					return false, errors.New("connection failed")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
		{
			name: "repository error on create",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
					return false, nil
				}
				m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
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

			handler := &Handler{
				repo:     mockRepo,
				eventBus: NewMockEventBus(),
				logger:   logr.Discard(),
			}

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
		roleName    string
		spec        *dbopsv1alpha1.DatabaseRoleSpec
		namespace   string
		force       bool
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful deletion",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "force deletion",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     true,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
					assert.True(t, force)
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "repository error on delete",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
					return errors.New("role has dependent objects")
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

			handler := &Handler{
				repo:     mockRepo,
				eventBus: NewMockEventBus(),
				logger:   logr.Discard(),
			}

			err := handler.Delete(context.Background(), tt.roleName, tt.spec, tt.namespace, tt.force)

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
		roleName   string
		spec       *dbopsv1alpha1.DatabaseRoleSpec
		namespace  string
		setupMock  func(*MockRepository)
		wantExists bool
		wantErr    bool
	}{
		{
			name:     "role exists",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:     "role does not exist",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:     "repository error",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
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

			exists, err := handler.Exists(context.Background(), tt.roleName, tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantExists, exists)
		})
	}
}

func TestHandler_Update(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		spec        *dbopsv1alpha1.DatabaseRoleSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantUpdated bool
		wantErr     bool
	}{
		{
			name:     "successful update",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
					return &Result{Updated: true, Message: "updated"}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantUpdated: true,
			wantErr:     false,
		},
		{
			name:     "no update needed",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
					return &Result{Updated: false, Message: "no changes"}, nil
				}
			},
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name:     "repository error",
			roleName: "testrole",
			spec: &dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: "testrole",
				InstanceRef: dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
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

			handler := &Handler{
				repo:     mockRepo,
				eventBus: NewMockEventBus(),
				logger:   logr.Discard(),
			}

			result, err := handler.Update(context.Background(), tt.roleName, tt.spec, tt.namespace)

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

func TestHandler_EventPublishing(t *testing.T) {
	t.Run("publishes RoleCreated event on create", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.ExistsFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
			return false, nil
		}
		mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
			return &Result{Created: true}, nil
		}
		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
			return "postgres", nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		}

		_, err := handler.Create(context.Background(), spec, "default")
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})

	t.Run("publishes RoleDeleted event on delete", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (string, error) {
			return "postgres", nil
		}
		mockRepo.DeleteFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
			return nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: "testrole",
			InstanceRef: dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		}

		err := handler.Delete(context.Background(), "testrole", spec, "default", false)
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})
}
