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

package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/storage"
	"github.com/db-provision-operator/internal/util"
)

// Repository handles backup storage/adapter operations.
type Repository struct {
	client        client.Client
	secretManager *secret.Manager
	logger        logr.Logger
}

// RepositoryConfig holds dependencies for the repository.
type RepositoryConfig struct {
	Client        client.Client
	SecretManager *secret.Manager
	Logger        logr.Logger
}

// NewRepository creates a new backup repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// ExecuteBackup performs the actual backup operation.
func (r *Repository) ExecuteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*BackupExecutionResult, error) {
	log := r.logger.WithValues("backup", backup.Name, "namespace", backup.Namespace)

	// Get the database
	database, err := r.GetDatabase(ctx, backup)
	if err != nil {
		return nil, fmt.Errorf("get database: %w", err)
	}

	// Get the instance
	instance, err := r.GetInstance(ctx, database)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}

	// Get credentials
	creds, err := r.getCredentialsWithRetry(ctx, instance)
	if err != nil {
		return nil, fmt.Errorf("get credentials: %w", err)
	}

	// Get TLS credentials if enabled
	tlsCreds, err := r.getTLSCredentials(ctx, instance)
	if err != nil {
		return nil, fmt.Errorf("get TLS credentials: %w", err)
	}

	// Build connection config
	var tlsCA, tlsCert, tlsKey []byte
	if tlsCreds != nil {
		tlsCA = tlsCreds.CA
		tlsCert = tlsCreds.Cert
		tlsKey = tlsCreds.Key
	}
	connConfig := adapter.BuildConnectionConfig(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create database adapter
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, connConfig)
	if err != nil {
		return nil, fmt.Errorf("create adapter: %w", err)
	}
	defer func() { _ = dbAdapter.Close() }()

	// Connect with retry
	retryConfig := util.ConnectionRetryConfig()
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		return dbAdapter.Connect(ctx)
	})
	if result.LastError != nil {
		return nil, fmt.Errorf("connect to database: %w", result.LastError)
	}

	// Create backup writer
	now := metav1.Now()
	backupWriter, err := storage.NewBackupWriter(ctx, &storage.BackupWriterConfig{
		StorageConfig:     &backup.Spec.Storage,
		CompressionConfig: backup.Spec.Compression,
		EncryptionConfig:  backup.Spec.Encryption,
		SecretManager:     r.secretManager,
		Namespace:         backup.Namespace,
		BackupName:        backup.Name,
		DatabaseName:      database.Spec.Name,
		Timestamp:         now.Time,
		Extension:         r.getBackupExtension(instance.Spec.Engine, backup),
	})
	if err != nil {
		return nil, fmt.Errorf("create backup writer: %w", err)
	}

	// Build backup options
	backupOpts := r.buildBackupOptions(backup, database, instance.Spec.Engine, backupWriter)

	// Execute backup with retry
	backupRetryConfig := util.BackupRetryConfig()
	var backupResult *adapterpkg.BackupResult
	startTime := time.Now()

	result = util.RetryWithBackoff(ctx, backupRetryConfig, func() error {
		var err error
		backupResult, err = dbAdapter.Backup(ctx, backupOpts)
		return err
	})
	if result.LastError != nil {
		return nil, fmt.Errorf("execute backup: %w", result.LastError)
	}

	// Close backup writer to finalize (compress, encrypt, write to storage)
	if err := backupWriter.Close(); err != nil {
		return nil, fmt.Errorf("finalize backup: %w", err)
	}

	duration := time.Since(startTime)
	log.Info("Backup executed successfully",
		"path", backupWriter.Path(),
		"size", backupWriter.UncompressedSize(),
		"duration", duration)

	return &BackupExecutionResult{
		Path:                backupWriter.Path(),
		SizeBytes:           backupWriter.UncompressedSize(),
		CompressedSizeBytes: backupResult.CompressedSizeBytes,
		Checksum:            backupResult.Checksum,
		Format:              backupResult.Format,
		Duration:            duration,
		Instance:            instance.Name,
		Database:            database.Spec.Name,
		Engine:              string(instance.Spec.Engine),
		Version:             instance.Status.Version,
	}, nil
}

// BackupExecutionResult contains the result of executing a backup.
type BackupExecutionResult struct {
	Path                string
	SizeBytes           int64
	CompressedSizeBytes int64
	Checksum            string
	Format              string
	Duration            time.Duration
	Instance            string
	Database            string
	Engine              string
	Version             string
}

// DeleteBackup removes a backup file from storage.
func (r *Repository) DeleteBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) error {
	if backup.Status.Backup == nil || backup.Status.Backup.Path == "" {
		return nil
	}

	log := r.logger.WithValues("backup", backup.Name, "path", backup.Status.Backup.Path)
	log.Info("Deleting backup file from storage")

	err := storage.DeleteBackup(ctx, &storage.Config{
		StorageConfig: &backup.Spec.Storage,
		SecretManager: r.secretManager,
		Namespace:     backup.Namespace,
	}, backup.Status.Backup.Path)
	if err != nil {
		return fmt.Errorf("delete backup file: %w", err)
	}

	log.Info("Backup file deleted successfully")
	return nil
}

