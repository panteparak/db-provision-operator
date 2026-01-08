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
// GCS Backend Validation Tests
// ============================

func TestNewGCSBackend_EmptyBucket(t *testing.T) {
	ctx := context.Background()

	gcsConfig := &dbopsv1alpha1.GCSStorageConfig{
		Bucket: "",
		SecretRef: dbopsv1alpha1.SecretKeySelector{
			Name: "test-secret",
			Key:  "credentials.json",
		},
	}

	_, err := NewGCSBackend(ctx, gcsConfig, nil, "default")
	if err == nil {
		t.Error("Expected error for empty bucket name")
	}
	if err.Error() != "GCS bucket name is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================
// GCS Backend buildKey Tests
// ============================

func TestGCSBackend_BuildKey(t *testing.T) {
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
			prefix:     "project/env/prod/db-backups",
			objectPath: "daily/backup-2026-01-08.sql.gz",
			want:       "project/env/prod/db-backups/daily/backup-2026-01-08.sql.gz",
		},
		{
			name:       "special characters in path",
			prefix:     "backups",
			objectPath: "db_name-backup_2026-01-08T12:00:00Z.sql",
			want:       "backups/db_name-backup_2026-01-08T12:00:00Z.sql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create backend directly without constructor to test buildKey
			backend := &GCSBackend{
				bucket: "test-bucket",
				prefix: tt.prefix,
			}

			got := backend.buildKey(tt.objectPath)
			if got != tt.want {
				t.Errorf("buildKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ============================
// GCS Backend Close Tests
// ============================

func TestGCSBackend_Close_NilClient(t *testing.T) {
	// Create backend with nil client - should handle gracefully
	// Note: In production, client would never be nil, but we test the Close method
	// behavior when client operations might fail
	backend := &GCSBackend{
		bucket: "test-bucket",
		prefix: "",
		client: nil,
	}

	// This will panic with nil client, which is expected behavior
	// In real usage, the client is always set by the constructor
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when closing with nil client")
		}
	}()

	_ = backend.Close()
}

// Note: Testing GCSBackend.Close() properly requires a real GCS client,
// which would need integration tests. The Close() method calls client.Close()
// which performs cleanup of the underlying HTTP client and connections.
