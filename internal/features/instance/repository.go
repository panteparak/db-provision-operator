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

package instance

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service"
)

// Repository handles instance operations via the service layer.
type Repository struct {
	client        client.Client
	secretManager *secret.Manager
	logger        logr.Logger
}

// RepositoryConfig holds dependencies for the repository.
type RepositoryConfig struct {
	Client        client.Client
	SecretManager *secret.Manager
	Logger        logr.Logger
}

// NewRepository creates a new instance repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// withService creates an instance service connection and executes the given function.
func (r *Repository) withService(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, fn func(svc *service.InstanceService) error) error {
	// Get credentials
	creds, err := r.secretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err := r.secretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to get TLS credentials")
		} else {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config
	cfg := service.ConfigFromInstance(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = logf.FromContext(ctx)

	// Create instance service
	svc, err := service.NewInstanceService(cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	// Connect
	if err := svc.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// Execute the function with the service
	return fn(svc)
}

// Connect establishes a connection and verifies it with a ping.
func (r *Repository) Connect(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error) {
	var result ConnectResult

	err := r.withService(ctx, instance, func(svc *service.InstanceService) error {
		// Ping to verify connection
		if err := svc.Ping(ctx); err != nil {
			return fmt.Errorf("ping: %w", err)
		}

		// Get version
		version, err := svc.GetVersion(ctx)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to get version")
		}

		result.Connected = true
		result.Version = version
		result.Message = "Connected successfully"
		return nil
	})

	return &result, err
}

// Ping verifies the connection is still active.
func (r *Repository) Ping(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) error {
	return r.withService(ctx, instance, func(svc *service.InstanceService) error {
		return svc.Ping(ctx)
	})
}

// GetVersion returns the database version.
func (r *Repository) GetVersion(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (string, error) {
	var version string

	err := r.withService(ctx, instance, func(svc *service.InstanceService) error {
		var err error
		version, err = svc.GetVersion(ctx)
		return err
	})

	return version, err
}

// GetInstance retrieves a DatabaseInstance by name and namespace.
func (r *Repository) GetInstance(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, instance); err != nil {
		return nil, err
	}
	return instance, nil
}
