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

package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseBackupScheduleReconciler reconciles a DatabaseBackupSchedule object
type DatabaseBackupScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackupschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackupschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackupschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseBackupScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseBackupSchedule resource
	var schedule dbopsv1alpha1.DatabaseBackupSchedule
	if err := r.Get(ctx, req.NamespacedName, &schedule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseBackupSchedule")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(&schedule) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(&schedule) {
		if controllerutil.ContainsFinalizer(&schedule, util.FinalizerDatabaseBackupSchedule) {
			if err := r.handleDeletion(ctx, &schedule); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&schedule, util.FinalizerDatabaseBackupSchedule)
			if err := r.Update(ctx, &schedule); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&schedule, util.FinalizerDatabaseBackupSchedule) {
		controllerutil.AddFinalizer(&schedule, util.FinalizerDatabaseBackupSchedule)
		if err := r.Update(ctx, &schedule); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle paused schedule
	if schedule.Spec.Paused {
		schedule.Status.Phase = dbopsv1alpha1.PhasePaused
		schedule.Status.NextBackupTime = nil
		util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionTrue, "Paused", "Schedule is paused")
		if err := r.Status().Update(ctx, &schedule); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	// Parse cron schedule with timezone
	loc, err := r.getTimezone(schedule.Spec.Timezone)
	if err != nil {
		schedule.Status.Phase = dbopsv1alpha1.PhaseActive // Keep Active but set error condition
		util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionFalse, "InvalidTimezone", fmt.Sprintf("Invalid timezone: %v", err))
		if statusErr := r.Status().Update(ctx, &schedule); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	cronSchedule, err := cronParser.Parse(schedule.Spec.Schedule)
	if err != nil {
		schedule.Status.Phase = dbopsv1alpha1.PhaseActive // Keep Active but set error condition
		util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionFalse, "InvalidSchedule", fmt.Sprintf("Invalid cron schedule: %v", err))
		if statusErr := r.Status().Update(ctx, &schedule); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	// Calculate next backup time
	now := time.Now().In(loc)
	nextTime := cronSchedule.Next(now)
	nextTimeUTC := nextTime.UTC()
	nextBackupTime := metav1.NewTime(nextTimeUTC)
	schedule.Status.NextBackupTime = &nextBackupTime

	// Record the next scheduled backup time metric
	databaseName := schedule.Spec.Template.Spec.DatabaseRef.Name
	metrics.SetScheduleNextBackup(databaseName, schedule.Namespace, float64(nextTimeUTC.Unix()))

	// Get backups created by this schedule
	backups, err := r.listBackupsForSchedule(ctx, &schedule)
	if err != nil {
		log.Error(err, "Failed to list backups for schedule")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Check for running backups
	runningBackups := r.filterBackupsByPhase(backups, dbopsv1alpha1.PhaseRunning)

	// Determine if backup is due
	var lastBackupTime time.Time
	if schedule.Status.LastBackup != nil && schedule.Status.LastBackup.StartedAt != nil {
		lastBackupTime = schedule.Status.LastBackup.StartedAt.Time
	}

	// Calculate the expected run time based on the schedule
	expectedRunTime := cronSchedule.Next(lastBackupTime.In(loc))

	backupDue := now.After(expectedRunTime) || now.Equal(expectedRunTime)

	// Also trigger if this is a new schedule with no backups
	if schedule.Status.LastBackup == nil && len(backups) == 0 {
		// For new schedules, wait for the first scheduled time
		backupDue = now.After(nextTime) || now.Equal(nextTime)
		if !backupDue {
			// First run - wait for scheduled time
			backupDue = false
		} else {
			backupDue = true
		}
	}

	// Handle concurrency policy if backup is due
	if backupDue && len(runningBackups) > 0 {
		switch schedule.Spec.ConcurrencyPolicy {
		case dbopsv1alpha1.ConcurrencyPolicyForbid:
			// Skip this run
			log.Info("Skipping backup due to concurrency policy Forbid - backup already running",
				"runningBackup", runningBackups[0].Name)
			schedule.Status.Phase = dbopsv1alpha1.PhaseActive
			util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionTrue, "WaitingForCompletion",
				fmt.Sprintf("Backup %s is still running", runningBackups[0].Name))
			if err := r.Status().Update(ctx, &schedule); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil

		case dbopsv1alpha1.ConcurrencyPolicyReplace:
			// Delete running backups
			for _, backup := range runningBackups {
				log.Info("Deleting running backup due to concurrency policy Replace", "backup", backup.Name)
				if err := r.Delete(ctx, &backup); err != nil && !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete running backup", "backup", backup.Name)
				}
			}
			// Proceed to create new backup
		}
		// ConcurrencyPolicyAllow: proceed to create backup even with running ones
	}

	// Create backup if due
	if backupDue {
		backup, err := r.createBackupFromTemplate(ctx, &schedule)
		if err != nil {
			log.Error(err, "Failed to create backup from template")
			metrics.RecordScheduledBackup(schedule.Namespace, metrics.StatusFailure)
			schedule.Status.Phase = dbopsv1alpha1.PhaseActive // Keep Active but set error condition
			util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionFalse, "CreateBackupFailed", fmt.Sprintf("Failed to create backup: %v", err))
			if statusErr := r.Status().Update(ctx, &schedule); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, err
		}

		log.Info("Created scheduled backup", "backup", backup.Name)
		metrics.RecordScheduledBackup(schedule.Namespace, metrics.StatusSuccess)

		// Update last backup info
		now := metav1.Now()
		schedule.Status.LastBackup = &dbopsv1alpha1.ScheduledBackupInfo{
			Name:      backup.Name,
			Status:    string(dbopsv1alpha1.PhasePending),
			StartedAt: &now,
		}

		// Increment statistics
		if schedule.Status.Statistics == nil {
			schedule.Status.Statistics = &dbopsv1alpha1.BackupStatistics{}
		}
		schedule.Status.Statistics.TotalBackups++
	}

	// Apply retention policy
	if schedule.Spec.Retention != nil {
		if err := r.enforceRetention(ctx, &schedule, backups); err != nil {
			log.Error(err, "Failed to enforce retention policy")
			// Don't fail the reconciliation for retention errors
		}
	}

	// Update recent backups list
	recentBackups := r.getRecentBackupsList(&schedule, backups)
	schedule.Status.RecentBackups = recentBackups

	// Update statistics from backups
	r.updateStatistics(&schedule, backups)

	// Set status to active
	schedule.Status.Phase = dbopsv1alpha1.PhaseActive
	util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionTrue, "Active", "Schedule is active")

	if err := r.Status().Update(ctx, &schedule); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Calculate requeue time - wait until next backup time
	requeueAfter := time.Until(nextTimeUTC)
	if requeueAfter < 0 {
		requeueAfter = 10 * time.Second // If we missed it, requeue quickly
	} else if requeueAfter > 1*time.Hour {
		// Don't wait more than an hour - allows periodic status updates
		requeueAfter = 1 * time.Hour
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// getTimezone parses and returns the timezone location
func (r *DatabaseBackupScheduleReconciler) getTimezone(tz string) (*time.Location, error) {
	if tz == "" || tz == "UTC" {
		return time.UTC, nil
	}
	return time.LoadLocation(tz)
}

// listBackupsForSchedule lists all backups created by this schedule
func (r *DatabaseBackupScheduleReconciler) listBackupsForSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
	var backupList dbopsv1alpha1.DatabaseBackupList
	if err := r.List(ctx, &backupList, client.InNamespace(schedule.Namespace), client.MatchingLabels{
		"dbops.dbprovision.io/schedule": schedule.Name,
	}); err != nil {
		return nil, err
	}
	return backupList.Items, nil
}

