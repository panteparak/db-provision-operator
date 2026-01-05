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

// Labels for backups
const (
	LabelDatabaseBackup  = "dbops.dbprovision.io/database"
	LabelBackupSchedule  = "dbops.dbprovision.io/schedule"
)

// DatabaseBackupSpec defines the desired state of DatabaseBackup.
type DatabaseBackupSpec struct {
	// DatabaseRef references the Database to backup
	// +kubebuilder:validation:Required
	DatabaseRef DatabaseReference `json:"databaseRef"`

	// Storage defines where to store the backup
	// +kubebuilder:validation:Required
	Storage StorageConfig `json:"storage"`

	// Compression configures backup compression
	// +optional
	Compression *CompressionConfig `json:"compression,omitempty"`

	// Encryption configures backup encryption
	// +optional
	Encryption *EncryptionConfig `json:"encryption,omitempty"`

	// TTL is the time-to-live for the backup (e.g., "168h" for 7 days)
	// +optional
	TTL string `json:"ttl,omitempty"`

	// ActiveDeadlineSeconds is the timeout for the backup operation
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3600
	ActiveDeadlineSeconds int64 `json:"activeDeadlineSeconds,omitempty"`

	// PostgreSQL-specific backup configuration
	// +optional
	Postgres *PostgresBackupConfig `json:"postgres,omitempty"`

	// MySQL-specific backup configuration
	// +optional
	MySQL *MySQLBackupConfig `json:"mysql,omitempty"`
}

// DatabaseBackupStatus defines the observed state of DatabaseBackup.
type DatabaseBackupStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
	Phase Phase `json:"phase,omitempty"`

	// StartedAt is the backup start time
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// CompletedAt is the backup completion time
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Backup contains backup-specific status information
	Backup *BackupInfo `json:"backup,omitempty"`

	// Source contains information about the backup source
	Source *BackupSourceInfo `json:"source,omitempty"`

	// ExpiresAt is when the backup will be deleted
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// BackupInfo contains backup file information
type BackupInfo struct {
	// Path is the full path to the backup file
	Path string `json:"path,omitempty"`

	// SizeBytes is the backup size in bytes (uncompressed)
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// CompressedSizeBytes is the backup size in bytes (compressed)
	CompressedSizeBytes int64 `json:"compressedSizeBytes,omitempty"`

	// Checksum is the backup file checksum
	Checksum string `json:"checksum,omitempty"`

	// Format is the backup format (e.g., custom, plain, directory)
	Format string `json:"format,omitempty"`
}

// BackupSourceInfo contains information about the backup source
type BackupSourceInfo struct {
	// Instance is the DatabaseInstance name
	Instance string `json:"instance,omitempty"`

	// Database is the database name
	Database string `json:"database,omitempty"`

	// Engine is the database engine type
	Engine string `json:"engine,omitempty"`

	// Version is the database server version
	Version string `json:"version,omitempty"`

	// Timestamp is the point-in-time of the backup
	Timestamp *metav1.Time `json:"timestamp,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbbak
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.databaseRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.backup.compressedSizeBytes`
// +kubebuilder:printcolumn:name="Started",type=date,JSONPath=`.status.startedAt`
// +kubebuilder:printcolumn:name="Completed",type=date,JSONPath=`.status.completedAt`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseBackup is the Schema for the databasebackups API.
type DatabaseBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseBackupSpec   `json:"spec,omitempty"`
	Status DatabaseBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseBackupList contains a list of DatabaseBackup.
type DatabaseBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseBackup{}, &DatabaseBackupList{})
}
