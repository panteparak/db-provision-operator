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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// MockRepository is a mock implementation of RepositoryInterface for testing.
type MockRepository struct {
	// GetInstance returns a ClusterDatabaseInstance by name (cluster-scoped, no namespace)
	GetInstanceFunc func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error)
	ConnectFunc     func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error)
	PingFunc        func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error
	GetVersionFunc  func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (string, error)

	// Call tracking
	Calls []MockCall
}

// MockCall records a method call for verification.
type MockCall struct {
	Method string
	Args   []interface{}
}

// NewMockRepository creates a new mock repository with default implementations.
func NewMockRepository() *MockRepository {
	m := &MockRepository{
		Calls: make([]MockCall, 0),
	}

	// Set default implementations
	m.GetInstanceFunc = func(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return &dbopsv1alpha1.ClusterDatabaseInstance{
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
		}, nil
	}

	m.ConnectFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
		return &ConnectResult{
			Connected: true,
			Version:   "15.1",
			Message:   "Connected successfully",
		}, nil
	}

	m.PingFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error {
		return nil
	}

	m.GetVersionFunc = func(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (string, error) {
		return "15.1", nil
	}

	return m
}

// recordCall records a method call for verification.
func (m *MockRepository) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// GetInstance implements RepositoryInterface.
// Note: Cluster-scoped resource, so only name is needed (no namespace).
func (m *MockRepository) GetInstance(ctx context.Context, name string) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
	m.recordCall("GetInstance", name)
	return m.GetInstanceFunc(ctx, name)
}

// Connect implements RepositoryInterface.
func (m *MockRepository) Connect(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (*ConnectResult, error) {
	m.recordCall("Connect", instance)
	return m.ConnectFunc(ctx, instance)
}

// Ping implements RepositoryInterface.
func (m *MockRepository) Ping(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) error {
	m.recordCall("Ping", instance)
	return m.PingFunc(ctx, instance)
}

// GetVersion implements RepositoryInterface.
func (m *MockRepository) GetVersion(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (string, error) {
	m.recordCall("GetVersion", instance)
	return m.GetVersionFunc(ctx, instance)
}

// WasCalled checks if a method was called.
func (m *MockRepository) WasCalled(method string) bool {
	for _, call := range m.Calls {
		if call.Method == method {
			return true
		}
	}
	return false
}

// CallCount returns the number of times a method was called.
func (m *MockRepository) CallCount(method string) int {
	count := 0
	for _, call := range m.Calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

// MockEventBus is a mock event bus for testing.
type MockEventBus struct {
	PublishedEvents []eventbus.Event
}

// NewMockEventBus creates a new mock event bus.
func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		PublishedEvents: make([]eventbus.Event, 0),
	}
}

// Publish records a published event.
func (m *MockEventBus) Publish(ctx context.Context, event eventbus.Event) error {
	m.PublishedEvents = append(m.PublishedEvents, event)
	return nil
}

// PublishAsync records a published event (async version).
func (m *MockEventBus) PublishAsync(ctx context.Context, event eventbus.Event) {
	m.PublishedEvents = append(m.PublishedEvents, event)
}

// Subscribe is a no-op for the mock.
func (m *MockEventBus) Subscribe(eventName string, handlerName string, handler eventbus.Handler) {}

// Unsubscribe is a no-op for the mock.
func (m *MockEventBus) Unsubscribe(eventName string, handlerName string) {}

// Handlers returns an empty list for the mock.
func (m *MockEventBus) Handlers(eventName string) []eventbus.HandlerInfo {
	return nil
}

// Ensure MockRepository implements RepositoryInterface.
var _ RepositoryInterface = (*MockRepository)(nil)
