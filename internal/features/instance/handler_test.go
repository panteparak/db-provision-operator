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

package instance

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

func newTestInstance(name, namespace string) *dbopsv1alpha1.DatabaseInstance {
	return &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
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
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr: true,
		},
		{
			name: "connection failed",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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

			result, err := handler.Connect(context.Background(), "test-instance", "default")

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
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr: true,
		},
		{
			name: "ping failed",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) error {
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

			err := handler.Ping(context.Background(), "test-instance", "default")

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
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.GetVersionFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (string, error) {
					return "15.2", nil
				}
			},
			wantErr: false,
			wantVer: "15.2",
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return nil, errors.New("not found")
				}
			},
			wantErr: true,
		},
		{
			name: "get version failed",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.GetVersionFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (string, error) {
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

			version, err := handler.GetVersion(context.Background(), "test-instance", "default")

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
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) error {
					return nil
				}
			},
			wantErr: false,
			healthy: true,
		},
		{
			name: "instance is not healthy",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) error {
					return errors.New("connection lost")
				}
			},
			wantErr: false, // IsHealthy returns false on ping failure, not an error
			healthy: false,
		},
		{
			name: "instance not found",
			setupMock: func(m *MockRepository) {
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
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

			healthy, err := handler.IsHealthy(context.Background(), "test-instance", "default")

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
				m.GetInstanceFunc = func(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
					return newTestInstance(name, namespace), nil
				}
				m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
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
				_, _ = handler.Connect(context.Background(), "test-instance", "default")
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

	instance := newTestInstance("test-instance", "default")
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

	instance := newTestInstance("test-instance", "default")
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

	instance := newTestInstance("test-instance", "default")
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
