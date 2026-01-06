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

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// AzureBackend implements Backend for Azure Blob Storage
type AzureBackend struct {
	client         *azblob.Client
	containerName  string
	prefix         string
	storageAccount string
}

// NewAzureBackend creates a new Azure Blob storage backend
func NewAzureBackend(ctx context.Context, azureConfig *dbopsv1alpha1.AzureStorageConfig, secretManager *secret.Manager, namespace string) (*AzureBackend, error) {
	if azureConfig.Container == "" {
		return nil, fmt.Errorf("Azure container name is required")
	}
	if azureConfig.StorageAccount == "" {
		return nil, fmt.Errorf("Azure storage account is required")
	}

	// Get credentials from secret
	accountKey, err := getAzureCredentials(ctx, secretManager, azureConfig.SecretRef, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get Azure credentials: %w", err)
	}

	// Create Azure Blob client with shared key credential
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", azureConfig.StorageAccount)

	cred, err := azblob.NewSharedKeyCredential(azureConfig.StorageAccount, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}

	client, err := azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}

	return &AzureBackend{
		client:         client,
		containerName:  azureConfig.Container,
		prefix:         strings.TrimPrefix(azureConfig.Prefix, "/"),
		storageAccount: azureConfig.StorageAccount,
	}, nil
}

// getAzureCredentials retrieves Azure credentials from a Kubernetes secret
func getAzureCredentials(ctx context.Context, secretManager *secret.Manager, secretRef dbopsv1alpha1.SecretReference, namespace string) (string, error) {
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

	// Look for account key in common key names
	keyNames := []string{"AZURE_STORAGE_ACCOUNT_KEY", "accountKey", "storageAccountKey", "key"}
	for _, keyName := range keyNames {
		if key, ok := secretData[keyName]; ok {
			return key, nil
		}
	}

	return "", fmt.Errorf("secret %s does not contain Azure storage account key (tried: %v)", secretRef.Name, keyNames)
}

// buildKey creates the full key path including prefix
func (b *AzureBackend) buildKey(objectPath string) string {
	if b.prefix == "" {
		return objectPath
	}
	return path.Join(b.prefix, objectPath)
}

// Write writes data to Azure Blob at the specified path
func (b *AzureBackend) Write(ctx context.Context, objectPath string, reader io.Reader) error {
	key := b.buildKey(objectPath)

	// Read all data into memory for upload
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	_, err = b.client.UploadBuffer(ctx, b.containerName, key, data, nil)
	if err != nil {
		return fmt.Errorf("failed to upload to Azure Blob: %w", err)
	}

	return nil
}

// Read reads data from Azure Blob at the specified path
func (b *AzureBackend) Read(ctx context.Context, objectPath string) (io.ReadCloser, error) {
	key := b.buildKey(objectPath)

	resp, err := b.client.DownloadStream(ctx, b.containerName, key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to download from Azure Blob: %w", err)
	}

	return resp.Body, nil
}

// Delete deletes the object at the specified path
func (b *AzureBackend) Delete(ctx context.Context, objectPath string) error {
	key := b.buildKey(objectPath)

	_, err := b.client.DeleteBlob(ctx, b.containerName, key, nil)
	if err != nil {
		// Check if blob doesn't exist (already deleted)
		if strings.Contains(err.Error(), "BlobNotFound") {
			return nil
		}
		return fmt.Errorf("failed to delete from Azure Blob: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the specified path
func (b *AzureBackend) Exists(ctx context.Context, objectPath string) (bool, error) {
	key := b.buildKey(objectPath)

	blobClient := b.client.ServiceClient().NewContainerClient(b.containerName).NewBlobClient(key)
	_, err := blobClient.GetProperties(ctx, &blob.GetPropertiesOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "BlobNotFound") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check blob existence: %w", err)
	}

	return true, nil
}

// List lists objects with the specified prefix
func (b *AzureBackend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	fullPrefix := b.buildKey(prefix)

	var objects []ObjectInfo

	containerClient := b.client.ServiceClient().NewContainerClient(b.containerName)
	pager := containerClient.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: &fullPrefix,
	})

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list blobs: %w", err)
		}

		for _, blobInfo := range page.Segment.BlobItems {
			// Remove prefix from path for relative path
			relativePath := strings.TrimPrefix(*blobInfo.Name, b.prefix+"/")
			if b.prefix == "" {
				relativePath = *blobInfo.Name
			}

			var lastModified int64
			if blobInfo.Properties.LastModified != nil {
				lastModified = blobInfo.Properties.LastModified.Unix()
			}

			var size int64
			if blobInfo.Properties.ContentLength != nil {
				size = *blobInfo.Properties.ContentLength
			}

			objects = append(objects, ObjectInfo{
				Path:         relativePath,
				Size:         size,
				LastModified: lastModified,
			})
		}
	}

	return objects, nil
}

// GetSize returns the size of the object at the specified path
func (b *AzureBackend) GetSize(ctx context.Context, objectPath string) (int64, error) {
	key := b.buildKey(objectPath)

	blobClient := b.client.ServiceClient().NewContainerClient(b.containerName).NewBlobClient(key)
	props, err := blobClient.GetProperties(ctx, &blob.GetPropertiesOptions{})
	if err != nil {
		return 0, fmt.Errorf("failed to get blob properties: %w", err)
	}

	return *props.ContentLength, nil
}

// Close closes the Azure backend (no-op for Azure)
func (b *AzureBackend) Close() error {
	return nil
}
