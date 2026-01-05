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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EngineType defines the database engine type
// +kubebuilder:validation:Enum=postgres;mysql;cockroachdb
type EngineType string

const (
	EngineTypePostgres    EngineType = "postgres"
	EngineTypeMySQL       EngineType = "mysql"
	EngineTypeCockroachDB EngineType = "cockroachdb"
)

// DeletionPolicy defines what happens when a resource is deleted
// +kubebuilder:validation:Enum=Retain;Delete;Snapshot
type DeletionPolicy string

const (
	DeletionPolicyRetain   DeletionPolicy = "Retain"
	DeletionPolicyDelete   DeletionPolicy = "Delete"
	DeletionPolicySnapshot DeletionPolicy = "Snapshot"
)

// Phase represents the current state of a resource
type Phase string

const (
	PhasePending   Phase = "Pending"
	PhaseCreating  Phase = "Creating"
	PhaseReady     Phase = "Ready"
	PhaseFailed    Phase = "Failed"
	PhaseDeleting  Phase = "Deleting"
	PhaseRunning   Phase = "Running"
	PhaseCompleted Phase = "Completed"
	PhasePaused    Phase = "Paused"
	PhaseActive    Phase = "Active"
)

// SecretKeySelector contains a reference to a secret key
type SecretKeySelector struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key within the secret
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// SecretReference contains a reference to a secret with multiple keys
type SecretReference struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// CredentialSecretRef references credentials in a secret
type CredentialSecretRef struct {
	// Name of the secret containing credentials
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Keys defines the key names for username and password
	// +optional
	Keys *CredentialKeys `json:"keys,omitempty"`
}

// CredentialKeys defines the key names within a credential secret
type CredentialKeys struct {
	// Username key in the secret (default: "username")
	// +kubebuilder:default=username
	Username string `json:"username,omitempty"`

	// Password key in the secret (default: "password")
	// +kubebuilder:default=password
	Password string `json:"password,omitempty"`
}

// TLSConfig defines TLS configuration for database connections
type TLSConfig struct {
	// Enabled enables TLS for connections
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Mode specifies the TLS verification mode
	// +kubebuilder:validation:Enum=disable;require;verify-ca;verify-full
	// +kubebuilder:default=disable
	Mode string `json:"mode,omitempty"`

	// SecretRef references a secret containing TLS certificates
	// +optional
	SecretRef *TLSSecretRef `json:"secretRef,omitempty"`
}

// TLSSecretRef references TLS certificates in a secret
type TLSSecretRef struct {
	// Name of the secret containing TLS certificates
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Keys defines the key names for TLS certificates
	// +optional
	Keys *TLSKeys `json:"keys,omitempty"`
}

// TLSKeys defines the key names within a TLS secret
type TLSKeys struct {
	// CA certificate key (default: "ca.crt")
	// +kubebuilder:default=ca.crt
	CA string `json:"ca,omitempty"`

	// Client certificate key for mTLS (optional)
	// +optional
	Cert string `json:"cert,omitempty"`

	// Client key for mTLS (optional)
	// +optional
	Key string `json:"key,omitempty"`
}

