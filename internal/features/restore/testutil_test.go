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

package restore

import (
	"context"
	"io"
	"strings"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/shared/eventbus"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/storage"
)

// MockRepository is a mock implementation of restore repository operations for testing.
type MockRepository struct {
	GetBackupFunc                  func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error)
	GetDatabaseFunc                func(ctx context.Context, namespace string, dbRef *dbopsv1alpha1.DatabaseReference) (*dbopsv1alpha1.Database, error)
	GetInstanceFunc                func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error)
	ResolveInstanceFunc            func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error)
	CreateRestoreReaderFunc        func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error)
	ExecuteRestoreFunc             func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error)
	ExecuteRestoreWithResolvedFunc func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error)
	GetEngineFunc                  func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (string, error)
	GetEngineWithRefsFunc          func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (string, error)

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
	m.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
		return &dbopsv1alpha1.DatabaseBackup{
			Spec: dbopsv1alpha1.DatabaseBackupSpec{
				DatabaseRef: dbopsv1alpha1.DatabaseReference{
					Name: "testdb",
				},
				Storage: dbopsv1alpha1.StorageConfig{
					Type: dbopsv1alpha1.StorageTypePVC,
					PVC: &dbopsv1alpha1.PVCStorageConfig{
						ClaimName: "backup-pvc",
					},
				},
			},
			Status: dbopsv1alpha1.DatabaseBackupStatus{
				Phase: dbopsv1alpha1.PhaseCompleted,
				Backup: &dbopsv1alpha1.BackupInfo{
					Path: "/backups/testdb-backup.dump",
				},
				Source: &dbopsv1alpha1.BackupSourceInfo{
					Instance: "test-instance",
					Database: "testdb",
					Engine:   "postgres",
				},
			},
		}, nil
	}

	m.GetDatabaseFunc = func(ctx context.Context, namespace string, dbRef *dbopsv1alpha1.DatabaseReference) (*dbopsv1alpha1.Database, error) {
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

	m.GetInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
		return &dbopsv1alpha1.DatabaseInstance{
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
			},
			Status: dbopsv1alpha1.DatabaseInstanceStatus{
				Phase:   dbopsv1alpha1.PhaseReady,
				Version: "15.1",
			},
		}, nil
	}

	m.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
		return &instanceresolver.ResolvedInstance{
			Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
			CredentialNamespace: namespace,
			Phase:               dbopsv1alpha1.PhaseReady,
			Name:                "test-instance",
			Version:             "15.1",
		}, nil
	}

	m.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("mock backup data")), nil
	}

	m.ExecuteRestoreFunc = func(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
		return &adapterpkg.RestoreResult{
			TargetDatabase: opts.Database,
			TablesRestored: 10,
		}, nil
	}

	m.ExecuteRestoreWithResolvedFunc = func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
		return &adapterpkg.RestoreResult{
			TargetDatabase: opts.Database,
			TablesRestored: 10,
		}, nil
	}

	m.GetEngineFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (string, error) {
		return "postgres", nil
	}

	m.GetEngineWithRefsFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (string, error) {
		return "postgres", nil
	}

	return m
}

// recordCall records a method call for verification.
func (m *MockRepository) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// GetBackup implements the get backup operation.
func (m *MockRepository) GetBackup(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
	m.recordCall("GetBackup", namespace, backupRef)
	return m.GetBackupFunc(ctx, namespace, backupRef)
}

// GetDatabase implements the get database operation.
func (m *MockRepository) GetDatabase(ctx context.Context, namespace string, dbRef *dbopsv1alpha1.DatabaseReference) (*dbopsv1alpha1.Database, error) {
	m.recordCall("GetDatabase", namespace, dbRef)
	return m.GetDatabaseFunc(ctx, namespace, dbRef)
}

// GetInstance implements the get instance operation.
func (m *MockRepository) GetInstance(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
	m.recordCall("GetInstance", namespace, instanceRef)
	return m.GetInstanceFunc(ctx, namespace, instanceRef)
}

// ResolveInstance implements the resolve instance operation.
func (m *MockRepository) ResolveInstance(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
	m.recordCall("ResolveInstance", namespace, instanceRef, clusterInstanceRef)
	return m.ResolveInstanceFunc(ctx, namespace, instanceRef, clusterInstanceRef)
}

// CreateRestoreReader implements the create restore reader operation.
func (m *MockRepository) CreateRestoreReader(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
	m.recordCall("CreateRestoreReader", cfg)
	return m.CreateRestoreReaderFunc(ctx, cfg)
}

// ExecuteRestore implements the execute restore operation.
func (m *MockRepository) ExecuteRestore(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
	m.recordCall("ExecuteRestore", instance, opts)
	return m.ExecuteRestoreFunc(ctx, instance, opts)
}

// ExecuteRestoreWithResolved implements the execute restore with resolved instance operation.
func (m *MockRepository) ExecuteRestoreWithResolved(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
	m.recordCall("ExecuteRestoreWithResolved", resolved, opts)
	return m.ExecuteRestoreWithResolvedFunc(ctx, resolved, opts)
}

// GetEngine implements the get engine operation.
func (m *MockRepository) GetEngine(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (string, error) {
	m.recordCall("GetEngine", namespace, instanceRef)
	return m.GetEngineFunc(ctx, namespace, instanceRef)
}

// GetEngineWithRefs implements the get engine with refs operation.
func (m *MockRepository) GetEngineWithRefs(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (string, error) {
	m.recordCall("GetEngineWithRefs", namespace, instanceRef, clusterInstanceRef)
	return m.GetEngineWithRefsFunc(ctx, namespace, instanceRef, clusterInstanceRef)
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
