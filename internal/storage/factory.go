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

package storage

import (
	"context"
	"fmt"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// Config holds configuration for creating a storage backend
type Config struct {
	// StorageConfig from the CRD
	StorageConfig *dbopsv1alpha1.StorageConfig

	// SecretManager for retrieving credentials
	SecretManager *secret.Manager

	// Namespace for looking up secrets
	Namespace string
}

// NewBackend creates a new storage backend based on the configuration
func NewBackend(ctx context.Context, config *Config) (Backend, error) {
	if config.StorageConfig == nil {
		return nil, fmt.Errorf("storage configuration is required")
	}

	switch config.StorageConfig.Type {
	case dbopsv1alpha1.StorageTypePVC:
		if config.StorageConfig.PVC == nil {
			return nil, fmt.Errorf("PVC configuration is required for PVC storage type")
		}
		return NewPVCBackend(config.StorageConfig.PVC)

	case dbopsv1alpha1.StorageTypeS3:
		if config.StorageConfig.S3 == nil {
			return nil, fmt.Errorf("S3 configuration is required for S3 storage type")
		}
		return NewS3Backend(ctx, config.StorageConfig.S3, config.SecretManager, config.Namespace)

	case dbopsv1alpha1.StorageTypeGCS:
		if config.StorageConfig.GCS == nil {
			return nil, fmt.Errorf("GCS configuration is required for GCS storage type")
		}
		return NewGCSBackend(ctx, config.StorageConfig.GCS, config.SecretManager, config.Namespace)

	case dbopsv1alpha1.StorageTypeAzure:
		if config.StorageConfig.Azure == nil {
			return nil, fmt.Errorf("Azure configuration is required for Azure storage type")
		}
		return NewAzureBackend(ctx, config.StorageConfig.Azure, config.SecretManager, config.Namespace)

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.StorageConfig.Type)
	}
}

// GenerateBackupPath generates a path for a backup file
func GenerateBackupPath(prefix, backupName, extension string) string {
	if prefix != "" {
		return fmt.Sprintf("%s/%s.%s", prefix, backupName, extension)
	}
	return fmt.Sprintf("%s.%s", backupName, extension)
}