// HealthCheckConfig defines health check settings
type HealthCheckConfig struct {
	// Enabled enables periodic health checks
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// IntervalSeconds defines how often to check (default: 30)
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=5
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`

	// TimeoutSeconds defines the health check timeout (default: 5)
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

// InstanceReference references a DatabaseInstance
type InstanceReference struct {
	// Name of the DatabaseInstance
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the DatabaseInstance (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// DatabaseReference references a Database
type DatabaseReference struct {
	// Name of the Database resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Database (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// UserReference references a DatabaseUser
type UserReference struct {
	// Name of the DatabaseUser resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the DatabaseUser (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// BackupReference references a DatabaseBackup
type BackupReference struct {
	// Name of the DatabaseBackup resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the DatabaseBackup (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// StorageType defines the backup storage type
// +kubebuilder:validation:Enum=gcs;s3;azure;pvc
type StorageType string

const (
	StorageTypeGCS   StorageType = "gcs"
	StorageTypeS3    StorageType = "s3"
	StorageTypeAzure StorageType = "azure"
	StorageTypePVC   StorageType = "pvc"
)

// StorageConfig defines backup storage configuration
type StorageConfig struct {
	// Type of storage backend
	// +kubebuilder:validation:Required
	Type StorageType `json:"type"`

	// GCS configuration (required when type is "gcs")
	// +optional
	GCS *GCSStorageConfig `json:"gcs,omitempty"`

	// S3 configuration (required when type is "s3")
	// +optional
	S3 *S3StorageConfig `json:"s3,omitempty"`

	// Azure configuration (required when type is "azure")
	// +optional
	Azure *AzureStorageConfig `json:"azure,omitempty"`

	// PVC configuration (required when type is "pvc")
	// +optional
	PVC *PVCStorageConfig `json:"pvc,omitempty"`
}

// GCSStorageConfig defines Google Cloud Storage configuration
type GCSStorageConfig struct {
	// Bucket name
	// +kubebuilder:validation:Required
	Bucket string `json:"bucket"`

	// Prefix (path prefix within the bucket)
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// SecretRef references a secret containing GCS credentials
	// +kubebuilder:validation:Required
	SecretRef SecretKeySelector `json:"secretRef"`
}

// S3StorageConfig defines S3-compatible storage configuration
type S3StorageConfig struct {
	// Bucket name
	// +kubebuilder:validation:Required
	Bucket string `json:"bucket"`

	// Region of the S3 bucket
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// Prefix (path prefix within the bucket)
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// Endpoint for S3-compatible storage (e.g., MinIO)
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// SecretRef references a secret containing S3 credentials
	// +kubebuilder:validation:Required
	SecretRef S3SecretRef `json:"secretRef"`

	// ForcePathStyle enables path-style addressing (required for MinIO)
	// +optional
	ForcePathStyle bool `json:"forcePathStyle,omitempty"`
}

// S3SecretRef references S3 credentials in a secret
type S3SecretRef struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Keys defines the key names for S3 credentials
	// +optional
	Keys *S3SecretKeys `json:"keys,omitempty"`
}

// S3SecretKeys defines the key names within an S3 secret
type S3SecretKeys struct {
	// Access key ID (default: "AWS_ACCESS_KEY_ID")
	// +kubebuilder:default=AWS_ACCESS_KEY_ID
	AccessKey string `json:"accessKey,omitempty"`

	// Secret access key (default: "AWS_SECRET_ACCESS_KEY")
	// +kubebuilder:default=AWS_SECRET_ACCESS_KEY
	SecretKey string `json:"secretKey,omitempty"`
}

// AzureStorageConfig defines Azure Blob Storage configuration
type AzureStorageConfig struct {
	// Container name
	// +kubebuilder:validation:Required
	Container string `json:"container"`

	// StorageAccount name
	// +kubebuilder:validation:Required
	StorageAccount string `json:"storageAccount"`

	// Prefix (path prefix within the container)
	// +optional
	Prefix string `json:"prefix,omitempty"`

	// SecretRef references a secret containing Azure credentials
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`
}

// PVCStorageConfig defines PVC-based storage configuration
type PVCStorageConfig struct {
	// ClaimName is the name of the PersistentVolumeClaim
	// +kubebuilder:validation:Required
	ClaimName string `json:"claimName"`

	// SubPath within the PVC
	// +optional
	SubPath string `json:"subPath,omitempty"`
}

// CompressionAlgorithm defines the compression algorithm
// +kubebuilder:validation:Enum=gzip;lz4;zstd;none
type CompressionAlgorithm string

const (
	CompressionGzip CompressionAlgorithm = "gzip"
	CompressionLZ4  CompressionAlgorithm = "lz4"
	CompressionZstd CompressionAlgorithm = "zstd"
	CompressionNone CompressionAlgorithm = "none"
)

// CompressionConfig defines backup compression settings
type CompressionConfig struct {
	// Enabled enables compression
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Algorithm specifies the compression algorithm
	// +kubebuilder:validation:Enum=gzip;lz4;zstd
	// +kubebuilder:default=gzip
	Algorithm CompressionAlgorithm `json:"algorithm,omitempty"`

	// Level specifies the compression level (1-9)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=9
	// +kubebuilder:default=6
	Level int32 `json:"level,omitempty"`
}

// EncryptionAlgorithm defines the encryption algorithm
// +kubebuilder:validation:Enum=aes-256-gcm;aes-256-cbc
type EncryptionAlgorithm string

const (
	EncryptionAES256GCM EncryptionAlgorithm = "aes-256-gcm"
	EncryptionAES256CBC EncryptionAlgorithm = "aes-256-cbc"
)

// EncryptionConfig defines backup encryption settings
type EncryptionConfig struct {
	// Enabled enables encryption
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Algorithm specifies the encryption algorithm
	// +kubebuilder:validation:Enum=aes-256-gcm;aes-256-cbc
	// +kubebuilder:default=aes-256-gcm
	Algorithm EncryptionAlgorithm `json:"algorithm,omitempty"`

	// SecretRef references a secret containing the encryption key
	// +optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`
}

// SecretFormat defines the output secret format
// +kubebuilder:validation:Enum=kubernetes;vault;external-secrets
type SecretFormat string

const (
	SecretFormatKubernetes      SecretFormat = "kubernetes"
	SecretFormatVault           SecretFormat = "vault"
	SecretFormatExternalSecrets SecretFormat = "external-secrets"
)

// SecretTemplate defines the template for generated secrets
type SecretTemplate struct {
	// Type is the secret type (default: Opaque)
	// +kubebuilder:default=Opaque
	Type corev1.SecretType `json:"type,omitempty"`

	// Labels to add to the secret
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to the secret
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Data defines templated data keys
	// Available variables: .Username, .Password, .Host, .Port, .Database, .SSLMode
	// +optional
	Data map[string]string `json:"data,omitempty"`
}

