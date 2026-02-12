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

package clusterrole

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
)

// Repository handles cluster role operations via the service layer.
// Unlike the namespaced role repository, this only works with ClusterDatabaseInstance.
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

// NewRepository creates a new cluster role repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// withService creates a role service connection and executes the given function.
// ClusterDatabaseRole only supports ClusterDatabaseInstance (cluster-scoped).
func (r *Repository) withService(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, fn func(svc *service.RoleService, instanceSpec *dbopsv1alpha1.DatabaseInstanceSpec) error) error {
	// Get the ClusterDatabaseInstance
	instance, err := r.GetInstance(ctx, spec)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}

	// Check if instance is ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return fmt.Errorf("instance not ready: phase is %s", instance.Status.Phase)
	}

	// Get admin credentials from the operator namespace (cluster-scoped instances use operator namespace)
	credentialNamespace := instance.Spec.Connection.SecretRef.Namespace
	if credentialNamespace == "" {
		// Default to operator namespace for cluster-scoped resources
		credentialNamespace = "db-provision-operator-system"
	}

	creds, err := r.secretManager.GetCredentials(ctx, credentialNamespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err := r.secretManager.GetTLSCredentials(ctx, credentialNamespace, instance.Spec.TLS)
		if err == nil {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config
	cfg := service.ConfigFromInstance(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = logf.FromContext(ctx)

	// Create role service
	svc, err := service.NewRoleService(cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	return fn(svc, &instance.Spec)
}

// Create creates a new cluster-scoped database role.
func (r *Repository) Create(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error) {
	var result *Result

	// Convert ClusterDatabaseRoleSpec to DatabaseRoleSpec for service compatibility
	roleSpec := r.toRoleSpec(spec)

	err := r.withService(ctx, spec, func(svc *service.RoleService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		svcResult, err := svc.Create(ctx, roleSpec)
		if err != nil {
			return fmt.Errorf("create role: %w", err)
		}

		result = &Result{
			Created: svcResult.Created,
			Updated: svcResult.Updated,
			Message: svcResult.Message,
		}
		return nil
	})

	return result, err
}

// Exists checks if a role exists.
func (r *Repository) Exists(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
	var exists bool

	err := r.withService(ctx, spec, func(svc *service.RoleService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		var err error
		exists, err = svc.Exists(ctx, roleName)
		return err
	})

	return exists, err
}

// Update updates role settings.
func (r *Repository) Update(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error) {
	var result *Result

	roleSpec := r.toRoleSpec(spec)

	err := r.withService(ctx, spec, func(svc *service.RoleService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		svcResult, err := svc.Update(ctx, roleName, roleSpec)
		if err != nil {
			return fmt.Errorf("update role: %w", err)
		}

		result = &Result{
			Updated: true,
			Message: svcResult.Message,
		}
		return nil
	})

	return result, err
}

// Delete deletes a cluster-scoped database role.
func (r *Repository) Delete(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error {
	return r.withService(ctx, spec, func(svc *service.RoleService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		_, err := svc.Delete(ctx, roleName)
		return err
	})
}

// GetInstance returns the ClusterDatabaseInstance for a given spec.
func (r *Repository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{}

	// ClusterDatabaseInstance is cluster-scoped (no namespace)
	if err := r.client.Get(ctx, client.ObjectKey{
		Name: spec.ClusterInstanceRef.Name,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// GetEngine returns the database engine type for a given spec.
func (r *Repository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (string, error) {
	instance, err := r.GetInstance(ctx, spec)
	if err != nil {
		return "", err
	}
	return string(instance.Spec.Engine), nil
}

// DetectDrift detects configuration drift between the CR spec and actual role state.
func (r *Repository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
	var result *drift.Result

	roleSpec := r.toRoleSpec(spec)

	err := r.withService(ctx, spec, func(svc *service.RoleService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		result, err = driftSvc.DetectRoleDrift(ctx, roleSpec)
		return err
	})

	return result, err
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
func (r *Repository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	var correctionResult *drift.CorrectionResult

	roleSpec := r.toRoleSpec(spec)

	err := r.withService(ctx, spec, func(svc *service.RoleService, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		correctionResult, err = driftSvc.CorrectRoleDrift(ctx, roleSpec, driftResult)
		return err
	})

	return correctionResult, err
}

// toRoleSpec converts ClusterDatabaseRoleSpec to DatabaseRoleSpec for service compatibility.
// The service layer uses DatabaseRoleSpec, so we need to adapt the cluster-scoped spec.
func (r *Repository) toRoleSpec(spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) *dbopsv1alpha1.DatabaseRoleSpec {
	return &dbopsv1alpha1.DatabaseRoleSpec{
		// ClusterDatabaseRole only uses ClusterInstanceRef, convert to the ref format
		ClusterInstanceRef: &spec.ClusterInstanceRef,
		RoleName:           spec.RoleName,
		Postgres:           spec.Postgres,
		MySQL:              spec.MySQL,
		DriftPolicy:        spec.DriftPolicy,
	}
}
