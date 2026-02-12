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

package clusterinstance

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// newTestClusterInstance creates a ClusterDatabaseInstance for handler tests.
// Note: Cluster-scoped resources don't have a namespace.
func newTestClusterInstance(name string) *dbopsv1alpha1.ClusterDatabaseInstance {
	return &dbopsv1alpha1.ClusterDatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name:      "test-secret",
					Namespace: "default", // Cluster-scoped resources require namespace in secretRef
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase:   dbopsv1alpha1.PhaseReady,
			Version: "15.1",
		},
	}
}

func TestHandler_Connect(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*MockRepository)
		wantErr   bool
		wantVer   string
	}{
		{
			name: "successful connection",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
					return &ConnectResult{
						Connected: true,
						Version:   "15.1",
						Message:   "Connected successfully",
					}, nil
				}
			},
			wantErr: false,
			wantVer: "15.1",
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr: true,
		},
		{
			name: "connection failed",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
					return nil, errors.New("connection refused")
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

			// Note: ClusterDatabaseInstance handler only takes name (no namespace)
			result, err := handler.Connect(context.Background(), "test-cluster-instance")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.True(t, result.Connected)
			assert.Equal(t, tt.wantVer, result.Version)
		})
	}
}

func TestHandler_Ping(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*MockRepository)
		wantErr   bool
	}{
		{
			name: "ping successful",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr: true,
		},
		{
			name: "ping failed",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error {
					return errors.New("connection lost")
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

			err := handler.Ping(context.Background(), "test-cluster-instance")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestHandler_GetVersion(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*MockRepository)
		wantErr   bool
		wantVer   string
	}{
		{
			name: "get version successful",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.GetVersionFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (string, error) {
					return "15.2", nil
				}
			},
			wantErr: false,
			wantVer: "15.2",
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr: true,
		},
		{
			name: "get version failed",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.GetVersionFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (string, error) {
					return "", errors.New("failed to get version")
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

			version, err := handler.GetVersion(context.Background(), "test-cluster-instance")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantVer, version)
		})
	}
}

func TestHandler_IsHealthy(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*MockRepository)
		wantErr   bool
		healthy   bool
	}{
		{
			name: "instance is healthy",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error {
					return nil
				}
			},
			wantErr: false,
			healthy: true,
		},
		{
			name: "instance is not healthy",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error {
					return errors.New("connection lost")
				}
			},
			wantErr: false, // IsHealthy returns false on ping failure, not an error
			healthy: false,
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return nil, errors.New("not found")
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

			healthy, err := handler.IsHealthy(context.Background(), "test-cluster-instance")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.healthy, healthy)
		})
	}
}

func TestHandler_EventPublishing(t *testing.T) {
	tests := []struct {
		name          string
		action        string
		setupMock     func(*MockRepository)
		expectedEvent string
	}{
		{
			name:   "publishes InstanceConnected event on connect",
			action: "connect",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
					return newTestClusterInstance(name), nil
				}
				m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
					return &ConnectResult{
						Connected: true,
						Version:   "15.1",
					}, nil
				}
			},
			expectedEvent: eventbus.EventInstanceConnected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			mockEventBus := NewMockEventBus()
			tt.setupMock(mockRepo)

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   mockEventBus,
				Logger:     logr.Discard(),
			})

			switch tt.action {
			case "connect":
				_, _ = handler.Connect(context.Background(), "test-cluster-instance")
			}

			// Verify event was published
			found := false
			for _, event := range mockEventBus.PublishedEvents {
				if event.EventName() == tt.expectedEvent {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected event %s to be published", tt.expectedEvent)
		})
	}
}

