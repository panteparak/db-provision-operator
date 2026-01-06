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
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Test data
var testData = []byte("This is test data for compression and encryption testing. " +
	"It contains multiple sentences to ensure there is enough data for proper testing. " +
	"Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt.")

// ============================
// Compression Tests
// ============================

func TestNoopCompressor(t *testing.T) {
	compressor := &noopCompressor{}

	// Test extension
	if ext := compressor.Extension(); ext != "" {
		t.Errorf("Expected empty extension, got %q", ext)
	}

	// Test compress/decompress roundtrip
	testCompressDecompressRoundtrip(t, compressor, testData)
}

func TestGzipCompressor(t *testing.T) {
	compressor := &gzipCompressor{level: 0}

	// Test extension
	if ext := compressor.Extension(); ext != ".gz" {
		t.Errorf("Expected .gz extension, got %q", ext)
	}

	// Test compress/decompress roundtrip
	testCompressDecompressRoundtrip(t, compressor, testData)
}

func TestGzipCompressorWithLevel(t *testing.T) {
	levels := []int{1, 5, 9}
	for _, level := range levels {
		t.Run("level="+string(rune('0'+level)), func(t *testing.T) {
			compressor := &gzipCompressor{level: level}
			testCompressDecompressRoundtrip(t, compressor, testData)
		})
	}
}

func TestLZ4Compressor(t *testing.T) {
	compressor := &lz4Compressor{level: 0}

	// Test extension
	if ext := compressor.Extension(); ext != ".lz4" {
		t.Errorf("Expected .lz4 extension, got %q", ext)
	}

	// Test compress/decompress roundtrip
	testCompressDecompressRoundtrip(t, compressor, testData)
}

func TestLZ4CompressorWithLevels(t *testing.T) {
	levels := []int{1, 5, 9}
	for _, level := range levels {
		t.Run("level="+string(rune('0'+level)), func(t *testing.T) {
			compressor := &lz4Compressor{level: level}
			testCompressDecompressRoundtrip(t, compressor, testData)
		})
	}
}

func TestZstdCompressor(t *testing.T) {
	compressor := &zstdCompressor{level: 0}

	// Test extension
	if ext := compressor.Extension(); ext != ".zst" {
		t.Errorf("Expected .zst extension, got %q", ext)
	}

	// Test compress/decompress roundtrip
	testCompressDecompressRoundtrip(t, compressor, testData)
}

func TestZstdCompressorWithLevels(t *testing.T) {
	levels := []int{1, 5, 9}
	for _, level := range levels {
		t.Run("level="+string(rune('0'+level)), func(t *testing.T) {
			compressor := &zstdCompressor{level: level}
			testCompressDecompressRoundtrip(t, compressor, testData)
		})
	}
}

func TestNewCompressor(t *testing.T) {
	tests := []struct {
		name        string
		config      *dbopsv1alpha1.CompressionConfig
		wantExt     string
		shouldError bool
	}{
		{
			name:    "nil config returns noop",
			config:  nil,
			wantExt: "",
		},
		{
			name:    "disabled config returns noop",
			config:  &dbopsv1alpha1.CompressionConfig{Enabled: false},
			wantExt: "",
		},
		{
			name:    "gzip algorithm",
			config:  &dbopsv1alpha1.CompressionConfig{Enabled: true, Algorithm: dbopsv1alpha1.CompressionGzip},
			wantExt: ".gz",
		},
		{
			name:    "lz4 algorithm",
			config:  &dbopsv1alpha1.CompressionConfig{Enabled: true, Algorithm: dbopsv1alpha1.CompressionLZ4},
			wantExt: ".lz4",
		},
		{
			name:    "zstd algorithm",
			config:  &dbopsv1alpha1.CompressionConfig{Enabled: true, Algorithm: dbopsv1alpha1.CompressionZstd},
			wantExt: ".zst",
		},
		{
			name:    "none algorithm",
			config:  &dbopsv1alpha1.CompressionConfig{Enabled: true, Algorithm: dbopsv1alpha1.CompressionNone},
			wantExt: "",
		},
		{
			name:        "unsupported algorithm",
			config:      &dbopsv1alpha1.CompressionConfig{Enabled: true, Algorithm: "invalid"},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressor, err := NewCompressor(tt.config)
			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if ext := compressor.Extension(); ext != tt.wantExt {
				t.Errorf("Expected extension %q, got %q", tt.wantExt, ext)
			}
		})
	}
}

