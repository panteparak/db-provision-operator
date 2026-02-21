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
	"github.com/db-provision-operator/internal/shared/instanceresolver"
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

// Repository handles grant operations via the service layer.
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

// NewRepository creates a new grant repository.
func NewRepository(cfg RepositoryConfig) *Repository {
	return &Repository{
		client:           cfg.Client,
		secretManager:    cfg.SecretManager,
		instanceResolver: instanceresolver.New(cfg.Client),
		logger:           cfg.Logger,
	}
}

// ResolveTarget resolves the target of the grant (user or role).
func (r *Repository) ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*TargetInfo, error) {
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
		userNamespace := namespace
		if spec.UserRef.Namespace != "" {
			userNamespace = spec.UserRef.Namespace
		}

		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: userNamespace,
			Name:      spec.UserRef.Name,
		}, user); err != nil {
			return nil, &TargetResolutionError{Err: fmt.Errorf("get user: %w", err)}
		}

		if user.Status.Phase != dbopsv1alpha1.PhaseReady {
			return nil, &TargetResolutionError{Err: fmt.Errorf("user not ready: phase is %s", user.Status.Phase)}
		}

		return &TargetInfo{
			Type:         "user",
			Name:         user.Name,
			Namespace:    user.Namespace,
			DatabaseName: user.Spec.Username,
		}, nil
	}

	// Target is a DatabaseRole
	role := &dbopsv1alpha1.DatabaseRole{}
	roleNamespace := namespace
	if spec.RoleRef.Namespace != "" {
		roleNamespace = spec.RoleRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: roleNamespace,
		Name:      spec.RoleRef.Name,
	}, role); err != nil {
		return nil, &TargetResolutionError{Err: fmt.Errorf("get role: %w", err)}
	}

	if role.Status.Phase != dbopsv1alpha1.PhaseReady {
		return nil, &TargetResolutionError{Err: fmt.Errorf("role not ready: phase is %s", role.Status.Phase)}
	}

	return &TargetInfo{
		Type:         "role",
		Name:         role.Name,
		Namespace:    role.Namespace,
		DatabaseName: role.Spec.RoleName,
	}, nil
}

