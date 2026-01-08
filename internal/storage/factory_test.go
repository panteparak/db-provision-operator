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
	"strings"
	"testing"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// ============================
// NewBackend Tests
// ============================

func TestNewBackend_NilStorageConfig(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: nil,
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for nil StorageConfig")
	}
	if err.Error() != "storage configuration is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewBackend_UnsupportedStorageType(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: "unsupported",
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for unsupported storage type")
	}
	if !strings.Contains(err.Error(), "unsupported storage type") {
		t.Errorf("Expected 'unsupported storage type' error, got: %v", err)
	}
}

func TestNewBackend_EmptyStorageType(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: "",
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for empty storage type")
	}
	if !strings.Contains(err.Error(), "unsupported storage type") {
		t.Errorf("Expected 'unsupported storage type' error, got: %v", err)
	}
}

// ============================
// NewBackend Routing Tests - PVC
// ============================

func TestNewBackend_PVC_Success(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypePVC,
			PVC: &dbopsv1alpha1.PVCStorageConfig{
				ClaimName: "test-pvc",
				SubPath:   "backups",
			},
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	backend, err := NewBackend(ctx, config)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if backend == nil {
		t.Fatal("Expected non-nil backend")
	}

	// Verify it's a PVC backend
	_, ok := backend.(*PVCBackend)
	if !ok {
		t.Errorf("Expected *PVCBackend, got %T", backend)
	}
}

func TestNewBackend_PVC_MissingConfig(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypePVC,
			PVC:  nil,
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for missing PVC config")
	}
	if !strings.Contains(err.Error(), "PVC configuration is required") {
		t.Errorf("Expected 'PVC configuration is required' error, got: %v", err)
	}
}

// ============================
// NewBackend Routing Tests - S3
// ============================

func TestNewBackend_S3_MissingConfig(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypeS3,
			S3:   nil,
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for missing S3 config")
	}
	if !strings.Contains(err.Error(), "S3 configuration is required") {
		t.Errorf("Expected 'S3 configuration is required' error, got: %v", err)
	}
}

func TestNewBackend_S3_ValidationError(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypeS3,
			S3: &dbopsv1alpha1.S3StorageConfig{
				Bucket: "", // Empty bucket should fail validation
				Region: "us-east-1",
			},
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for empty S3 bucket")
	}
	if !strings.Contains(err.Error(), "bucket") {
		t.Errorf("Expected bucket validation error, got: %v", err)
	}
}

// ============================
// NewBackend Routing Tests - GCS
// ============================

func TestNewBackend_GCS_MissingConfig(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypeGCS,
			GCS:  nil,
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for missing GCS config")
	}
	if !strings.Contains(err.Error(), "GCS configuration is required") {
		t.Errorf("Expected 'GCS configuration is required' error, got: %v", err)
	}
}

func TestNewBackend_GCS_ValidationError(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypeGCS,
			GCS: &dbopsv1alpha1.GCSStorageConfig{
				Bucket: "", // Empty bucket should fail validation
				SecretRef: dbopsv1alpha1.SecretKeySelector{
					Name: "test-secret",
					Key:  "credentials.json",
				},
			},
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for empty GCS bucket")
	}
	if !strings.Contains(err.Error(), "bucket") {
		t.Errorf("Expected bucket validation error, got: %v", err)
	}
}

// ============================
// NewBackend Routing Tests - Azure
// ============================

func TestNewBackend_Azure_MissingConfig(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type:  dbopsv1alpha1.StorageTypeAzure,
			Azure: nil,
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for missing Azure config")
	}
	if !strings.Contains(err.Error(), "Azure configuration is required") {
		t.Errorf("Expected 'Azure configuration is required' error, got: %v", err)
	}
}

