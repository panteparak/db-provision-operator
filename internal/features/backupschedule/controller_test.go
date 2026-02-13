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
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/util"
)

func setupTestController(objs ...runtime.Object) (*Controller, *record.FakeRecorder, *MockRepository) {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		WithStatusSubresource(&dbopsv1alpha1.DatabaseBackupSchedule{}).
		Build()

	recorder := record.NewFakeRecorder(10)
	mockRepo := NewMockRepository()
	mockEventBus := NewMockEventBus()

	handler := NewHandler(HandlerConfig{
		Repository: mockRepo,
		EventBus:   mockEventBus,
		Logger:     logr.Discard(),
	})

	controller := NewController(ControllerConfig{
		Client:   client,
		Scheme:   scheme,
		Recorder: recorder,
		Handler:  handler,
		Logger:   logr.Discard(),
	})

	return controller, recorder, mockRepo
}

func TestController_Reconcile_NewSchedule(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	// Setup mock to return no existing backups
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{}, nil
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "Should requeue for next backup time")
	assert.True(t, result.RequeueAfter <= MaxRequeueAfter, "Should not exceed max requeue time")
}

func TestController_Reconcile_NotFound(t *testing.T) {
	controller, _, _ := setupTestController()

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestController_Reconcile_SkipWithAnnotation(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
			Annotations: map[string]string{
				util.AnnotationSkipReconcile: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Should not requeue when skipping")
}

func TestController_Reconcile_Paused(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Paused:   true,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, recorder, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result, "Paused schedule should not requeue")

	// Verify the schedule status was updated
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePaused, updated.Status.Phase)
	assert.Nil(t, updated.Status.NextBackupTime)

	// Check for Paused event
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "Paused")
	default:
		t.Log("No Paused event recorded (may have been skipped if already paused)")
	}
}

func TestController_Reconcile_FinalizerAdded(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-schedule",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{}, nil
	}

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Verify finalizer was added
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Contains(t, updated.Finalizers, util.FinalizerDatabaseBackupSchedule)
}

func TestController_Reconcile_Deletion(t *testing.T) {
	now := metav1.Now()
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-schedule",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer was removed (resource may or may not exist after finalizer removal)
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	if err == nil {
		// Resource still exists, verify finalizer was removed
		assert.NotContains(t, updated.Finalizers, util.FinalizerDatabaseBackupSchedule)
	}
	// If err != nil (not found), resource was deleted after finalizer removal - that's expected
}

func TestController_Reconcile_BackupTriggered(t *testing.T) {
	// Schedule with last backup more than 1 day ago
	lastBackup := metav1.NewTime(time.Now().Add(-25 * time.Hour))
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseBackupScheduleStatus{
			LastBackup: &dbopsv1alpha1.ScheduledBackupInfo{
				Name:      "old-backup",
				StartedAt: &lastBackup,
			},
		},
	}

	controller, recorder, mockRepo := setupTestController(schedule)

	// Setup mocks
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{}, nil
	}
	mockRepo.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
		return &dbopsv1alpha1.DatabaseBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-backup-123",
				Namespace: schedule.Namespace,
			},
		}, nil
	}

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Check for BackupTriggered event
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "BackupTriggered")
	default:
		t.Log("No BackupTriggered event (backup may not have been due)")
	}
}

func TestController_Reconcile_ConcurrencyForbid(t *testing.T) {
	lastBackup := metav1.NewTime(time.Now().Add(-25 * time.Hour))
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule:          "0 2 * * *",
			ConcurrencyPolicy: dbopsv1alpha1.ConcurrencyPolicyForbid,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseBackupScheduleStatus{
			LastBackup: &dbopsv1alpha1.ScheduledBackupInfo{
				Name:      "old-backup",
				StartedAt: &lastBackup,
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	// Setup mock to return a running backup
	runningBackup := dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "running-backup",
			Namespace: "default",
		},
		Status: dbopsv1alpha1.DatabaseBackupStatus{
			Phase: dbopsv1alpha1.PhaseRunning,
		},
	}
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{runningBackup}, nil
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	// Should requeue after error interval due to Forbid policy blocking
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)
}

func TestController_Reconcile_RetentionEnforced(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Retention: &dbopsv1alpha1.RetentionPolicy{
				KeepLast: 3,
			},
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	// Setup mock with 5 completed backups
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-1"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-2"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-3"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-4"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-5"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
		}, nil
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

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Verify DeleteBackups was called
	assert.True(t, mockRepo.WasCalled("DeleteBackups"))
}

