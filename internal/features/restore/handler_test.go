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
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/storage"
)

func newTestRestore(name, namespace string) *dbopsv1alpha1.DatabaseRestore {
	return &dbopsv1alpha1.DatabaseRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "test-uid",
		},
		Spec: dbopsv1alpha1.DatabaseRestoreSpec{
			BackupRef: &dbopsv1alpha1.BackupReference{
				Name: "test-backup",
			},
			Target: dbopsv1alpha1.RestoreTarget{
				InstanceRef: &dbopsv1alpha1.InstanceReference{
					Name: "test-instance",
				},
				DatabaseName: "restored-db",
			},
		},
	}
}

func TestHandler_ValidateSpec(t *testing.T) {
	tests := []struct {
		name        string
		restore     *dbopsv1alpha1.DatabaseRestore
		setupMock   func(*MockRepository)
		wantErr     bool
		errContains string
	}{
		{
			name: "valid spec with backup ref",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: "test-backup",
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: "test-instance",
						},
						DatabaseName: "restored-db",
					},
				},
			},
			setupMock: func(m *MockRepository) {
				m.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
					return &dbopsv1alpha1.DatabaseBackup{
						Status: dbopsv1alpha1.DatabaseBackupStatus{
							Phase: dbopsv1alpha1.PhaseCompleted,
						},
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name: "missing source - neither backupRef nor fromPath",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: "test-instance",
						},
						DatabaseName: "restored-db",
					},
				},
			},
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "either backupRef or fromPath must be specified",
		},
		{
			name: "in-place restore without confirmation",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: "test-backup",
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InPlace: true,
						DatabaseRef: &dbopsv1alpha1.DatabaseReference{
							Name: "target-db",
						},
					},
				},
			},
			setupMock:   func(m *MockRepository) {},
			wantErr:     true,
			errContains: "acknowledgeDataLoss",
		},
		{
			name: "valid in-place restore with confirmation",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: "test-backup",
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InPlace: true,
						DatabaseRef: &dbopsv1alpha1.DatabaseReference{
							Name: "target-db",
						},
					},
					Confirmation: &dbopsv1alpha1.RestoreConfirmation{
						AcknowledgeDataLoss: dbopsv1alpha1.RestoreConfirmDataLoss,
					},
				},
			},
			setupMock: func(m *MockRepository) {
				m.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
					return &dbopsv1alpha1.DatabaseBackup{
						Status: dbopsv1alpha1.DatabaseBackupStatus{
							Phase: dbopsv1alpha1.PhaseCompleted,
						},
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name: "backup not completed",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					BackupRef: &dbopsv1alpha1.BackupReference{
						Name: "test-backup",
					},
					Target: dbopsv1alpha1.RestoreTarget{
						InstanceRef: &dbopsv1alpha1.InstanceReference{
							Name: "test-instance",
						},
						DatabaseName: "restored-db",
					},
				},
			},
			setupMock: func(m *MockRepository) {
				m.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
					return &dbopsv1alpha1.DatabaseBackup{
						ObjectMeta: metav1.ObjectMeta{
							Name: "test-backup",
						},
						Status: dbopsv1alpha1.DatabaseBackupStatus{
							Phase: dbopsv1alpha1.PhaseRunning,
						},
					}, nil
				}
			},
			wantErr:     true,
			errContains: "not completed",
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

			err := handler.ValidateSpec(context.Background(), tt.restore)

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

func TestHandler_CheckDeadline(t *testing.T) {
	tests := []struct {
		name         string
		restore      *dbopsv1alpha1.DatabaseRestore
		wantExceeded bool
	}{
		{
			name: "no deadline set",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					ActiveDeadlineSeconds: 0,
				},
				Status: dbopsv1alpha1.DatabaseRestoreStatus{
					StartedAt: &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				},
			},
			wantExceeded: false,
		},
		{
			name: "not started yet",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					ActiveDeadlineSeconds: 60,
				},
				Status: dbopsv1alpha1.DatabaseRestoreStatus{
					StartedAt: nil,
				},
			},
			wantExceeded: false,
		},
		{
			name: "deadline exceeded",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					ActiveDeadlineSeconds: 60,
				},
				Status: dbopsv1alpha1.DatabaseRestoreStatus{
					StartedAt: &metav1.Time{Time: time.Now().Add(-2 * time.Minute)},
				},
			},
			wantExceeded: true,
		},
		{
			name: "deadline not exceeded",
			restore: &dbopsv1alpha1.DatabaseRestore{
				Spec: dbopsv1alpha1.DatabaseRestoreSpec{
					ActiveDeadlineSeconds: 3600,
				},
				Status: dbopsv1alpha1.DatabaseRestoreStatus{
					StartedAt: &metav1.Time{Time: time.Now().Add(-1 * time.Minute)},
				},
			},
			wantExceeded: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				logger: logr.Discard(),
			}

			exceeded, _ := handler.CheckDeadline(tt.restore)
			assert.Equal(t, tt.wantExceeded, exceeded)
		})
	}
}

