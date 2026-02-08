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
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

func newTestSchedule(name, namespace string) *dbopsv1alpha1.DatabaseBackupSchedule {
	return &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *", // 2 AM daily
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}
}

func newTestBackup(name, namespace string, phase dbopsv1alpha1.Phase) dbopsv1alpha1.DatabaseBackup {
	return dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.Now(),
		},
		Status: dbopsv1alpha1.DatabaseBackupStatus{
			Phase: phase,
		},
	}
}

func TestHandler_TriggerBackup(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*MockRepository)
		wantErr   bool
		triggered bool
	}{
		{
			name: "successful trigger",
			setupMock: func(m *MockRepository) {
				m.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
					return &dbopsv1alpha1.DatabaseBackup{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-backup-123",
							Namespace: schedule.Namespace,
						},
					}, nil
				}
			},
			wantErr:   false,
			triggered: true,
		},
		{
			name: "create backup failed",
			setupMock: func(m *MockRepository) {
				m.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
					return nil, errors.New("failed to create backup")
				}
			},
			wantErr:   true,
			triggered: false,
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

			schedule := newTestSchedule("test-schedule", "default")
			result, err := handler.TriggerBackup(context.Background(), schedule)

			if tt.wantErr {
				require.Error(t, err)
				assert.False(t, result.Triggered)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.triggered, result.Triggered)
			if tt.triggered {
				assert.NotEmpty(t, result.BackupName)
			}
		})
	}
}

func TestHandler_TriggerBackup_PublishesEvent(t *testing.T) {
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	mockRepo.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
		return &dbopsv1alpha1.DatabaseBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-backup-123",
				Namespace: schedule.Namespace,
			},
		}, nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	_, err := handler.TriggerBackup(context.Background(), schedule)
	require.NoError(t, err)

	// Verify event was published
	found := false
	for _, event := range mockEventBus.PublishedEvents {
		if event.EventName() == EventScheduledBackupTriggered {
			found = true
			break
		}
	}
	assert.True(t, found, "Expected ScheduledBackupTriggered event to be published")
}

func TestHandler_EvaluateSchedule(t *testing.T) {
	tests := []struct {
		name          string
		schedule      *dbopsv1alpha1.DatabaseBackupSchedule
		setupMock     func(*MockRepository)
		wantErr       bool
		wantBackupDue bool
	}{
		{
			name:     "new schedule no backups yet",
			schedule: newTestSchedule("test-schedule", "default"),
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{}, nil
				}
			},
			wantErr:       false,
			wantBackupDue: false, // New schedules wait for first scheduled time
		},
		{
			name: "backup is due",
			schedule: func() *dbopsv1alpha1.DatabaseBackupSchedule {
				s := newTestSchedule("test-schedule", "default")
				// Set last backup to more than 1 day ago
				lastBackup := metav1.NewTime(time.Now().Add(-25 * time.Hour))
				s.Status.LastBackup = &dbopsv1alpha1.ScheduledBackupInfo{
					Name:      "last-backup",
					StartedAt: &lastBackup,
				}
				return s
			}(),
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{
						newTestBackup("old-backup", "default", dbopsv1alpha1.PhaseCompleted),
					}, nil
				}
			},
			wantErr:       false,
			wantBackupDue: true,
		},
		{
			name: "invalid cron schedule",
			schedule: func() *dbopsv1alpha1.DatabaseBackupSchedule {
				s := newTestSchedule("test-schedule", "default")
				s.Spec.Schedule = "invalid cron"
				return s
			}(),
			setupMock: func(m *MockRepository) {},
			wantErr:   true,
		},
		{
			name: "invalid timezone",
			schedule: func() *dbopsv1alpha1.DatabaseBackupSchedule {
				s := newTestSchedule("test-schedule", "default")
				s.Spec.Timezone = "Invalid/Timezone"
				return s
			}(),
			setupMock: func(m *MockRepository) {},
			wantErr:   true,
		},
		{
			name:     "list backups failed",
			schedule: newTestSchedule("test-schedule", "default"),
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return nil, errors.New("failed to list")
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

			result, err := handler.EvaluateSchedule(context.Background(), tt.schedule)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantBackupDue, result.BackupDue)
			assert.False(t, result.NextBackupTime.IsZero())
		})
	}
}

