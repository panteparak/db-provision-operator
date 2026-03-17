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

package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
)

func TestHandler_Create(t *testing.T) {
	tests := []struct {
		name        string
		spec        *dbopsv1alpha1.DatabaseUserSpec
		namespace   string
		password    string
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
		wantCreated bool
	}{
		{
			name: "successful creation",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			password:  "securepass123",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
					return false, nil
				}
				m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
					return &Result{Created: true, Message: "created"}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantCreated: true,
		},
		{
			name: "user already exists",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			password:  "securepass123",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
					return true, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantErr:     false,
			wantCreated: false,
		},
		{
			name: "empty username returns error",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:   "default",
			password:    "securepass123",
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "username is required",
		},
		{
			name: "empty password returns error",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace:   "default",
			password:    "",
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "password is required",
		},
		{
			name: "repository error on exists check",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			password:  "securepass123",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
					return false, errors.New("connection failed")
				}
			},
			wantErr:     true,
			errContains: "check existence",
		},
		{
			name: "repository error on create",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			password:  "securepass123",
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
					return false, nil
				}
				m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
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

			result, err := handler.Create(context.Background(), tt.spec, tt.namespace, tt.password)

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
		username    string
		spec        *dbopsv1alpha1.DatabaseUserSpec
		namespace   string
		force       bool
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful deletion",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "force deletion",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     true,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
					assert.True(t, force)
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "repository error on delete",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			force:     false,
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
				m.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
					return errors.New("user has active connections")
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

			err := handler.Delete(context.Background(), tt.username, tt.spec, tt.namespace, tt.force)

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
		username   string
		spec       *dbopsv1alpha1.DatabaseUserSpec
		namespace  string
		setupMock  func(*MockRepository)
		wantExists bool
		wantErr    bool
	}{
		{
			name:     "user exists",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
					return true, nil
				}
			},
			wantExists: true,
			wantErr:    false,
		},
		{
			name:     "user does not exist",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
					return false, nil
				}
			},
			wantExists: false,
			wantErr:    false,
		},
		{
			name:     "repository error",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
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

			exists, err := handler.Exists(context.Background(), tt.username, tt.spec, tt.namespace)

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
		username    string
		spec        *dbopsv1alpha1.DatabaseUserSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantUpdated bool
		wantErr     bool
	}{
		{
			name:     "successful update",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
					return &Result{Updated: true, Message: "updated"}, nil
				}
				m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
					return "postgres", nil
				}
			},
			wantUpdated: true,
			wantErr:     false,
		},
		{
			name:     "no update needed",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
					return &Result{Updated: false, Message: "no changes"}, nil
				}
			},
			wantUpdated: false,
			wantErr:     false,
		},
		{
			name:     "repository error",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
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

			result, err := handler.Update(context.Background(), tt.username, tt.spec, tt.namespace)

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

func TestHandler_RotatePassword(t *testing.T) {
	tests := []struct {
		name        string
		username    string
		spec        *dbopsv1alpha1.DatabaseUserSpec
		namespace   string
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful password rotation",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.SetPasswordFunc = func(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:     "set password error",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.SetPasswordFunc = func(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
					return errors.New("permission denied")
				}
			},
			wantErr:     true,
			errContains: "set password",
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

			err := handler.RotatePassword(context.Background(), tt.username, tt.spec, tt.namespace)

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

func TestHandler_EventPublishing(t *testing.T) {
	t.Run("publishes UserCreated event on create", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
			return false, nil
		}
		mockRepo.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
			return &Result{Created: true, SecretName: "testuser-credentials"}, nil
		}
		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
			return "postgres", nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		}

		_, err := handler.Create(context.Background(), spec, "default", "securepass123")
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})

	t.Run("publishes UserDeleted event on delete", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
			return "postgres", nil
		}
		mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
			return nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username: "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "test-instance",
			},
		}

		err := handler.Delete(context.Background(), "testuser", spec, "default", false)
		require.NoError(t, err)

		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})
}