// GetDatabase retrieves the Database referenced by the backup.
func (r *Repository) GetDatabase(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (*dbopsv1alpha1.Database, error) {
	dbNamespace := backup.Namespace
	if backup.Spec.DatabaseRef.Namespace != "" {
		dbNamespace = backup.Spec.DatabaseRef.Namespace
	}

	database := &dbopsv1alpha1.Database{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.DatabaseRef.Name,
		Namespace: dbNamespace,
	}, database); err != nil {
		return nil, err
	}

	return database, nil
}

// GetInstance retrieves the DatabaseInstance for a database.
func (r *Repository) GetInstance(ctx context.Context, database *dbopsv1alpha1.Database) (*dbopsv1alpha1.DatabaseInstance, error) {
	instanceNamespace := database.Namespace
	if database.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = database.Spec.InstanceRef.Namespace
	}

	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      database.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// GetEngine returns the database engine type for a backup.
func (r *Repository) GetEngine(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (string, error) {
	database, err := r.GetDatabase(ctx, backup)
	if err != nil {
		return "", err
	}

	instance, err := r.GetInstance(ctx, database)
	if err != nil {
		return "", err
	}

	return string(instance.Spec.Engine), nil
}

// getCredentialsWithRetry gets credentials with retry logic.
func (r *Repository) getCredentialsWithRetry(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*secret.Credentials, error) {
	var creds *secret.Credentials
	retryConfig := util.ConnectionRetryConfig()
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		creds, err = r.secretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
		return err
	})
	if result.LastError != nil {
		return nil, result.LastError
	}
	return creds, nil
}

// getTLSCredentials gets TLS credentials if TLS is enabled.
func (r *Repository) getTLSCredentials(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*secret.TLSCredentials, error) {
	if instance.Spec.TLS == nil || !instance.Spec.TLS.Enabled {
		return nil, nil
	}

	var tlsCreds *secret.TLSCredentials
	retryConfig := util.ConnectionRetryConfig()
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		tlsCreds, err = r.secretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		return err
	})
	if result.LastError != nil {
		return nil, result.LastError
	}
	return tlsCreds, nil
}

// getBackupExtension returns the appropriate file extension based on engine and backup config.
func (r *Repository) getBackupExtension(engine dbopsv1alpha1.EngineType, backup *dbopsv1alpha1.DatabaseBackup) string {
	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if backup.Spec.Postgres != nil {
			switch backup.Spec.Postgres.Format {
			case "plain":
				return ".sql"
			case "custom":
				return ".dump"
			case "directory":
				return ".dir"
			case "tar":
				return ".tar"
			}
		}
		return ".dump" // default for PostgreSQL
	case dbopsv1alpha1.EngineTypeMySQL:
		return ".sql"
	default:
		return ".bak"
	}
}

// buildBackupOptions builds backup options from the spec.
func (r *Repository) buildBackupOptions(backup *dbopsv1alpha1.DatabaseBackup, database *dbopsv1alpha1.Database, engine dbopsv1alpha1.EngineType, writer *storage.BackupWriter) adapterpkg.BackupOptions {
	opts := adapterpkg.BackupOptions{
		Database: database.Spec.Name,
		BackupID: string(backup.UID),
		Writer:   writer,
	}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if backup.Spec.Postgres != nil {
			opts.Method = string(backup.Spec.Postgres.Method)
			opts.Format = string(backup.Spec.Postgres.Format)
			opts.Jobs = backup.Spec.Postgres.Jobs
			opts.DataOnly = backup.Spec.Postgres.DataOnly
			opts.SchemaOnly = backup.Spec.Postgres.SchemaOnly
			opts.Blobs = backup.Spec.Postgres.Blobs
			opts.NoOwner = backup.Spec.Postgres.NoOwner
			opts.NoPrivileges = backup.Spec.Postgres.NoPrivileges
			opts.Schemas = backup.Spec.Postgres.Schemas
			opts.ExcludeSchemas = backup.Spec.Postgres.ExcludeSchemas
			opts.Tables = backup.Spec.Postgres.Tables
			opts.ExcludeTables = backup.Spec.Postgres.ExcludeTables
			opts.LockWaitTimeout = backup.Spec.Postgres.LockWaitTimeout
		} else {
			// Set defaults for PostgreSQL
			opts.Method = "pg_dump"
			opts.Format = "custom"
			opts.Jobs = 1
			opts.Blobs = true
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if backup.Spec.MySQL != nil {
			opts.SingleTransaction = backup.Spec.MySQL.SingleTransaction
			opts.Quick = backup.Spec.MySQL.Quick
			opts.LockTables = backup.Spec.MySQL.LockTables
			opts.Routines = backup.Spec.MySQL.Routines
			opts.Triggers = backup.Spec.MySQL.Triggers
			opts.Events = backup.Spec.MySQL.Events
			opts.ExtendedInsert = backup.Spec.MySQL.ExtendedInsert
			opts.SetGtidPurged = string(backup.Spec.MySQL.SetGtidPurged)
		} else {
			// Set defaults for MySQL
			opts.SingleTransaction = true
			opts.Quick = true
			opts.Routines = true
			opts.Triggers = true
		}
	}

	return opts
}