func TestNewBackend_Azure_ValidationError(t *testing.T) {
	ctx := context.Background()

	config := &Config{
		StorageConfig: &dbopsv1alpha1.StorageConfig{
			Type: dbopsv1alpha1.StorageTypeAzure,
			Azure: &dbopsv1alpha1.AzureStorageConfig{
				Container:      "", // Empty container should fail validation
				StorageAccount: "mystorageaccount",
				SecretRef: dbopsv1alpha1.SecretReference{
					Name: "test-secret",
				},
			},
		},
		SecretManager: nil,
		Namespace:     "default",
	}

	_, err := NewBackend(ctx, config)
	if err == nil {
		t.Error("Expected error for empty Azure container")
	}
	if !strings.Contains(err.Error(), "container") {
		t.Errorf("Expected container validation error, got: %v", err)
	}
}

// ============================
// GenerateBackupPath Tests
// ============================

func TestGenerateBackupPath(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		backupName string
		extension  string
		want       string
	}{
		{
			name:       "without prefix",
			prefix:     "",
			backupName: "mydb-backup-2026-01-08",
			extension:  "sql",
			want:       "mydb-backup-2026-01-08.sql",
		},
		{
			name:       "with prefix",
			prefix:     "backups",
			backupName: "mydb-backup-2026-01-08",
			extension:  "sql",
			want:       "backups/mydb-backup-2026-01-08.sql",
		},
		{
			name:       "with nested prefix",
			prefix:     "backups/daily",
			backupName: "mydb-backup-2026-01-08",
			extension:  "sql",
			want:       "backups/daily/mydb-backup-2026-01-08.sql",
		},
		{
			name:       "with compressed extension",
			prefix:     "backups",
			backupName: "mydb-backup-2026-01-08",
			extension:  "sql.gz",
			want:       "backups/mydb-backup-2026-01-08.sql.gz",
		},
		{
			name:       "with encrypted extension",
			prefix:     "backups",
			backupName: "mydb-backup-2026-01-08",
			extension:  "sql.gz.enc",
			want:       "backups/mydb-backup-2026-01-08.sql.gz.enc",
		},
		{
			name:       "empty backup name",
			prefix:     "backups",
			backupName: "",
			extension:  "sql",
			want:       "backups/.sql",
		},
		{
			name:       "complex backup name",
			prefix:     "",
			backupName: "prod-db_backup-2026-01-08T12-00-00Z",
			extension:  "dump",
			want:       "prod-db_backup-2026-01-08T12-00-00Z.dump",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateBackupPath(tt.prefix, tt.backupName, tt.extension)
			if got != tt.want {
				t.Errorf("GenerateBackupPath(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.backupName, tt.extension, got, tt.want)
			}
		})
	}
}

func TestGenerateBackupPath_TableDriven(t *testing.T) {
	// Additional edge cases
	tests := []struct {
		name       string
		prefix     string
		backupName string
		extension  string
		want       string
	}{
		{
			name:       "all empty",
			prefix:     "",
			backupName: "",
			extension:  "",
			want:       ".",
		},
		{
			name:       "only extension",
			prefix:     "",
			backupName: "",
			extension:  "sql",
			want:       ".sql",
		},
		{
			name:       "prefix only",
			prefix:     "backups",
			backupName: "",
			extension:  "",
			want:       "backups/.",
		},
		{
			name:       "spaces in name",
			prefix:     "",
			backupName: "backup with spaces",
			extension:  "sql",
			want:       "backup with spaces.sql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateBackupPath(tt.prefix, tt.backupName, tt.extension)
			if got != tt.want {
				t.Errorf("GenerateBackupPath(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.backupName, tt.extension, got, tt.want)
			}
		})
	}
}

// ============================
// Storage Type Constants Tests
// ============================

func TestStorageTypeConstants(t *testing.T) {
	// Verify storage type constants match expected values
	tests := []struct {
		storageType dbopsv1alpha1.StorageType
		want        string
	}{
		{dbopsv1alpha1.StorageTypePVC, "pvc"},
		{dbopsv1alpha1.StorageTypeS3, "s3"},
		{dbopsv1alpha1.StorageTypeGCS, "gcs"},
		{dbopsv1alpha1.StorageTypeAzure, "azure"},
	}

	for _, tt := range tests {
		t.Run(string(tt.storageType), func(t *testing.T) {
			if string(tt.storageType) != tt.want {
				t.Errorf("StorageType %v = %q, want %q", tt.storageType, string(tt.storageType), tt.want)
			}
		})
	}
}
