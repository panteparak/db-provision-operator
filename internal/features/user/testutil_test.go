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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// MockRepository is a mock implementation of user repository operations for testing.
type MockRepository struct {
	CreateFunc       func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error)
	ExistsFunc       func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error)
	UpdateFunc       func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error)
	DeleteFunc       func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error
	SetPasswordFunc  func(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error
	GetInstanceFunc  func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)
	GetEngineFunc    func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error)
	DetectDriftFunc  func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error)
	CorrectDriftFunc func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)

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
	m.CreateFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
		return &Result{Created: true, Message: "created"}, nil
	}
	m.ExistsFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
		return false, nil
	}
	m.UpdateFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
		return &Result{Updated: true, Message: "updated"}, nil
	}
	m.DeleteFunc = func(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
		return nil
	}
	m.SetPasswordFunc = func(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
		return nil
	}
	m.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
			},
		}, nil
	}
	m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
		return "postgres", nil
	}
	m.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}
	m.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		return &drift.CorrectionResult{}, nil
	}

	return m
}

// recordCall records a method call for verification.
func (m *MockRepository) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// Create implements the create operation.
func (m *MockRepository) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
	m.recordCall("Create", spec, namespace, password)
	return m.CreateFunc(ctx, spec, namespace, password)
}

// Exists implements the exists check.
func (m *MockRepository) Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
	m.recordCall("Exists", username, spec, namespace)
	return m.ExistsFunc(ctx, username, spec, namespace)
}

// Update implements the update operation.
func (m *MockRepository) Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
	m.recordCall("Update", username, spec, namespace)
	return m.UpdateFunc(ctx, username, spec, namespace)
}

// Delete implements the delete operation.
func (m *MockRepository) Delete(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
	m.recordCall("Delete", username, spec, namespace, force)
	return m.DeleteFunc(ctx, username, spec, namespace, force)
}

// SetPassword implements the set password operation.
func (m *MockRepository) SetPassword(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
	m.recordCall("SetPassword", username, password, spec, namespace)
	return m.SetPasswordFunc(ctx, username, password, spec, namespace)
}

// GetInstance implements the get instance operation.
func (m *MockRepository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	m.recordCall("GetInstance", spec, namespace)
	return m.GetInstanceFunc(ctx, spec, namespace)
}

// GetEngine implements the get engine operation.
func (m *MockRepository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
	m.recordCall("GetEngine", spec, namespace)
	return m.GetEngineFunc(ctx, spec, namespace)
}

// DetectDrift implements drift detection.
func (m *MockRepository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	m.recordCall("DetectDrift", spec, namespace, allowDestructive)
	return m.DetectDriftFunc(ctx, spec, namespace, allowDestructive)
}

// CorrectDrift implements drift correction.
func (m *MockRepository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	m.recordCall("CorrectDrift", spec, namespace, driftResult, allowDestructive)
	return m.CorrectDriftFunc(ctx, spec, namespace, driftResult, allowDestructive)
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
