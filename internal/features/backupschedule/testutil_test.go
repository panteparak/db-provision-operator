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

package backupschedule

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// MockRepository is a mock implementation of RepositoryInterface for testing.
type MockRepository struct {
	ListBackupsForScheduleFunc          func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error)
	FilterBackupsByPhaseFunc            func(backups []dbopsv1alpha1.DatabaseBackup, phase dbopsv1alpha1.Phase) []dbopsv1alpha1.DatabaseBackup
	CreateBackupFromTemplateFunc        func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error)
	DeleteBackupsFunc                   func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error)
	GetCompletedBackupsSortedByTimeFunc func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup
	GetBackupsSortedByTimeFunc          func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup
	GetLatestBackupFunc                 func(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup

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
	m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{}, nil
	}

	m.FilterBackupsByPhaseFunc = func(backups []dbopsv1alpha1.DatabaseBackup, phase dbopsv1alpha1.Phase) []dbopsv1alpha1.DatabaseBackup {
		var result []dbopsv1alpha1.DatabaseBackup
		for _, backup := range backups {
			if backup.Status.Phase == phase {
				result = append(result, backup)
			}
		}
		return result
	}

	m.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
		return &dbopsv1alpha1.DatabaseBackup{}, nil
	}

	m.DeleteBackupsFunc = func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
		var deleted []string
		for _, backup := range backups {
			deleted = append(deleted, backup.Name)
		}
		return deleted, nil
	}

	m.GetCompletedBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		var completed []dbopsv1alpha1.DatabaseBackup
		for _, backup := range backups {
			if backup.Status.Phase == dbopsv1alpha1.PhaseCompleted {
				completed = append(completed, backup)
			}
		}
		return completed
	}

	m.GetBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		return backups
	}

	m.GetLatestBackupFunc = func(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup {
		if len(backups) == 0 {
			return nil
		}
		return &backups[0]
	}

	return m
}

// recordCall records a method call for verification.
func (m *MockRepository) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// ListBackupsForSchedule implements RepositoryInterface.
func (m *MockRepository) ListBackupsForSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
	m.recordCall("ListBackupsForSchedule", schedule)
	return m.ListBackupsForScheduleFunc(ctx, schedule)
}

// FilterBackupsByPhase implements RepositoryInterface.
func (m *MockRepository) FilterBackupsByPhase(backups []dbopsv1alpha1.DatabaseBackup, phase dbopsv1alpha1.Phase) []dbopsv1alpha1.DatabaseBackup {
	m.recordCall("FilterBackupsByPhase", backups, phase)
	return m.FilterBackupsByPhaseFunc(backups, phase)
}

// CreateBackupFromTemplate implements RepositoryInterface.
func (m *MockRepository) CreateBackupFromTemplate(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
	m.recordCall("CreateBackupFromTemplate", schedule)
	return m.CreateBackupFromTemplateFunc(ctx, schedule)
}

// DeleteBackups implements RepositoryInterface.
func (m *MockRepository) DeleteBackups(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
	m.recordCall("DeleteBackups", backups)
	return m.DeleteBackupsFunc(ctx, backups)
}

// GetCompletedBackupsSortedByTime implements RepositoryInterface.
func (m *MockRepository) GetCompletedBackupsSortedByTime(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
	m.recordCall("GetCompletedBackupsSortedByTime", backups)
	return m.GetCompletedBackupsSortedByTimeFunc(backups)
}

// GetBackupsSortedByTime implements RepositoryInterface.
func (m *MockRepository) GetBackupsSortedByTime(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
	m.recordCall("GetBackupsSortedByTime", backups)
	return m.GetBackupsSortedByTimeFunc(backups)
}

// GetLatestBackup implements RepositoryInterface.
func (m *MockRepository) GetLatestBackup(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup {
	m.recordCall("GetLatestBackup", backups)
	return m.GetLatestBackupFunc(backups)
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
