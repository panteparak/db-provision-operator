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
	"os"
	"path/filepath"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// PVCBackend implements Backend for PVC-based storage
type PVCBackend struct {
	claimName string
	subPath   string
	basePath  string
}

// NewPVCBackend creates a new PVC storage backend
func NewPVCBackend(config *dbopsv1alpha1.PVCStorageConfig) (*PVCBackend, error) {
	if config.ClaimName == "" {
		return nil, fmt.Errorf("PVC claim name is required")
	}

	// The base path is where the PVC is mounted
	// In a real Kubernetes environment, this would be a mounted path
	// For the operator running in a pod, this is typically /data or similar
	basePath := "/data"
	if config.SubPath != "" {
		basePath = filepath.Join(basePath, config.SubPath)
	}

	return &PVCBackend{
		claimName: config.ClaimName,
		subPath:   config.SubPath,
		basePath:  basePath,
	}, nil
}

// Write writes data to the PVC at the specified path
func (b *PVCBackend) Write(ctx context.Context, path string, reader io.Reader) error {
	fullPath := filepath.Join(b.basePath, path)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create the file
	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer file.Close()

	// Copy data
	if _, err := io.Copy(file, reader); err != nil {
		// Clean up partial file on error
		os.Remove(fullPath)
		return fmt.Errorf("failed to write data to %s: %w", fullPath, err)
	}

	return nil
}

// Read reads data from the PVC at the specified path
func (b *PVCBackend) Read(ctx context.Context, path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(b.basePath, path)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to open file %s: %w", fullPath, err)
	}

	return file, nil
}

// Delete deletes the file at the specified path
func (b *PVCBackend) Delete(ctx context.Context, path string) error {
	fullPath := filepath.Join(b.basePath, path)

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete file %s: %w", fullPath, err)
	}

	return nil
}

// Exists checks if a file exists at the specified path
func (b *PVCBackend) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := filepath.Join(b.basePath, path)

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file %s: %w", fullPath, err)
	}

	return true, nil
}

// List lists files with the specified prefix
func (b *PVCBackend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	searchPath := filepath.Join(b.basePath, prefix)
	dir := filepath.Dir(searchPath)

	var objects []ObjectInfo

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Get relative path from base
		relPath, err := filepath.Rel(b.basePath, path)
		if err != nil {
			return err
		}

		objects = append(objects, ObjectInfo{
			Path:         relPath,
			Size:         info.Size(),
			LastModified: info.ModTime().Unix(),
		})

		return nil
	})

	if err != nil {
		if os.IsNotExist(err) {
			return []ObjectInfo{}, nil
		}
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return objects, nil
}

// GetSize returns the size of the file at the specified path
func (b *PVCBackend) GetSize(ctx context.Context, path string) (int64, error) {
	fullPath := filepath.Join(b.basePath, path)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("file not found: %s", path)
		}
		return 0, fmt.Errorf("failed to stat file %s: %w", fullPath, err)
	}

	return info.Size(), nil
}

// Close closes the PVC backend (no-op for PVC)
func (b *PVCBackend) Close() error {
	return nil
}

// GetBasePath returns the base path for testing purposes
func (b *PVCBackend) GetBasePath() string {
	return b.basePath
}

// SetBasePath sets the base path (useful for testing)
func (b *PVCBackend) SetBasePath(path string) {
	b.basePath = path
}
