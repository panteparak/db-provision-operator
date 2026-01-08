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
// S3 Backend Validation Tests
// ============================

func TestNewS3Backend_EmptyBucket(t *testing.T) {
	ctx := context.Background()

	s3Config := &dbopsv1alpha1.S3StorageConfig{
		Bucket: "",
		Region: "us-east-1",
		SecretRef: dbopsv1alpha1.S3SecretRef{
			Name: "test-secret",
		},
	}

	_, err := NewS3Backend(ctx, s3Config, nil, "default")
	if err == nil {
		t.Error("Expected error for empty bucket name")
	}
	if err.Error() != "S3 bucket name is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestNewS3Backend_EmptyRegion(t *testing.T) {
	ctx := context.Background()

	s3Config := &dbopsv1alpha1.S3StorageConfig{
		Bucket: "my-bucket",
		Region: "",
		SecretRef: dbopsv1alpha1.S3SecretRef{
			Name: "test-secret",
		},
	}

	_, err := NewS3Backend(ctx, s3Config, nil, "default")
	if err == nil {
		t.Error("Expected error for empty region")
	}
	if err.Error() != "S3 region is required" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// ============================
// S3 Backend buildKey Tests
// ============================

func TestS3Backend_BuildKey(t *testing.T) {
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
			name:       "prefix with leading slash removed",
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
			name:       "complex prefix",
			prefix:     "env/prod/db-backups",
			objectPath: "daily/backup-2026-01-08.sql.gz",
			want:       "env/prod/db-backups/daily/backup-2026-01-08.sql.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create backend directly without constructor to test buildKey
			backend := &S3Backend{
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
// S3 Backend Close Tests
// ============================

func TestS3Backend_Close(t *testing.T) {
	// Create backend directly - Close should be no-op
	backend := &S3Backend{
		bucket: "test-bucket",
		prefix: "",
	}

	err := backend.Close()
	if err != nil {
		t.Errorf("Close() should return nil for S3 backend, got: %v", err)
	}
}

func TestS3Backend_CloseMultipleTimes(t *testing.T) {
	backend := &S3Backend{
		bucket: "test-bucket",
		prefix: "backups",
	}

	// Should be safe to call multiple times
	for i := 0; i < 3; i++ {
		err := backend.Close()
		if err != nil {
			t.Errorf("Close() call %d should return nil, got: %v", i+1, err)
		}
	}
}
