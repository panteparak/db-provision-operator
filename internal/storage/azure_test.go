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
	"testing"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// ============================
// Azure Backend Validation Tests
// ============================

func TestNewAzureBackend_EmptyContainer(t *testing.T) {
	ctx := context.Background()

	azureConfig := &dbopsv1alpha1.AzureStorageConfig{
		Container:      "",
		StorageAccount: "mystorageaccount",
		SecretRef: dbopsv1alpha1.SecretReference{
			Name: "test-secret",
		},
	}

	_, err := NewAzureBackend(ctx, azureConfig, nil, "default")
	if err == nil {
		t.Error("Expected error for empty container name")
	}
	if err.Error() != "Azure container name is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewAzureBackend_EmptyStorageAccount(t *testing.T) {
	ctx := context.Background()

	azureConfig := &dbopsv1alpha1.AzureStorageConfig{
		Container:      "my-container",
		StorageAccount: "",
		SecretRef: dbopsv1alpha1.SecretReference{
			Name: "test-secret",
		},
	}

	_, err := NewAzureBackend(ctx, azureConfig, nil, "default")
	if err == nil {
		t.Error("Expected error for empty storage account")
	}
	if err.Error() != "Azure storage account is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewAzureBackend_BothEmpty(t *testing.T) {
	ctx := context.Background()

	azureConfig := &dbopsv1alpha1.AzureStorageConfig{
		Container:      "",
		StorageAccount: "",
		SecretRef: dbopsv1alpha1.SecretReference{
			Name: "test-secret",
		},
	}

	_, err := NewAzureBackend(ctx, azureConfig, nil, "default")
	if err == nil {
		t.Error("Expected error when both container and storage account are empty")
	}
	// Container is checked first
	if err.Error() != "Azure container name is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================
// Azure Backend buildKey Tests
// ============================

func TestAzureBackend_BuildKey(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		objectPath string
		want       string
	}{
		{
			name:       "no prefix",
			prefix:     "",
			objectPath: "backup.sql",
			want:       "backup.sql",
		},
		{
			name:       "with prefix",
			prefix:     "backups",
			objectPath: "backup.sql",
			want:       "backups/backup.sql",
		},
		{
			name:       "prefix with nested path",
			prefix:     "backups/daily",
			objectPath: "backup.sql",
			want:       "backups/daily/backup.sql",
		},
		{
			name:       "nested object path",
			prefix:     "backups",
			objectPath: "2026/01/backup.sql",
			want:       "backups/2026/01/backup.sql",
		},
		{
			name:       "empty object path with prefix",
			prefix:     "backups",
			objectPath: "",
			want:       "backups",
		},
		{
			name:       "complex prefix and path",
			prefix:     "env/prod/db-backups",
			objectPath: "daily/backup-2026-01-08.sql.gz",
			want:       "env/prod/db-backups/daily/backup-2026-01-08.sql.gz",
		},
		{
			name:       "special characters in path",
			prefix:     "backups",
			objectPath: "db_name-backup_2026-01-08T12:00:00Z.sql",
			want:       "backups/db_name-backup_2026-01-08T12:00:00Z.sql",
		},
		{
			name:       "blob with extension",
			prefix:     "backups",
			objectPath: "backup.sql.gz.enc",
			want:       "backups/backup.sql.gz.enc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create backend directly without constructor to test buildKey
			backend := &AzureBackend{
				containerName:  "test-container",
				storageAccount: "teststorageaccount",
				prefix:         tt.prefix,
			}

			got := backend.buildKey(tt.objectPath)
			if got != tt.want {
				t.Errorf("buildKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================
// Azure Backend Close Tests
// ============================

func TestAzureBackend_Close(t *testing.T) {
	// Create backend directly - Close should be no-op
	backend := &AzureBackend{
		containerName:  "test-container",
		storageAccount: "teststorageaccount",
		prefix:         "",
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() should return nil for Azure backend, got: %v", err)
	}
}

func TestAzureBackend_CloseMultipleTimes(t *testing.T) {
	backend := &AzureBackend{
		containerName:  "test-container",
		storageAccount: "teststorageaccount",
		prefix:         "backups",
	}

	// Should be safe to call multiple times
	for i := 0; i < 3; i++ {
		err := backend.Close()
		if err != nil {
			t.Errorf("Close() call %d should return nil, got: %v", i+1, err)
		}
	}
}

func TestAzureBackend_CloseWithNilClient(t *testing.T) {
	// Backend with nil client - Close should still be no-op
	backend := &AzureBackend{
		client:         nil,
		containerName:  "test-container",
		storageAccount: "teststorageaccount",
		prefix:         "",
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() should return nil even with nil client, got: %v", err)
	}
}