func TestHandler_IsTerminal(t *testing.T) {
	tests := []struct {
		name         string
		phase        dbopsv1alpha1.Phase
		wantTerminal bool
	}{
		{
			name:         "pending is not terminal",
			phase:        dbopsv1alpha1.PhasePending,
			wantTerminal: false,
		},
		{
			name:         "running is not terminal",
			phase:        dbopsv1alpha1.PhaseRunning,
			wantTerminal: false,
		},
		{
			name:         "completed is terminal",
			phase:        dbopsv1alpha1.PhaseCompleted,
			wantTerminal: true,
		},
		{
			name:         "failed is terminal",
			phase:        dbopsv1alpha1.PhaseFailed,
			wantTerminal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &Handler{
				logger: logr.Discard(),
			}

			restore := &dbopsv1alpha1.DatabaseRestore{
				Status: dbopsv1alpha1.DatabaseRestoreStatus{
					Phase: tt.phase,
				},
			}

			assert.Equal(t, tt.wantTerminal, handler.IsTerminal(restore))
		})
	}
}

func TestHandler_Execute(t *testing.T) {
	tests := []struct {
		name        string
		restore     *dbopsv1alpha1.DatabaseRestore
		setupMock   func(*MockRepository)
		paused      bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "successful execution with backup ref",
			restore: newTestRestore("test-restore", "default"),
			setupMock: func(m *MockRepository) {
				m.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
					return &instanceresolver.ResolvedInstance{
						Name:                "test-instance",
						Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
						Phase:               dbopsv1alpha1.PhaseReady,
						CredentialNamespace: namespace,
					}, nil
				}
				m.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
					return &dbopsv1alpha1.DatabaseBackup{
						Spec: dbopsv1alpha1.DatabaseBackupSpec{
							Storage: dbopsv1alpha1.StorageConfig{
								Type: dbopsv1alpha1.StorageTypePVC,
							},
						},
						Status: dbopsv1alpha1.DatabaseBackupStatus{
							Phase: dbopsv1alpha1.PhaseCompleted,
							Backup: &dbopsv1alpha1.BackupInfo{
								Path: "/backups/test.dump",
							},
						},
					}, nil
				}
				m.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("mock data")), nil
				}
				m.ExecuteRestoreWithResolvedFunc = func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
					return &adapterpkg.RestoreResult{
						TargetDatabase: opts.Database,
						TablesRestored: 5,
					}, nil
				}
			},
			wantErr: false,
		},
		{
			name:        "paused operations",
			restore:     newTestRestore("test-restore", "default"),
			setupMock:   func(m *MockRepository) {},
			paused:      true,
			wantErr:     true,
			errContains: "paused",
		},
		{
			name:    "instance not ready",
			restore: newTestRestore("test-restore", "default"),
			setupMock: func(m *MockRepository) {
				m.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
					return &instanceresolver.ResolvedInstance{
						Name:                "test-instance",
						Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
						Phase:               dbopsv1alpha1.PhasePending, // Not ready
						CredentialNamespace: namespace,
					}, nil
				}
			},
			wantErr:     true,
			errContains: "not ready",
		},
		{
			name:    "execute restore error",
			restore: newTestRestore("test-restore", "default"),
			setupMock: func(m *MockRepository) {
				m.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
					return &instanceresolver.ResolvedInstance{
						Name:                "test-instance",
						Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
						Phase:               dbopsv1alpha1.PhaseReady,
						CredentialNamespace: namespace,
					}, nil
				}
				m.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
					return &dbopsv1alpha1.DatabaseBackup{
						Spec: dbopsv1alpha1.DatabaseBackupSpec{
							Storage: dbopsv1alpha1.StorageConfig{
								Type: dbopsv1alpha1.StorageTypePVC,
							},
						},
						Status: dbopsv1alpha1.DatabaseBackupStatus{
							Phase: dbopsv1alpha1.PhaseCompleted,
							Backup: &dbopsv1alpha1.BackupInfo{
								Path: "/backups/test.dump",
							},
						},
					}, nil
				}
				m.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
					return io.NopCloser(strings.NewReader("mock data")), nil
				}
				m.ExecuteRestoreWithResolvedFunc = func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
					return nil, errors.New("connection refused")
				}
			},
			wantErr:     true,
			errContains: "execute restore",
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
				paused:   tt.paused,
			}

			result, err := handler.Execute(context.Background(), tt.restore)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.NotEmpty(t, result.TargetDatabase)
		})
	}
}