func TestHandler_GetOwnedObjects(t *testing.T) {
	tests := []struct {
		name            string
		username        string
		spec            *dbopsv1alpha1.DatabaseUserSpec
		namespace       string
		setupMock       func(*MockRepository)
		wantErr         bool
		errContains     string
		wantOwnsObjects bool
		wantObjectCount int
		wantBlocked     bool
	}{
		{
			name:     "user owns no objects",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
					return []OwnedObject{}, nil
				}
			},
			wantErr:         false,
			wantOwnsObjects: false,
			wantObjectCount: 0,
			wantBlocked:     false,
		},
		{
			name:     "user owns tables - blocks deletion",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
					return []OwnedObject{
						{Schema: "public", Name: "users", Type: "table"},
						{Schema: "public", Name: "orders", Type: "table"},
					}, nil
				}
			},
			wantErr:         false,
			wantOwnsObjects: true,
			wantObjectCount: 2,
			wantBlocked:     true,
		},
		{
			name:     "user owns mixed objects",
			username: "appuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "appuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
					return []OwnedObject{
						{Schema: "public", Name: "mytable", Type: "table"},
						{Schema: "public", Name: "myseq", Type: "sequence"},
						{Schema: "app", Name: "myfunc", Type: "function"},
					}, nil
				}
			},
			wantErr:         false,
			wantOwnsObjects: true,
			wantObjectCount: 3,
			wantBlocked:     true,
		},
		{
			name:     "repository error",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
					return nil, errors.New("connection failed")
				}
			},
			wantErr:     true,
			errContains: "get owned objects",
		},
		{
			name:     "uses custom service role in resolution",
			username: "testuser",
			spec: &dbopsv1alpha1.DatabaseUserSpec{
				Username: "testuser",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					ServiceRole: &dbopsv1alpha1.ServiceRoleConfig{
						Name: "custom_service_role",
					},
				},
			},
			namespace: "default",
			setupMock: func(m *MockRepository) {
				m.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
					return []OwnedObject{
						{Schema: "public", Name: "mytable", Type: "table"},
					}, nil
				}
			},
			wantErr:         false,
			wantOwnsObjects: true,
			wantObjectCount: 1,
			wantBlocked:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			if tt.setupMock != nil {
				tt.setupMock(mockRepo)
			}

			handler := &Handler{
				repo:   mockRepo,
				logger: logr.Discard(),
			}

			result, err := handler.GetOwnedObjects(context.Background(), tt.username, tt.spec, tt.namespace)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantOwnsObjects, result.OwnsObjects)
			assert.Equal(t, tt.wantObjectCount, len(result.OwnedObjects))
			assert.Equal(t, tt.wantBlocked, result.BlocksDeletion)

			// Verify resolution message contains correct role
			if tt.wantOwnsObjects {
				assert.NotEmpty(t, result.Resolution)
				assert.Contains(t, result.Resolution, "REASSIGN OWNED BY")
				assert.Contains(t, result.Resolution, tt.username)

				// Check custom service role if specified
				if tt.spec.PasswordRotation != nil && tt.spec.PasswordRotation.ServiceRole != nil && tt.spec.PasswordRotation.ServiceRole.Name != "" {
					assert.Contains(t, result.Resolution, tt.spec.PasswordRotation.ServiceRole.Name)
				}
			}
		})
	}
}

func TestHandler_RotateWithStrategy(t *testing.T) {
	t.Run("role-inheritance creates user with service role", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		var ensuredRole string
		mockRepo.EnsureServiceRoleFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
			ensuredRole = roleName
			return nil
		}
		var createdUsername, createdRoleName string
		mockRepo.CreateUserWithRoleFunc = func(ctx context.Context, username, password, roleName string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
			createdUsername = username
			createdRoleName = roleName
			assert.NotEmpty(t, password, "password should be generated")
			return nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled:  true,
					Schedule: "0 0 1 * *",
					Strategy: dbopsv1alpha1.RotationStrategyRoleInheritance,
					ServiceRole: &dbopsv1alpha1.ServiceRoleConfig{
						Name:       "svc_myapp",
						AutoCreate: true,
					},
				},
			},
		}

		result, err := handler.RotateWithStrategy(context.Background(), user)
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "svc_myapp", ensuredRole)
		assert.Equal(t, "svc_myapp", result.ServiceRole)
		assert.Equal(t, "svc_myapp", createdRoleName)
		assert.Contains(t, createdUsername, "myapp_")
		assert.Equal(t, createdUsername, result.NewUsername)
		assert.NotEmpty(t, result.NewPassword)

		// Verify event published
		assert.Len(t, mockEventBus.PublishedEvents, 1)
	})

	t.Run("default service role name when not specified", func(t *testing.T) {
		mockRepo := NewMockRepository()

		var ensuredRole string
		mockRepo.EnsureServiceRoleFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
			ensuredRole = roleName
			return nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled:  true,
					Schedule: "0 0 1 * *",
				},
			},
		}

		result, err := handler.RotateWithStrategy(context.Background(), user)
		require.NoError(t, err)
		assert.Equal(t, "svc_myapp", ensuredRole)
		assert.Equal(t, "svc_myapp", result.ServiceRole)
	})

	t.Run("unsupported strategy returns error", func(t *testing.T) {
		handler := &Handler{
			repo:     NewMockRepository(),
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled:  true,
					Strategy: "unknown-strategy",
				},
			},
		}

		result, err := handler.RotateWithStrategy(context.Background(), user)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "unsupported rotation strategy")
	})

	t.Run("ensure service role error propagates", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.EnsureServiceRoleFunc = func(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
			return errors.New("connection refused")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled:  true,
					Schedule: "0 0 1 * *",
				},
			},
		}

		result, err := handler.RotateWithStrategy(context.Background(), user)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "ensure service role")
	})

	t.Run("create user with role error propagates", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CreateUserWithRoleFunc = func(ctx context.Context, username, password, roleName string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
			return errors.New("role does not exist")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled:  true,
					Schedule: "0 0 1 * *",
				},
			},
		}

		result, err := handler.RotateWithStrategy(context.Background(), user)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "create rotated user")
	})

	t.Run("skips service role creation when autoCreate is false", func(t *testing.T) {
		mockRepo := NewMockRepository()

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled:  true,
					Schedule: "0 0 1 * *",
					ServiceRole: &dbopsv1alpha1.ServiceRoleConfig{
						Name:       "existing_role",
						AutoCreate: false,
					},
				},
			},
		}

		_, err := handler.RotateWithStrategy(context.Background(), user)
		require.NoError(t, err)
		assert.False(t, mockRepo.WasCalled("EnsureServiceRole"))
		assert.True(t, mockRepo.WasCalled("CreateUserWithRole"))
	})
}

