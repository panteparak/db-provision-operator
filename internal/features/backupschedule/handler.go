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
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for backup schedule operations.
type Handler struct {
	repo       RepositoryInterface
	eventBus   eventbus.Bus
	logger     logr.Logger
	cronParser cron.Parser
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository RepositoryInterface
	EventBus   eventbus.Bus
	Logger     logr.Logger
}

// NewHandler creates a new backupschedule handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:       cfg.Repository,
		eventBus:   cfg.EventBus,
		logger:     cfg.Logger,
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// TriggerBackup triggers a backup from the schedule template.
func (h *Handler) TriggerBackup(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*TriggerResult, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	backup, err := h.repo.CreateBackupFromTemplate(ctx, schedule)
	if err != nil {
		metrics.RecordScheduledBackup(schedule.Namespace, metrics.StatusFailure)
		return &TriggerResult{
			Triggered: false,
			Message:   fmt.Sprintf("Failed to create backup: %v", err),
		}, err
	}

	log.Info("Triggered scheduled backup", "backup", backup.Name)
	metrics.RecordScheduledBackup(schedule.Namespace, metrics.StatusSuccess)

	// Publish event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, NewScheduledBackupTriggered(
			schedule.Name,
			backup.Name,
			schedule.Spec.Template.Spec.DatabaseRef.Name,
			schedule.Namespace,
		))
	}

	return &TriggerResult{
		Triggered:  true,
		BackupName: backup.Name,
		Message:    "Backup triggered successfully",
	}, nil
}

// EvaluateSchedule evaluates whether a backup is due based on the cron schedule.
func (h *Handler) EvaluateSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*ScheduleEvaluation, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	// Parse timezone
	loc, err := h.getTimezone(schedule.Spec.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %w", err)
	}

	// Parse cron schedule
	cronSchedule, err := h.cronParser.Parse(schedule.Spec.Schedule)
	if err != nil {
		return nil, fmt.Errorf("invalid cron schedule: %w", err)
	}

	now := time.Now().In(loc)
	nextTime := cronSchedule.Next(now)

	// Get backups for this schedule
	backups, err := h.repo.ListBackupsForSchedule(ctx, schedule)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	// Check for running backups
	runningBackups := h.repo.FilterBackupsByPhase(backups, dbopsv1alpha1.PhaseRunning)

	// Determine last backup time
	var lastBackupTime *time.Time
	if schedule.Status.LastBackup != nil && schedule.Status.LastBackup.StartedAt != nil {
		t := schedule.Status.LastBackup.StartedAt.Time
		lastBackupTime = &t
	}

	// Determine if backup is due
	var backupDue bool
	if lastBackupTime != nil {
		expectedRunTime := cronSchedule.Next(lastBackupTime.In(loc))
		backupDue = now.After(expectedRunTime) || now.Equal(expectedRunTime)
	} else if len(backups) == 0 {
		// For new schedules with no backups, check if we've passed the first scheduled time
		// Don't trigger immediately, wait for the scheduled time
		backupDue = false
	}

	log.V(1).Info("Evaluated schedule",
		"backupDue", backupDue,
		"nextTime", nextTime,
		"runningBackups", len(runningBackups))

	return &ScheduleEvaluation{
		BackupDue:      backupDue,
		NextBackupTime: nextTime.UTC(),
		LastBackupTime: lastBackupTime,
		RunningBackups: len(runningBackups),
		Message:        "Schedule evaluated",
	}, nil
}

