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

package testutil

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Test constants for storage fixtures
const (
	// TestNamespace is the default namespace for test fixtures
	TestNamespace = "test-namespace"

	// S3 constants
	TestS3Bucket       = "test-backup-bucket"
	TestS3Region       = "us-east-1"
	TestS3Prefix       = "backups/"
	TestS3SecretName   = "s3-credentials"
	TestS3AccessKey    = "AKIAIOSFODNN7EXAMPLE"
	TestS3SecretKey    = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	TestMinIOEndpoint  = "minio.minio-system.svc.cluster.local:9000"

	// GCS constants
	TestGCSBucket     = "test-gcs-backup-bucket"
	TestGCSPrefix     = "db-backups/"
	TestGCSSecretName = "gcs-credentials"
	TestGCSSecretKey  = "credentials.json"

	// Azure constants
	TestAzureContainer      = "backups"
	TestAzureStorageAccount = "testbackupstorage"
	TestAzurePrefix         = "database-backups/"
	TestAzureSecretName     = "azure-storage-credentials"

	// PVC constants
	TestPVCName    = "backup-pvc"
	TestPVCSubPath = "mysql-backups"
)

// Sample GCS service account JSON for testing
const TestGCSServiceAccountJSON = `{
  "type": "service_account",
  "project_id": "test-project",
  "private_key_id": "key-id-12345",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy0AHB7MSpVrLgCJhKJ...\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "backup-sa@test-project.iam.gserviceaccount.com",
  "client_id": "123456789012345678901",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/backup-sa%40test-project.iam.gserviceaccount.com"
}`

// ========================================
// S3 Storage Configuration Fixtures
// ========================================

// NewS3StorageConfig creates a basic S3 storage configuration
func NewS3StorageConfig() dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeS3,
		S3: &dbopsv1alpha1.S3StorageConfig{
			Bucket: TestS3Bucket,
			Region: TestS3Region,
			Prefix: TestS3Prefix,
			SecretRef: dbopsv1alpha1.S3SecretRef{
				Name: TestS3SecretName,
			},
		},
	}
}

// NewS3StorageConfigWithEndpoint creates an S3 storage configuration with a custom endpoint (for MinIO)
func NewS3StorageConfigWithEndpoint(endpoint string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeS3,
		S3: &dbopsv1alpha1.S3StorageConfig{
			Bucket:         TestS3Bucket,
			Region:         TestS3Region,
			Prefix:         TestS3Prefix,
			Endpoint:       endpoint,
			ForcePathStyle: true,
			SecretRef: dbopsv1alpha1.S3SecretRef{
				Name: TestS3SecretName,
			},
		},
	}
}

// NewMinIOStorageConfig creates a storage configuration for MinIO
func NewMinIOStorageConfig() dbopsv1alpha1.StorageConfig {
	return NewS3StorageConfigWithEndpoint(TestMinIOEndpoint)
}

// NewS3StorageConfigWithKeys creates an S3 storage configuration with custom secret keys
func NewS3StorageConfigWithKeys(accessKeyName, secretKeyName string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeS3,
		S3: &dbopsv1alpha1.S3StorageConfig{
			Bucket: TestS3Bucket,
			Region: TestS3Region,
			Prefix: TestS3Prefix,
			SecretRef: dbopsv1alpha1.S3SecretRef{
				Name: TestS3SecretName,
				Keys: &dbopsv1alpha1.S3SecretKeys{
					AccessKey: accessKeyName,
					SecretKey: secretKeyName,
				},
			},
		},
	}
}

// NewS3StorageConfigFull creates a fully customized S3 storage configuration
func NewS3StorageConfigFull(bucket, region, prefix, endpoint, secretName string, forcePathStyle bool) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeS3,
		S3: &dbopsv1alpha1.S3StorageConfig{
			Bucket:         bucket,
			Region:         region,
			Prefix:         prefix,
			Endpoint:       endpoint,
			ForcePathStyle: forcePathStyle,
			SecretRef: dbopsv1alpha1.S3SecretRef{
				Name: secretName,
			},
		},
	}
}

// ========================================
// GCS Storage Configuration Fixtures
// ========================================

// NewGCSStorageConfig creates a basic GCS storage configuration
func NewGCSStorageConfig() dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeGCS,
		GCS: &dbopsv1alpha1.GCSStorageConfig{
			Bucket: TestGCSBucket,
			Prefix: TestGCSPrefix,
			SecretRef: dbopsv1alpha1.SecretKeySelector{
				Name: TestGCSSecretName,
				Key:  TestGCSSecretKey,
			},
		},
	}
}

