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

package grant

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
	"github.com/db-provision-operator/internal/service/drift"
)

// Repository handles grant operations via the service layer.
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

// NewRepository creates a new grant repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// withService creates a grant service connection and executes the given function.
func (r *Repository) withService(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, fn func(svc *service.GrantService, user *dbopsv1alpha1.DatabaseUser, instance *dbopsv1alpha1.DatabaseInstance) error) error {
	// Get the DatabaseUser
	user := &dbopsv1alpha1.DatabaseUser{}
	userNamespace := namespace
	if spec.UserRef.Namespace != "" {
		userNamespace = spec.UserRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: userNamespace,
		Name:      spec.UserRef.Name,
	}, user); err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	if user.Status.Phase != dbopsv1alpha1.PhaseReady {
		return fmt.Errorf("user not ready: phase is %s", user.Status.Phase)
	}

	// Get the DatabaseInstance from the user's instanceRef
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceNamespace := user.Namespace
	if user.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = user.Spec.InstanceRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      user.Spec.InstanceRef.Name,
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

	// Create grant service
	svc, err := service.NewGrantService(cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	return fn(svc, user, instance)
}

// Apply applies grants to a database user.
func (r *Repository) Apply(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, user *dbopsv1alpha1.DatabaseUser, _ *dbopsv1alpha1.DatabaseInstance) error {
		svcResult, err := svc.Apply(ctx, service.ApplyGrantServiceOptions{
			Username: user.Spec.Username,
			Spec:     spec,
		})
		if err != nil {
			return fmt.Errorf("apply grants: %w", err)
		}

		result = &Result{
			Applied:           true,
			Roles:             svcResult.AppliedRoles,
			DirectGrants:      int32(svcResult.AppliedDirectGrants),
			DefaultPrivileges: int32(svcResult.AppliedDefaultPrivileges),
			Message:           "Grants applied successfully",
		}
		return nil
	})

	return result, err
}

// Revoke revokes grants from a database user.
func (r *Repository) Revoke(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
	return r.withService(ctx, spec, namespace, func(svc *service.GrantService, user *dbopsv1alpha1.DatabaseUser, _ *dbopsv1alpha1.DatabaseInstance) error {
		_, err := svc.Revoke(ctx, service.ApplyGrantServiceOptions{
			Username: user.Spec.Username,
			Spec:     spec,
		})
		return err
	})
}

// Exists checks if grants have been applied by verifying the user has the expected grants.
func (r *Repository) Exists(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error) {
	var exists bool

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, user *dbopsv1alpha1.DatabaseUser, _ *dbopsv1alpha1.DatabaseInstance) error {
		// Check if user has any grants
		grants, err := svc.GetGrants(ctx, user.Spec.Username)
		if err != nil {
			return err
		}
		exists = len(grants) > 0
		return nil
	})

	return exists, err
}

// GetUser returns the DatabaseUser for a given spec.
func (r *Repository) GetUser(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseUser, error) {
	user := &dbopsv1alpha1.DatabaseUser{}
	userNamespace := namespace
	if spec.UserRef.Namespace != "" {
		userNamespace = spec.UserRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: userNamespace,
		Name:      spec.UserRef.Name,
	}, user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetInstance returns the DatabaseInstance for a given spec (via the user's instanceRef).
func (r *Repository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	user, err := r.GetUser(ctx, spec, namespace)
	if err != nil {
		return nil, err
	}

	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceNamespace := user.Namespace
	if user.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = user.Spec.InstanceRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      user.Spec.InstanceRef.Name,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

// GetEngine returns the database engine type for a given spec.
func (r *Repository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
	instance, err := r.GetInstance(ctx, spec, namespace)
	if err != nil {
		return "", err
	}
	return string(instance.Spec.Engine), nil
}

// DetectDrift detects configuration drift between the CR spec and actual grant state.
func (r *Repository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	var result *drift.Result

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, user *dbopsv1alpha1.DatabaseUser, _ *dbopsv1alpha1.DatabaseInstance) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		result, err = driftSvc.DetectGrantDrift(ctx, spec, user.Spec.Username)
		return err
	})

	return result, err
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
func (r *Repository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	var correctionResult *drift.CorrectionResult

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, user *dbopsv1alpha1.DatabaseUser, _ *dbopsv1alpha1.DatabaseInstance) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		correctionResult, err = driftSvc.CorrectGrantDrift(ctx, spec, user.Spec.Username, driftResult)
		return err
	})

	return correctionResult, err
}