// EnforceRetention enforces the retention policy by deleting old backups.
func (h *Handler) EnforceRetention(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*RetentionResult, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	if schedule.Spec.Retention == nil {
		return &RetentionResult{
			DeletedCount: 0,
			Message:      "No retention policy configured",
		}, nil
	}

	backups, err := h.repo.ListBackupsForSchedule(ctx, schedule)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	// Get completed backups sorted by time (newest first)
	completedBackups := h.repo.GetCompletedBackupsSortedByTime(backups)

	var toDelete []dbopsv1alpha1.DatabaseBackup

	// Apply KeepLast retention
	if schedule.Spec.Retention.KeepLast > 0 && int32(len(completedBackups)) > schedule.Spec.Retention.KeepLast {
		excess := completedBackups[schedule.Spec.Retention.KeepLast:]
		for _, backup := range excess {
			log.Info("Marking backup for deletion due to keepLast policy", "backup", backup.Name)
			toDelete = append(toDelete, backup)
		}
	}

	// Apply KeepDaily retention (delete backups older than N days)
	if schedule.Spec.Retention.KeepDaily > 0 {
		cutoff := time.Now().AddDate(0, 0, -int(schedule.Spec.Retention.KeepDaily))
		for _, backup := range completedBackups {
			if backup.CreationTimestamp.Before(&metav1.Time{Time: cutoff}) {
				// Check if already in toDelete
				found := false
				for _, d := range toDelete {
					if d.Name == backup.Name {
						found = true
						break
					}
				}
				if !found {
					log.Info("Marking backup for deletion due to keepDaily policy", "backup", backup.Name)
					toDelete = append(toDelete, backup)
				}
			}
		}
	}

	// Delete marked backups
	deletedNames, err := h.repo.DeleteBackups(ctx, toDelete)
	if err != nil {
		log.Error(err, "Failed to delete some backups during retention enforcement")
	}

	return &RetentionResult{
		DeletedCount: len(deletedNames),
		DeletedNames: deletedNames,
		Message:      fmt.Sprintf("Deleted %d backups due to retention policy", len(deletedNames)),
	}, nil
}

// HandleConcurrency handles concurrency policy for running backups.
func (h *Handler) HandleConcurrency(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*ConcurrencyResult, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	backups, err := h.repo.ListBackupsForSchedule(ctx, schedule)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	runningBackups := h.repo.FilterBackupsByPhase(backups, dbopsv1alpha1.PhaseRunning)

	if len(runningBackups) == 0 {
		return &ConcurrencyResult{
			ShouldProceed: true,
			Message:       "No running backups",
		}, nil
	}

	switch schedule.Spec.ConcurrencyPolicy {
	case dbopsv1alpha1.ConcurrencyPolicyForbid:
		log.Info("Skipping backup due to concurrency policy Forbid",
			"runningBackup", runningBackups[0].Name)
		return &ConcurrencyResult{
			ShouldProceed: false,
			Message:       fmt.Sprintf("Backup %s is still running (Forbid policy)", runningBackups[0].Name),
		}, nil

	case dbopsv1alpha1.ConcurrencyPolicyReplace:
		log.Info("Cancelling running backups due to concurrency policy Replace",
			"count", len(runningBackups))
		deletedNames, err := h.repo.DeleteBackups(ctx, runningBackups)
		if err != nil {
			log.Error(err, "Failed to delete some running backups")
		}
		return &ConcurrencyResult{
			ShouldProceed:  true,
			CancelledCount: len(deletedNames),
			CancelledNames: deletedNames,
			Message:        fmt.Sprintf("Cancelled %d running backups (Replace policy)", len(deletedNames)),
		}, nil

	case dbopsv1alpha1.ConcurrencyPolicyAllow:
		fallthrough
	default:
		// Allow concurrent backups
		return &ConcurrencyResult{
			ShouldProceed: true,
			Message:       "Allowing concurrent backup (Allow policy)",
		}, nil
	}
}

// UpdateStatistics updates backup statistics based on completed backups.
func (h *Handler) UpdateStatistics(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.BackupStatistics, error) {
	backups, err := h.repo.ListBackupsForSchedule(ctx, schedule)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	stats := &dbopsv1alpha1.BackupStatistics{}

	var totalDuration, totalSize int64

	for _, backup := range backups {
		switch backup.Status.Phase {
		case dbopsv1alpha1.PhaseCompleted:
			stats.SuccessfulBackups++
			if backup.Status.StartedAt != nil && backup.Status.CompletedAt != nil {
				duration := backup.Status.CompletedAt.Sub(backup.Status.StartedAt.Time)
				totalDuration += int64(duration.Seconds())
			}
			if backup.Status.Backup != nil {
				totalSize += backup.Status.Backup.SizeBytes
			}
		case dbopsv1alpha1.PhaseFailed:
			stats.FailedBackups++
		}
	}

	stats.TotalBackups = stats.SuccessfulBackups + stats.FailedBackups
	stats.TotalStorageBytes = totalSize

	if stats.SuccessfulBackups > 0 {
		stats.AverageDurationSeconds = totalDuration / stats.SuccessfulBackups
		stats.AverageSizeBytes = totalSize / stats.SuccessfulBackups
	}

	return stats, nil
}