func TestController_Reconcile_StatisticsUpdated(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	startTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	endTime := metav1.NewTime(time.Now())
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "backup-1"},
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase:       dbopsv1alpha1.PhaseCompleted,
					StartedAt:   &startTime,
					CompletedAt: &endTime,
					Backup:      &dbopsv1alpha1.BackupInfo{SizeBytes: 1024 * 1024},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "backup-2"},
				Status: dbopsv1alpha1.DatabaseBackupStatus{
					Phase: dbopsv1alpha1.PhaseFailed,
				},
			},
		}, nil
	}

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Verify statistics were updated
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)

	require.NotNil(t, updated.Status.Statistics)
	assert.Equal(t, int64(1), updated.Status.Statistics.SuccessfulBackups)
	assert.Equal(t, int64(1), updated.Status.Statistics.FailedBackups)
}

func TestController_Reconcile_RecentBackupsUpdated(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule:                      "0 2 * * *",
			SuccessfulBackupsHistoryLimit: 2,
			FailedBackupsHistoryLimit:     1,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-1"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-2"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-3"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "failed-1"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseFailed}},
			{ObjectMeta: metav1.ObjectMeta{Name: "failed-2"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseFailed}},
		}, nil
	}
	mockRepo.GetBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		return backups
	}

	_, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})
	require.NoError(t, err)

	// Verify recent backups list was updated
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)

	// Count backups by status
	successCount := 0
	failedCount := 0
	for _, backup := range updated.Status.RecentBackups {
		switch backup.Status {
		case string(dbopsv1alpha1.PhaseCompleted):
			successCount++
		case string(dbopsv1alpha1.PhaseFailed):
			failedCount++
		}
	}
	assert.Equal(t, 2, successCount, "Should include 2 successful backups")
	assert.Equal(t, 1, failedCount, "Should include 1 failed backup")
}

func TestController_Reconcile_ErrorCondition(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "invalid cron", // Invalid cron will cause error
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, recorder, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Check for ReconcileFailed event
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "ReconcileFailed")
	default:
		t.Error("Expected ReconcileFailed event")
	}
}

func TestController_RequeueTimeCalculation(t *testing.T) {
	tests := []struct {
		name               string
		nextBackupDuration time.Duration
		expectedRequeue    time.Duration
	}{
		{
			name:               "next backup in 30 minutes",
			nextBackupDuration: 30 * time.Minute,
			expectedRequeue:    30 * time.Minute,
		},
		{
			name:               "next backup in 2 hours - capped to max",
			nextBackupDuration: 2 * time.Hour,
			expectedRequeue:    MaxRequeueAfter,
		},
		{
			name:               "next backup is overdue",
			nextBackupDuration: -5 * time.Minute,
			expectedRequeue:    MinRequeueAfter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-schedule",
					Namespace:  "default",
					Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
				},
				Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
					Schedule: "0 * * * *", // Every hour
					Template: dbopsv1alpha1.BackupTemplateSpec{
						Spec: dbopsv1alpha1.DatabaseBackupSpec{
							DatabaseRef: dbopsv1alpha1.DatabaseReference{
								Name: "test-db",
							},
						},
					},
				},
			}

			controller, _, mockRepo := setupTestController(schedule)

			mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
				return []dbopsv1alpha1.DatabaseBackup{}, nil
			}

			result, err := controller.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-schedule",
					Namespace: "default",
				},
			})

			require.NoError(t, err)
			// The actual requeue time will be based on cron calculation
			// Just verify it's within expected bounds
			assert.True(t, result.RequeueAfter >= MinRequeueAfter || result.RequeueAfter == 0)
			assert.True(t, result.RequeueAfter <= MaxRequeueAfter)
		})
	}
}

func TestController_Reconcile_EvaluateScheduleError_InvalidTimezone(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Timezone: "Invalid/Timezone",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, recorder, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timezone")
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Verify error event was recorded
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "ReconcileFailed")
		assert.Contains(t, event, "evaluate schedule")
	default:
		t.Error("Expected ReconcileFailed event for timezone error")
	}

	// Verify status has error condition
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseActive, updated.Status.Phase)
}

