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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// MockRepository is a mock implementation of cluster grant repository operations for testing.
type MockRepository struct {
	ApplyFunc         func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error)
	RevokeFunc        func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error
	ExistsFunc        func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error)
	GetInstanceFunc   func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error)
	GetEngineFunc     func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error)
	ResolveTargetFunc func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error)
	DetectDriftFunc   func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error)
	CorrectDriftFunc  func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)

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
	m.ApplyFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
		return &Result{Applied: true, Message: "applied", DirectGrants: 1}, nil
	}
	m.RevokeFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
		return nil
	}
	m.ExistsFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
		return true, nil
	}
	m.GetInstanceFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
		return &dbopsv1alpha1.ClusterDatabaseInstance{
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
			},
		}, nil
	}
	m.GetEngineFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
		return "postgres", nil
	}
	m.ResolveTargetFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
		if spec.UserRef != nil {
			return &TargetInfo{
				Type:          "user",
				Name:          spec.UserRef.Name,
				Namespace:     spec.UserRef.Namespace,
				DatabaseName:  spec.UserRef.Name,
				IsClusterRole: false,
			}, nil
		}
		if spec.RoleRef != nil {
			return &TargetInfo{
				Type:          "role",
				Name:          spec.RoleRef.Name,
				Namespace:     spec.RoleRef.Namespace,
				DatabaseName:  spec.RoleRef.Name,
				IsClusterRole: spec.RoleRef.Namespace == "",
			}, nil
		}
		return nil, nil
	}
	m.DetectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
		return &drift.Result{}, nil
	}
	m.CorrectDriftFunc = func(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
		return &drift.CorrectionResult{}, nil
	}

	return m
}

// recordCall records a method call for verification.
func (m *MockRepository) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// Apply implements the apply operation.
func (m *MockRepository) Apply(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
	m.recordCall("Apply", spec)
	return m.ApplyFunc(ctx, spec)
}

// Revoke implements the revoke operation.
func (m *MockRepository) Revoke(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
	m.recordCall("Revoke", spec)
	return m.RevokeFunc(ctx, spec)
}

// Exists implements the exists check.
func (m *MockRepository) Exists(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
	m.recordCall("Exists", spec)
	return m.ExistsFunc(ctx, spec)
}

// GetInstance implements the get instance operation.
func (m *MockRepository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
	m.recordCall("GetInstance", spec)
	return m.GetInstanceFunc(ctx, spec)
}

// GetEngine implements the get engine operation.
func (m *MockRepository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
	m.recordCall("GetEngine", spec)
	return m.GetEngineFunc(ctx, spec)
}

// ResolveTarget implements target resolution.
func (m *MockRepository) ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
	m.recordCall("ResolveTarget", spec)
	return m.ResolveTargetFunc(ctx, spec)
}

// DetectDrift implements drift detection.
func (m *MockRepository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
	m.recordCall("DetectDrift", spec, allowDestructive)
	return m.DetectDriftFunc(ctx, spec, allowDestructive)
}

// CorrectDrift implements drift correction.
func (m *MockRepository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	m.recordCall("CorrectDrift", spec, driftResult, allowDestructive)
	return m.CorrectDriftFunc(ctx, spec, driftResult, allowDestructive)
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