func TestHandler_EvaluateSchedule_RunningBackups(t *testing.T) {
	mockRepo := NewMockRepository()

	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{
			newTestBackup("running-backup", "default", dbopsv1alpha1.PhaseRunning),
		}, nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	result, err := handler.EvaluateSchedule(context.Background(), schedule)

	require.NoError(t, err)
	assert.Equal(t, 1, result.RunningBackups)
}

func TestHandler_EnforceRetention(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*MockRepository)
		retention   *dbopsv1alpha1.RetentionPolicy
		wantErr     bool
		wantDeleted int
	}{
		{
			name:        "no retention policy",
			setupMock:   func(m *MockRepository) {},
			retention:   nil,
			wantErr:     false,
			wantDeleted: 0,
		},
		{
			name: "keepLast retention",
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{
						newTestBackup("backup-1", "default", dbopsv1alpha1.PhaseCompleted),
						newTestBackup("backup-2", "default", dbopsv1alpha1.PhaseCompleted),
						newTestBackup("backup-3", "default", dbopsv1alpha1.PhaseCompleted),
						newTestBackup("backup-4", "default", dbopsv1alpha1.PhaseCompleted),
						newTestBackup("backup-5", "default", dbopsv1alpha1.PhaseCompleted),
					}, nil
				}
				m.GetCompletedBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
					// Return all completed backups sorted (newest first)
					var completed []dbopsv1alpha1.DatabaseBackup
					for _, b := range backups {
						if b.Status.Phase == dbopsv1alpha1.PhaseCompleted {
							completed = append(completed, b)
						}
					}
					return completed
				}
				m.DeleteBackupsFunc = func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
					var names []string
					for _, b := range backups {
						names = append(names, b.Name)
					}
					return names, nil
				}
			},
			retention: &dbopsv1alpha1.RetentionPolicy{
				KeepLast: 3,
			},
			wantErr:     false,
			wantDeleted: 2, // 5 backups - 3 to keep = 2 deleted
		},
		{
			name: "list backups failed",
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return nil, errors.New("failed to list")
				}
			},
			retention: &dbopsv1alpha1.RetentionPolicy{
				KeepLast: 3,
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

			schedule := newTestSchedule("test-schedule", "default")
			schedule.Spec.Retention = tt.retention

			result, err := handler.EnforceRetention(context.Background(), schedule)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantDeleted, result.DeletedCount)
		})
	}
}

func TestHandler_EnforceRetention_KeepDaily(t *testing.T) {
	mockRepo := NewMockRepository()

	oldBackup := newTestBackup("old-backup", "default", dbopsv1alpha1.PhaseCompleted)
	oldBackup.CreationTimestamp = metav1.NewTime(time.Now().AddDate(0, 0, -10)) // 10 days ago

	newBackup := newTestBackup("new-backup", "default", dbopsv1alpha1.PhaseCompleted)
	newBackup.CreationTimestamp = metav1.NewTime(time.Now().Add(-1 * time.Hour)) // 1 hour ago

	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{newBackup, oldBackup}, nil
	}
	mockRepo.GetCompletedBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		return backups
	}
	mockRepo.DeleteBackupsFunc = func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
		var names []string
		for _, b := range backups {
			names = append(names, b.Name)
		}
		return names, nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	schedule.Spec.Retention = &dbopsv1alpha1.RetentionPolicy{
		KeepDaily: 7, // Keep backups from last 7 days
	}

	result, err := handler.EnforceRetention(context.Background(), schedule)

	require.NoError(t, err)
	assert.Equal(t, 1, result.DeletedCount) // Old backup should be deleted
	assert.Contains(t, result.DeletedNames, "old-backup")
}