// NewGCSStorageConfigWithNamespace creates a GCS storage configuration with a secret in a specific namespace
func NewGCSStorageConfigWithNamespace(secretNamespace string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeGCS,
		GCS: &dbopsv1alpha1.GCSStorageConfig{
			Bucket: TestGCSBucket,
			Prefix: TestGCSPrefix,
			SecretRef: dbopsv1alpha1.SecretKeySelector{
				Name:      TestGCSSecretName,
				Namespace: secretNamespace,
				Key:       TestGCSSecretKey,
			},
		},
	}
}

// NewGCSStorageConfigFull creates a fully customized GCS storage configuration
func NewGCSStorageConfigFull(bucket, prefix, secretName, secretNamespace, secretKey string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeGCS,
		GCS: &dbopsv1alpha1.GCSStorageConfig{
			Bucket: bucket,
			Prefix: prefix,
			SecretRef: dbopsv1alpha1.SecretKeySelector{
				Name:      secretName,
				Namespace: secretNamespace,
				Key:       secretKey,
			},
		},
	}
}

// ========================================
// Azure Storage Configuration Fixtures
// ========================================

// NewAzureStorageConfig creates a basic Azure Blob storage configuration
func NewAzureStorageConfig() dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeAzure,
		Azure: &dbopsv1alpha1.AzureStorageConfig{
			Container:      TestAzureContainer,
			StorageAccount: TestAzureStorageAccount,
			Prefix:         TestAzurePrefix,
			SecretRef: dbopsv1alpha1.SecretReference{
				Name: TestAzureSecretName,
			},
		},
	}
}

// NewAzureStorageConfigWithNamespace creates an Azure storage configuration with a secret in a specific namespace
func NewAzureStorageConfigWithNamespace(secretNamespace string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeAzure,
		Azure: &dbopsv1alpha1.AzureStorageConfig{
			Container:      TestAzureContainer,
			StorageAccount: TestAzureStorageAccount,
			Prefix:         TestAzurePrefix,
			SecretRef: dbopsv1alpha1.SecretReference{
				Name:      TestAzureSecretName,
				Namespace: secretNamespace,
			},
		},
	}
}

// NewAzureStorageConfigFull creates a fully customized Azure storage configuration
func NewAzureStorageConfigFull(container, storageAccount, prefix, secretName, secretNamespace string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypeAzure,
		Azure: &dbopsv1alpha1.AzureStorageConfig{
			Container:      container,
			StorageAccount: storageAccount,
			Prefix:         prefix,
			SecretRef: dbopsv1alpha1.SecretReference{
				Name:      secretName,
				Namespace: secretNamespace,
			},
		},
	}
}

// ========================================
// PVC Storage Configuration Fixtures
// ========================================

// NewPVCStorageConfig creates a basic PVC storage configuration
func NewPVCStorageConfig() dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypePVC,
		PVC: &dbopsv1alpha1.PVCStorageConfig{
			ClaimName: TestPVCName,
		},
	}
}

// NewPVCStorageConfigWithSubPath creates a PVC storage configuration with a sub-path
func NewPVCStorageConfigWithSubPath(subPath string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypePVC,
		PVC: &dbopsv1alpha1.PVCStorageConfig{
			ClaimName: TestPVCName,
			SubPath:   subPath,
		},
	}
}

// NewPVCStorageConfigFull creates a fully customized PVC storage configuration
func NewPVCStorageConfigFull(claimName, subPath string) dbopsv1alpha1.StorageConfig {
	return dbopsv1alpha1.StorageConfig{
		Type: dbopsv1alpha1.StorageTypePVC,
		PVC: &dbopsv1alpha1.PVCStorageConfig{
			ClaimName: claimName,
			SubPath:   subPath,
		},
	}
}

// ========================================
// Secret Fixtures for Storage
// ========================================

// NewS3CredentialSecret creates an S3 credential secret for testing
func NewS3CredentialSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte(TestS3AccessKey),
			"AWS_SECRET_ACCESS_KEY": []byte(TestS3SecretKey),
		},
	}
}

// NewS3CredentialSecretWithKeys creates an S3 credential secret with custom key names
func NewS3CredentialSecretWithKeys(name, namespace, accessKeyName, secretKeyName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			accessKeyName: []byte(TestS3AccessKey),
			secretKeyName: []byte(TestS3SecretKey),
		},
	}
}

// NewS3CredentialSecretWithData creates an S3 credential secret with specific credentials
func NewS3CredentialSecretWithData(name, namespace, accessKey, secretKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"AWS_ACCESS_KEY_ID":     []byte(accessKey),
			"AWS_SECRET_ACCESS_KEY": []byte(secretKey),
		},
	}
}

// NewGCSCredentialSecret creates a GCS credential secret for testing
func NewGCSCredentialSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			TestGCSSecretKey: []byte(TestGCSServiceAccountJSON),
		},
	}
}