func TestHandler_HandleDisconnect(t *testing.T) {
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	instance := newTestClusterInstance("test-cluster-instance")
	handler.HandleDisconnect(context.Background(), instance, "connection lost")

	// Verify InstanceDisconnected event was published
	found := false
	for _, event := range mockEventBus.PublishedEvents {
		if event.EventName() == eventbus.EventInstanceDisconnected {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected InstanceDisconnected event to be published")
}

func TestHandler_HandleHealthCheckPassed(t *testing.T) {
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	instance := newTestClusterInstance("test-cluster-instance")
	handler.HandleHealthCheckPassed(context.Background(), instance)

	// Verify InstanceHealthy event was published
	found := false
	for _, event := range mockEventBus.PublishedEvents {
		if event.EventName() == eventbus.EventInstanceHealthy {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected InstanceHealthy event to be published")
}

func TestHandler_HandleHealthCheckFailed(t *testing.T) {
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	instance := newTestClusterInstance("test-cluster-instance")
	handler.HandleHealthCheckFailed(context.Background(), instance, "timeout")

	// Verify InstanceUnhealthy event was published
	found := false
	for _, event := range mockEventBus.PublishedEvents {
		if event.EventName() == eventbus.EventInstanceUnhealthy {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected InstanceUnhealthy event to be published")
}

func TestHandler_UpdateInfoMetric(t *testing.T) {
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	tests := []struct {
		name     string
		instance *dbopsv1alpha1.ClusterDatabaseInstance
	}{
		{
			name: "with version",
			instance: func() *dbopsv1alpha1.ClusterDatabaseInstance {
				inst := newTestClusterInstance("test-instance")
				inst.Status.Version = "15.1"
				inst.Status.Phase = dbopsv1alpha1.PhaseReady
				return inst
			}(),
		},
		{
			name: "without version",
			instance: func() *dbopsv1alpha1.ClusterDatabaseInstance {
				inst := newTestClusterInstance("test-instance")
				inst.Status.Version = ""
				inst.Status.Phase = ""
				return inst
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			handler.UpdateInfoMetric(tt.instance)
		})
	}
}

func TestHandler_CleanupMetrics(t *testing.T) {
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	instance := newTestClusterInstance("test-cluster-instance")

	// Should not panic
	handler.CleanupMetrics(instance)
}

func TestHandler_ConnectWithDifferentEngines(t *testing.T) {
	tests := []struct {
		name    string
		engine  dbopsv1alpha1.EngineType
		port    int32
		version string
	}{
		{
			name:    "postgres",
			engine:  dbopsv1alpha1.EngineTypePostgres,
			port:    5432,
			version: "15.1",
		},
		{
			name:    "mysql",
			engine:  dbopsv1alpha1.EngineTypeMySQL,
			port:    3306,
			version: "8.0.32",
		},
		{
			name:    "mariadb",
			engine:  dbopsv1alpha1.EngineTypeMariaDB,
			port:    3306,
			version: "10.11.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			mockEventBus := NewMockEventBus()

			mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
				inst := newTestClusterInstance(name)
				inst.Spec.Engine = tt.engine
				inst.Spec.Connection.Port = tt.port
				return inst, nil
			}
			mockRepo.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
				return &ConnectResult{
					Connected: true,
					Version:   tt.version,
					Message:   "Connected successfully",
				}, nil
			}

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   mockEventBus,
				Logger:     logr.Discard(),
			})

			result, err := handler.Connect(context.Background(), "test-cluster-instance")

			require.NoError(t, err)
			assert.True(t, result.Connected)
			assert.Equal(t, tt.version, result.Version)

			// Verify event was published with correct engine
			found := false
			for _, event := range mockEventBus.PublishedEvents {
				if event.EventName() == eventbus.EventInstanceConnected {
					found = true
					break
				}
			}
			assert.True(t, found, "Expected InstanceConnected event to be published")
		})
	}
}

func TestHandler_ConnectWithNilEventBus(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return newTestClusterInstance(name), nil
	}
	mockRepo.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
		}, nil
	}

	// Handler with nil event bus
	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   nil, // nil event bus
		Logger:     logr.Discard(),
	})

	// Should not panic even without event bus
	result, err := handler.Connect(context.Background(), "test-cluster-instance")

	require.NoError(t, err)
	assert.True(t, result.Connected)
}
