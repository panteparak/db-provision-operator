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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// Encryptor provides encryption and decryption functionality
type Encryptor interface {
	// Encrypt encrypts data from reader and returns encrypted bytes
	Encrypt(plaintext []byte) ([]byte, error)

	// Decrypt decrypts ciphertext and returns plaintext bytes
	Decrypt(ciphertext []byte) ([]byte, error)

	// Extension returns the file extension for encrypted files
	Extension() string
}

// NewEncryptor creates a new encryptor based on the configuration
func NewEncryptor(ctx context.Context, config *dbopsv1alpha1.EncryptionConfig, secretManager *secret.Manager, namespace string) (Encryptor, error) {
	if config == nil || !config.Enabled {
		return &noopEncryptor{}, nil
	}

	if config.SecretRef == nil {
		return nil, fmt.Errorf("encryption secret reference is required when encryption is enabled")
	}

	// Get encryption key from secret
	key, err := getEncryptionKey(ctx, secretManager, config.SecretRef, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	switch config.Algorithm {
	case dbopsv1alpha1.EncryptionAES256GCM, "":
		return newAESGCMEncryptor(key)
	case dbopsv1alpha1.EncryptionAES256CBC:
		return newAESCBCEncryptor(key)
	default:
		return nil, fmt.Errorf("unsupported encryption algorithm: %s", config.Algorithm)
	}
}

// getEncryptionKey retrieves the encryption key from a Kubernetes secret
func getEncryptionKey(ctx context.Context, secretManager *secret.Manager, secretRef *dbopsv1alpha1.SecretKeySelector, namespace string) ([]byte, error) {
	secretNamespace := namespace
	if secretRef.Namespace != "" {
		secretNamespace = secretRef.Namespace
	}

	secretData, err := secretManager.GetSecretData(ctx, secretRef.Name, secretNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	keyStr, ok := secretData[secretRef.Key]
	if !ok {
		return nil, fmt.Errorf("secret %s does not contain key %s", secretRef.Name, secretRef.Key)
	}

	key := []byte(keyStr)
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be exactly 32 bytes for AES-256, got %d bytes", len(key))
	}

	return key, nil
}

// noopEncryptor provides no encryption
type noopEncryptor struct{}

func (e *noopEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	return plaintext, nil
}

func (e *noopEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return ciphertext, nil
}

func (e *noopEncryptor) Extension() string {
	return ""
}

// aesGCMEncryptor provides AES-256-GCM encryption
type aesGCMEncryptor struct {
	gcm cipher.AEAD
}

func newAESGCMEncryptor(key []byte) (*aesGCMEncryptor, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &aesGCMEncryptor{gcm: gcm}, nil
}

func (e *aesGCMEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Prepend nonce to ciphertext
	ciphertext := e.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (e *aesGCMEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

func (e *aesGCMEncryptor) Extension() string {
	return ".enc"
}

// aesCBCEncryptor provides AES-256-CBC encryption
type aesCBCEncryptor struct {
	block cipher.Block
}

func newAESCBCEncryptor(key []byte) (*aesCBCEncryptor, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	return &aesCBCEncryptor{block: block}, nil
}

func (e *aesCBCEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	// Pad plaintext to block size using PKCS7
	blockSize := e.block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	// Generate IV
	iv := make([]byte, blockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt
	ciphertext := make([]byte, len(iv)+len(padtext))
	copy(ciphertext, iv)
	mode := cipher.NewCBCEncrypter(e.block, iv)
	mode.CryptBlocks(ciphertext[blockSize:], padtext)

	return ciphertext, nil
}

func (e *aesCBCEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	blockSize := e.block.BlockSize()
	if len(ciphertext) < blockSize*2 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	if len(ciphertext)%blockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}

	// Extract IV
	iv := ciphertext[:blockSize]
	ciphertext = ciphertext[blockSize:]

	// Decrypt
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(e.block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	padding := int(plaintext[len(plaintext)-1])
	if padding > blockSize || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		if plaintext[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}

	return plaintext[:len(plaintext)-padding], nil
}

func (e *aesCBCEncryptor) Extension() string {
	return ".enc"
}

// EncryptingWriter provides an io.WriteCloser that encrypts data
// Note: This buffers all data before encryption due to the nature of block ciphers
type EncryptingWriter struct {
	encryptor  Encryptor
	underlying io.WriteCloser
	buffer     []byte
}

// NewEncryptingWriter creates a writer that encrypts data before writing
func NewEncryptingWriter(w io.WriteCloser, encryptor Encryptor) *EncryptingWriter {
	return &EncryptingWriter{
		encryptor:  encryptor,
		underlying: w,
		buffer:     make([]byte, 0),
	}
}

func (w *EncryptingWriter) Write(p []byte) (n int, err error) {
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *EncryptingWriter) Close() error {
	// Encrypt buffered data
	encrypted, err := w.encryptor.Encrypt(w.buffer)
	if err != nil {
		w.underlying.Close()
		return fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Write encrypted data
	if _, err := w.underlying.Write(encrypted); err != nil {
		w.underlying.Close()
		return fmt.Errorf("failed to write encrypted data: %w", err)
	}

	return w.underlying.Close()
}

// DecryptingReader provides an io.ReadCloser that decrypts data
// Note: This reads all data at once due to the nature of block ciphers
type DecryptingReader struct {
	decrypted   []byte
	offset      int
	underlying  io.ReadCloser
	encryptor   Encryptor
	initialized bool
}

// NewDecryptingReader creates a reader that decrypts data while reading
func NewDecryptingReader(r io.ReadCloser, encryptor Encryptor) *DecryptingReader {
	return &DecryptingReader{
		underlying: r,
		encryptor:  encryptor,
	}
}

func (r *DecryptingReader) Read(p []byte) (n int, err error) {
	if !r.initialized {
		// Read all data from underlying reader
		ciphertext, err := io.ReadAll(r.underlying)
		if err != nil {
			return 0, fmt.Errorf("failed to read encrypted data: %w", err)
		}

		// Decrypt
		r.decrypted, err = r.encryptor.Decrypt(ciphertext)
		if err != nil {
			return 0, fmt.Errorf("failed to decrypt data: %w", err)
		}
		r.initialized = true
	}

	if r.offset >= len(r.decrypted) {
		return 0, io.EOF
	}

	n = copy(p, r.decrypted[r.offset:])
	r.offset += n
	return n, nil
}

func (r *DecryptingReader) Close() error {
	return r.underlying.Close()
}
