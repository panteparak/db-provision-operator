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

package database

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// Repository handles database operations via the service layer.
// It encapsulates the logic for creating database connections and executing operations.
type Repository struct {
	client           client.Client
	secretManager    *secret.Manager
	instanceResolver *instanceresolver.Resolver
	logger           logr.Logger
}

// RepositoryConfig holds dependencies for the repository.
type RepositoryConfig struct {
	Client        client.Client
	SecretManager *secret.Manager
	Logger        logr.Logger
}

// NewRepository creates a new database repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:           cfg.Client,
		secretManager:    cfg.SecretManager,
		instanceResolver: instanceresolver.New(cfg.Client),
		logger:           cfg.Logger,
	}
}

// withService creates a database service connection and executes the given function.
// It handles all the boilerplate of getting the instance, credentials, and connecting.
// Supports both namespaced DatabaseInstance and cluster-scoped ClusterDatabaseInstance.
func (r *Repository) withService(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, fn func(svc *service.DatabaseService, instanceSpec *dbopsv1alpha1.DatabaseInstanceSpec) error) error {
	// Resolve the instance (supports both instanceRef and clusterInstanceRef)
	resolved, err := r.instanceResolver.Resolve(ctx, spec.InstanceRef, spec.ClusterInstanceRef, namespace)
	if err != nil {
		return fmt.Errorf("resolve instance: %w", err)
	}

	// Check if instance is ready
	if !resolved.IsReady() {
		return fmt.Errorf("instance not ready: phase is %s", resolved.Phase)
	}

	// Get credentials from the credential namespace
	creds, err := r.secretManager.GetCredentials(ctx, resolved.CredentialNamespace, resolved.Spec.Connection.SecretRef)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if resolved.Spec.TLS != nil && resolved.Spec.TLS.Enabled {
		tlsCreds, err := r.secretManager.GetTLSCredentials(ctx, resolved.CredentialNamespace, resolved.Spec.TLS)
		if err != nil {
			logf.FromContext(ctx).Error(err, "Failed to get TLS credentials")
		} else {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config
	cfg := service.ConfigFromInstance(resolved.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = logf.FromContext(ctx)

	// Create database service
	svc, err := service.NewDatabaseService(cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	// Connect
	if err := svc.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	// Execute the function with the service
	return fn(svc, resolved.Spec)
}

// Create creates a new database.
func (r *Repository) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		svcResult, err := svc.CreateOnly(ctx, spec)
		if err != nil {
			return fmt.Errorf("create database: %w", err)
		}

		result = &Result{
			Created: svcResult.Created,
			Message: svcResult.Message,
		}
		return nil
	})

	return result, err
}

// Exists checks if a database exists.
func (r *Repository) Exists(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
	var exists bool

	err := r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		var err error
		exists, err = svc.Exists(ctx, name)
		return err
	})

	return exists, err
}

// Update updates database settings.
func (r *Repository) Update(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		svcResult, err := svc.Update(ctx, name, spec)
		if err != nil {
			return fmt.Errorf("update database: %w", err)
		}

		result = &Result{
			Updated: svcResult.Created, // Service uses Created field for updates too
			Message: svcResult.Message,
		}
		return nil
	})

	return result, err
}

// Delete deletes a database.
func (r *Repository) Delete(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
	return r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		_, err := svc.Delete(ctx, name, force)
		return err
	})
}

// VerifyAccess verifies that a database is accepting connections.
func (r *Repository) VerifyAccess(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
	return r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		return svc.VerifyAccess(ctx, name)
	})
}

// GetInfo gets information about a database.
func (r *Repository) GetInfo(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
	var info *Info

	err := r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		dbInfo, err := svc.Get(ctx, name)
		if err != nil {
			return err
		}

		if dbInfo != nil {
			info = &Info{
				Name:      dbInfo.Name,
				Owner:     dbInfo.Owner,
				SizeBytes: dbInfo.SizeBytes,
				Encoding:  dbInfo.Encoding,
				Collation: dbInfo.Collation,
				Charset:   dbInfo.Charset,
			}
		}
		return nil
	})

	return info, err
}

// GetInstance returns the DatabaseInstance for a given spec.
// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
// This method only works with namespaced DatabaseInstance references.
func (r *Repository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	// Only works with namespaced instances
	if spec.InstanceRef == nil {
		return nil, fmt.Errorf("instanceRef not specified (use ResolveInstance for clusterInstanceRef)")
	}

	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceRef := spec.InstanceRef
	instanceNamespace := namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	if err := r.client.Get(ctx, client.ObjectKey{
		Namespace: instanceNamespace,
		Name:      instanceRef.Name,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// ResolveInstance resolves the instance reference (supports both instanceRef and clusterInstanceRef).
func (r *Repository) ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*instanceresolver.ResolvedInstance, error) {
	return r.instanceResolver.Resolve(ctx, spec.InstanceRef, spec.ClusterInstanceRef, namespace)
}

// GetEngine returns the database engine type for a given spec.
func (r *Repository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error) {
	resolved, err := r.ResolveInstance(ctx, spec, namespace)
	if err != nil {
		return "", err
	}
	return resolved.Engine(), nil
}

// DetectDrift detects configuration drift between the CR spec and actual database state.
// This compares PostgreSQL settings (encoding, extensions, schemas) or MySQL settings
// (charset, collation) against the actual database and returns any differences found.
func (r *Repository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	var result *drift.Result

	err := r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		result, err = driftSvc.DetectDatabaseDrift(ctx, spec)
		return err
	})

	return result, err
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
// Only non-destructive corrections are applied unless allowDestructive is true.
// Returns a CorrectionResult showing what was corrected, skipped, or failed.
func (r *Repository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	var correctionResult *drift.CorrectionResult

	err := r.withService(ctx, spec, namespace, func(svc *service.DatabaseService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		correctionResult, err = driftSvc.CorrectDatabaseDrift(ctx, spec, driftResult)
		return err
	})

	return correctionResult, err
}