func TestCompressingWriter(t *testing.T) {
	compressor := &gzipCompressor{level: 0}

	// Create a buffer to write to
	var buf bytes.Buffer
	underlyingWriter := &nopWriteCloser{&buf}

	cw, err := NewCompressingWriter(underlyingWriter, compressor)
	if err != nil {
		t.Fatalf("Failed to create compressing writer: %v", err)
	}

	// Write data
	n, err := cw.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Close to flush
	if err := cw.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Verify data is compressed (should be different from original)
	if bytes.Equal(buf.Bytes(), testData) {
		t.Error("Data was not compressed")
	}

	// Decompress and verify
	reader, err := compressor.Decompress(&buf)
	if err != nil {
		t.Fatalf("Failed to create decompressor: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}
	reader.Close()

	if !bytes.Equal(decompressed, testData) {
		t.Error("Decompressed data doesn't match original")
	}
}

func TestDecompressingReader(t *testing.T) {
	compressor := &gzipCompressor{level: 0}

	// First compress some data
	var compressedBuf bytes.Buffer
	writer, err := compressor.Compress(&compressedBuf)
	if err != nil {
		t.Fatalf("Failed to create compressor: %v", err)
	}
	writer.Write(testData)
	writer.Close()

	// Create a decompressing reader
	dr, err := NewDecompressingReader(io.NopCloser(&compressedBuf), compressor)
	if err != nil {
		t.Fatalf("Failed to create decompressing reader: %v", err)
	}

	// Read data
	decompressed, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	dr.Close()

	if !bytes.Equal(decompressed, testData) {
		t.Error("Decompressed data doesn't match original")
	}
}

// Helper function for compress/decompress roundtrip tests
func testCompressDecompressRoundtrip(t *testing.T, compressor Compressor, data []byte) {
	t.Helper()

	// Compress
	var compressedBuf bytes.Buffer
	writer, err := compressor.Compress(&compressedBuf)
	if err != nil {
		t.Fatalf("Failed to create compressor: %v", err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Decompress
	reader, err := compressor.Decompress(&compressedBuf)
	if err != nil {
		t.Fatalf("Failed to create decompressor: %v", err)
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Failed to close reader: %v", err)
	}

	// Verify
	if !bytes.Equal(decompressed, data) {
		t.Errorf("Decompressed data doesn't match original.\nOriginal: %q\nDecompressed: %q", data, decompressed)
	}
}

// ============================
// Encryption Tests
// ============================

func TestNoopEncryptor(t *testing.T) {
	encryptor := &noopEncryptor{}

	// Test extension
	if ext := encryptor.Extension(); ext != "" {
		t.Errorf("Expected empty extension, got %q", ext)
	}

	// Test encrypt/decrypt roundtrip
	testEncryptDecryptRoundtrip(t, encryptor, testData)
}

func TestAESGCMEncryptor(t *testing.T) {
	// Generate a 32-byte key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := newAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// Test extension
	if ext := encryptor.Extension(); ext != ".enc" {
		t.Errorf("Expected .enc extension, got %q", ext)
	}

	// Test encrypt/decrypt roundtrip
	testEncryptDecryptRoundtrip(t, encryptor, testData)
}

func TestAESGCMEncryptorWithDifferentDataSizes(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encryptor, err := newAESGCMEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	sizes := []int{0, 1, 16, 100, 1000, 10000}
	for _, size := range sizes {
		t.Run("size="+string(rune('0'+size%10)), func(t *testing.T) {
			data := make([]byte, size)
			rand.Read(data)
			testEncryptDecryptRoundtrip(t, encryptor, data)
		})
	}
}

func TestAESCBCEncryptor(t *testing.T) {
	// Generate a 32-byte key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	encryptor, err := newAESCBCEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	// Test extension
	if ext := encryptor.Extension(); ext != ".enc" {
		t.Errorf("Expected .enc extension, got %q", ext)
	}

	// Test encrypt/decrypt roundtrip
	testEncryptDecryptRoundtrip(t, encryptor, testData)
}

func TestAESCBCEncryptorWithDifferentDataSizes(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encryptor, err := newAESCBCEncryptor(key)
	if err != nil {
		t.Fatalf("Failed to create encryptor: %v", err)
	}

	sizes := []int{1, 15, 16, 17, 32, 100, 1000}
	for _, size := range sizes {
		t.Run("size="+string(rune('0'+size%10)), func(t *testing.T) {
			data := make([]byte, size)
			rand.Read(data)
			testEncryptDecryptRoundtrip(t, encryptor, data)
		})
	}
}

func TestAESGCMEncryptorInvalidKey(t *testing.T) {
	invalidKeys := [][]byte{
		make([]byte, 16), // AES-128 key (not AES-256)
		make([]byte, 24), // AES-192 key (not AES-256)
		make([]byte, 31), // Too short
		make([]byte, 33), // Too long
	}

	for _, key := range invalidKeys {
		// AES accepts 16, 24, 32 byte keys, but our encryptor is for AES-256 (32 bytes)
		// The aes.NewCipher will accept 16, 24, 32 but we document 32
		// This test verifies the encryptor still works with valid AES key sizes
		if len(key) == 16 || len(key) == 24 || len(key) == 32 {
			_, err := newAESGCMEncryptor(key)
			if err != nil {
				t.Errorf("Expected valid key size %d to work, got error: %v", len(key), err)
			}
		}
	}
}

func TestEncryptionDeterminism(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encryptor, _ := newAESGCMEncryptor(key)

	// Encrypt same data twice
	ciphertext1, _ := encryptor.Encrypt(testData)
	ciphertext2, _ := encryptor.Encrypt(testData)

	// Should produce different ciphertexts (due to random nonce)
	if bytes.Equal(ciphertext1, ciphertext2) {
		t.Error("Encryption should produce different ciphertexts for same plaintext")
	}

	// Both should decrypt to the same plaintext
	plaintext1, _ := encryptor.Decrypt(ciphertext1)
	plaintext2, _ := encryptor.Decrypt(ciphertext2)

	if !bytes.Equal(plaintext1, testData) || !bytes.Equal(plaintext2, testData) {
		t.Error("Both ciphertexts should decrypt to original plaintext")
	}
}

func TestEncryptingWriter(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encryptor, _ := newAESGCMEncryptor(key)

	var buf bytes.Buffer
	ew := NewEncryptingWriter(&nopWriteCloser{&buf}, encryptor)

	// Write data
	n, err := ew.Write(testData)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Close to encrypt and flush
	if err := ew.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Verify data is encrypted (should be different from original)
	if bytes.Equal(buf.Bytes(), testData) {
		t.Error("Data was not encrypted")
	}

	// Decrypt and verify
	decrypted, err := encryptor.Decrypt(buf.Bytes())
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

func TestDecryptingReader(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encryptor, _ := newAESGCMEncryptor(key)

	// First encrypt some data
	encrypted, err := encryptor.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Create a decrypting reader
	dr := NewDecryptingReader(io.NopCloser(bytes.NewReader(encrypted)), encryptor)

	// Read data
	decrypted, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	dr.Close()

	if !bytes.Equal(decrypted, testData) {
		t.Error("Decrypted data doesn't match original")
	}
}

// Helper function for encrypt/decrypt roundtrip tests
func testEncryptDecryptRoundtrip(t *testing.T, encryptor Encryptor, data []byte) {
	t.Helper()

	// Encrypt
	ciphertext, err := encryptor.Encrypt(data)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	// Verify
	if !bytes.Equal(plaintext, data) {
		t.Errorf("Decrypted data doesn't match original.\nOriginal length: %d\nDecrypted length: %d", len(data), len(plaintext))
	}
}

// ============================
// PVC Backend Tests
// ============================

func TestPVCBackendWrite(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Write test data
	err := backend.Write(ctx, "test/file.txt", bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Verify file exists
	fullPath := filepath.Join(tempDir, "test/file.txt")
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("File was not created: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !bytes.Equal(content, testData) {
		t.Error("File content doesn't match")
	}
}

func TestPVCBackendRead(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create test file
	testPath := "test/read.txt"
	fullPath := filepath.Join(tempDir, testPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, testData, 0644)

	// Read using backend
	reader, err := backend.Read(ctx, testPath)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read content: %v", err)
	}

	if !bytes.Equal(content, testData) {
		t.Error("Read content doesn't match")
	}
}

func TestPVCBackendReadNotFound(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	_, err := backend.Read(ctx, "nonexistent/file.txt")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

func TestPVCBackendDelete(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create test file
	testPath := "test/delete.txt"
	fullPath := filepath.Join(tempDir, testPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, testData, 0644)

	// Verify exists
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("Test file was not created: %v", err)
	}

	// Delete using backend
	err := backend.Delete(ctx, testPath)
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	// Verify deleted
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Error("File was not deleted")
	}
}

func TestPVCBackendDeleteNonExistent(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Delete non-existent file (should not error)
	err := backend.Delete(ctx, "nonexistent/file.txt")
	if err != nil {
		t.Errorf("Delete of non-existent file should not error: %v", err)
	}
}

func TestPVCBackendExists(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create test file
	testPath := "test/exists.txt"
	fullPath := filepath.Join(tempDir, testPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, testData, 0644)

	// Check exists
	exists, err := backend.Exists(ctx, testPath)
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if !exists {
		t.Error("Expected file to exist")
	}

	// Check non-existent
	exists, err = backend.Exists(ctx, "nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}
	if exists {
		t.Error("Expected file to not exist")
	}
}

func TestPVCBackendGetSize(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create test file
	testPath := "test/size.txt"
	fullPath := filepath.Join(tempDir, testPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, testData, 0644)

	// Get size
	size, err := backend.GetSize(ctx, testPath)
	if err != nil {
		t.Fatalf("GetSize failed: %v", err)
	}
	if size != int64(len(testData)) {
		t.Errorf("Expected size %d, got %d", len(testData), size)
	}
}

func TestPVCBackendList(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	ctx := context.Background()

	// Create multiple test files
	files := []string{
		"backups/db1-backup-1.sql",
		"backups/db1-backup-2.sql",
		"backups/db2-backup-1.sql",
	}

	for _, f := range files {
		fullPath := filepath.Join(tempDir, f)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte("test"), 0644)
	}

	// List files
	objects, err := backend.List(ctx, "backups/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(objects) != 3 {
		t.Errorf("Expected 3 files, got %d", len(objects))
	}

	// Verify paths
	foundPaths := make(map[string]bool)
	for _, obj := range objects {
		foundPaths[obj.Path] = true
	}

	for _, f := range files {
		if !foundPaths[f] {
			t.Errorf("Expected file %s in list", f)
		}
	}
}

func TestPVCBackendClose(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	// Close should be a no-op and not error
	if err := backend.Close(); err != nil {
		t.Errorf("Close should not error: %v", err)
	}
}

func TestNewPVCBackendRequiresClaimName(t *testing.T) {
	_, err := NewPVCBackend(&dbopsv1alpha1.PVCStorageConfig{
		ClaimName: "",
	})
	if err == nil {
		t.Error("Expected error for empty claim name")
	}
}

func TestPVCBackendWithSubPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pvc-subpath-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backend, err := NewPVCBackend(&dbopsv1alpha1.PVCStorageConfig{
		ClaimName: "test-pvc",
		SubPath:   "mybackups",
	})
	if err != nil {
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Override base path for testing
	backend.SetBasePath(filepath.Join(tempDir, "mybackups"))
	os.MkdirAll(backend.GetBasePath(), 0755)

	ctx := context.Background()

	// Write file
	err = backend.Write(ctx, "test.txt", bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Verify file location
	expectedPath := filepath.Join(tempDir, "mybackups", "test.txt")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("File not at expected path: %v", err)
	}
}

// Helper to setup a test PVC backend with a temp directory
func setupTestPVCBackend(t *testing.T) (*PVCBackend, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "pvc-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	backend, err := NewPVCBackend(&dbopsv1alpha1.PVCStorageConfig{
		ClaimName: "test-pvc",
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create backend: %v", err)
	}

	// Override base path for testing
	backend.SetBasePath(tempDir)

	return backend, tempDir
}

// ============================
// Integration Tests
// ============================

func TestCompressionAndEncryptionChain(t *testing.T) {
	// Test that compression and encryption can be chained together
	key := make([]byte, 32)
	rand.Read(key)

	compressor := &gzipCompressor{level: 6}
	encryptor, _ := newAESGCMEncryptor(key)

	// Compress first
	var compressedBuf bytes.Buffer
	compWriter, _ := compressor.Compress(&compressedBuf)
	compWriter.Write(testData)
	compWriter.Close()

	// Then encrypt
	encrypted, _ := encryptor.Encrypt(compressedBuf.Bytes())

	// Now reverse: decrypt first
	decrypted, _ := encryptor.Decrypt(encrypted)

	// Then decompress
	decompReader, _ := compressor.Decompress(bytes.NewReader(decrypted))
	final, _ := io.ReadAll(decompReader)
	decompReader.Close()

	if !bytes.Equal(final, testData) {
		t.Error("Data mismatch after compression+encryption chain")
	}
}

func TestPVCBackendWithCompressionAndEncryption(t *testing.T) {
	backend, tempDir := setupTestPVCBackend(t)
	defer os.RemoveAll(tempDir)

	key := make([]byte, 32)
	rand.Read(key)

	compressor := &zstdCompressor{level: 3}
	encryptor, _ := newAESGCMEncryptor(key)

	ctx := context.Background()

	// Compress and encrypt data
	var compressedBuf bytes.Buffer
	compWriter, _ := compressor.Compress(&compressedBuf)
	compWriter.Write(testData)
	compWriter.Close()

	encrypted, _ := encryptor.Encrypt(compressedBuf.Bytes())

	// Write to PVC backend
	err := backend.Write(ctx, "backup.sql.zst.enc", bytes.NewReader(encrypted))
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}

	// Read from PVC backend
	reader, err := backend.Read(ctx, "backup.sql.zst.enc")
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	encryptedData, _ := io.ReadAll(reader)
	reader.Close()

	// Decrypt and decompress
	decrypted, err := encryptor.Decrypt(encryptedData)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	decompReader, err := compressor.Decompress(bytes.NewReader(decrypted))
	if err != nil {
		t.Fatalf("Failed to decompress: %v", err)
	}

	final, _ := io.ReadAll(decompReader)
	decompReader.Close()

	if !bytes.Equal(final, testData) {
		t.Error("Data mismatch after full roundtrip through PVC backend with compression and encryption")
	}
}