func TestHandler_HandleConcurrency(t *testing.T) {
	tests := []struct {
		name          string
		policy        dbopsv1alpha1.ConcurrencyPolicy
		setupMock     func(*MockRepository)
		wantProceed   bool
		wantCancelled int
	}{
		{
			name:   "no running backups",
			policy: dbopsv1alpha1.ConcurrencyPolicyForbid,
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{}, nil
				}
			},
			wantProceed:   true,
			wantCancelled: 0,
		},
		{
			name:   "forbid policy with running backup",
			policy: dbopsv1alpha1.ConcurrencyPolicyForbid,
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{
						newTestBackup("running-backup", "default", dbopsv1alpha1.PhaseRunning),
					}, nil
				}
			},
			wantProceed:   false,
			wantCancelled: 0,
		},
		{
			name:   "replace policy with running backup",
			policy: dbopsv1alpha1.ConcurrencyPolicyReplace,
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{
						newTestBackup("running-backup", "default", dbopsv1alpha1.PhaseRunning),
					}, nil
				}
				m.DeleteBackupsFunc = func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
					return []string{"running-backup"}, nil
				}
			},
			wantProceed:   true,
			wantCancelled: 1,
		},
		{
			name:   "allow policy with running backup",
			policy: dbopsv1alpha1.ConcurrencyPolicyAllow,
			setupMock: func(m *MockRepository) {
				m.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
					return []dbopsv1alpha1.DatabaseBackup{
						newTestBackup("running-backup", "default", dbopsv1alpha1.PhaseRunning),
					}, nil
				}
			},
			wantProceed:   true,
			wantCancelled: 0,
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

			schedule := newTestSchedule("test-schedule", "default")
			schedule.Spec.ConcurrencyPolicy = tt.policy

			result, err := handler.HandleConcurrency(context.Background(), schedule)

			require.NoError(t, err)
			assert.Equal(t, tt.wantProceed, result.ShouldProceed)
			assert.Equal(t, tt.wantCancelled, result.CancelledCount)
		})
	}
}

func TestHandler_HandleConcurrency_ListError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return nil, errors.New("failed to list")
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	_, err := handler.HandleConcurrency(context.Background(), schedule)

	require.Error(t, err)
}

func TestHandler_UpdateStatistics(t *testing.T) {
	mockRepo := NewMockRepository()

	completedBackup := newTestBackup("completed-backup", "default", dbopsv1alpha1.PhaseCompleted)
	startTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	endTime := metav1.NewTime(time.Now())
	completedBackup.Status.StartedAt = &startTime
	completedBackup.Status.CompletedAt = &endTime
	completedBackup.Status.Backup = &dbopsv1alpha1.BackupInfo{
		SizeBytes: 1024 * 1024, // 1MB
	}

	failedBackup := newTestBackup("failed-backup", "default", dbopsv1alpha1.PhaseFailed)

	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{completedBackup, failedBackup}, nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	stats, err := handler.UpdateStatistics(context.Background(), schedule)

	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.SuccessfulBackups)
	assert.Equal(t, int64(1), stats.FailedBackups)
	assert.Equal(t, int64(2), stats.TotalBackups)
	assert.Equal(t, int64(1024*1024), stats.TotalStorageBytes)
}

func TestHandler_UpdateStatistics_ListError(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return nil, errors.New("failed to list")
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	_, err := handler.UpdateStatistics(context.Background(), schedule)

	require.Error(t, err)
}

