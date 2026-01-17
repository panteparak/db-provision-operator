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

package restore

import (
	"context"
	"fmt"
	"io"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/storage"
	"github.com/db-provision-operator/internal/util"
)

// Repository handles restore operations via the adapter layer.
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

// NewRepository creates a new restore repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// GetBackup retrieves the referenced DatabaseBackup.
func (r *Repository) GetBackup(ctx context.Context, namespace string, backupRef *dbopsv1alpha1.BackupReference) (*dbopsv1alpha1.DatabaseBackup, error) {
	backupNamespace := namespace
	if backupRef.Namespace != "" {
		backupNamespace = backupRef.Namespace
	}

	backup := &dbopsv1alpha1.DatabaseBackup{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      backupRef.Name,
		Namespace: backupNamespace,
	}, backup); err != nil {
		return nil, err
	}

	return backup, nil
}

// GetDatabase retrieves the referenced Database.
func (r *Repository) GetDatabase(ctx context.Context, namespace string, dbRef *dbopsv1alpha1.DatabaseReference) (*dbopsv1alpha1.Database, error) {
	dbNamespace := namespace
	if dbRef.Namespace != "" {
		dbNamespace = dbRef.Namespace
	}

	db := &dbopsv1alpha1.Database{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      dbRef.Name,
		Namespace: dbNamespace,
	}, db); err != nil {
		return nil, err
	}

	return db, nil
}

// GetInstance retrieves the referenced DatabaseInstance.
func (r *Repository) GetInstance(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (*dbopsv1alpha1.DatabaseInstance, error) {
	instanceNamespace := namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      instanceRef.Name,
		Namespace: instanceNamespace,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// CreateRestoreReader creates a reader for the backup data.
func (r *Repository) CreateRestoreReader(ctx context.Context, cfg *storage.RestoreReaderConfig) (io.ReadCloser, error) {
	return storage.NewRestoreReader(ctx, cfg)
}

// ExecuteRestore performs the actual database restore operation.
func (r *Repository) ExecuteRestore(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, opts adapterpkg.RestoreOptions) (*adapterpkg.RestoreResult, error) {
	// Get credentials with retry
	retryConfig := util.ConnectionRetryConfig()
	var creds *secret.Credentials
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		creds, err = r.secretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
		return err
	})
	if result.LastError != nil {
		return nil, fmt.Errorf("failed to get credentials after %d attempts: %w", result.Attempts, result.LastError)
	}

	// Get TLS credentials if TLS is enabled
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		result := util.RetryWithBackoff(ctx, retryConfig, func() error {
			tlsCreds, err := r.secretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
			if err != nil {
				return err
			}
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
			return nil
		})
		if result.LastError != nil {
			return nil, fmt.Errorf("failed to get TLS credentials: %w", result.LastError)
		}
	}

	// Build connection config
	connConfig := adapter.BuildConnectionConfig(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create database adapter
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, connConfig)
	if err != nil {
		return nil, fmt.Errorf("unsupported database engine %s: %w", instance.Spec.Engine, err)
	}
	defer func() { _ = dbAdapter.Close() }()

	// Connect with retry
	result = util.RetryWithBackoff(ctx, retryConfig, func() error {
		return dbAdapter.Connect(ctx)
	})
	if result.LastError != nil {
		return nil, fmt.Errorf("failed to connect to database after %d attempts: %w", result.Attempts, result.LastError)
	}

	// Execute restore with retry
	restoreRetryConfig := util.BackupRetryConfig()
	var restoreResult *adapterpkg.RestoreResult
	result = util.RetryWithBackoff(ctx, restoreRetryConfig, func() error {
		var err error
		restoreResult, err = dbAdapter.Restore(ctx, opts)
		return err
	})
	if result.LastError != nil {
		return nil, fmt.Errorf("restore failed after %d attempts: %w", result.Attempts, result.LastError)
	}

	return restoreResult, nil
}

// GetEngine returns the database engine type for a given instance.
func (r *Repository) GetEngine(ctx context.Context, namespace string, instanceRef *dbopsv1alpha1.InstanceReference) (string, error) {
	instance, err := r.GetInstance(ctx, namespace, instanceRef)
	if err != nil {
		return "", err
	}
	return string(instance.Spec.Engine), nil
}