// filterBackupsByPhase filters backups by their phase
func (r *DatabaseBackupScheduleReconciler) filterBackupsByPhase(backups []dbopsv1alpha1.DatabaseBackup, phase dbopsv1alpha1.Phase) []dbopsv1alpha1.DatabaseBackup {
	var result []dbopsv1alpha1.DatabaseBackup
	for _, backup := range backups {
		if backup.Status.Phase == phase {
			result = append(result, backup)
		}
	}
	return result
}

// createBackupFromTemplate creates a DatabaseBackup from the schedule template
func (r *DatabaseBackupScheduleReconciler) createBackupFromTemplate(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
	// Generate a unique name with timestamp
	backupName := fmt.Sprintf("%s-%s", schedule.Name, time.Now().Format("20060102-150405"))

	// Build labels
	labels := make(map[string]string)
	for k, v := range schedule.Spec.Template.Metadata.Labels {
		labels[k] = v
	}
	labels["dbops.dbprovision.io/schedule"] = schedule.Name
	labels["dbops.dbprovision.io/scheduled-backup"] = "true"

	// Build annotations
	annotations := make(map[string]string)
	for k, v := range schedule.Spec.Template.Metadata.Annotations {
		annotations[k] = v
	}

	backup := &dbopsv1alpha1.DatabaseBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:        backupName,
			Namespace:   schedule.Namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         schedule.APIVersion,
					Kind:               schedule.Kind,
					Name:               schedule.Name,
					UID:                schedule.UID,
					Controller:         boolPtr(true),
					BlockOwnerDeletion: boolPtr(true),
				},
			},
		},
		Spec: schedule.Spec.Template.Spec,
	}

	if err := r.Create(ctx, backup); err != nil {
		return nil, err
	}

	return backup, nil
}

