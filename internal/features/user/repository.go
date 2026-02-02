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

package user

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

// Repository handles user operations via the service layer.
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

// NewRepository creates a new user repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// withService creates a user service connection and executes the given function.
func (r *Repository) withService(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, fn func(svc *service.UserService, instance *dbopsv1alpha1.DatabaseInstance) error) error {
	// Get the DatabaseInstance
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceRef := spec.InstanceRef
	instanceNamespace := namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      instanceRef.Name,
	}, instance); err != nil {
		return fmt.Errorf("get instance: %w", err)
	}

	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return fmt.Errorf("instance not ready: phase is %s", instance.Status.Phase)
	}

	// Get admin credentials
	creds, err := r.secretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err := r.secretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if err == nil {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config
	cfg := service.ConfigFromInstance(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = logf.FromContext(ctx)

	// Create user service
	svc, err := service.NewUserService(cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	return fn(svc, instance)
}

// Create creates a new database user.
func (r *Repository) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, namespace, func(svc *service.UserService, _ *dbopsv1alpha1.DatabaseInstance) error {
		svcResult, err := svc.Create(ctx, service.CreateUserServiceOptions{
			Spec:     spec,
			Password: password,
		})
		if err != nil {
			return fmt.Errorf("create user: %w", err)
		}

		result = &Result{
			Created: svcResult.Created,
			Message: svcResult.Message,
		}
		return nil
	})

	return result, err
}

// Exists checks if a user exists.
func (r *Repository) Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
	var exists bool

	err := r.withService(ctx, spec, namespace, func(svc *service.UserService, _ *dbopsv1alpha1.DatabaseInstance) error {
		var err error
		exists, err = svc.Exists(ctx, username)
		return err
	})

	return exists, err
}

// Update updates user settings.
func (r *Repository) Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, namespace, func(svc *service.UserService, _ *dbopsv1alpha1.DatabaseInstance) error {
		svcResult, err := svc.Update(ctx, username, spec)
		if err != nil {
			return fmt.Errorf("update user: %w", err)
		}

		result = &Result{
			Updated: true,
			Message: svcResult.Message,
		}
		return nil
	})

	return result, err
}

// Delete deletes a database user.
func (r *Repository) Delete(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, _ bool) error {
	return r.withService(ctx, spec, namespace, func(svc *service.UserService, _ *dbopsv1alpha1.DatabaseInstance) error {
		_, err := svc.Delete(ctx, username)
		return err
	})
}

// SetPassword sets a new password for a user.
func (r *Repository) SetPassword(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
	return r.withService(ctx, spec, namespace, func(svc *service.UserService, _ *dbopsv1alpha1.DatabaseInstance) error {
		_, err := svc.UpdatePassword(ctx, username, password)
		return err
	})
}

// GetInstance returns the DatabaseInstance for a given spec.
func (r *Repository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceRef := spec.InstanceRef
	instanceNamespace := namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      instanceRef.Name,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// GetEngine returns the database engine type for a given spec.
func (r *Repository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error) {
	instance, err := r.GetInstance(ctx, spec, namespace)
	if err != nil {
		return "", err
	}
	return string(instance.Spec.Engine), nil
}