// NewGCSCredentialSecretWithKey creates a GCS credential secret with a custom key name
func NewGCSCredentialSecretWithKey(name, namespace, keyName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			keyName: []byte(TestGCSServiceAccountJSON),
		},
	}
}

// NewGCSCredentialSecretWithData creates a GCS credential secret with specific JSON data
func NewGCSCredentialSecretWithData(name, namespace, jsonData string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			TestGCSSecretKey: []byte(jsonData),
		},
	}
}

// NewAzureCredentialSecret creates an Azure credential secret for testing (using storage account key)
func NewAzureCredentialSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"AZURE_STORAGE_ACCOUNT": []byte(TestAzureStorageAccount),
			"AZURE_STORAGE_KEY":     []byte("dGVzdC1zdG9yYWdlLWtleS1iYXNlNjQtZW5jb2RlZA=="),
		},
	}
}

// NewAzureCredentialSecretWithConnectionString creates an Azure credential secret using a connection string
func NewAzureCredentialSecretWithConnectionString(name, namespace, connectionString string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"AZURE_STORAGE_CONNECTION_STRING": []byte(connectionString),
		},
	}
}

// NewAzureCredentialSecretWithData creates an Azure credential secret with specific credentials
func NewAzureCredentialSecretWithData(name, namespace, storageAccount, storageKey string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"AZURE_STORAGE_ACCOUNT": []byte(storageAccount),
			"AZURE_STORAGE_KEY":     []byte(storageKey),
		},
	}
}

// ========================================
// SecretRef Fixtures
// ========================================

// NewSecretKeySelector creates a SecretKeySelector for testing
func NewSecretKeySelector(name, key string) dbopsv1alpha1.SecretKeySelector {
	return dbopsv1alpha1.SecretKeySelector{
		Name: name,
		Key:  key,
	}
}

// NewSecretKeySelectorWithNamespace creates a SecretKeySelector with namespace
func NewSecretKeySelectorWithNamespace(name, namespace, key string) dbopsv1alpha1.SecretKeySelector {
	return dbopsv1alpha1.SecretKeySelector{
		Name:      name,
		Namespace: namespace,
		Key:       key,
	}
}

// NewSecretReference creates a SecretReference for testing
func NewSecretReference(name string) dbopsv1alpha1.SecretReference {
	return dbopsv1alpha1.SecretReference{
		Name: name,
	}
}

// NewSecretReferenceWithNamespace creates a SecretReference with namespace
func NewSecretReferenceWithNamespace(name, namespace string) dbopsv1alpha1.SecretReference {
	return dbopsv1alpha1.SecretReference{
		Name:      name,
		Namespace: namespace,
	}
}

// NewS3SecretRef creates an S3SecretRef for testing
func NewS3SecretRef(name string) dbopsv1alpha1.S3SecretRef {
	return dbopsv1alpha1.S3SecretRef{
		Name: name,
	}
}

// NewS3SecretRefWithKeys creates an S3SecretRef with custom keys
func NewS3SecretRefWithKeys(name, accessKeyName, secretKeyName string) dbopsv1alpha1.S3SecretRef {
	return dbopsv1alpha1.S3SecretRef{
		Name: name,
		Keys: &dbopsv1alpha1.S3SecretKeys{
			AccessKey: accessKeyName,
			SecretKey: secretKeyName,
		},
	}
}

// ========================================
// Compression and Encryption Configuration Fixtures
// ========================================

// NewCompressionConfig creates a compression configuration for testing
func NewCompressionConfig(algorithm dbopsv1alpha1.CompressionAlgorithm, level int32) *dbopsv1alpha1.CompressionConfig {
	return &dbopsv1alpha1.CompressionConfig{
		Enabled:   true,
		Algorithm: algorithm,
		Level:     level,
	}
}

// NewCompressionConfigGzip creates a gzip compression configuration
func NewCompressionConfigGzip() *dbopsv1alpha1.CompressionConfig {
	return NewCompressionConfig(dbopsv1alpha1.CompressionGzip, 6)
}

// NewCompressionConfigLZ4 creates an LZ4 compression configuration
func NewCompressionConfigLZ4() *dbopsv1alpha1.CompressionConfig {
	return NewCompressionConfig(dbopsv1alpha1.CompressionLZ4, 1)
}

// NewCompressionConfigZstd creates a Zstandard compression configuration
func NewCompressionConfigZstd() *dbopsv1alpha1.CompressionConfig {
	return NewCompressionConfig(dbopsv1alpha1.CompressionZstd, 3)
}

// NewCompressionConfigDisabled creates a disabled compression configuration
func NewCompressionConfigDisabled() *dbopsv1alpha1.CompressionConfig {
	return &dbopsv1alpha1.CompressionConfig{
		Enabled: false,
	}
}