// enforceRetention deletes old backups based on retention policy
func (r *DatabaseBackupScheduleReconciler) enforceRetention(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule, backups []dbopsv1alpha1.DatabaseBackup) error {
	log := logf.FromContext(ctx)

	if schedule.Spec.Retention == nil {
		return nil
	}

	// Filter completed backups (only apply retention to completed backups)
	completedBackups := r.filterBackupsByPhase(backups, dbopsv1alpha1.PhaseCompleted)

	// Sort by creation timestamp (newest first)
	sort.Slice(completedBackups, func(i, j int) bool {
		return completedBackups[i].CreationTimestamp.After(completedBackups[j].CreationTimestamp.Time)
	})

	// Apply KeepLast retention
	if schedule.Spec.Retention.KeepLast > 0 && int32(len(completedBackups)) > schedule.Spec.Retention.KeepLast {
		toDelete := completedBackups[schedule.Spec.Retention.KeepLast:]
		for _, backup := range toDelete {
			log.Info("Deleting backup due to retention policy (keepLast)", "backup", backup.Name)
			if err := r.Delete(ctx, &backup); err != nil && !errors.IsNotFound(err) {
				log.Error(err, "Failed to delete backup", "backup", backup.Name)
			}
		}
	}

	// Apply KeepDaily retention (delete backups older than N days)
	if schedule.Spec.Retention.KeepDaily > 0 {
		cutoff := time.Now().AddDate(0, 0, -int(schedule.Spec.Retention.KeepDaily))
		for _, backup := range completedBackups {
			if backup.CreationTimestamp.Before(&metav1.Time{Time: cutoff}) {
				log.Info("Deleting backup due to retention policy (keepDays)", "backup", backup.Name)
				if err := r.Delete(ctx, &backup); err != nil && !errors.IsNotFound(err) {
					log.Error(err, "Failed to delete backup", "backup", backup.Name)
				}
			}
		}
	}

	return nil
}

// getRecentBackupsList builds the recent backups list for status
func (r *DatabaseBackupScheduleReconciler) getRecentBackupsList(schedule *dbopsv1alpha1.DatabaseBackupSchedule, backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.RecentBackupInfo {
	// Sort by creation timestamp (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreationTimestamp.After(backups[j].CreationTimestamp.Time)
	})

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

	for _, backup := range backups {
		include := false

		if backup.Status.Phase == dbopsv1alpha1.PhaseCompleted && successCount < successLimit {
			include = true
			successCount++
		} else if backup.Status.Phase == dbopsv1alpha1.PhaseFailed && failedCount < failedLimit {
			include = true
			failedCount++
		} else if backup.Status.Phase == dbopsv1alpha1.PhaseRunning || backup.Status.Phase == dbopsv1alpha1.PhasePending {
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

// updateStatistics updates backup statistics from completed backups
func (r *DatabaseBackupScheduleReconciler) updateStatistics(schedule *dbopsv1alpha1.DatabaseBackupSchedule, backups []dbopsv1alpha1.DatabaseBackup) {
	if schedule.Status.Statistics == nil {
		schedule.Status.Statistics = &dbopsv1alpha1.BackupStatistics{}
	}

	var completedCount, failedCount int64
	var totalDuration, totalSize int64

	for _, backup := range backups {
		switch backup.Status.Phase {
		case dbopsv1alpha1.PhaseCompleted:
			completedCount++
			if backup.Status.StartedAt != nil && backup.Status.CompletedAt != nil {
				duration := backup.Status.CompletedAt.Sub(backup.Status.StartedAt.Time)
				totalDuration += int64(duration.Seconds())
			}
			if backup.Status.Backup != nil {
				totalSize += backup.Status.Backup.SizeBytes
			}
		case dbopsv1alpha1.PhaseFailed:
			failedCount++
		}
	}

	schedule.Status.Statistics.SuccessfulBackups = completedCount
	schedule.Status.Statistics.FailedBackups = failedCount
	schedule.Status.Statistics.TotalStorageBytes = totalSize

	if completedCount > 0 {
		schedule.Status.Statistics.AverageDurationSeconds = totalDuration / completedCount
		schedule.Status.Statistics.AverageSizeBytes = totalSize / completedCount
	}

	// Update last backup status if we have backups
	if len(backups) > 0 {
		// Find the most recent backup
		var latestBackup *dbopsv1alpha1.DatabaseBackup
		for i := range backups {
			if latestBackup == nil || backups[i].CreationTimestamp.After(latestBackup.CreationTimestamp.Time) {
				latestBackup = &backups[i]
			}
		}

		if latestBackup != nil && schedule.Status.LastBackup != nil {
			schedule.Status.LastBackup.Status = string(latestBackup.Status.Phase)
			if latestBackup.Status.CompletedAt != nil {
				schedule.Status.LastBackup.CompletedAt = latestBackup.Status.CompletedAt
			}
		}
	}
}

// boolPtr returns a pointer to a bool
func boolPtr(b bool) *bool {
	return &b
}

// handleDeletion cleans up resources when a schedule is deleted
func (r *DatabaseBackupScheduleReconciler) handleDeletion(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) error {
	log := logf.FromContext(ctx)
	log.Info("Handling deletion of DatabaseBackupSchedule", "name", schedule.Name)

	// Clean up schedule metrics
	databaseName := schedule.Spec.Template.Spec.DatabaseRef.Name
	metrics.DeleteScheduleMetrics(databaseName, schedule.Namespace)

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseBackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseBackupSchedule{}).
		Owns(&dbopsv1alpha1.DatabaseBackup{}).
		Named("databasebackupschedule").
		Complete(r)
}
