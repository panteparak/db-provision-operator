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
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// BackupWriter provides a writer that handles compression, encryption, and storage
type BackupWriter struct {
	backend    Backend
	compressor Compressor
	encryptor  Encryptor
	path       string
	buffer     *bytes.Buffer
}

// BackupWriterConfig contains configuration for creating a backup writer
type BackupWriterConfig struct {
	StorageConfig     *dbopsv1alpha1.StorageConfig
	CompressionConfig *dbopsv1alpha1.CompressionConfig
	EncryptionConfig  *dbopsv1alpha1.EncryptionConfig
	SecretManager     *secret.Manager
	Namespace         string
	BackupName        string
	DatabaseName      string
	Timestamp         time.Time
	Extension         string // e.g., ".sql" or ".dump"
}

// NewBackupWriter creates a new backup writer
func NewBackupWriter(ctx context.Context, cfg *BackupWriterConfig) (*BackupWriter, error) {
	// Create storage backend
	backend, err := NewBackend(ctx, &Config{
		StorageConfig: cfg.StorageConfig,
		SecretManager: cfg.SecretManager,
		Namespace:     cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Create compressor
	compressor, err := NewCompressor(cfg.CompressionConfig)
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	// Create encryptor
	encryptor, err := NewEncryptor(ctx, cfg.EncryptionConfig, cfg.SecretManager, cfg.Namespace)
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	// Generate backup path
	baseName := fmt.Sprintf("%s-%s-%s",
		cfg.DatabaseName,
		cfg.BackupName,
		cfg.Timestamp.Format("20060102-150405"))

	// Add extension
	path := baseName + cfg.Extension

	// Add compression extension
	path += compressor.Extension()

	// Add encryption extension
	path += encryptor.Extension()

	// Add prefix from storage config
	prefix := getStoragePrefix(cfg.StorageConfig)
	if prefix != "" {
		path = prefix + "/" + path
	}

	return &BackupWriter{
		backend:    backend,
		compressor: compressor,
		encryptor:  encryptor,
		path:       path,
		buffer:     &bytes.Buffer{},
	}, nil
}

// getStoragePrefix extracts the prefix from storage config
func getStoragePrefix(cfg *dbopsv1alpha1.StorageConfig) string {
	if cfg == nil {
		return ""
	}
	switch cfg.Type {
	case dbopsv1alpha1.StorageTypeS3:
		if cfg.S3 != nil {
			return cfg.S3.Prefix
		}
	case dbopsv1alpha1.StorageTypeGCS:
		if cfg.GCS != nil {
			return cfg.GCS.Prefix
		}
	case dbopsv1alpha1.StorageTypeAzure:
		if cfg.Azure != nil {
			return cfg.Azure.Prefix
		}
	case dbopsv1alpha1.StorageTypePVC:
		if cfg.PVC != nil {
			return cfg.PVC.SubPath
		}
	}
	return ""
}

// Write writes data to the buffer
func (w *BackupWriter) Write(p []byte) (n int, err error) {
	return w.buffer.Write(p)
}

// Close finalizes the backup by compressing, encrypting, and writing to storage
func (w *BackupWriter) Close() error {
	// First compress the data
	var compressedBuf bytes.Buffer
	compWriter, err := w.compressor.Compress(&compressedBuf)
	if err != nil {
		return fmt.Errorf("failed to create compression writer: %w", err)
	}
	if _, err := compWriter.Write(w.buffer.Bytes()); err != nil {
		compWriter.Close()
		return fmt.Errorf("failed to compress data: %w", err)
	}
	if err := compWriter.Close(); err != nil {
		return fmt.Errorf("failed to close compression writer: %w", err)
	}

	// Then encrypt the compressed data
	encryptedData, err := w.encryptor.Encrypt(compressedBuf.Bytes())
	if err != nil {
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Finally write to storage
	if err := w.backend.Write(context.Background(), w.path, bytes.NewReader(encryptedData)); err != nil {
		return fmt.Errorf("failed to write to storage: %w", err)
	}

	return nil
}

// Path returns the backup file path
func (w *BackupWriter) Path() string {
	return w.path
}

// UncompressedSize returns the size of the uncompressed data
func (w *BackupWriter) UncompressedSize() int64 {
	return int64(w.buffer.Len())
}

// Backend returns the storage backend for cleanup purposes
func (w *BackupWriter) Backend() Backend {
	return w.backend
}

// RestoreReader provides a reader that handles storage, decryption, and decompression
type RestoreReader struct {
	backend    Backend
	compressor Compressor
	encryptor  Encryptor
	reader     io.ReadCloser
}

// RestoreReaderConfig contains configuration for creating a restore reader
type RestoreReaderConfig struct {
	StorageConfig     *dbopsv1alpha1.StorageConfig
	CompressionConfig *dbopsv1alpha1.CompressionConfig
	EncryptionConfig  *dbopsv1alpha1.EncryptionConfig
	SecretManager     *secret.Manager
	Namespace         string
	BackupPath        string
}

// NewRestoreReader creates a new restore reader
func NewRestoreReader(ctx context.Context, cfg *RestoreReaderConfig) (*RestoreReader, error) {
	// Create storage backend
	backend, err := NewBackend(ctx, &Config{
		StorageConfig: cfg.StorageConfig,
		SecretManager: cfg.SecretManager,
		Namespace:     cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}

	// Create compressor (for decompression)
	compressor, err := NewCompressor(cfg.CompressionConfig)
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to create compressor: %w", err)
	}

	// Create encryptor (for decryption)
	encryptor, err := NewEncryptor(ctx, cfg.EncryptionConfig, cfg.SecretManager, cfg.Namespace)
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to create encryptor: %w", err)
	}

	// Read the encrypted data from storage
	storageReader, err := backend.Read(ctx, cfg.BackupPath)
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to read from storage: %w", err)
	}

	// Read all encrypted data
	encryptedData, err := io.ReadAll(storageReader)
	storageReader.Close()
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to read encrypted data: %w", err)
	}

	// Decrypt the data
	compressedData, err := encryptor.Decrypt(encryptedData)
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	// Create decompression reader
	decompReader, err := compressor.Decompress(bytes.NewReader(compressedData))
	if err != nil {
		backend.Close()
		return nil, fmt.Errorf("failed to create decompression reader: %w", err)
	}

	return &RestoreReader{
		backend:    backend,
		compressor: compressor,
		encryptor:  encryptor,
		reader:     decompReader,
	}, nil
}

