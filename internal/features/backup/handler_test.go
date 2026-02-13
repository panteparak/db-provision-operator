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
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func newTestBackup(name, namespace string) *dbopsv1alpha1.DatabaseBackup {
	return &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: dbopsv1alpha1.DatabaseBackupSpec{
			DatabaseRef: dbopsv1alpha1.DatabaseReference{
				Name: "testdb",
			},
			Storage: dbopsv1alpha1.StorageConfig{
				Type: dbopsv1alpha1.StorageTypePVC,
				PVC: &dbopsv1alpha1.PVCStorageConfig{
					ClaimName: "backup-pvc",
					SubPath:   "backups",
				},
			},
		},
	}
}

func TestHandler_Execute(t *testing.T) {
	tests := []struct {
		name        string
		backup      *dbopsv1alpha1.DatabaseBackup
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
		wantSuccess bool
	}{
		{
			name:   "successful execution",
			backup: newTestBackup("testbackup", "default"),
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
					return "postgres", nil
				}
				m.ExecuteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
					return &BackupExecutionResult{
						Path:      "/backups/testdb-backup.dump",
						SizeBytes: 1024 * 1024,
						Checksum:  "abc123",
						Format:    "custom",
						Duration:  5 * time.Second,
						Database:  "testdb",
					}, nil
				}
			},
			wantErr:     false,
			wantSuccess: true,
		},
		{
			name:   "get engine error",
			backup: newTestBackup("testbackup", "default"),
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
					return "", errors.New("database not found")
				}
			},
			wantErr:     true,
			errContains: "get engine",
		},
		{
			name:   "execute backup error",
			backup: newTestBackup("testbackup", "default"),
			setupMock: func(m *MockRepository) {
				m.GetEngineFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
					return "postgres", nil
				}
				m.ExecuteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
					return nil, errors.New("connection refused")
				}
			},
			wantErr:     true,
			errContains: "execute backup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := &Handler{
				repo:              mockRepo,
				eventBus:          NewMockEventBus(),
				logger:            logr.Discard(),
				instanceConnected: make(map[string]bool),
			}

			result, err := handler.Execute(context.Background(), tt.backup)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantSuccess, result.Success)
		})
	}
}

func TestHandler_Delete(t *testing.T) {
	tests := []struct {
		name      string
		backup    *dbopsv1alpha1.DatabaseBackup
		setupMock func(*MockRepository)
		wantErr   bool
	}{
		{
			name:   "successful deletion",
			backup: newTestBackup("testbackup", "default"),
			setupMock: func(m *MockRepository) {
				m.DeleteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
					return nil
				}
			},
			wantErr: false,
		},
		{
			name:   "deletion error propagated",
			backup: newTestBackup("testbackup", "default"),
			setupMock: func(m *MockRepository) {
				m.DeleteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
					return errors.New("file not found")
				}
			},
			wantErr: true, // Delete propagates errors to allow caller to decide
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			tt.setupMock(mockRepo)

			handler := &Handler{
				repo:              mockRepo,
				eventBus:          NewMockEventBus(),
				logger:            logr.Discard(),
				instanceConnected: make(map[string]bool),
			}

			err := handler.Delete(context.Background(), tt.backup)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestHandler_GetStatus(t *testing.T) {
	now := metav1.Now()
	completedAt := metav1.Now()

	tests := []struct {
		name      string
		backup    *dbopsv1alpha1.DatabaseBackup
		wantPhase dbopsv1alpha1.Phase
	}{
		{
			name: "pending backup",
			backup: &dbopsv1alpha1.DatabaseBackup{
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase:   dbopsv1alpha1.PhasePending,
					Message: "Waiting to start",
				},
			},
			wantPhase: dbopsv1alpha1.PhasePending,
		},
		{
			name: "completed backup",
			backup: &dbopsv1alpha1.DatabaseBackup{
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase:       dbopsv1alpha1.PhaseReady,
					Message:     "Backup completed",
					StartedAt:   &now,
					CompletedAt: &completedAt,
				},
			},
			wantPhase: dbopsv1alpha1.PhaseReady,
		},
		{
			name: "failed backup",
			backup: &dbopsv1alpha1.DatabaseBackup{
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase:   dbopsv1alpha1.PhaseFailed,
					Message: "Backup failed",
				},
			},
			wantPhase: dbopsv1alpha1.PhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				logger: logr.Discard(),
			}

			status, err := handler.GetStatus(context.Background(), tt.backup)

			require.NoError(t, err)
			require.NotNil(t, status)
			assert.Equal(t, tt.wantPhase, status.Phase)
		})
	}
}

func TestHandler_IsExpired(t *testing.T) {
	past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	future := metav1.NewTime(time.Now().Add(1 * time.Hour))

	tests := []struct {
		name        string
		backup      *dbopsv1alpha1.DatabaseBackup
		wantExpired bool
	}{
		{
			name: "no expiry set",
			backup: &dbopsv1alpha1.DatabaseBackup{
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					ExpiresAt: nil,
				},
			},
			wantExpired: false,
		},
		{
			name: "expired",
			backup: &dbopsv1alpha1.DatabaseBackup{
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					ExpiresAt: &past,
				},
			},
			wantExpired: true,
		},
		{
			name: "not expired",
			backup: &dbopsv1alpha1.DatabaseBackup{
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					ExpiresAt: &future,
				},
			},
			wantExpired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				logger: logr.Discard(),
			}

			expired := handler.IsExpired(tt.backup)
			assert.Equal(t, tt.wantExpired, expired)
		})
	}
}

func TestHandler_InstanceConnectivity(t *testing.T) {
	handler := &Handler{
		logger:            logr.Discard(),
		instanceConnected: make(map[string]bool),
	}

	// Initially, unknown instances are assumed connected
	assert.True(t, handler.IsInstanceConnected("default", "test-instance"))

	// Simulate disconnect event
	handler.instanceConnected["default/test-instance"] = false
	assert.False(t, handler.IsInstanceConnected("default", "test-instance"))

	// Simulate reconnect event
	handler.instanceConnected["default/test-instance"] = true
	assert.True(t, handler.IsInstanceConnected("default", "test-instance"))
}

func TestHandler_EventPublishing(t *testing.T) {
	t.Run("publishes events on successful backup", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.GetEngineFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
			return "postgres", nil
		}
		mockRepo.ExecuteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
			return &BackupExecutionResult{
				Path:      "/backups/testdb-backup.dump",
				SizeBytes: 1024 * 1024,
				Checksum:  "abc123",
				Database:  "testdb",
			}, nil
		}

		handler := &Handler{
			repo:              mockRepo,
			eventBus:          mockEventBus,
			logger:            logr.Discard(),
			instanceConnected: make(map[string]bool),
		}

		backup := newTestBackup("testbackup", "default")
		_, err := handler.Execute(context.Background(), backup)
		require.NoError(t, err)

		// Should have BackupStarted and BackupCompleted events
		assert.Len(t, mockEventBus.PublishedEvents, 2)
	})

	t.Run("publishes BackupFailed event on error", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.GetEngineFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
			return "postgres", nil
		}
		mockRepo.ExecuteBackupFunc = func(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
			return nil, errors.New("backup failed")
		}

		handler := &Handler{
			repo:              mockRepo,
			eventBus:          mockEventBus,
			logger:            logr.Discard(),
			instanceConnected: make(map[string]bool),
		}

		backup := newTestBackup("testbackup", "default")
		_, err := handler.Execute(context.Background(), backup)
		require.Error(t, err)

		// Should have BackupStarted and BackupFailed events
		assert.Len(t, mockEventBus.PublishedEvents, 2)
	})
}