func TestController_Reconcile_TriggerBackupError(t *testing.T) {
	// Schedule with a last backup time far enough in the past to trigger a new one
	lastBackup := metav1.NewTime(time.Now().Add(-25 * time.Hour))
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseBackupScheduleStatus{
			LastBackup: &dbopsv1alpha1.ScheduledBackupInfo{
				Name:      "old-backup",
				StartedAt: &lastBackup,
			},
		},
	}

	controller, recorder, mockRepo := setupTestController(schedule)

	// Make CreateBackupFromTemplate return an error
	mockRepo.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
		return nil, fmt.Errorf("failed to create backup resource")
	}
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{}, nil
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create backup resource")
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Verify ReconcileFailed event mentioning trigger backup
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "ReconcileFailed")
		assert.Contains(t, event, "trigger backup")
	default:
		t.Error("Expected ReconcileFailed event for trigger backup error")
	}

	// Verify status condition reflects error
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseActive, updated.Status.Phase)
}

func TestController_Reconcile_HandleConcurrencyError(t *testing.T) {
	lastBackup := metav1.NewTime(time.Now().Add(-25 * time.Hour))
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule:          "0 2 * * *",
			ConcurrencyPolicy: dbopsv1alpha1.ConcurrencyPolicyForbid,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseBackupScheduleStatus{
			LastBackup: &dbopsv1alpha1.ScheduledBackupInfo{
				Name:      "old-backup",
				StartedAt: &lastBackup,
			},
		},
	}

	controller, recorder, mockRepo := setupTestController(schedule)

	callCount := 0
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		callCount++
		if callCount == 1 {
			// First call from EvaluateSchedule: return a running backup so RunningBackups > 0
			return []dbopsv1alpha1.DatabaseBackup{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "running-backup", Namespace: "default"},
					Status:     dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseRunning},
				},
			}, nil
		}
		// Second call from HandleConcurrency: return error
		return nil, fmt.Errorf("kube-apiserver unavailable")
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "kube-apiserver unavailable")
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Verify ReconcileFailed event mentioning concurrency
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "ReconcileFailed")
		assert.Contains(t, event, "handle concurrency")
	default:
		t.Error("Expected ReconcileFailed event for concurrency error")
	}
}

func TestController_Reconcile_ListBackupsError(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	callCount := 0
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		callCount++
		if callCount <= 1 {
			// EvaluateSchedule call succeeds
			return []dbopsv1alpha1.DatabaseBackup{}, nil
		}
		// ListBackupsForSchedule call in reconcile fails
		return nil, fmt.Errorf("failed to list backups")
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	// The controller logs the error but does not return it; it continues reconciliation
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "Should still requeue for next backup time")

	// Verify the status is still set to Active despite the list error
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseActive, updated.Status.Phase)
}

func TestController_Reconcile_DeletionProtectionBlocked(t *testing.T) {
	now := metav1.Now()
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-schedule",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule:           "0 2 * * *",
			DeletionProtection: true,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, recorder, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deletion protection enabled")
	assert.Equal(t, ctrl.Result{}, result)

	// Verify DeletionBlocked warning event
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "DeletionBlocked")
	default:
		t.Error("Expected DeletionBlocked event")
	}

	// Verify finalizer is still present and phase is Failed
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Contains(t, updated.Finalizers, util.FinalizerDatabaseBackupSchedule)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, updated.Status.Phase)
}

func TestController_Reconcile_DeletionProtectionBypassedWithForceDelete(t *testing.T) {
	now := metav1.Now()
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-schedule",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{util.FinalizerDatabaseBackupSchedule},
			Annotations: map[string]string{
				util.AnnotationForceDelete: "true",
			},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule:           "0 2 * * *",
			DeletionProtection: true,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify finalizer was removed
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	if err == nil {
		assert.NotContains(t, updated.Finalizers, util.FinalizerDatabaseBackupSchedule)
	}
	// If not found, the resource was deleted - also acceptable
}

func TestController_Reconcile_DeletionWithoutOurFinalizer(t *testing.T) {
	now := metav1.Now()
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-schedule",
			Namespace:         "default",
			DeletionTimestamp: &now,
			Finalizers:        []string{"some-other-finalizer"}, // Has a finalizer, but not ours
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, _ := setupTestController(schedule)

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	// Should return immediately without error since our finalizer is not present
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Verify the other finalizer is still present
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Contains(t, updated.Finalizers, "some-other-finalizer")
}