// NewEncryptionConfig creates an encryption configuration for testing
func NewEncryptionConfig(secretName, secretKey string) *dbopsv1alpha1.EncryptionConfig {
	return &dbopsv1alpha1.EncryptionConfig{
		Enabled:   true,
		Algorithm: dbopsv1alpha1.EncryptionAES256GCM,
		SecretRef: &dbopsv1alpha1.SecretKeySelector{
			Name: secretName,
			Key:  secretKey,
		},
	}
}

// NewEncryptionConfigWithAlgorithm creates an encryption configuration with a specific algorithm
func NewEncryptionConfigWithAlgorithm(algorithm dbopsv1alpha1.EncryptionAlgorithm, secretName, secretKey string) *dbopsv1alpha1.EncryptionConfig {
	return &dbopsv1alpha1.EncryptionConfig{
		Enabled:   true,
		Algorithm: algorithm,
		SecretRef: &dbopsv1alpha1.SecretKeySelector{
			Name: secretName,
			Key:  secretKey,
		},
	}
}

// NewEncryptionConfigDisabled creates a disabled encryption configuration
func NewEncryptionConfigDisabled() *dbopsv1alpha1.EncryptionConfig {
	return &dbopsv1alpha1.EncryptionConfig{
		Enabled: false,
	}
}

// NewEncryptionKeySecret creates a secret containing an encryption key for testing
func NewEncryptionKeySecret(name, namespace, keyName string) *corev1.Secret {
	// 32-byte key for AES-256
	key := []byte("01234567890123456789012345678901")
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			keyName: key,
		},
	}
}

// ========================================
// Retention Policy Fixtures
// ========================================

// NewRetentionPolicy creates a retention policy for testing
func NewRetentionPolicy(keepLast, keepDaily, keepWeekly, keepMonthly int32) *dbopsv1alpha1.RetentionPolicy {
	return &dbopsv1alpha1.RetentionPolicy{
		KeepLast:    keepLast,
		KeepDaily:   keepDaily,
		KeepWeekly:  keepWeekly,
		KeepMonthly: keepMonthly,
	}
}

// NewRetentionPolicySimple creates a simple retention policy keeping only the last N backups
func NewRetentionPolicySimple(keepLast int32) *dbopsv1alpha1.RetentionPolicy {
	return &dbopsv1alpha1.RetentionPolicy{
		KeepLast: keepLast,
	}
}

// NewRetentionPolicyFull creates a full retention policy with all options
func NewRetentionPolicyFull(keepLast, keepHourly, keepDaily, keepWeekly, keepMonthly, keepYearly int32, minAge string) *dbopsv1alpha1.RetentionPolicy {
	return &dbopsv1alpha1.RetentionPolicy{
		KeepLast:    keepLast,
		KeepHourly:  keepHourly,
		KeepDaily:   keepDaily,
		KeepWeekly:  keepWeekly,
		KeepMonthly: keepMonthly,
		KeepYearly:  keepYearly,
		MinAge:      minAge,
	}
}

// ========================================
// Sample Data for Testing
// ========================================

var (
	// SampleStorageTypes provides all supported storage types
	SampleStorageTypes = []dbopsv1alpha1.StorageType{
		dbopsv1alpha1.StorageTypeS3,
		dbopsv1alpha1.StorageTypeGCS,
		dbopsv1alpha1.StorageTypeAzure,
		dbopsv1alpha1.StorageTypePVC,
	}

	// SampleS3Regions provides common S3 regions for testing
	SampleS3Regions = []string{
		"us-east-1",
		"us-west-2",
		"eu-west-1",
		"ap-southeast-1",
		"ap-northeast-1",
	}

	// SampleCompressionAlgorithms provides all supported compression algorithms
	SampleCompressionAlgorithms = []dbopsv1alpha1.CompressionAlgorithm{
		dbopsv1alpha1.CompressionGzip,
		dbopsv1alpha1.CompressionLZ4,
		dbopsv1alpha1.CompressionZstd,
		dbopsv1alpha1.CompressionNone,
	}

	// SampleEncryptionAlgorithms provides all supported encryption algorithms
	SampleEncryptionAlgorithms = []dbopsv1alpha1.EncryptionAlgorithm{
		dbopsv1alpha1.EncryptionAES256GCM,
		dbopsv1alpha1.EncryptionAES256CBC,
	}

	// SampleBackupData provides sample backup data for testing
	SampleBackupData = map[string][]byte{
		"backups/testdb/backup-001.sql":    []byte("-- SQL dump for testdb\nCREATE TABLE users (id INT);\n"),
		"backups/testdb/backup-002.sql.gz": []byte{0x1f, 0x8b, 0x08, 0x00}, // gzip magic bytes
		"backups/myapp/backup-001.sql":     []byte("-- SQL dump for myapp\nCREATE TABLE orders (id INT);\n"),
	}
)