// withService creates a grant service connection and executes the given function.
// Supports both namespaced DatabaseInstance and cluster-scoped ClusterDatabaseInstance.
// Works with both user and role targets.
func (r *Repository) withService(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, fn func(svc *service.GrantService, targetName string, instanceSpec *dbopsv1alpha1.DatabaseInstanceSpec) error) error {
	// Resolve the target (user or role)
	target, err := r.ResolveTarget(ctx, spec, namespace)
	if err != nil {
		return fmt.Errorf("resolve target: %w", err)
	}

	// Resolve the instance based on target type
	var resolved *instanceresolver.ResolvedInstance
	if spec.UserRef != nil {
		// Get instance from user
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
		resolved, err = r.instanceResolver.Resolve(ctx, user.Spec.InstanceRef, user.Spec.ClusterInstanceRef, user.Namespace)
	} else {
		// Get instance from role
		role := &dbopsv1alpha1.DatabaseRole{}
		roleNamespace := namespace
		if spec.RoleRef.Namespace != "" {
			roleNamespace = spec.RoleRef.Namespace
		}
		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: roleNamespace,
			Name:      spec.RoleRef.Name,
		}, role); err != nil {
			return fmt.Errorf("get role: %w", err)
		}
		resolved, err = r.instanceResolver.Resolve(ctx, role.Spec.InstanceRef, role.Spec.ClusterInstanceRef, role.Namespace)
	}

	if err != nil {
		return fmt.Errorf("resolve instance: %w", err)
	}

	if !resolved.IsReady() {
		return fmt.Errorf("instance not ready: phase is %s", resolved.Phase)
	}

	// Get admin credentials from the credential namespace
	creds, err := r.secretManager.GetCredentials(ctx, resolved.CredentialNamespace, resolved.Spec.Connection.SecretRef)
	if err != nil {
		return fmt.Errorf("get credentials: %w", err)
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if resolved.Spec.TLS != nil && resolved.Spec.TLS.Enabled {
		tlsCreds, err := r.secretManager.GetTLSCredentials(ctx, resolved.CredentialNamespace, resolved.Spec.TLS)
		if err == nil {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config
	cfg := service.ConfigFromInstance(resolved.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
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

	return fn(svc, target.DatabaseName, resolved.Spec)
}

// Apply applies grants to a database user or role.
func (r *Repository) Apply(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
	var result *Result

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		svcResult, err := svc.Apply(ctx, service.ApplyGrantServiceOptions{
			Username: targetName,
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

// Revoke revokes grants from a database user or role.
func (r *Repository) Revoke(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
	return r.withService(ctx, spec, namespace, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		_, err := svc.Revoke(ctx, service.ApplyGrantServiceOptions{
			Username: targetName,
			Spec:     spec,
		})
		return err
	})
}

// Exists checks if grants have been applied by verifying the target has the expected grants.
func (r *Repository) Exists(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error) {
	var exists bool

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
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

// GetUser returns the DatabaseUser for a given spec.
// Returns an error if userRef is not specified.
func (r *Repository) GetUser(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseUser, error) {
	if spec.UserRef == nil {
		return nil, fmt.Errorf("userRef is not specified")
	}

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

// GetRole returns the DatabaseRole for a given spec.
// Returns an error if roleRef is not specified.
func (r *Repository) GetRole(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseRole, error) {
	if spec.RoleRef == nil {
		return nil, fmt.Errorf("roleRef is not specified")
	}

	role := &dbopsv1alpha1.DatabaseRole{}
	roleNamespace := namespace
	if spec.RoleRef.Namespace != "" {
		roleNamespace = spec.RoleRef.Namespace
	}

	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: roleNamespace,
		Name:      spec.RoleRef.Name,
	}, role); err != nil {
		return nil, err
	}

	return role, nil
}

// GetInstance returns the DatabaseInstance for a given spec (via the target's instanceRef).
// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
// This method only works with namespaced DatabaseInstance references.
func (r *Repository) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	// Try user first, then role
	if spec.UserRef != nil {
		user, err := r.GetUser(ctx, spec, namespace)
		if err != nil {
			return nil, err
		}

		// Only works with namespaced instances
		if user.Spec.InstanceRef == nil {
			return nil, fmt.Errorf("user uses clusterInstanceRef, use ResolveInstance instead")
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

	if spec.RoleRef != nil {
		role, err := r.GetRole(ctx, spec, namespace)
		if err != nil {
			return nil, err
		}

		// Only works with namespaced instances
		if role.Spec.InstanceRef == nil {
			return nil, fmt.Errorf("role uses clusterInstanceRef, use ResolveInstance instead")
		}

		instance := &dbopsv1alpha1.DatabaseInstance{}
		instanceNamespace := role.Namespace
		if role.Spec.InstanceRef.Namespace != "" {
			instanceNamespace = role.Spec.InstanceRef.Namespace
		}

		if err := r.client.Get(ctx, types.NamespacedName{
			Namespace: instanceNamespace,
			Name:      role.Spec.InstanceRef.Name,
		}, instance); err != nil {
			return nil, err
		}

		return instance, nil
	}

	return nil, fmt.Errorf("neither userRef nor roleRef is specified")
}

// ResolveInstance resolves the instance reference via the target's instanceRef or clusterInstanceRef.
func (r *Repository) ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*instanceresolver.ResolvedInstance, error) {
	// Try user first, then role
	if spec.UserRef != nil {
		user, err := r.GetUser(ctx, spec, namespace)
		if err != nil {
			return nil, err
		}
		return r.instanceResolver.Resolve(ctx, user.Spec.InstanceRef, user.Spec.ClusterInstanceRef, user.Namespace)
	}

	if spec.RoleRef != nil {
		role, err := r.GetRole(ctx, spec, namespace)
		if err != nil {
			return nil, err
		}
		return r.instanceResolver.Resolve(ctx, role.Spec.InstanceRef, role.Spec.ClusterInstanceRef, role.Namespace)
	}

	return nil, fmt.Errorf("neither userRef nor roleRef is specified")
}

// GetEngine returns the database engine type for a given spec.
func (r *Repository) GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error) {
	resolved, err := r.ResolveInstance(ctx, spec, namespace)
	if err != nil {
		return "", err
	}
	return resolved.Engine(), nil
}

// DetectDrift detects configuration drift between the CR spec and actual grant state.
func (r *Repository) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	var result *drift.Result

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		result, err = driftSvc.DetectGrantDrift(ctx, spec, targetName)
		return err
	})

	return result, err
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
func (r *Repository) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	var correctionResult *drift.CorrectionResult

	err := r.withService(ctx, spec, namespace, func(svc *service.GrantService, targetName string, _ *dbopsv1alpha1.DatabaseInstanceSpec) error {
		driftCfg := &drift.Config{
			AllowDestructive: allowDestructive,
			Logger:           logf.FromContext(ctx),
		}
		driftSvc := drift.NewService(svc.Adapter(), driftCfg)

		var err error
		correctionResult, err = driftSvc.CorrectGrantDrift(ctx, spec, targetName, driftResult)
		return err
	})

	return correctionResult, err
}