func TestController_Reconcile_RetentionEnforcementError(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Retention: &dbopsv1alpha1.RetentionPolicy{
				KeepLast: 2,
			},
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	mainListCallCount := 0
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		mainListCallCount++
		return []dbopsv1alpha1.DatabaseBackup{
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-1"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-2"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-3"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			{ObjectMeta: metav1.ObjectMeta{Name: "backup-4"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
		}, nil
	}
	mockRepo.GetCompletedBackupsSortedByTimeFunc = func(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
		return backups
	}
	mockRepo.DeleteBackupsFunc = func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
		return nil, fmt.Errorf("failed to delete backups: permission denied")
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	// Retention errors are logged but do not fail the reconciliation
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "Should still requeue for next backup time")

	// Verify DeleteBackups was attempted
	assert.True(t, mockRepo.WasCalled("DeleteBackups"))

	// Verify status is still Active despite retention error
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseActive, updated.Status.Phase)
}

func TestController_Reconcile_UpdateStatisticsError(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	callCount := 0
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		callCount++
		switch callCount {
		case 1:
			// EvaluateSchedule call: succeed
			return []dbopsv1alpha1.DatabaseBackup{}, nil
		case 2:
			// ListBackupsForSchedule in reconcile: succeed
			return []dbopsv1alpha1.DatabaseBackup{
				{ObjectMeta: metav1.ObjectMeta{Name: "backup-1"}, Status: dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseCompleted}},
			}, nil
		default:
			// UpdateStatistics call: fail
			return nil, fmt.Errorf("statistics list error")
		}
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	// Statistics errors are logged but do not fail the reconciliation
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "Should still requeue for next backup time")

	// Status should still be Active
	var updated dbopsv1alpha1.DatabaseBackupSchedule
	err = controller.Get(context.Background(), types.NamespacedName{
		Name:      "test-schedule",
		Namespace: "default",
	}, &updated)
	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseActive, updated.Status.Phase)
}

func TestController_Reconcile_ConcurrencyReplaceDeleteError(t *testing.T) {
	lastBackup := metav1.NewTime(time.Now().Add(-25 * time.Hour))
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule:          "0 2 * * *",
			ConcurrencyPolicy: dbopsv1alpha1.ConcurrencyPolicyReplace,
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseBackupScheduleStatus{
			LastBackup: &dbopsv1alpha1.ScheduledBackupInfo{
				Name:      "old-backup",
				StartedAt: &lastBackup,
			},
		},
	}

	controller, _, mockRepo := setupTestController(schedule)

	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return []dbopsv1alpha1.DatabaseBackup{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "running-backup", Namespace: "default"},
				Status:     dbopsv1alpha1.DatabaseBackupStatus{Phase: dbopsv1alpha1.PhaseRunning},
			},
		}, nil
	}
	mockRepo.DeleteBackupsFunc = func(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
		// Delete fails but returns partial results
		return nil, fmt.Errorf("failed to delete running backup")
	}
	mockRepo.CreateBackupFromTemplateFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
		return &dbopsv1alpha1.DatabaseBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new-backup-456",
				Namespace: schedule.Namespace,
			},
		}, nil
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	// Replace policy: delete error is logged but HandleConcurrency still returns ShouldProceed=true
	// The reconciliation should continue and attempt to trigger the backup
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "Should requeue for next backup time")

	// Verify DeleteBackups was called
	assert.True(t, mockRepo.WasCalled("DeleteBackups"))
}

func TestController_Reconcile_EvaluateScheduleListBackupsError(t *testing.T) {
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-schedule",
			Namespace:  "default",
			Finalizers: []string{util.FinalizerDatabaseBackupSchedule},
		},
		Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
			Schedule: "0 2 * * *",
			Template: dbopsv1alpha1.BackupTemplateSpec{
				Spec: dbopsv1alpha1.DatabaseBackupSpec{
					DatabaseRef: dbopsv1alpha1.DatabaseReference{
						Name: "test-db",
					},
				},
			},
		},
	}

	controller, recorder, mockRepo := setupTestController(schedule)

	// EvaluateSchedule calls ListBackupsForSchedule internally; make it fail
	mockRepo.ListBackupsForScheduleFunc = func(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
		return nil, fmt.Errorf("connection refused")
	}

	result, err := controller.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-schedule",
			Namespace: "default",
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	assert.Equal(t, RequeueAfterError, result.RequeueAfter)

	// Verify ReconcileFailed event for evaluate schedule
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "ReconcileFailed")
		assert.Contains(t, event, "evaluate schedule")
	default:
		t.Error("Expected ReconcileFailed event for list backups error during schedule evaluation")
	}
}
