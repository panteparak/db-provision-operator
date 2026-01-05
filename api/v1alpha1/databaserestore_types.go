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

// Restore confirmation value
const (
	RestoreConfirmDataLoss = "I-UNDERSTAND-DATA-LOSS"
)

// DatabaseRestoreSpec defines the desired state of DatabaseRestore.
type DatabaseRestoreSpec struct {
	// BackupRef references the DatabaseBackup to restore from
	// +optional
	BackupRef *BackupReference `json:"backupRef,omitempty"`

	// FromPath allows restoring from a direct path instead of a backup reference
	// +optional
	FromPath *RestoreFromPath `json:"fromPath,omitempty"`

	// Target defines where to restore the backup
	// +kubebuilder:validation:Required
	Target RestoreTarget `json:"target"`

	// Confirmation contains safety confirmations for destructive operations
	// +optional
	Confirmation *RestoreConfirmation `json:"confirmation,omitempty"`

	// ActiveDeadlineSeconds is the timeout for the restore operation
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=7200
	ActiveDeadlineSeconds int64 `json:"activeDeadlineSeconds,omitempty"`

	// PostgreSQL-specific restore configuration
	// +optional
	Postgres *PostgresRestoreConfig `json:"postgres,omitempty"`

	// MySQL-specific restore configuration
	// +optional
	MySQL *MySQLRestoreConfig `json:"mysql,omitempty"`
}

// RestoreFromPath defines restoring from a direct path
type RestoreFromPath struct {
	// Storage defines where the backup is stored
	// +kubebuilder:validation:Required
	Storage StorageConfig `json:"storage"`

	// Compression settings used for the backup
	// +optional
	Compression *CompressionConfig `json:"compression,omitempty"`

	// Encryption settings used for the backup
	// +optional
	Encryption *EncryptionConfig `json:"encryption,omitempty"`
}

// RestoreTarget defines where to restore the backup
type RestoreTarget struct {
	// InstanceRef references the target DatabaseInstance
	// +optional
	InstanceRef *InstanceReference `json:"instanceRef,omitempty"`

	// DatabaseName is the target database name (for restore to new database)
	// +optional
	DatabaseName string `json:"databaseName,omitempty"`

	// InPlace enables in-place restore (destructive!)
	// +optional
	InPlace bool `json:"inPlace,omitempty"`

	// DatabaseRef references the target Database for in-place restore
	// +optional
	DatabaseRef *DatabaseReference `json:"databaseRef,omitempty"`
}

// RestoreConfirmation contains safety confirmations
type RestoreConfirmation struct {
	// AcknowledgeDataLoss must be set to "I-UNDERSTAND-DATA-LOSS" for destructive operations
	// +optional
	AcknowledgeDataLoss string `json:"acknowledgeDataLoss,omitempty"`
}

// DatabaseRestoreStatus defines the observed state of DatabaseRestore.
type DatabaseRestoreStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
	Phase Phase `json:"phase,omitempty"`

	// StartedAt is the restore start time
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the restore completion time
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Restore contains restore-specific status information
	Restore *RestoreInfo `json:"restore,omitempty"`

	// Progress contains restore progress information
	Progress *RestoreProgress `json:"progress,omitempty"`

	// Warnings contains any warnings encountered during restore
	Warnings []string `json:"warnings,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RestoreInfo contains restore-specific information
type RestoreInfo struct {
	// SourceBackup is the source backup name
	SourceBackup string `json:"sourceBackup,omitempty"`

	// TargetInstance is the target instance name
	TargetInstance string `json:"targetInstance,omitempty"`

	// TargetDatabase is the target database name
	TargetDatabase string `json:"targetDatabase,omitempty"`
}

// RestoreProgress contains restore progress information
type RestoreProgress struct {
	// Percentage is the restore progress percentage (0-100)
	Percentage int32 `json:"percentage,omitempty"`

	// CurrentPhase is the current restore phase
	CurrentPhase string `json:"currentPhase,omitempty"`

	// TablesRestored is the number of tables restored
	TablesRestored int32 `json:"tablesRestored,omitempty"`

	// TablesTotal is the total number of tables to restore
	TablesTotal int32 `json:"tablesTotal,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbrestore
// +kubebuilder:printcolumn:name="Backup",type=string,JSONPath=`.spec.backupRef.name`
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.status.restore.targetDatabase`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Progress",type=integer,JSONPath=`.status.progress.percentage`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseRestore is the Schema for the databaserestores API.
type DatabaseRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRestoreSpec   `json:"spec,omitempty"`
	Status DatabaseRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseRestoreList contains a list of DatabaseRestore.
type DatabaseRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseRestore{}, &DatabaseRestoreList{})
}
