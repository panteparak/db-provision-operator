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
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
)

// S3Backend implements Backend for S3-compatible storage
type S3Backend struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3Backend creates a new S3 storage backend
func NewS3Backend(ctx context.Context, s3Config *dbopsv1alpha1.S3StorageConfig, secretManager *secret.Manager, namespace string) (*S3Backend, error) {
	if s3Config.Bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}
	if s3Config.Region == "" {
		return nil, fmt.Errorf("S3 region is required")
	}

	// Get credentials from secret
	accessKey, secretKey, err := getS3Credentials(ctx, secretManager, s3Config.SecretRef, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 credentials: %w", err)
	}

	// Build AWS config options
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(s3Config.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	}

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with optional endpoint and path style
	s3Opts := []func(*s3.Options){}
	if s3Config.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3Config.Endpoint)
		})
	}
	if s3Config.ForcePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(cfg, s3Opts...)

	return &S3Backend{
		client: client,
		bucket: s3Config.Bucket,
		prefix: strings.TrimPrefix(s3Config.Prefix, "/"),
	}, nil
}

// getS3Credentials retrieves S3 credentials from a Kubernetes secret
func getS3Credentials(ctx context.Context, secretManager *secret.Manager, secretRef dbopsv1alpha1.S3SecretRef, namespace string) (string, string, error) {
	// Determine key names
	accessKeyName := "AWS_ACCESS_KEY_ID"
	secretKeyName := "AWS_SECRET_ACCESS_KEY"
	if secretRef.Keys != nil {
		if secretRef.Keys.AccessKey != "" {
			accessKeyName = secretRef.Keys.AccessKey
		}
		if secretRef.Keys.SecretKey != "" {
			secretKeyName = secretRef.Keys.SecretKey
		}
	}

	// Get secret data
	secretData, err := secretManager.GetSecretData(ctx, secretRef.Name, namespace)
	if err != nil {
		return "", "", fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	accessKey, ok := secretData[accessKeyName]
	if !ok {
		return "", "", fmt.Errorf("secret %s does not contain key %s", secretRef.Name, accessKeyName)
	}

	secretKey, ok := secretData[secretKeyName]
	if !ok {
		return "", "", fmt.Errorf("secret %s does not contain key %s", secretRef.Name, secretKeyName)
	}

	return accessKey, secretKey, nil
}

// buildKey creates the full key path including prefix
func (b *S3Backend) buildKey(objectPath string) string {
	if b.prefix == "" {
		return objectPath
	}
	return path.Join(b.prefix, objectPath)
}

// Write writes data to S3 at the specified path
func (b *S3Backend) Write(ctx context.Context, objectPath string, reader io.Reader) error {
	key := b.buildKey(objectPath)

	// Read all data into memory (required for S3 PutObject)
	// For large files, consider using multipart upload
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	_, err = b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// Read reads data from S3 at the specified path
func (b *S3Backend) Read(ctx context.Context, objectPath string) (io.ReadCloser, error) {
	key := b.buildKey(objectPath)

	result, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}

	return result.Body, nil
}

// Delete deletes the object at the specified path
func (b *S3Backend) Delete(ctx context.Context, objectPath string) error {
	key := b.buildKey(objectPath)

	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object from S3: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the specified path
func (b *S3Backend) Exists(ctx context.Context, objectPath string) (bool, error) {
	key := b.buildKey(objectPath)

	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// List lists objects with the specified prefix
func (b *S3Backend) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	fullPrefix := b.buildKey(prefix)

	var objects []ObjectInfo
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		for _, obj := range page.Contents {
			// Remove prefix from path for relative path
			relativePath := strings.TrimPrefix(*obj.Key, b.prefix+"/")
			if b.prefix == "" {
				relativePath = *obj.Key
			}

			objects = append(objects, ObjectInfo{
				Path:         relativePath,
				Size:         *obj.Size,
				LastModified: obj.LastModified.Unix(),
			})
		}
	}

	return objects, nil
}

// GetSize returns the size of the object at the specified path
func (b *S3Backend) GetSize(ctx context.Context, objectPath string) (int64, error) {
	key := b.buildKey(objectPath)

	result, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get object metadata: %w", err)
	}

	return *result.ContentLength, nil
}

// Close closes the S3 backend (no-op for S3)
func (b *S3Backend) Close() error {
	return nil
}
