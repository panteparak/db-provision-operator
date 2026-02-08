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

// Package backupschedule provides the DatabaseBackupSchedule feature module for managing scheduled database backups.
package backupschedule

import (
	"context"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// API defines the public interface for the backupschedule module.
type API interface {
	// TriggerBackup triggers a backup from the schedule template.
	TriggerBackup(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*TriggerResult, error)

	// EvaluateSchedule evaluates whether a backup is due based on the cron schedule.
	EvaluateSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*ScheduleEvaluation, error)

	// EnforceRetention enforces the retention policy by deleting old backups.
	EnforceRetention(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*RetentionResult, error)

	// HandleConcurrency handles concurrency policy for running backups.
	HandleConcurrency(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*ConcurrencyResult, error)

	// UpdateStatistics updates backup statistics based on completed backups.
	UpdateStatistics(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.BackupStatistics, error)
}

// TriggerResult represents the result of triggering a scheduled backup.
type TriggerResult struct {
	Triggered  bool
	BackupName string
	Message    string
}

// ScheduleEvaluation represents the evaluation of whether a backup is due.
type ScheduleEvaluation struct {
	BackupDue      bool
	NextBackupTime time.Time
	LastBackupTime *time.Time
	RunningBackups int
	Message        string
}

// RetentionResult represents the result of enforcing retention policy.
type RetentionResult struct {
	DeletedCount int
	DeletedNames []string
	Message      string
}

// ConcurrencyResult represents the result of handling concurrency policy.
type ConcurrencyResult struct {
	ShouldProceed  bool
	CancelledCount int
	CancelledNames []string
	Message        string
}

// RepositoryInterface defines the repository operations for backup schedule.
type RepositoryInterface interface {
	// ListBackupsForSchedule lists all backups created by this schedule.
	ListBackupsForSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error)

	// FilterBackupsByPhase filters backups by their phase.
	FilterBackupsByPhase(backups []dbopsv1alpha1.DatabaseBackup, phase dbopsv1alpha1.Phase) []dbopsv1alpha1.DatabaseBackup

	// CreateBackupFromTemplate creates a DatabaseBackup from the schedule template.
	CreateBackupFromTemplate(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error)

	// DeleteBackups deletes multiple backups.
	DeleteBackups(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error)

	// GetCompletedBackupsSortedByTime returns completed backups sorted by creation time (newest first).
	GetCompletedBackupsSortedByTime(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup

	// GetBackupsSortedByTime returns all backups sorted by creation time (newest first).
	GetBackupsSortedByTime(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup

	// GetLatestBackup returns the most recent backup.
	GetLatestBackup(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
