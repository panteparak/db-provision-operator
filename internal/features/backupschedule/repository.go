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
	"sort"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Repository handles K8s client operations for backup schedules.
type Repository struct {
	client client.Client
	logger logr.Logger
}

// RepositoryConfig holds dependencies for the repository.
type RepositoryConfig struct {
	Client client.Client
	Logger logr.Logger
}

// NewRepository creates a new backupschedule repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client: cfg.Client,
		logger: cfg.Logger,
	}
}

// ListBackupsForSchedule lists all backups created by this schedule.
func (r *Repository) ListBackupsForSchedule(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) ([]dbopsv1alpha1.DatabaseBackup, error) {
	var backupList dbopsv1alpha1.DatabaseBackupList
	if err := r.client.List(ctx, &backupList, client.InNamespace(schedule.Namespace), client.MatchingLabels{
		"dbops.dbprovision.io/schedule": schedule.Name,
	}); err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	return backupList.Items, nil
}

// FilterBackupsByPhase filters backups by their phase.
func (r *Repository) FilterBackupsByPhase(backups []dbopsv1alpha1.DatabaseBackup, phase dbopsv1alpha1.Phase) []dbopsv1alpha1.DatabaseBackup {
	var result []dbopsv1alpha1.DatabaseBackup
	for _, backup := range backups {
		if backup.Status.Phase == phase {
			result = append(result, backup)
		}
	}
	return result
}

// CreateBackupFromTemplate creates a DatabaseBackup from the schedule template.
func (r *Repository) CreateBackupFromTemplate(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (*dbopsv1alpha1.DatabaseBackup, error) {
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

	if err := r.client.Create(ctx, backup); err != nil {
		return nil, fmt.Errorf("create backup: %w", err)
	}

	logf.FromContext(ctx).Info("Created backup from template", "backup", backupName, "schedule", schedule.Name)
	return backup, nil
}

// DeleteBackup deletes a backup.
func (r *Repository) DeleteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
	if err := r.client.Delete(ctx, backup); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete backup %s: %w", backup.Name, err)
	}
	logf.FromContext(ctx).Info("Deleted backup", "backup", backup.Name)
	return nil
}

// DeleteBackups deletes multiple backups.
func (r *Repository) DeleteBackups(ctx context.Context, backups []dbopsv1alpha1.DatabaseBackup) ([]string, error) {
	var deleted []string
	for i := range backups {
		if err := r.DeleteBackup(ctx, &backups[i]); err != nil {
			logf.FromContext(ctx).Error(err, "Failed to delete backup", "backup", backups[i].Name)
			continue
		}
		deleted = append(deleted, backups[i].Name)
	}
	return deleted, nil
}

// GetCompletedBackupsSortedByTime returns completed backups sorted by creation time (newest first).
func (r *Repository) GetCompletedBackupsSortedByTime(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
	completedBackups := r.FilterBackupsByPhase(backups, dbopsv1alpha1.PhaseCompleted)

	sort.Slice(completedBackups, func(i, j int) bool {
		return completedBackups[i].CreationTimestamp.After(completedBackups[j].CreationTimestamp.Time)
	})

	return completedBackups
}

// GetBackupsSortedByTime returns all backups sorted by creation time (newest first).
func (r *Repository) GetBackupsSortedByTime(backups []dbopsv1alpha1.DatabaseBackup) []dbopsv1alpha1.DatabaseBackup {
	sorted := make([]dbopsv1alpha1.DatabaseBackup, len(backups))
	copy(sorted, backups)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreationTimestamp.After(sorted[j].CreationTimestamp.Time)
	})

	return sorted
}

// GetLatestBackup returns the most recent backup.
func (r *Repository) GetLatestBackup(backups []dbopsv1alpha1.DatabaseBackup) *dbopsv1alpha1.DatabaseBackup {
	if len(backups) == 0 {
		return nil
	}

	var latest *dbopsv1alpha1.DatabaseBackup
	for i := range backups {
		if latest == nil || backups[i].CreationTimestamp.After(latest.CreationTimestamp.Time) {
			latest = &backups[i]
		}
	}
	return latest
}

// boolPtr returns a pointer to a bool.
func boolPtr(b bool) *bool {
	return &b
}
