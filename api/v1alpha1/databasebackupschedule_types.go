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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseBackupScheduleSpec defines the desired state of DatabaseBackupSchedule.
type DatabaseBackupScheduleSpec struct {
	// Schedule is the cron expression for the backup schedule
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Schedule string `json:"schedule"`

	// Timezone is the timezone for the schedule (e.g., "Asia/Bangkok")
	// +kubebuilder:default=UTC
	Timezone string `json:"timezone,omitempty"`

	// Paused suspends the schedule
	// +optional
	Paused bool `json:"paused,omitempty"`

	// ConcurrencyPolicy defines how to handle concurrent backups
	// +kubebuilder:validation:Enum=Allow;Forbid;Replace
	// +kubebuilder:default=Forbid
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// Template defines the DatabaseBackup to create
	// +kubebuilder:validation:Required
	Template BackupTemplateSpec `json:"template"`

	// Retention defines the backup retention policy
	// +optional
	Retention *RetentionPolicy `json:"retention,omitempty"`

	// SuccessfulBackupsHistoryLimit is the number of successful backups to keep in status
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=5
	SuccessfulBackupsHistoryLimit int32 `json:"successfulBackupsHistoryLimit,omitempty"`

	// FailedBackupsHistoryLimit is the number of failed backups to keep in status
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	FailedBackupsHistoryLimit int32 `json:"failedBackupsHistoryLimit,omitempty"`

	// DeletionProtection prevents accidental deletion
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`
}

// BackupTemplateSpec defines the template for created backups
type BackupTemplateSpec struct {
	// Metadata for the created backup
	// +optional
	Metadata BackupTemplateMeta `json:"metadata,omitempty"`

	// Spec for the created backup
	// +kubebuilder:validation:Required
	Spec DatabaseBackupSpec `json:"spec"`
}

// BackupTemplateMeta defines metadata for created backups
type BackupTemplateMeta struct {
	// Labels to add to created backups
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to created backups
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DatabaseBackupScheduleStatus defines the observed state of DatabaseBackupSchedule.
type DatabaseBackupScheduleStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Active;Paused
	Phase Phase `json:"phase,omitempty"`

	// LastBackup contains information about the last backup
	LastBackup *ScheduledBackupInfo `json:"lastBackup,omitempty"`

	// NextBackupTime is the next scheduled backup time
	NextBackupTime *metav1.Time `json:"nextBackupTime,omitempty"`

	// Statistics contains backup statistics
	Statistics *BackupStatistics `json:"statistics,omitempty"`

	// RecentBackups lists recent backup names and statuses
	RecentBackups []RecentBackupInfo `json:"recentBackups,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ScheduledBackupInfo contains information about a scheduled backup
type ScheduledBackupInfo struct {
	// Name of the backup
	Name string `json:"name,omitempty"`

	// Status of the backup
	Status string `json:"status,omitempty"`

	// StartedAt is when the backup started
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is when the backup completed
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// BackupStatistics contains backup statistics
type BackupStatistics struct {
	// TotalBackups is the total number of backups created
	TotalBackups int64 `json:"totalBackups,omitempty"`

	// SuccessfulBackups is the number of successful backups
	SuccessfulBackups int64 `json:"successfulBackups,omitempty"`

	// FailedBackups is the number of failed backups
	FailedBackups int64 `json:"failedBackups,omitempty"`

	// AverageDurationSeconds is the average backup duration
	AverageDurationSeconds int64 `json:"averageDurationSeconds,omitempty"`

	// AverageSizeBytes is the average backup size
	AverageSizeBytes int64 `json:"averageSizeBytes,omitempty"`

	// TotalStorageBytes is the total storage used by all backups
	TotalStorageBytes int64 `json:"totalStorageBytes,omitempty"`
}

// RecentBackupInfo contains information about a recent backup
type RecentBackupInfo struct {
	// Name of the backup
	Name string `json:"name,omitempty"`

	// Status of the backup
	Status string `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbbaksched
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.schedule`
// +kubebuilder:printcolumn:name="Paused",type=boolean,JSONPath=`.spec.paused`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Last Backup",type=string,JSONPath=`.status.lastBackup.name`
// +kubebuilder:printcolumn:name="Next Backup",type=date,JSONPath=`.status.nextBackupTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseBackupSchedule is the Schema for the databasebackupschedules API.
type DatabaseBackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseBackupScheduleSpec   `json:"spec,omitempty"`
	Status DatabaseBackupScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseBackupScheduleList contains a list of DatabaseBackupSchedule.
type DatabaseBackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseBackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseBackupSchedule{}, &DatabaseBackupScheduleList{})
}
