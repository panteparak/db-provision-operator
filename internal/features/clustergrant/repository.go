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

package clustergrant

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service"
	"github.com/db-provision-operator/internal/service/drift"
)

// TargetResolutionError indicates that the grant target (user/role) could not
// be resolved because it was not found or not in Ready phase. During deletion,
// controllers can treat this as a no-op since the target is gone or being
// deleted, and any granted privileges will be implicitly revoked when the
// database user/role is dropped.
type TargetResolutionError struct {
	Err error
}

func (e *TargetResolutionError) Error() string {
	return e.Err.Error()
}

func (e *TargetResolutionError) Unwrap() error {
	return e.Err
}

// Repository handles cluster grant operations via the service layer.
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

// NewRepository creates a new cluster grant repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:        cfg.Client,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// getOperatorNamespace returns the namespace where the operator is running.
func getOperatorNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	return "db-provision-operator-system"
}

// withService creates a grant service connection and executes the given function.
// ClusterDatabaseGrant only works with ClusterDatabaseInstance.
func (r *Repository) withService(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, fn func(svc *service.GrantService, targetName string, instanceSpec *dbopsv1alpha1.DatabaseInstanceSpec) error) error {
	// Get the ClusterDatabaseInstance
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name: spec.ClusterInstanceRef.Name,
	}, instance); err != nil {
		return fmt.Errorf("get cluster instance: %w", err)
	}

	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return fmt.Errorf("cluster instance not ready: phase is %s", instance.Status.Phase)
	}

	// Resolve the target (user or role)
	target, err := r.ResolveTarget(ctx, spec)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	// Determine credential namespace (use secretRef.namespace or operator namespace)
	credentialNamespace := getOperatorNamespace()
	if instance.Spec.Connection.SecretRef.Namespace != "" {
		credentialNamespace = instance.Spec.Connection.SecretRef.Namespace
	}

	// Get admin credentials
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

	// Create grant service
	svc, err := service.NewGrantService(cfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	return fn(svc, target.DatabaseName, &instance.Spec)
}

// ResolveTarget resolves the target of the grant (user or role).
func (r *Repository) ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
	// Exactly one of userRef or roleRef must be specified
	if spec.UserRef == nil && spec.RoleRef == nil {
		return nil, fmt.Errorf("either userRef or roleRef must be specified")
	}
	if spec.UserRef != nil && spec.RoleRef != nil {
		return nil, fmt.Errorf("only one of userRef or roleRef can be specified")
	}

	if spec.UserRef != nil {
		// Target is a DatabaseUser
		user := &dbopsv1alpha1.DatabaseUser{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: spec.UserRef.Namespace,
			Name:      spec.UserRef.Name,
		}, user); err != nil {
			return nil, &TargetResolutionError{Err: fmt.Errorf("get user: %w", err)}
		}

		if user.Status.Phase != dbopsv1alpha1.PhaseReady {
			return nil, &TargetResolutionError{Err: fmt.Errorf("user not ready: phase is %s", user.Status.Phase)}
		}

		return &TargetInfo{
			Type:          "user",
			Name:          user.Name,
			Namespace:     user.Namespace,
			DatabaseName:  user.Spec.Username,
			IsClusterRole: false,
		}, nil
	}

	// Target is a role
	if spec.RoleRef.Namespace == "" {
		// ClusterDatabaseRole (cluster-scoped)
		role := &dbopsv1alpha1.ClusterDatabaseRole{}
		if err := r.client.Get(ctx, types.NamespacedName{
			Name: spec.RoleRef.Name,
		}, role); err != nil {
			return nil, &TargetResolutionError{Err: fmt.Errorf("get cluster role: %w", err)}
		}

		if role.Status.Phase != dbopsv1alpha1.PhaseReady {
			return nil, &TargetResolutionError{Err: fmt.Errorf("cluster role not ready: phase is %s", role.Status.Phase)}
		}

		return &TargetInfo{
			Type:          "role",
			Name:          role.Name,
			Namespace:     "",
			DatabaseName:  role.Spec.RoleName,
			IsClusterRole: true,
		}, nil
	}

	// DatabaseRole (namespace-scoped)
	role := &dbopsv1alpha1.DatabaseRole{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: spec.RoleRef.Namespace,
		Name:      spec.RoleRef.Name,
	}, role); err != nil {
		return nil, &TargetResolutionError{Err: fmt.Errorf("get role: %w", err)}
	}

	if role.Status.Phase != dbopsv1alpha1.PhaseReady {
		return nil, &TargetResolutionError{Err: fmt.Errorf("role not ready: phase is %s", role.Status.Phase)}
	}

	return &TargetInfo{
		Type:          "role",
		Name:          role.Name,
		Namespace:     role.Namespace,
		DatabaseName:  role.Spec.RoleName,
		IsClusterRole: false,
	}, nil
}

// convertToGrantSpec converts ClusterDatabaseGrantSpec to DatabaseGrantSpec for the service layer.
// This allows reuse of the existing grant service implementation.
func (r *Repository) convertToGrantSpec(spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) *dbopsv1alpha1.DatabaseGrantSpec {
	return &dbopsv1alpha1.DatabaseGrantSpec{
		// UserRef is set by the caller after resolving the target
		Postgres:           spec.Postgres,
		MySQL:              spec.MySQL,
		DriftPolicy:        spec.DriftPolicy,
		DeletionProtection: spec.DeletionProtection,
	}
}

// Apply applies grants to a database user or role.
func (r *Repository) Apply(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		grantSpec := r.convertToGrantSpec(spec)
		svcResult, err := svc.Apply(ctx, service.ApplyGrantServiceOptions{
			Username: targetName,
			Spec:     grantSpec,
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

// Revoke revokes grants from a database user or role.
func (r *Repository) Revoke(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
	return r.withService(ctx, spec, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		grantSpec := r.convertToGrantSpec(spec)
		_, err := svc.Revoke(ctx, service.ApplyGrantServiceOptions{
			Username: targetName,
			Spec:     grantSpec,
		})
		return err
	})
}

// Exists checks if grants have been applied by verifying the target has the expected grants.
func (r *Repository) Exists(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
	var exists bool

	err := r.withService(ctx, spec, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		// Check if target has any grants
		grants, err := svc.GetGrants(ctx, targetName)
		if err != nil {
			return err
		}
		exists = len(grants) > 0
		return nil
	})

	return exists, err
}

// GetInstance returns the ClusterDatabaseInstance for a given spec.
func (r *Repository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name: spec.ClusterInstanceRef.Name,
	}, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

// GetEngine returns the database engine type for a given spec.
func (r *Repository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
	instance, err := r.GetInstance(ctx, spec)
	if err != nil {
		return "", err
	}
	return string(instance.Spec.Engine), nil
}

// DetectDrift detects configuration drift between the CR spec and actual grant state.
func (r *Repository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
	var result *drift.Result

	err := r.withService(ctx, spec, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		grantSpec := r.convertToGrantSpec(spec)
		var err error
		result, err = driftSvc.DetectGrantDrift(ctx, grantSpec, targetName)
		return err
	})

	return result, err
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
func (r *Repository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	var correctionResult *drift.CorrectionResult

	err := r.withService(ctx, spec, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		grantSpec := r.convertToGrantSpec(spec)
		var err error
		correctionResult, err = driftSvc.CorrectGrantDrift(ctx, grantSpec, targetName, driftResult)
		return err
	})

	return correctionResult, err
}
