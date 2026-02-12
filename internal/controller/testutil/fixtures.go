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

package testutil

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// DatabaseInstanceBuilder provides a fluent interface for building DatabaseInstance objects.
type DatabaseInstanceBuilder struct {
	instance *dbopsv1alpha1.DatabaseInstance
}

// NewDatabaseInstance creates a new DatabaseInstanceBuilder with defaults.
func NewDatabaseInstance(name, namespace string) *DatabaseInstanceBuilder {
	return &DatabaseInstanceBuilder{
		instance: &dbopsv1alpha1.DatabaseInstance{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseInstance",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseInstanceSpec{
				Engine: dbopsv1alpha1.EngineTypePostgres,
				Connection: dbopsv1alpha1.ConnectionConfig{
					Host:     "localhost",
					Port:     5432,
					Database: "postgres",
				},
			},
		},
	}
}

// WithEngine sets the engine type.
func (b *DatabaseInstanceBuilder) WithEngine(engine dbopsv1alpha1.EngineType) *DatabaseInstanceBuilder {
	b.instance.Spec.Engine = engine
	return b
}

// WithHost sets the host.
func (b *DatabaseInstanceBuilder) WithHost(host string) *DatabaseInstanceBuilder {
	b.instance.Spec.Connection.Host = host
	return b
}

// WithPort sets the port.
func (b *DatabaseInstanceBuilder) WithPort(port int32) *DatabaseInstanceBuilder {
	b.instance.Spec.Connection.Port = port
	return b
}

// WithDatabase sets the database name.
func (b *DatabaseInstanceBuilder) WithDatabase(database string) *DatabaseInstanceBuilder {
	b.instance.Spec.Connection.Database = database
	return b
}

// WithSecretRef sets the secret reference.
func (b *DatabaseInstanceBuilder) WithSecretRef(name string) *DatabaseInstanceBuilder {
	b.instance.Spec.Connection.SecretRef = &dbopsv1alpha1.CredentialSecretRef{
		Name: name,
	}
	return b
}