// GetRecentBackupsList builds the recent backups list for status.
func (h *Handler) GetRecentBackupsList(schedule *dbopsv1alpha1.DatabaseBackupSchedule, backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.RecentBackupInfo {
	sorted := h.repo.GetBackupsSortedByTime(backups)

	// Calculate limits
	successLimit := schedule.Spec.SuccessfulBackupsHistoryLimit
	if successLimit == 0 {
		successLimit = 5
	}
	failedLimit := schedule.Spec.FailedBackupsHistoryLimit
	if failedLimit == 0 {
		failedLimit = 3
	}

	var recentBackups []dbopsv1alpha1.RecentBackupInfo
	successCount := int32(0)
	failedCount := int32(0)

	for _, backup := range sorted {
		include := false

		switch backup.Status.Phase {
		case dbopsv1alpha1.PhaseCompleted:
			if successCount < successLimit {
				include = true
				successCount++
			}
		case dbopsv1alpha1.PhaseFailed:
			if failedCount < failedLimit {
				include = true
				failedCount++
			}
		case dbopsv1alpha1.PhaseRunning, dbopsv1alpha1.PhasePending:
			// Always include running/pending backups
			include = true
		}

		if include {
			recentBackups = append(recentBackups, dbopsv1alpha1.RecentBackupInfo{
				Name:   backup.Name,
				Status: string(backup.Status.Phase),
			})
		}
	}

	return recentBackups
}

// UpdateLastBackupStatus updates the last backup status from the latest backup.
func (h *Handler) UpdateLastBackupStatus(schedule *dbopsv1alpha1.DatabaseBackupSchedule, backups []dbopsv1alpha1.DatabaseBackup) {
	latest := h.repo.GetLatestBackup(backups)
	if latest == nil || schedule.Status.LastBackup == nil {
		return
	}

	schedule.Status.LastBackup.Status = string(latest.Status.Phase)
	if latest.Status.CompletedAt != nil {
		schedule.Status.LastBackup.CompletedAt = latest.Status.CompletedAt
	}
}

// OnBackupCompleted handles the BackupCompleted event.
func (h *Handler) OnBackupCompleted(ctx context.Context, event *eventbus.BackupCompleted) error {
	logf.FromContext(ctx).V(1).Info("Backup completed, schedule statistics may need update",
		"backup", event.BackupName,
		"namespace", event.Namespace,
		"size", event.SizeBytes,
		"duration", event.Duration)
	// Statistics will be updated during reconciliation
	return nil
}

// OnBackupFailed handles the BackupFailed event.
func (h *Handler) OnBackupFailed(ctx context.Context, event *eventbus.BackupFailed) error {
	logf.FromContext(ctx).V(1).Info("Backup failed, schedule statistics may need update",
		"backup", event.BackupName,
		"namespace", event.Namespace,
		"error", event.Error)
	// Statistics will be updated during reconciliation
	return nil
}

// getTimezone parses and returns the timezone location.
func (h *Handler) getTimezone(tz string) (*time.Location, error) {
	if tz == "" || tz == "UTC" {
		return time.UTC, nil
	}
	return time.LoadLocation(tz)
}

// UpdateInfoMetric updates the info metric for a backup schedule.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(schedule *dbopsv1alpha1.DatabaseBackupSchedule) {
	databaseRef := schedule.Spec.Template.Spec.DatabaseRef.Name
	cron := schedule.Spec.Schedule
	paused := "false"
	if schedule.Spec.Paused {
		paused = "true"
	}

	metrics.SetScheduleInfo(
		schedule.Name,
		schedule.Namespace,
		databaseRef,
		cron,
		paused,
	)
}

// CleanupInfoMetric removes the info metric for a deleted schedule.
func (h *Handler) CleanupInfoMetric(schedule *dbopsv1alpha1.DatabaseBackupSchedule) {
	metrics.DeleteScheduleInfo(schedule.Name, schedule.Namespace)
}

// ListBackupsForSchedule lists all backups created by this schedule.
// This method is a wrapper around the repository method for use by the controller.
func (h *Handler) ListBackupsForSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
	return h.repo.ListBackupsForSchedule(ctx, schedule)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