func TestHandler_EventPublishing(t *testing.T) {
	t.Run("publishes events on successful restore", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
			return &instanceresolver.ResolvedInstance{
				Name:                "test-instance",
				Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
				Phase:               dbopsv1alpha1.PhaseReady,
				CredentialNamespace: namespace,
			}, nil
		}
		mockRepo.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
			return &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-backup",
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
					},
				},
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase: dbopsv1alpha1.PhaseCompleted,
					Backup: &dbopsv1alpha1.BackupInfo{
						Path: "/backups/test.dump",
					},
				},
			}, nil
		}
		mockRepo.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("mock data")), nil
		}
		mockRepo.ExecuteRestoreWithResolvedFunc = func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
			return &adapterpkg.RestoreResult{
				TargetDatabase: opts.Database,
				TablesRestored: 5,
			}, nil
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		restore := newTestRestore("test-restore", "default")
		_, err := handler.Execute(context.Background(), restore)
		require.NoError(t, err)

		// Should have RestoreStarted and RestoreCompleted events
		assert.Len(t, mockEventBus.PublishedEvents, 2)
	})

	t.Run("publishes RestoreFailed event on error", func(t *testing.T) {
		mockRepo := NewMockRepository()
		mockEventBus := NewMockEventBus()

		mockRepo.ResolveInstanceFunc = func(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference, clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference) (*instanceresolver.ResolvedInstance, error) {
			return &instanceresolver.ResolvedInstance{
				Name:                "test-instance",
				Spec:                &dbopsv1alpha1.DatabaseInstanceSpec{Engine: dbopsv1alpha1.EngineTypePostgres},
				Phase:               dbopsv1alpha1.PhaseReady,
				CredentialNamespace: namespace,
			}, nil
		}
		mockRepo.GetBackupFunc = func(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
			return &dbopsv1alpha1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-backup",
				},
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					Storage: dbopsv1alpha1.StorageConfig{
						Type: dbopsv1alpha1.StorageTypePVC,
					},
				},
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase: dbopsv1alpha1.PhaseCompleted,
					Backup: &dbopsv1alpha1.BackupInfo{
						Path: "/backups/test.dump",
					},
				},
			}, nil
		}
		mockRepo.CreateRestoreReaderFunc = func(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("mock data")), nil
		}
		mockRepo.ExecuteRestoreWithResolvedFunc = func(ctx context.Context, resolved *instanceresolver.ResolvedInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
			return nil, errors.New("restore failed")
		}

		handler := &Handler{
			repo:     mockRepo,
			eventBus: mockEventBus,
			logger:   logr.Discard(),
		}

		restore := newTestRestore("test-restore", "default")
		_, err := handler.Execute(context.Background(), restore)
		require.Error(t, err)

		// Should have RestoreStarted and RestoreFailed events
		assert.Len(t, mockEventBus.PublishedEvents, 2)
	})
}

func TestHandler_InstancePauseResume(t *testing.T) {
	handler := &Handler{
		logger: logr.Discard(),
		paused: false,
	}

	// Initially not paused
	assert.False(t, handler.paused)

	// Simulate disconnect
	handler.paused = true
	assert.True(t, handler.paused)

	// Simulate reconnect
	handler.paused = false
	assert.False(t, handler.paused)
}