// WithSecretRefAndNamespace sets the secret reference with namespace.
func (b *DatabaseInstanceBuilder) WithSecretRefAndNamespace(name, namespace string) *DatabaseInstanceBuilder {
	b.instance.Spec.Connection.SecretRef = &dbopsv1alpha1.CredentialSecretRef{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithExistingSecret sets an existing secret reference.
func (b *DatabaseInstanceBuilder) WithExistingSecret(name string, keys *dbopsv1alpha1.CredentialKeys) *DatabaseInstanceBuilder {
	b.instance.Spec.Connection.ExistingSecret = &dbopsv1alpha1.CredentialSecretRef{
		Name: name,
		Keys: keys,
	}
	return b
}

// WithTLS sets TLS configuration.
func (b *DatabaseInstanceBuilder) WithTLS(enabled bool, mode string) *DatabaseInstanceBuilder {
	b.instance.Spec.TLS = &dbopsv1alpha1.TLSConfig{
		Enabled: enabled,
		Mode:    mode,
	}
	return b
}

// WithHealthCheck sets health check configuration.
func (b *DatabaseInstanceBuilder) WithHealthCheck(enabled bool, intervalSeconds, timeoutSeconds int32) *DatabaseInstanceBuilder {
	b.instance.Spec.HealthCheck = &dbopsv1alpha1.HealthCheckConfig{
		Enabled:         enabled,
		IntervalSeconds: intervalSeconds,
		TimeoutSeconds:  timeoutSeconds,
	}
	return b
}

// WithLabels sets labels on the instance.
func (b *DatabaseInstanceBuilder) WithLabels(labels map[string]string) *DatabaseInstanceBuilder {
	b.instance.Labels = labels
	return b
}

// WithAnnotations sets annotations on the instance.
func (b *DatabaseInstanceBuilder) WithAnnotations(annotations map[string]string) *DatabaseInstanceBuilder {
	b.instance.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the instance.
func (b *DatabaseInstanceBuilder) WithFinalizers(finalizers ...string) *DatabaseInstanceBuilder {
	b.instance.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the instance.
func (b *DatabaseInstanceBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseInstanceBuilder {
	b.instance.Status.Phase = phase
	b.instance.Status.Message = message
	return b
}

// WithStatusReady sets the instance status to Ready.
func (b *DatabaseInstanceBuilder) WithStatusReady() *DatabaseInstanceBuilder {
	b.instance.Status.Phase = dbopsv1alpha1.PhaseReady
	return b
}

// Build returns the constructed DatabaseInstance.
func (b *DatabaseInstanceBuilder) Build() *dbopsv1alpha1.DatabaseInstance {
	return b.instance.DeepCopy()
}

// DatabaseBuilder provides a fluent interface for building Database objects.
type DatabaseBuilder struct {
	database *dbopsv1alpha1.Database
}

// NewDatabase creates a new DatabaseBuilder with defaults.
func NewDatabase(name, namespace string) *DatabaseBuilder {
	return &DatabaseBuilder{
		database: &dbopsv1alpha1.Database{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "Database",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseSpec{
				Name:               name,
				DeletionPolicy:     dbopsv1alpha1.DeletionPolicyRetain,
				DeletionProtection: true,
			},
		},
	}
}

// WithDatabaseName sets the database name in the spec.
func (b *DatabaseBuilder) WithDatabaseName(name string) *DatabaseBuilder {
	b.database.Spec.Name = name
	return b
}

// WithInstanceRef sets the instance reference.
func (b *DatabaseBuilder) WithInstanceRef(name string) *DatabaseBuilder {
	b.database.Spec.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name: name,
	}
	return b
}

// WithInstanceRefAndNamespace sets the instance reference with namespace.
func (b *DatabaseBuilder) WithInstanceRefAndNamespace(name, namespace string) *DatabaseBuilder {
	b.database.Spec.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithDeletionPolicy sets the deletion policy.
func (b *DatabaseBuilder) WithDeletionPolicy(policy dbopsv1alpha1.DeletionPolicy) *DatabaseBuilder {
	b.database.Spec.DeletionPolicy = policy
	return b
}

// WithDeletionProtection sets deletion protection.
func (b *DatabaseBuilder) WithDeletionProtection(enabled bool) *DatabaseBuilder {
	b.database.Spec.DeletionProtection = enabled
	return b
}

// WithLabels sets labels on the database.
func (b *DatabaseBuilder) WithLabels(labels map[string]string) *DatabaseBuilder {
	b.database.Labels = labels
	return b
}

// WithAnnotations sets annotations on the database.
func (b *DatabaseBuilder) WithAnnotations(annotations map[string]string) *DatabaseBuilder {
	b.database.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the database.
func (b *DatabaseBuilder) WithFinalizers(finalizers ...string) *DatabaseBuilder {
	b.database.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the database.
func (b *DatabaseBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseBuilder {
	b.database.Status.Phase = phase
	b.database.Status.Message = message
	return b
}

// WithStatusReady sets the database status to Ready.
func (b *DatabaseBuilder) WithStatusReady() *DatabaseBuilder {
	b.database.Status.Phase = dbopsv1alpha1.PhaseReady
	return b
}

// Build returns the constructed Database.
func (b *DatabaseBuilder) Build() *dbopsv1alpha1.Database {
	return b.database.DeepCopy()
}

// DatabaseUserBuilder provides a fluent interface for building DatabaseUser objects.
type DatabaseUserBuilder struct {
	user *dbopsv1alpha1.DatabaseUser
}

// NewDatabaseUser creates a new DatabaseUserBuilder with defaults.
func NewDatabaseUser(name, namespace string) *DatabaseUserBuilder {
	return &DatabaseUserBuilder{
		user: &dbopsv1alpha1.DatabaseUser{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseUser",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseUserSpec{
				Username: name,
			},
		},
	}
}

// WithUsername sets the username.
func (b *DatabaseUserBuilder) WithUsername(username string) *DatabaseUserBuilder {
	b.user.Spec.Username = username
	return b
}

// WithInstanceRef sets the instance reference.
func (b *DatabaseUserBuilder) WithInstanceRef(name string) *DatabaseUserBuilder {
	b.user.Spec.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name: name,
	}
	return b
}

// WithInstanceRefAndNamespace sets the instance reference with namespace.
func (b *DatabaseUserBuilder) WithInstanceRefAndNamespace(name, namespace string) *DatabaseUserBuilder {
	b.user.Spec.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithPasswordSecret sets the password secret configuration.
func (b *DatabaseUserBuilder) WithPasswordSecret(secretName string, generate bool) *DatabaseUserBuilder {
	b.user.Spec.PasswordSecret = &dbopsv1alpha1.PasswordConfig{
		SecretName: secretName,
		Generate:   generate,
	}
	return b
}

// WithExistingPasswordSecret sets an existing password secret.
func (b *DatabaseUserBuilder) WithExistingPasswordSecret(name, key string) *DatabaseUserBuilder {
	b.user.Spec.ExistingPasswordSecret = &dbopsv1alpha1.ExistingPasswordSecret{
		Name: name,
		Key:  key,
	}
	return b
}

// WithLabels sets labels on the user.
func (b *DatabaseUserBuilder) WithLabels(labels map[string]string) *DatabaseUserBuilder {
	b.user.Labels = labels
	return b
}

// WithAnnotations sets annotations on the user.
func (b *DatabaseUserBuilder) WithAnnotations(annotations map[string]string) *DatabaseUserBuilder {
	b.user.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the user.
func (b *DatabaseUserBuilder) WithFinalizers(finalizers ...string) *DatabaseUserBuilder {
	b.user.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the user.
func (b *DatabaseUserBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseUserBuilder {
	b.user.Status.Phase = phase
	b.user.Status.Message = message
	return b
}

// WithStatusReady sets the user status to Ready.
func (b *DatabaseUserBuilder) WithStatusReady() *DatabaseUserBuilder {
	b.user.Status.Phase = dbopsv1alpha1.PhaseReady
	return b
}

// Build returns the constructed DatabaseUser.
func (b *DatabaseUserBuilder) Build() *dbopsv1alpha1.DatabaseUser {
	return b.user.DeepCopy()
}

// DatabaseRoleBuilder provides a fluent interface for building DatabaseRole objects.
type DatabaseRoleBuilder struct {
	role *dbopsv1alpha1.DatabaseRole
}

// NewDatabaseRole creates a new DatabaseRoleBuilder with defaults.
func NewDatabaseRole(name, namespace string) *DatabaseRoleBuilder {
	return &DatabaseRoleBuilder{
		role: &dbopsv1alpha1.DatabaseRole{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseRole",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseRoleSpec{
				RoleName: name,
			},
		},
	}
}

// WithRoleName sets the role name.
func (b *DatabaseRoleBuilder) WithRoleName(roleName string) *DatabaseRoleBuilder {
	b.role.Spec.RoleName = roleName
	return b
}

// WithInstanceRef sets the instance reference.
func (b *DatabaseRoleBuilder) WithInstanceRef(name string) *DatabaseRoleBuilder {
	b.role.Spec.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name: name,
	}
	return b
}

// WithInstanceRefAndNamespace sets the instance reference with namespace.
func (b *DatabaseRoleBuilder) WithInstanceRefAndNamespace(name, namespace string) *DatabaseRoleBuilder {
	b.role.Spec.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithLabels sets labels on the role.
func (b *DatabaseRoleBuilder) WithLabels(labels map[string]string) *DatabaseRoleBuilder {
	b.role.Labels = labels
	return b
}

// WithAnnotations sets annotations on the role.
func (b *DatabaseRoleBuilder) WithAnnotations(annotations map[string]string) *DatabaseRoleBuilder {
	b.role.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the role.
func (b *DatabaseRoleBuilder) WithFinalizers(finalizers ...string) *DatabaseRoleBuilder {
	b.role.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the role.
func (b *DatabaseRoleBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseRoleBuilder {
	b.role.Status.Phase = phase
	b.role.Status.Message = message
	return b
}

// WithStatusReady sets the role status to Ready.
func (b *DatabaseRoleBuilder) WithStatusReady() *DatabaseRoleBuilder {
	b.role.Status.Phase = dbopsv1alpha1.PhaseReady
	return b
}

// Build returns the constructed DatabaseRole.
func (b *DatabaseRoleBuilder) Build() *dbopsv1alpha1.DatabaseRole {
	return b.role.DeepCopy()
}

// DatabaseGrantBuilder provides a fluent interface for building DatabaseGrant objects.
type DatabaseGrantBuilder struct {
	grant *dbopsv1alpha1.DatabaseGrant
}

// NewDatabaseGrant creates a new DatabaseGrantBuilder with defaults.
func NewDatabaseGrant(name, namespace string) *DatabaseGrantBuilder {
	return &DatabaseGrantBuilder{
		grant: &dbopsv1alpha1.DatabaseGrant{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseGrant",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseGrantSpec{},
		},
	}
}

// WithUserRef sets the user reference.
func (b *DatabaseGrantBuilder) WithUserRef(name string) *DatabaseGrantBuilder {
	b.grant.Spec.UserRef = dbopsv1alpha1.UserReference{
		Name: name,
	}
	return b
}

// WithUserRefAndNamespace sets the user reference with namespace.
func (b *DatabaseGrantBuilder) WithUserRefAndNamespace(name, namespace string) *DatabaseGrantBuilder {
	b.grant.Spec.UserRef = dbopsv1alpha1.UserReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithDatabaseRef sets the database reference.
func (b *DatabaseGrantBuilder) WithDatabaseRef(name string) *DatabaseGrantBuilder {
	b.grant.Spec.DatabaseRef = &dbopsv1alpha1.DatabaseReference{
		Name: name,
	}
	return b
}

// WithDatabaseRefAndNamespace sets the database reference with namespace.
func (b *DatabaseGrantBuilder) WithDatabaseRefAndNamespace(name, namespace string) *DatabaseGrantBuilder {
	b.grant.Spec.DatabaseRef = &dbopsv1alpha1.DatabaseReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithLabels sets labels on the grant.
func (b *DatabaseGrantBuilder) WithLabels(labels map[string]string) *DatabaseGrantBuilder {
	b.grant.Labels = labels
	return b
}

// WithAnnotations sets annotations on the grant.
func (b *DatabaseGrantBuilder) WithAnnotations(annotations map[string]string) *DatabaseGrantBuilder {
	b.grant.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the grant.
func (b *DatabaseGrantBuilder) WithFinalizers(finalizers ...string) *DatabaseGrantBuilder {
	b.grant.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the grant.
func (b *DatabaseGrantBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseGrantBuilder {
	b.grant.Status.Phase = phase
	b.grant.Status.Message = message
	return b
}

// WithStatusReady sets the grant status to Ready.
func (b *DatabaseGrantBuilder) WithStatusReady() *DatabaseGrantBuilder {
	b.grant.Status.Phase = dbopsv1alpha1.PhaseReady
	return b
}

// Build returns the constructed DatabaseGrant.
func (b *DatabaseGrantBuilder) Build() *dbopsv1alpha1.DatabaseGrant {
	return b.grant.DeepCopy()
}

// DatabaseBackupBuilder provides a fluent interface for building DatabaseBackup objects.
type DatabaseBackupBuilder struct {
	backup *dbopsv1alpha1.DatabaseBackup
}

// NewDatabaseBackup creates a new DatabaseBackupBuilder with defaults.
func NewDatabaseBackup(name, namespace string) *DatabaseBackupBuilder {
	return &DatabaseBackupBuilder{
		backup: &dbopsv1alpha1.DatabaseBackup{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseBackup",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseBackupSpec{
				ActiveDeadlineSeconds: 3600,
			},
		},
	}
}

// WithDatabaseRef sets the database reference.
func (b *DatabaseBackupBuilder) WithDatabaseRef(name string) *DatabaseBackupBuilder {
	b.backup.Spec.DatabaseRef = dbopsv1alpha1.DatabaseReference{
		Name: name,
	}
	return b
}

// WithDatabaseRefAndNamespace sets the database reference with namespace.
func (b *DatabaseBackupBuilder) WithDatabaseRefAndNamespace(name, namespace string) *DatabaseBackupBuilder {
	b.backup.Spec.DatabaseRef = dbopsv1alpha1.DatabaseReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithS3Storage sets S3 storage configuration.
func (b *DatabaseBackupBuilder) WithS3Storage(bucket, region, secretName string) *DatabaseBackupBuilder {
	b.backup.Spec.Storage = dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeS3,
		S3: &dbopsv1alpha1.S3StorageConfig{
			Bucket: bucket,
			Region: region,
			SecretRef: dbopsv1alpha1.S3SecretRef{
				Name: secretName,
			},
		},
	}
	return b
}

// WithGCSStorage sets GCS storage configuration.
func (b *DatabaseBackupBuilder) WithGCSStorage(bucket, secretName, secretKey string) *DatabaseBackupBuilder {
	b.backup.Spec.Storage = dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeGCS,
		GCS: &dbopsv1alpha1.GCSStorageConfig{
			Bucket: bucket,
			SecretRef: dbopsv1alpha1.SecretKeySelector{
				Name: secretName,
				Key:  secretKey,
			},
		},
	}
	return b
}

// WithPVCStorage sets PVC storage configuration.
func (b *DatabaseBackupBuilder) WithPVCStorage(claimName, subPath string) *DatabaseBackupBuilder {
	b.backup.Spec.Storage = dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypePVC,
		PVC: &dbopsv1alpha1.PVCStorageConfig{
			ClaimName: claimName,
			SubPath:   subPath,
		},
	}
	return b
}

// WithCompression sets compression configuration.
func (b *DatabaseBackupBuilder) WithCompression(algorithm dbopsv1alpha1.CompressionAlgorithm, level int32) *DatabaseBackupBuilder {
	b.backup.Spec.Compression = &dbopsv1alpha1.CompressionConfig{
		Enabled:   true,
		Algorithm: algorithm,
		Level:     level,
	}
	return b
}

// WithEncryption sets encryption configuration.
func (b *DatabaseBackupBuilder) WithEncryption(algorithm dbopsv1alpha1.EncryptionAlgorithm, secretName, secretKey string) *DatabaseBackupBuilder {
	b.backup.Spec.Encryption = &dbopsv1alpha1.EncryptionConfig{
		Enabled:   true,
		Algorithm: algorithm,
		SecretRef: &dbopsv1alpha1.SecretKeySelector{
			Name: secretName,
			Key:  secretKey,
		},
	}
	return b
}

// WithTTL sets the TTL for the backup.
func (b *DatabaseBackupBuilder) WithTTL(ttl string) *DatabaseBackupBuilder {
	b.backup.Spec.TTL = ttl
	return b
}

// WithActiveDeadlineSeconds sets the active deadline.
func (b *DatabaseBackupBuilder) WithActiveDeadlineSeconds(seconds int64) *DatabaseBackupBuilder {
	b.backup.Spec.ActiveDeadlineSeconds = seconds
	return b
}

// WithLabels sets labels on the backup.
func (b *DatabaseBackupBuilder) WithLabels(labels map[string]string) *DatabaseBackupBuilder {
	b.backup.Labels = labels
	return b
}

// WithAnnotations sets annotations on the backup.
func (b *DatabaseBackupBuilder) WithAnnotations(annotations map[string]string) *DatabaseBackupBuilder {
	b.backup.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the backup.
func (b *DatabaseBackupBuilder) WithFinalizers(finalizers ...string) *DatabaseBackupBuilder {
	b.backup.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the backup.
func (b *DatabaseBackupBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseBackupBuilder {
	b.backup.Status.Phase = phase
	b.backup.Status.Message = message
	return b
}

// WithStatusCompleted sets the backup status to Completed.
func (b *DatabaseBackupBuilder) WithStatusCompleted() *DatabaseBackupBuilder {
	b.backup.Status.Phase = dbopsv1alpha1.PhaseCompleted
	return b
}

// Build returns the constructed DatabaseBackup.
func (b *DatabaseBackupBuilder) Build() *dbopsv1alpha1.DatabaseBackup {
	return b.backup.DeepCopy()
}

// DatabaseRestoreBuilder provides a fluent interface for building DatabaseRestore objects.
type DatabaseRestoreBuilder struct {
	restore *dbopsv1alpha1.DatabaseRestore
}

// NewDatabaseRestore creates a new DatabaseRestoreBuilder with defaults.
func NewDatabaseRestore(name, namespace string) *DatabaseRestoreBuilder {
	return &DatabaseRestoreBuilder{
		restore: &dbopsv1alpha1.DatabaseRestore{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseRestore",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseRestoreSpec{
				ActiveDeadlineSeconds: 7200,
				Target:                dbopsv1alpha1.RestoreTarget{},
			},
		},
	}
}

// WithBackupRef sets the backup reference.
func (b *DatabaseRestoreBuilder) WithBackupRef(name string) *DatabaseRestoreBuilder {
	b.restore.Spec.BackupRef = &dbopsv1alpha1.BackupReference{
		Name: name,
	}
	return b
}

// WithBackupRefAndNamespace sets the backup reference with namespace.
func (b *DatabaseRestoreBuilder) WithBackupRefAndNamespace(name, namespace string) *DatabaseRestoreBuilder {
	b.restore.Spec.BackupRef = &dbopsv1alpha1.BackupReference{
		Name:      name,
		Namespace: namespace,
	}
	return b
}

// WithTargetInstance sets the target instance.
func (b *DatabaseRestoreBuilder) WithTargetInstance(name string) *DatabaseRestoreBuilder {
	b.restore.Spec.Target.InstanceRef = &dbopsv1alpha1.InstanceReference{
		Name: name,
	}
	return b
}

// WithTargetDatabase sets the target database name.
func (b *DatabaseRestoreBuilder) WithTargetDatabase(name string) *DatabaseRestoreBuilder {
	b.restore.Spec.Target.DatabaseName = name
	return b
}

// WithInPlace enables in-place restore.
func (b *DatabaseRestoreBuilder) WithInPlace(databaseRefName string) *DatabaseRestoreBuilder {
	b.restore.Spec.Target.InPlace = true
	b.restore.Spec.Target.DatabaseRef = &dbopsv1alpha1.DatabaseReference{
		Name: databaseRefName,
	}
	return b
}

// WithConfirmation sets the data loss confirmation.
func (b *DatabaseRestoreBuilder) WithConfirmation(acknowledgeDataLoss string) *DatabaseRestoreBuilder {
	b.restore.Spec.Confirmation = &dbopsv1alpha1.RestoreConfirmation{
		AcknowledgeDataLoss: acknowledgeDataLoss,
	}
	return b
}

// WithActiveDeadlineSeconds sets the active deadline.
func (b *DatabaseRestoreBuilder) WithActiveDeadlineSeconds(seconds int64) *DatabaseRestoreBuilder {
	b.restore.Spec.ActiveDeadlineSeconds = seconds
	return b
}

// WithLabels sets labels on the restore.
func (b *DatabaseRestoreBuilder) WithLabels(labels map[string]string) *DatabaseRestoreBuilder {
	b.restore.Labels = labels
	return b
}

// WithAnnotations sets annotations on the restore.
func (b *DatabaseRestoreBuilder) WithAnnotations(annotations map[string]string) *DatabaseRestoreBuilder {
	b.restore.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the restore.
func (b *DatabaseRestoreBuilder) WithFinalizers(finalizers ...string) *DatabaseRestoreBuilder {
	b.restore.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the restore.
func (b *DatabaseRestoreBuilder) WithStatus(phase dbopsv1alpha1.Phase, message string) *DatabaseRestoreBuilder {
	b.restore.Status.Phase = phase
	b.restore.Status.Message = message
	return b
}

// WithStatusCompleted sets the restore status to Completed.
func (b *DatabaseRestoreBuilder) WithStatusCompleted() *DatabaseRestoreBuilder {
	b.restore.Status.Phase = dbopsv1alpha1.PhaseCompleted
	return b
}

// Build returns the constructed DatabaseRestore.
func (b *DatabaseRestoreBuilder) Build() *dbopsv1alpha1.DatabaseRestore {
	return b.restore.DeepCopy()
}

// DatabaseBackupScheduleBuilder provides a fluent interface for building DatabaseBackupSchedule objects.
type DatabaseBackupScheduleBuilder struct {
	schedule *dbopsv1alpha1.DatabaseBackupSchedule
}

// NewDatabaseBackupSchedule creates a new DatabaseBackupScheduleBuilder with defaults.
func NewDatabaseBackupSchedule(name, namespace string) *DatabaseBackupScheduleBuilder {
	return &DatabaseBackupScheduleBuilder{
		schedule: &dbopsv1alpha1.DatabaseBackupSchedule{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "dbops.dbprovision.io/v1alpha1",
				Kind:       "DatabaseBackupSchedule",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: dbopsv1alpha1.DatabaseBackupScheduleSpec{
				Schedule:                      "0 0 * * *", // Daily at midnight
				Timezone:                      "UTC",
				ConcurrencyPolicy:             dbopsv1alpha1.ConcurrencyPolicyForbid,
				SuccessfulBackupsHistoryLimit: 5,
				FailedBackupsHistoryLimit:     3,
			},
		},
	}
}

// WithSchedule sets the cron schedule.
func (b *DatabaseBackupScheduleBuilder) WithSchedule(schedule string) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.Schedule = schedule
	return b
}

// WithTimezone sets the timezone.
func (b *DatabaseBackupScheduleBuilder) WithTimezone(timezone string) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.Timezone = timezone
	return b
}

// WithPaused sets the paused state.
func (b *DatabaseBackupScheduleBuilder) WithPaused(paused bool) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.Paused = paused
	return b
}

// WithConcurrencyPolicy sets the concurrency policy.
func (b *DatabaseBackupScheduleBuilder) WithConcurrencyPolicy(policy dbopsv1alpha1.ConcurrencyPolicy) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.ConcurrencyPolicy = policy
	return b
}

// WithBackupTemplate sets the backup template spec.
func (b *DatabaseBackupScheduleBuilder) WithBackupTemplate(databaseRefName string, storage dbopsv1alpha1.StorageConfig) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.Template = dbopsv1alpha1.BackupTemplateSpec{
		Spec: dbopsv1alpha1.DatabaseBackupSpec{
			DatabaseRef: dbopsv1alpha1.DatabaseReference{
				Name: databaseRefName,
			},
			Storage:               storage,
			ActiveDeadlineSeconds: 3600,
		},
	}
	return b
}

// WithRetention sets the retention policy.
func (b *DatabaseBackupScheduleBuilder) WithRetention(keepLast, keepDaily, keepWeekly, keepMonthly int32) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.Retention = &dbopsv1alpha1.RetentionPolicy{
		KeepLast:    keepLast,
		KeepDaily:   keepDaily,
		KeepWeekly:  keepWeekly,
		KeepMonthly: keepMonthly,
	}
	return b
}

// WithHistoryLimits sets the history limits.
func (b *DatabaseBackupScheduleBuilder) WithHistoryLimits(successful, failed int32) *DatabaseBackupScheduleBuilder {
	b.schedule.Spec.SuccessfulBackupsHistoryLimit = successful
	b.schedule.Spec.FailedBackupsHistoryLimit = failed
	return b
}

// WithLabels sets labels on the schedule.
func (b *DatabaseBackupScheduleBuilder) WithLabels(labels map[string]string) *DatabaseBackupScheduleBuilder {
	b.schedule.Labels = labels
	return b
}

// WithAnnotations sets annotations on the schedule.
func (b *DatabaseBackupScheduleBuilder) WithAnnotations(annotations map[string]string) *DatabaseBackupScheduleBuilder {
	b.schedule.Annotations = annotations
	return b
}

// WithFinalizers sets finalizers on the schedule.
func (b *DatabaseBackupScheduleBuilder) WithFinalizers(finalizers ...string) *DatabaseBackupScheduleBuilder {
	b.schedule.Finalizers = finalizers
	return b
}

// WithStatus sets the status on the schedule.
func (b *DatabaseBackupScheduleBuilder) WithStatus(phase dbopsv1alpha1.Phase) *DatabaseBackupScheduleBuilder {
	b.schedule.Status.Phase = phase
	return b
}

// WithStatusActive sets the schedule status to Active.
func (b *DatabaseBackupScheduleBuilder) WithStatusActive() *DatabaseBackupScheduleBuilder {
	b.schedule.Status.Phase = dbopsv1alpha1.PhaseActive
	return b
}

// Build returns the constructed DatabaseBackupSchedule.
func (b *DatabaseBackupScheduleBuilder) Build() *dbopsv1alpha1.DatabaseBackupSchedule {
	return b.schedule.DeepCopy()
}
