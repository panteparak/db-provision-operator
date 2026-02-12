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

package backup

import (
	"context"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/shared/eventbus"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// MockRepository is a mock implementation of backup repository operations for testing.
type MockRepository struct {
	ExecuteBackupFunc              func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error)
	DeleteBackupFunc               func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error
	GetDatabaseFunc                func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error)
	GetInstanceFunc                func(ctx context.Context, database *dbopsv1alpha1.Database) (*dbopsv1alpha1.DatabaseInstance, error)
	ResolveInstanceForDatabaseFunc func(ctx context.Context, database *dbopsv1alpha1.Database) (*instanceresolver.ResolvedInstance, error)
	GetEngineFunc                  func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error)

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
	m.ExecuteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
		return &BackupExecutionResult{
			Path:      "/backups/testdb-backup.dump",
			SizeBytes: 1024 * 1024, // 1MB
			Checksum:  "abc123",
			Format:    "custom",
			Duration:  5 * time.Second,
			Instance:  "test-instance",
			Database:  "testdb",
			Engine:    "postgres",
		}, nil
	}
	m.DeleteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
		return nil
	}
	m.GetDatabaseFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error) {
		return &dbopsv1alpha1.Database{
			Spec: dbopsv1alpha1.DatabaseSpec{
				Name: "testdb",
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
			},
			Status: dbopsv1alpha1.DatabaseStatus{
				Phase: dbopsv1alpha1.PhaseReady,
			},
		}, nil
	}
	m.GetInstanceFunc = func(ctx context.Context, database *dbopsv1alpha1.Database) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
			},
			Status: dbopsv1alpha1.DatabaseInstanceStatus{
				Phase: dbopsv1alpha1.PhaseReady,
			},
		}, nil
	}
	m.ResolveInstanceForDatabaseFunc = func(ctx context.Context, database *dbopsv1alpha1.Database) (*instanceresolver.ResolvedInstance, error) {
		return &instanceresolver.ResolvedInstance{
			Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
			CredentialNamespace: "default",
			Phase:               dbopsv1alpha1.PhaseReady,
			Name:                "test-instance",
		}, nil
	}
	m.GetEngineFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
		return "postgres", nil
	}

	return m
}

// recordCall records a method call for verification.
func (m *MockRepository) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// ExecuteBackup implements the backup execution.
func (m *MockRepository) ExecuteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
	m.recordCall("ExecuteBackup", backup)
	return m.ExecuteBackupFunc(ctx, backup)
}

// DeleteBackup implements the backup deletion.
func (m *MockRepository) DeleteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
	m.recordCall("DeleteBackup", backup)
	return m.DeleteBackupFunc(ctx, backup)
}

// GetDatabase implements the get database operation.
func (m *MockRepository) GetDatabase(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error) {
	m.recordCall("GetDatabase", backup)
	return m.GetDatabaseFunc(ctx, backup)
}

// GetInstance implements the get instance operation.
func (m *MockRepository) GetInstance(ctx context.Context, database *dbopsv1alpha1.Database) (*dbopsv1alpha1.DatabaseInstance, error) {
	m.recordCall("GetInstance", database)
	return m.GetInstanceFunc(ctx, database)
}

// ResolveInstanceForDatabase implements the resolve instance for database operation.
func (m *MockRepository) ResolveInstanceForDatabase(ctx context.Context, database *dbopsv1alpha1.Database) (*instanceresolver.ResolvedInstance, error) {
	m.recordCall("ResolveInstanceForDatabase", database)
	return m.ResolveInstanceForDatabaseFunc(ctx, database)
}

// GetEngine implements the get engine operation.
func (m *MockRepository) GetEngine(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
	m.recordCall("GetEngine", backup)
	return m.GetEngineFunc(ctx, backup)
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