// PasswordConfig defines password generation settings
type PasswordConfig struct {
	// Generate enables password generation
	// +kubebuilder:default=true
	Generate bool `json:"generate,omitempty"`

	// Length of the generated password (default: 32)
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=128
	// +kubebuilder:default=32
	Length int32 `json:"length,omitempty"`

	// IncludeSpecialChars includes special characters in the password
	// +kubebuilder:default=true
	IncludeSpecialChars bool `json:"includeSpecialChars,omitempty"`

	// ExcludeChars specifies characters to exclude from the password
	// +optional
	ExcludeChars string `json:"excludeChars,omitempty"`

	// SecretName is the name of the generated secret
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// SecretNamespace is the namespace for the generated secret (defaults to resource namespace)
	// +optional
	SecretNamespace string `json:"secretNamespace,omitempty"`

	// Format specifies the secret format
	// +kubebuilder:validation:Enum=kubernetes;vault;external-secrets
	// +kubebuilder:default=kubernetes
	Format SecretFormat `json:"format,omitempty"`

	// SecretTemplate defines the secret template
	// +optional
	SecretTemplate *SecretTemplate `json:"secretTemplate,omitempty"`
}

// ExistingPasswordSecret references an existing secret containing a password
type ExistingPasswordSecret struct {
	// Name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the secret (defaults to the resource namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key within the secret containing the password
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// PasswordRotationConfig defines password rotation settings
type PasswordRotationConfig struct {
	// Enabled enables automatic password rotation
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Schedule is a cron expression for rotation (e.g., "0 0 1 * *" for monthly)
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// MaxAge is the maximum age of a password before rotation (e.g., "90d")
	// +optional
	MaxAge string `json:"maxAge,omitempty"`
}

// RetentionPolicy defines backup retention settings
type RetentionPolicy struct {
	// KeepLast keeps the N most recent backups
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeepLast int32 `json:"keepLast,omitempty"`

	// KeepHourly keeps N hourly backups
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeepHourly int32 `json:"keepHourly,omitempty"`

	// KeepDaily keeps N daily backups
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeepDaily int32 `json:"keepDaily,omitempty"`

	// KeepWeekly keeps N weekly backups
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeepWeekly int32 `json:"keepWeekly,omitempty"`

	// KeepMonthly keeps N monthly backups
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeepMonthly int32 `json:"keepMonthly,omitempty"`

	// KeepYearly keeps N yearly backups
	// +kubebuilder:validation:Minimum=0
	// +optional
	KeepYearly int32 `json:"keepYearly,omitempty"`

	// MinAge is the minimum age before a backup can be deleted
	// +optional
	MinAge string `json:"minAge,omitempty"`
}

// ConcurrencyPolicy defines how to handle concurrent backups
// +kubebuilder:validation:Enum=Allow;Forbid;Replace
type ConcurrencyPolicy string

const (
	ConcurrencyPolicyAllow   ConcurrencyPolicy = "Allow"
	ConcurrencyPolicyForbid  ConcurrencyPolicy = "Forbid"
	ConcurrencyPolicyReplace ConcurrencyPolicy = "Replace"
)

// Condition represents a condition of a resource
type Condition struct {
	// Type of the condition
	Type string `json:"type"`

	// Status of the condition (True, False, Unknown)
	Status metav1.ConditionStatus `json:"status"`

	// LastTransitionTime is the last time the condition transitioned
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// Reason is a brief reason for the condition
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable description
	// +optional
	Message string `json:"message,omitempty"`
}

// Condition types
const (
	ConditionTypeReady              = "Ready"
	ConditionTypeConnected          = "Connected"
	ConditionTypeTLSVerified        = "TLSVerified"
	ConditionTypeSecretCreated      = "SecretCreated"
	ConditionTypePasswordRotated    = "PasswordRotated"
	ConditionTypeExtensionsInstalled = "ExtensionsInstalled"
	ConditionTypeSchemasCreated     = "SchemasCreated"
	ConditionTypeGrantsApplied      = "GrantsApplied"
	ConditionTypeRolesAssigned      = "RolesAssigned"
	ConditionTypeComplete           = "Complete"
	ConditionTypeScheduled          = "Scheduled"
	ConditionTypeRetentionEnforced  = "RetentionEnforced"
)

// Condition reasons
const (
	ReasonConnectionSuccessful = "ConnectionSuccessful"
	ReasonConnectionFailed     = "ConnectionFailed"
	ReasonCertificateValid     = "CertificateValid"
	ReasonCertificateInvalid   = "CertificateInvalid"
	ReasonDatabaseReady        = "DatabaseReady"
	ReasonDatabaseFailed       = "DatabaseFailed"
	ReasonUserReady            = "UserReady"
	ReasonUserFailed           = "UserFailed"
	ReasonBackupSucceeded      = "BackupSucceeded"
	ReasonBackupFailed         = "BackupFailed"
	ReasonRestoreSucceeded     = "RestoreSucceeded"
	ReasonRestoreFailed        = "RestoreFailed"
)