func TestHandler_GetRecentBackupsList(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.GetBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		return backups
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	backups := []dbopsv1alpha1.DatabaseBackup{
		newTestBackup("completed-1", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-2", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-3", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-4", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-5", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-6", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("failed-1", "default", dbopsv1alpha1.PhaseFailed),
		newTestBackup("failed-2", "default", dbopsv1alpha1.PhaseFailed),
		newTestBackup("failed-3", "default", dbopsv1alpha1.PhaseFailed),
		newTestBackup("failed-4", "default", dbopsv1alpha1.PhaseFailed),
		newTestBackup("running-1", "default", dbopsv1alpha1.PhaseRunning),
	}

	schedule := newTestSchedule("test-schedule", "default")
	result := handler.GetRecentBackupsList(schedule, backups)

	// Default limits: 5 successful, 3 failed, all running/pending
	successCount := 0
	failedCount := 0
	runningCount := 0
	for _, info := range result {
		switch info.Status {
		case string(dbopsv1alpha1.PhaseCompleted):
			successCount++
		case string(dbopsv1alpha1.PhaseFailed):
			failedCount++
		case string(dbopsv1alpha1.PhaseRunning):
			runningCount++
		}
	}

	assert.Equal(t, 5, successCount, "Should include 5 successful backups")
	assert.Equal(t, 3, failedCount, "Should include 3 failed backups")
	assert.Equal(t, 1, runningCount, "Should include all running backups")
}

func TestHandler_GetRecentBackupsList_CustomLimits(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.GetBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		return backups
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	backups := []dbopsv1alpha1.DatabaseBackup{
		newTestBackup("completed-1", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-2", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("completed-3", "default", dbopsv1alpha1.PhaseCompleted),
		newTestBackup("failed-1", "default", dbopsv1alpha1.PhaseFailed),
		newTestBackup("failed-2", "default", dbopsv1alpha1.PhaseFailed),
	}

	schedule := newTestSchedule("test-schedule", "default")
	schedule.Spec.SuccessfulBackupsHistoryLimit = 2
	schedule.Spec.FailedBackupsHistoryLimit = 1

	result := handler.GetRecentBackupsList(schedule, backups)

	successCount := 0
	failedCount := 0
	for _, info := range result {
		switch info.Status {
		case string(dbopsv1alpha1.PhaseCompleted):
			successCount++
		case string(dbopsv1alpha1.PhaseFailed):
			failedCount++
		}
	}

	assert.Equal(t, 2, successCount, "Should include 2 successful backups (custom limit)")
	assert.Equal(t, 1, failedCount, "Should include 1 failed backup (custom limit)")
}

func TestHandler_UpdateLastBackupStatus(t *testing.T) {
	mockRepo := NewMockRepository()

	latestBackup := newTestBackup("latest-backup", "default", dbopsv1alpha1.PhaseCompleted)
	completedAt := metav1.NewTime(time.Now())
	latestBackup.Status.CompletedAt = &completedAt

	mockRepo.GetLatestBackupFunc = func(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup {
		if len(backups) == 0 {
			return nil
		}
		return &backups[0]
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	startedAt := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	schedule.Status.LastBackup = &dbopsv1alpha1.ScheduledBackupInfo{
		Name:      "latest-backup",
		Status:    string(dbopsv1alpha1.PhaseRunning),
		StartedAt: &startedAt,
	}

	handler.UpdateLastBackupStatus(schedule, []dbopsv1alpha1.DatabaseBackup{latestBackup})

	assert.Equal(t, string(dbopsv1alpha1.PhaseCompleted), schedule.Status.LastBackup.Status)
	assert.NotNil(t, schedule.Status.LastBackup.CompletedAt)
}

func TestHandler_UpdateLastBackupStatus_NoBackups(t *testing.T) {
	mockRepo := NewMockRepository()
	mockRepo.GetLatestBackupFunc = func(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup {
		return nil
	}

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	schedule := newTestSchedule("test-schedule", "default")
	schedule.Status.LastBackup = nil

	// Should not panic
	handler.UpdateLastBackupStatus(schedule, []dbopsv1alpha1.DatabaseBackup{})
}

func TestHandler_OnBackupCompleted(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Repository: NewMockRepository(),
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	event := &eventbus.BackupCompleted{
		BackupName: "test-backup",
		Namespace:  "default",
		SizeBytes:  1024,
		Duration:   10 * time.Minute,
	}

	err := handler.OnBackupCompleted(context.Background(), event)
	require.NoError(t, err)
}

func TestHandler_OnBackupFailed(t *testing.T) {
	handler := NewHandler(HandlerConfig{
		Repository: NewMockRepository(),
		EventBus:   NewMockEventBus(),
		Logger:     logr.Discard(),
	})

	event := &eventbus.BackupFailed{
		BackupName: "test-backup",
		Namespace:  "default",
		Error:      "backup failed",
	}

	err := handler.OnBackupFailed(context.Background(), event)
	require.NoError(t, err)
}

func TestHandler_TimezoneHandling(t *testing.T) {
	tests := []struct {
		name     string
		timezone string
		wantErr  bool
	}{
		{
			name:     "empty timezone defaults to UTC",
			timezone: "",
			wantErr:  false,
		},
		{
			name:     "explicit UTC",
			timezone: "UTC",
			wantErr:  false,
		},
		{
			name:     "America/New_York",
			timezone: "America/New_York",
			wantErr:  false,
		},
		{
			name:     "Asia/Tokyo",
			timezone: "Asia/Tokyo",
			wantErr:  false,
		},
		{
			name:     "invalid timezone",
			timezone: "Invalid/Timezone",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockRepository()
			mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
				return []dbopsv1alpha1.DatabaseBackup{}, nil
			}

			handler := NewHandler(HandlerConfig{
				Repository: mockRepo,
				EventBus:   NewMockEventBus(),
				Logger:     logr.Discard(),
			})

			schedule := newTestSchedule("test-schedule", "default")
			schedule.Spec.Timezone = tt.timezone

			_, err := handler.EvaluateSchedule(context.Background(), schedule)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