// Read reads decompressed and decrypted data
func (r *RestoreReader) Read(p []byte) (n int, err error) {
	return r.reader.Read(p)
}

// Close closes the restore reader and backend
func (r *RestoreReader) Close() error {
	if err := r.reader.Close(); err != nil {
		r.backend.Close()
		return err
	}
	return r.backend.Close()
}

// Backend returns the storage backend
func (r *RestoreReader) Backend() Backend {
	return r.backend
}

// DeleteBackup deletes a backup from storage
func DeleteBackup(ctx context.Context, cfg *Config, path string) error {
	backend, err := NewBackend(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create storage backend: %w", err)
	}
	defer backend.Close()

	return backend.Delete(ctx, path)
}

// GetBackupSize returns the size of a backup file
func GetBackupSize(ctx context.Context, cfg *Config, path string) (int64, error) {
	backend, err := NewBackend(ctx, cfg)
	if err != nil {
		return 0, fmt.Errorf("failed to create storage backend: %w", err)
	}
	defer backend.Close()

	return backend.GetSize(ctx, path)
}

// ListBackups lists all backups with a given prefix
func ListBackups(ctx context.Context, cfg *Config, prefix string) ([]ObjectInfo, error) {
	backend, err := NewBackend(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend: %w", err)
	}
	defer backend.Close()

	return backend.List(ctx, prefix)
}