func TestHandler_CleanupDeprecatedUsers(t *testing.T) {
	pastTime := metav1.NewTime(time.Now().Add(-48 * time.Hour))
	futureTime := metav1.NewTime(time.Now().Add(48 * time.Hour))

	t.Run("deletes user past grace period", func(t *testing.T) {
		mockRepo := NewMockRepository()
		var deletedUser string
		mockRepo.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
			deletedUser = username
			return nil
		}

		handler := &Handler{
			repo:   mockRepo,
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
					OldUserPolicy: &dbopsv1alpha1.OldUserPolicy{
						Action:          dbopsv1alpha1.OldUserActionDelete,
						GracePeriodDays: 7,
						OwnershipCheck:  false,
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseUserStatus{
				Rotation: &dbopsv1alpha1.RotationStatus{
					PendingDeletion: []dbopsv1alpha1.PendingDeletionInfo{
						{
							User:        "myapp_20260301",
							DeleteAfter: &pastTime,
							Status:      "pending",
						},
					},
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
		assert.Equal(t, "myapp_20260301", deletedUser)
		assert.Empty(t, user.Status.Rotation.PendingDeletion)
	})

	t.Run("skips user not yet due", func(t *testing.T) {
		mockRepo := NewMockRepository()

		handler := &Handler{
			repo:   mockRepo,
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
					OldUserPolicy: &dbopsv1alpha1.OldUserPolicy{
						Action:          dbopsv1alpha1.OldUserActionDelete,
						GracePeriodDays: 7,
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseUserStatus{
				Rotation: &dbopsv1alpha1.RotationStatus{
					PendingDeletion: []dbopsv1alpha1.PendingDeletionInfo{
						{
							User:        "myapp_20260315",
							DeleteAfter: &futureTime,
							Status:      "pending",
						},
					},
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
		assert.False(t, mockRepo.WasCalled("Delete"))
		assert.Len(t, user.Status.Rotation.PendingDeletion, 1)
	})

	t.Run("blocks deletion when user owns objects", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.GetOwnedObjectsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error) {
			return []OwnedObject{
				{Schema: "public", Name: "vault_kv_store", Type: "table"},
			}, nil
		}

		handler := &Handler{
			repo:   mockRepo,
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
					OldUserPolicy: &dbopsv1alpha1.OldUserPolicy{
						Action:          dbopsv1alpha1.OldUserActionDelete,
						GracePeriodDays: 7,
						OwnershipCheck:  true,
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseUserStatus{
				Rotation: &dbopsv1alpha1.RotationStatus{
					ServiceRole: "svc_myapp",
					PendingDeletion: []dbopsv1alpha1.PendingDeletionInfo{
						{
							User:        "myapp_20260301",
							DeleteAfter: &pastTime,
							Status:      "pending",
						},
					},
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
		assert.False(t, mockRepo.WasCalled("Delete"))
		require.Len(t, user.Status.Rotation.PendingDeletion, 1)
		assert.Equal(t, "blocked", user.Status.Rotation.PendingDeletion[0].Status)
		assert.Contains(t, user.Status.Rotation.PendingDeletion[0].BlockedReason, "owns")
		assert.Contains(t, user.Status.Rotation.PendingDeletion[0].Resolution, "REASSIGN OWNED BY")
	})

	t.Run("disables user instead of deleting", func(t *testing.T) {
		mockRepo := NewMockRepository()
		var disabledUser string
		mockRepo.DisableLoginFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
			disabledUser = username
			return nil
		}

		handler := &Handler{
			repo:   mockRepo,
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
					OldUserPolicy: &dbopsv1alpha1.OldUserPolicy{
						Action:          dbopsv1alpha1.OldUserActionDisable,
						GracePeriodDays: 7,
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseUserStatus{
				Rotation: &dbopsv1alpha1.RotationStatus{
					PendingDeletion: []dbopsv1alpha1.PendingDeletionInfo{
						{
							User:        "myapp_20260301",
							DeleteAfter: &pastTime,
							Status:      "pending",
						},
					},
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
		assert.Equal(t, "myapp_20260301", disabledUser)
		assert.False(t, mockRepo.WasCalled("Delete"))
		assert.Empty(t, user.Status.Rotation.PendingDeletion)
	})

	t.Run("retains user without action", func(t *testing.T) {
		mockRepo := NewMockRepository()

		handler := &Handler{
			repo:   mockRepo,
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
					OldUserPolicy: &dbopsv1alpha1.OldUserPolicy{
						Action:          dbopsv1alpha1.OldUserActionRetain,
						GracePeriodDays: 7,
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseUserStatus{
				Rotation: &dbopsv1alpha1.RotationStatus{
					PendingDeletion: []dbopsv1alpha1.PendingDeletionInfo{
						{
							User:        "myapp_20260301",
							DeleteAfter: &pastTime,
							Status:      "pending",
						},
					},
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
		assert.False(t, mockRepo.WasCalled("Delete"))
		assert.False(t, mockRepo.WasCalled("DisableLogin"))
		assert.Empty(t, user.Status.Rotation.PendingDeletion)
	})

	t.Run("no-op when rotation status is nil", func(t *testing.T) {
		handler := &Handler{
			repo:   NewMockRepository(),
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
	})

	t.Run("skips already deleted entries", func(t *testing.T) {
		mockRepo := NewMockRepository()

		handler := &Handler{
			repo:   mockRepo,
			logger: logr.Discard(),
		}

		user := &dbopsv1alpha1.DatabaseUser{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp-user", Namespace: "default"},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username:    "myapp",
				InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					Enabled: true,
					OldUserPolicy: &dbopsv1alpha1.OldUserPolicy{
						Action: dbopsv1alpha1.OldUserActionDelete,
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseUserStatus{
				Rotation: &dbopsv1alpha1.RotationStatus{
					PendingDeletion: []dbopsv1alpha1.PendingDeletionInfo{
						{
							User:        "myapp_20260201",
							DeleteAfter: &pastTime,
							Status:      "deleted",
						},
					},
				},
			},
		}

		err := handler.CleanupDeprecatedUsers(context.Background(), user)
		require.NoError(t, err)
		assert.False(t, mockRepo.WasCalled("Delete"))
		assert.Empty(t, user.Status.Rotation.PendingDeletion)
	})
}

func TestHandler_GenerateRotatedUsername(t *testing.T) {
	handler := &Handler{logger: logr.Discard()}

	t.Run("default pattern uses date", func(t *testing.T) {
		user := &dbopsv1alpha1.DatabaseUser{
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username: "myapp",
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					UserNaming: "",
				},
			},
		}

		name := handler.generateRotatedUsername(user)
		assert.Contains(t, name, "myapp_")
		assert.Len(t, name, len("myapp_20260317"))
	})

	t.Run("custom pattern with timestamp", func(t *testing.T) {
		user := &dbopsv1alpha1.DatabaseUser{
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username: "app",
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					UserNaming: "{{.Username}}_v{{.Timestamp}}",
				},
			},
		}

		name := handler.generateRotatedUsername(user)
		assert.Contains(t, name, "app_v")
	})

	t.Run("truncates to 63 chars", func(t *testing.T) {
		user := &dbopsv1alpha1.DatabaseUser{
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username: "a_very_long_username_that_exceeds_the_postgresql_limit_of_63_chars",
				PasswordRotation: &dbopsv1alpha1.PasswordRotationConfig{
					UserNaming: "{{.Username}}_{{.Date}}",
				},
			},
		}

		name := handler.generateRotatedUsername(user)
		assert.LessOrEqual(t, len(name), 63)
	})
}

func TestHandler_DetectDrift(t *testing.T) {
	t.Run("successful drift detection", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
			result := drift.NewResult("user", spec.Username)
			result.AddDiff(drift.Diff{
				Field:    "connectionLimit",
				Expected: "10",
				Actual:   "5",
			})
			return result, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username:    "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
		}

		result, err := handler.DetectDrift(context.Background(), spec, "default", false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.HasDrift())
		assert.Len(t, result.Diffs, 1)
		assert.Equal(t, "connectionLimit", result.Diffs[0].Field)
		assert.Equal(t, "10", result.Diffs[0].Expected)
		assert.Equal(t, "5", result.Diffs[0].Actual)
	})

	t.Run("no drift detected", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
			return drift.NewResult("user", spec.Username), nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username:    "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
		}

		result, err := handler.DetectDrift(context.Background(), spec, "default", false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.HasDrift())
		assert.Empty(t, result.Diffs)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
			return nil, errors.New("connection refused")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username:    "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
		}

		result, err := handler.DetectDrift(context.Background(), spec, "default", false)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "detect drift: connection refused")
	})
}

func TestHandler_CorrectDrift(t *testing.T) {
	t.Run("successful correction", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
			cr := drift.NewCorrectionResult(spec.Username)
			cr.AddCorrected(drift.Diff{
				Field:    "connectionLimit",
				Expected: "10",
				Actual:   "5",
			})
			return cr, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username:    "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
		}

		driftResult := drift.NewResult("user", "testuser")
		driftResult.AddDiff(drift.Diff{
			Field:    "connectionLimit",
			Expected: "10",
			Actual:   "5",
		})

		result, err := handler.CorrectDrift(context.Background(), spec, "default", driftResult, false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.HasCorrections())
		assert.Len(t, result.Corrected, 1)
		assert.Equal(t, "connectionLimit", result.Corrected[0].Diff.Field)
	})

	t.Run("repository error", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
			return nil, errors.New("permission denied")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username:    "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
		}

		driftResult := drift.NewResult("user", "testuser")
		driftResult.AddDiff(drift.Diff{
			Field:    "connectionLimit",
			Expected: "10",
			Actual:   "5",
		})

		result, err := handler.CorrectDrift(context.Background(), spec, "default", driftResult, false)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "correct drift: permission denied")
	})

	t.Run("correction with results", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockRepo.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
			cr := drift.NewCorrectionResult(spec.Username)
			cr.AddCorrected(drift.Diff{
				Field:    "connectionLimit",
				Expected: "10",
				Actual:   "5",
			})
			cr.AddSkipped(drift.Diff{
				Field:       "superuser",
				Expected:    "false",
				Actual:      "true",
				Destructive: true,
			}, "destructive change not allowed")
			cr.AddFailed(drift.Diff{
				Field:    "validUntil",
				Expected: "2026-12-31",
				Actual:   "2025-12-31",
			}, errors.New("syntax error"))
			return cr, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: NewMockEventBus(),
			logger:   logr.Discard(),
		}

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username:    "testuser",
			InstanceRef: &dbopsv1alpha1.InstanceReference{Name: "test-instance"},
		}

		driftResult := drift.NewResult("user", "testuser")
		driftResult.AddDiff(drift.Diff{Field: "connectionLimit", Expected: "10", Actual: "5"})
		driftResult.AddDiff(drift.Diff{Field: "superuser", Expected: "false", Actual: "true", Destructive: true})
		driftResult.AddDiff(drift.Diff{Field: "validUntil", Expected: "2026-12-31", Actual: "2025-12-31"})

		result, err := handler.CorrectDrift(context.Background(), spec, "default", driftResult, false)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.HasCorrections())
		assert.True(t, result.HasFailures())
		assert.Len(t, result.Corrected, 1)
		assert.Len(t, result.Skipped, 1)
		assert.Len(t, result.Failed, 1)
		assert.Equal(t, "connectionLimit", result.Corrected[0].Diff.Field)
		assert.Equal(t, "superuser", result.Skipped[0].Diff.Field)
		assert.Equal(t, "destructive change not allowed", result.Skipped[0].Reason)
		assert.Equal(t, "validUntil", result.Failed[0].Diff.Field)
		assert.EqualError(t, result.Failed[0].Error, "syntax error")
	})
}
