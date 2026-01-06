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
	"io"
	"path"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// GCSBackend implements Backend for Google Cloud Storage
type GCSBackend struct {
	client *storage.Client
	bucket string
	prefix string
}

// NewGCSBackend creates a new GCS storage backend
func NewGCSBackend(ctx context.Context, gcsConfig *dbopsv1alpha1.GCSStorageConfig, secretManager *secret.Manager, namespace string) (*GCSBackend, error) {
	if gcsConfig.Bucket == "" {
		return nil, fmt.Errorf("GCS bucket name is required")
	}

	// Get credentials from secret
	credentialsJSON, err := getGCSCredentials(ctx, secretManager, gcsConfig.SecretRef, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCS credentials: %w", err)
	}

	// Create GCS client with credentials
	client, err := storage.NewClient(ctx, option.WithCredentialsJSON([]byte(credentialsJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSBackend{
		client: client,
		bucket: gcsConfig.Bucket,
		prefix: strings.TrimPrefix(gcsConfig.Prefix, "/"),
	}, nil
}

// getGCSCredentials retrieves GCS credentials from a Kubernetes secret
func getGCSCredentials(ctx context.Context, secretManager *secret.Manager, secretRef dbopsv1alpha1.SecretKeySelector, namespace string) (string, error) {
	// Determine namespace
	secretNamespace := namespace
	if secretRef.Namespace != "" {
		secretNamespace = secretRef.Namespace
	}

	// Get secret data
	secretData, err := secretManager.GetSecretData(ctx, secretRef.Name, secretNamespace)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	credentialsJSON, ok := secretData[secretRef.Key]
	if !ok {
		return "", fmt.Errorf("secret %s does not contain key %s", secretRef.Name, secretRef.Key)
	}

	return credentialsJSON, nil
}

// buildKey creates the full key path including prefix
func (b *GCSBackend) buildKey(objectPath string) string {
	if b.prefix == "" {
		return objectPath
	}
	return path.Join(b.prefix, objectPath)
}

// Write writes data to GCS at the specified path
func (b *GCSBackend) Write(ctx context.Context, objectPath string, reader io.Reader) error {
	key := b.buildKey(objectPath)

	obj := b.client.Bucket(b.bucket).Object(key)
	writer := obj.NewWriter(ctx)

	if _, err := io.Copy(writer, reader); err != nil {
		_ = writer.Close()
		return fmt.Errorf("failed to write to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close GCS writer: %w", err)
	}

	return nil
}

// Read reads data from GCS at the specified path
func (b *GCSBackend) Read(ctx context.Context, objectPath string) (io.ReadCloser, error) {
	key := b.buildKey(objectPath)

	obj := b.client.Bucket(b.bucket).Object(key)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, fmt.Errorf("object not found: %s", objectPath)
		}
		return nil, fmt.Errorf("failed to read from GCS: %w", err)
	}

	return reader, nil
}

// Delete deletes the object at the specified path
func (b *GCSBackend) Delete(ctx context.Context, objectPath string) error {
	key := b.buildKey(objectPath)

	obj := b.client.Bucket(b.bucket).Object(key)
	if err := obj.Delete(ctx); err != nil {
		if err == storage.ErrObjectNotExist {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete from GCS: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the specified path
func (b *GCSBackend) Exists(ctx context.Context, objectPath string) (bool, error) {
	key := b.buildKey(objectPath)

	obj := b.client.Bucket(b.bucket).Object(key)
	_, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// List lists objects with the specified prefix
func (b *GCSBackend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	fullPrefix := b.buildKey(prefix)

	var objects []ObjectInfo
	it := b.client.Bucket(b.bucket).Objects(ctx, &storage.Query{
		Prefix: fullPrefix,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		// Remove prefix from path for relative path
		relativePath := strings.TrimPrefix(attrs.Name, b.prefix+"/")
		if b.prefix == "" {
			relativePath = attrs.Name
		}

		objects = append(objects, ObjectInfo{
			Path:         relativePath,
			Size:         attrs.Size,
			LastModified: attrs.Updated.Unix(),
			Checksum:     fmt.Sprintf("%x", attrs.MD5),
		})
	}

	return objects, nil
}

// GetSize returns the size of the object at the specified path
func (b *GCSBackend) GetSize(ctx context.Context, objectPath string) (int64, error) {
	key := b.buildKey(objectPath)

	obj := b.client.Bucket(b.bucket).Object(key)
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return 0, fmt.Errorf("object not found: %s", objectPath)
		}
		return 0, fmt.Errorf("failed to get object attributes: %w", err)
	}

	return attrs.Size, nil
}

// Close closes the GCS backend
func (b *GCSBackend) Close() error {
	return b.client.Close()
}
