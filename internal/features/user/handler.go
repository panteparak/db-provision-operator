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
	"strings"
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// Handler contains the business logic for user operations.
type Handler struct {
	repo          RepositoryInterface
	secretManager *secret.Manager
	eventBus      eventbus.Bus
	logger        logr.Logger
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository    RepositoryInterface
	SecretManager *secret.Manager
	EventBus      eventbus.Bus
	Logger        logr.Logger
}

// NewHandler creates a new user handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:          cfg.Repository,
		secretManager: cfg.SecretManager,
		eventBus:      cfg.EventBus,
		logger:        cfg.Logger,
	}
}

// resolveInstanceName returns the instance name from whichever ref is set.
func resolveInstanceName(spec *dbopsv1alpha1.DatabaseUserSpec) string {
	if spec.InstanceRef != nil {
		return spec.InstanceRef.Name
	}
	if spec.ClusterInstanceRef != nil {
		return spec.ClusterInstanceRef.Name
	}
	return ""
}

// Create creates a new database user with the provided password.
func (h *Handler) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, password string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("user", spec.Username, "namespace", namespace)

	if spec.Username == "" {
		return nil, fmt.Errorf("username is required")
	}

	if password == "" {
		return nil, fmt.Errorf("password is required")
	}

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// Check if user already exists
	exists, err := h.repo.Exists(ctx, spec.Username, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("check existence: %w", err)
	}

	if exists {
		log.V(1).Info("User already exists")
		return &Result{Created: false, Message: "user already exists"}, nil
	}

	// Create the user with the provided password
	log.Info("Creating user")

	result, err := h.repo.Create(ctx, spec, namespace, password)
	if err != nil {
		metrics.RecordUserOperation(metrics.OperationCreate, engine, namespace, metrics.StatusFailure)
		return nil, fmt.Errorf("create: %w", err)
	}

	if result.Created {
		metrics.RecordUserOperation(metrics.OperationCreate, engine, namespace, metrics.StatusSuccess)
		log.Info("User created successfully")
	}

	// Publish event
	if result.Created && h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewUserCreated(
			spec.Username,
			resolveInstanceName(spec),
			namespace,
			result.SecretName,
		))
	}

	return result, nil
}

// Update updates an existing database user.
func (h *Handler) Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("user", username, "namespace", namespace)

	result, err := h.repo.Update(ctx, username, spec, namespace)
	if err != nil {
		log.Error(err, "Failed to update user")
		return nil, err
	}

	if result.Updated {
		log.Info("User updated")

		engine, _ := h.repo.GetEngine(ctx, spec, namespace)

		if h.eventBus != nil {
			h.eventBus.PublishAsync(ctx, eventbus.NewUserUpdated(
				username,
				resolveInstanceName(spec),
				namespace,
				[]string{"settings"},
			))
			metrics.RecordUserOperation(metrics.OperationUpdate, engine, namespace, metrics.StatusSuccess)
		}
	}

	return result, nil
}

// Delete removes a database user.
func (h *Handler) Delete(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error {
	log := logf.FromContext(ctx).WithValues("user", username, "namespace", namespace)

	engine, _ := h.repo.GetEngine(ctx, spec, namespace)

	log.Info("Deleting user")

	if err := h.repo.Delete(ctx, username, spec, namespace, force); err != nil {
		metrics.RecordUserOperation(metrics.OperationDelete, engine, namespace, metrics.StatusFailure)
		return fmt.Errorf("delete: %w", err)
	}

	metrics.RecordUserOperation(metrics.OperationDelete, engine, namespace, metrics.StatusSuccess)

	log.Info("User deleted successfully")

	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewUserDeleted(
			username,
			resolveInstanceName(spec),
			namespace,
		))
	}

	return nil
}

// Exists checks if a user exists.
func (h *Handler) Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
	return h.repo.Exists(ctx, username, spec, namespace)
}

// GetInstance returns the DatabaseInstance for the given spec.
// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
func (h *Handler) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	return h.repo.GetInstance(ctx, spec, namespace)
}

// ResolveInstance resolves the instance reference (supports both instanceRef and clusterInstanceRef).
func (h *Handler) ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*instanceresolver.ResolvedInstance, error) {
	return h.repo.ResolveInstance(ctx, spec, namespace)
}

// RotatePassword rotates the user's password.
func (h *Handler) RotatePassword(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error {
	log := logf.FromContext(ctx).WithValues("user", username, "namespace", namespace)

	// Generate new password using PasswordConfig from spec if available
	var passwordConfig *dbopsv1alpha1.PasswordConfig
	if spec.PasswordSecret != nil {
		passwordConfig = spec.PasswordSecret
	}
	newPassword, err := secret.GeneratePassword(passwordConfig)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}

	// Set new password
	if err := h.repo.SetPassword(ctx, username, newPassword, spec, namespace); err != nil {
		return fmt.Errorf("set password: %w", err)
	}

	log.Info("Password rotated")

	// Publish event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewPasswordRotated(
			username,
			namespace,
			"", // Secret name will be updated by controller
			"manual",
		))
	}

	return nil
}

// SetPassword updates the password for an existing user in the database.
func (h *Handler) SetPassword(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) error {
	return h.repo.SetPassword(ctx, username, password, spec, namespace)
}

// OnDatabaseCreated handles the DatabaseCreated event.
func (h *Handler) OnDatabaseCreated(ctx context.Context, event *eventbus.DatabaseCreated) error {
	logf.FromContext(ctx).V(1).Info("Database created, users can now be granted access",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	return nil
}

// OnDatabaseDeleted handles the DatabaseDeleted event.
func (h *Handler) OnDatabaseDeleted(ctx context.Context, event *eventbus.DatabaseDeleted) error {
	logf.FromContext(ctx).V(1).Info("Database deleted, user grants may need cleanup",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	return nil
}

// UpdateInfoMetric updates the info metric for a user.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(user *dbopsv1alpha1.DatabaseUser) {
	phase := string(user.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	metrics.SetUserInfo(
		user.Name,
		user.Namespace,
		resolveInstanceName(&user.Spec),
		user.Spec.Username,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted user.
func (h *Handler) CleanupInfoMetric(user *dbopsv1alpha1.DatabaseUser) {
	metrics.DeleteUserInfo(user.Name, user.Namespace)
}

// RotationResult contains the outcome of a rotation operation.
type RotationResult struct {
	// NewUsername is the newly created user
	NewUsername string
	// NewPassword is the generated password for the new user
	NewPassword string
	// ServiceRole is the service role used
	ServiceRole string
	// RequeueAfter is the suggested requeue interval
	RequeueAfter time.Duration
}

// RotateWithStrategy performs password rotation using the configured strategy.
func (h *Handler) RotateWithStrategy(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (*RotationResult, error) {
	strategy := user.Spec.PasswordRotation.Strategy
	if strategy == "" {
		strategy = dbopsv1alpha1.RotationStrategyRoleInheritance
	}

	switch strategy {
	case dbopsv1alpha1.RotationStrategyRoleInheritance:
		return h.rotateRoleInheritance(ctx, user)
	default:
		return nil, fmt.Errorf("unsupported rotation strategy: %s", strategy)
	}
}

// rotateRoleInheritance creates a new login user with membership in a service role.
func (h *Handler) rotateRoleInheritance(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (*RotationResult, error) {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	// Derive service role name
	serviceRoleName := fmt.Sprintf("svc_%s", user.Spec.Username)
	if user.Spec.PasswordRotation.ServiceRole != nil && user.Spec.PasswordRotation.ServiceRole.Name != "" {
		serviceRoleName = user.Spec.PasswordRotation.ServiceRole.Name
	}

	// Ensure service role exists (AutoCreate defaults to true)
	autoCreate := true
	if user.Spec.PasswordRotation.ServiceRole != nil {
		autoCreate = user.Spec.PasswordRotation.ServiceRole.AutoCreate
	}
	if autoCreate {
		if err := h.repo.EnsureServiceRole(ctx, serviceRoleName, &user.Spec, user.Namespace); err != nil {
			return nil, fmt.Errorf("ensure service role: %w", err)
		}
	}

	// Generate new username from naming template
	newUsername := h.generateRotatedUsername(user)
	log.Info("Rotating user", "newUsername", newUsername, "serviceRole", serviceRoleName)

	// Generate password
	var passwordConfig *dbopsv1alpha1.PasswordConfig
	if user.Spec.PasswordSecret != nil {
		passwordConfig = user.Spec.PasswordSecret
	}
	newPassword, err := secret.GeneratePassword(passwordConfig)
	if err != nil {
		return nil, fmt.Errorf("generate password: %w", err)
	}

	// Create new login user with role membership
	if err := h.repo.CreateUserWithRole(ctx, newUsername, newPassword, serviceRoleName, &user.Spec, user.Namespace); err != nil {
		return nil, fmt.Errorf("create rotated user: %w", err)
	}

	// Publish event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewPasswordRotated(
			newUsername,
			user.Namespace,
			"",
			"scheduled",
		))
	}

	engine, _ := h.repo.GetEngine(ctx, &user.Spec, user.Namespace)
	metrics.RecordUserOperation(metrics.OperationUpdate, engine, user.Namespace, metrics.StatusSuccess)

	log.Info("Password rotation completed", "newUsername", newUsername, "serviceRole", serviceRoleName)

	return &RotationResult{
		NewUsername: newUsername,
		NewPassword: newPassword,
		ServiceRole: serviceRoleName,
	}, nil
}

// generateRotatedUsername generates a username for a rotated user based on the naming template.
func (h *Handler) generateRotatedUsername(user *dbopsv1alpha1.DatabaseUser) string {
	pattern := user.Spec.PasswordRotation.UserNaming
	if pattern == "" {
		pattern = "{{.Username}}_{{.Date}}"
	}

	now := time.Now()
	name := pattern
	name = strings.ReplaceAll(name, "{{.Username}}", user.Spec.Username)
	name = strings.ReplaceAll(name, "{{.Date}}", now.Format("20060102"))
	name = strings.ReplaceAll(name, "{{.Timestamp}}", fmt.Sprintf("%d", now.Unix()))

	// Truncate to PostgreSQL's 63 char limit
	if len(name) > 63 {
		name = name[:63]
	}

	return name
}

// CleanupDeprecatedUsers processes users pending deletion based on the old user policy.
func (h *Handler) CleanupDeprecatedUsers(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) error {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	policy := user.Spec.PasswordRotation.OldUserPolicy
	if policy == nil {
		policy = &dbopsv1alpha1.OldUserPolicy{
			Action:          dbopsv1alpha1.OldUserActionDelete,
			GracePeriodDays: 7,
			OwnershipCheck:  true,
		}
	}

	if user.Status.Rotation == nil {
		return nil
	}

	now := time.Now()
	var remaining []dbopsv1alpha1.PendingDeletionInfo

	for _, pending := range user.Status.Rotation.PendingDeletion {
		// Skip already completed entries
		if pending.Status == "deleted" || pending.Status == "disabled" {
			continue
		}

		// Not yet due
		if pending.DeleteAfter != nil && now.Before(pending.DeleteAfter.Time) {
			remaining = append(remaining, pending)
			continue
		}

		switch policy.Action {
		case dbopsv1alpha1.OldUserActionDelete:
			// Check ownership if enabled
			if policy.OwnershipCheck {
				owned, err := h.repo.GetOwnedObjects(ctx, pending.User, &user.Spec, user.Namespace)
				if err != nil {
					log.Error(err, "Failed to check ownership for pending deletion", "pendingUser", pending.User)
					remaining = append(remaining, pending)
					continue
				}
				if len(owned) > 0 {
					log.Info("Deletion blocked: user owns objects", "pendingUser", pending.User, "objectCount", len(owned))
					pending.Status = "blocked"
					pending.BlockedReason = fmt.Sprintf("owns %d objects", len(owned))
					// Convert to API OwnedObjects
					apiObjects := make([]dbopsv1alpha1.OwnedObject, len(owned))
					for i, obj := range owned {
						apiObjects[i] = dbopsv1alpha1.OwnedObject{
							Schema: obj.Schema,
							Name:   obj.Name,
							Type:   obj.Type,
						}
					}
					pending.OwnedObjects = apiObjects
					pending.Resolution = fmt.Sprintf("REASSIGN OWNED BY %s TO %s", pending.User, user.Status.Rotation.ServiceRole)
					remaining = append(remaining, pending)
					continue
				}
			}

			if err := h.repo.Delete(ctx, pending.User, &user.Spec, user.Namespace, true); err != nil {
				log.Error(err, "Failed to delete deprecated user", "pendingUser", pending.User)
				remaining = append(remaining, pending)
				continue
			}
			log.Info("Deprecated user deleted", "pendingUser", pending.User)

		case dbopsv1alpha1.OldUserActionDisable:
			if err := h.repo.DisableLogin(ctx, pending.User, &user.Spec, user.Namespace); err != nil {
				log.Error(err, "Failed to disable deprecated user", "pendingUser", pending.User)
				remaining = append(remaining, pending)
				continue
			}
			log.Info("Deprecated user disabled", "pendingUser", pending.User)

		case dbopsv1alpha1.OldUserActionRetain:
			// No-op, remove from pending list
			log.V(1).Info("Retaining deprecated user", "pendingUser", pending.User)
		}
	}

	user.Status.Rotation.PendingDeletion = remaining
	return nil
}

// DetectDrift compares the CR spec to the actual user state and returns any differences.
func (h *Handler) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	log := logf.FromContext(ctx).WithValues("user", spec.Username, "namespace", namespace)
	log.V(1).Info("Detecting user drift")

	result, err := h.repo.DetectDrift(ctx, spec, namespace, allowDestructive)
	if err != nil {
		return nil, fmt.Errorf("detect drift: %w", err)
	}

	if result.HasDrift() {
		log.Info("Drift detected", "diffs", len(result.Diffs))
	} else {
		log.V(1).Info("No drift detected")
	}

	return result, nil
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
func (h *Handler) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	log := logf.FromContext(ctx).WithValues("user", spec.Username, "namespace", namespace)
	log.Info("Correcting user drift", "diffs", len(driftResult.Diffs))

	correctionResult, err := h.repo.CorrectDrift(ctx, spec, namespace, driftResult, allowDestructive)
	if err != nil {
		return nil, fmt.Errorf("correct drift: %w", err)
	}

	if correctionResult.HasCorrections() {
		log.Info("Drift corrected", "corrected", len(correctionResult.Corrected))
	}

	return correctionResult, nil
}

// GetOwnedObjects retrieves all database objects owned by the specified user.
// This is used for pre-deletion safety checks to prevent dropping users
// that own database objects.
func (h *Handler) GetOwnedObjects(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*OwnershipCheckResult, error) {
	log := logf.FromContext(ctx).WithValues("user", username, "namespace", namespace)
	log.V(1).Info("Checking owned objects")

	repoObjects, err := h.repo.GetOwnedObjects(ctx, username, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("get owned objects: %w", err)
	}

	result := &OwnershipCheckResult{
		OwnsObjects:    len(repoObjects) > 0,
		OwnedObjects:   repoObjects,
		BlocksDeletion: len(repoObjects) > 0,
	}

	if result.OwnsObjects {
		// Determine the service role for resolution message
		serviceRole := username + "_service"
		if spec.PasswordRotation != nil && spec.PasswordRotation.ServiceRole != nil && spec.PasswordRotation.ServiceRole.Name != "" {
			serviceRole = spec.PasswordRotation.ServiceRole.Name
		}
		result.Resolution = fmt.Sprintf("REASSIGN OWNED BY %s TO %s", username, serviceRole)
		log.Info("User owns objects, deletion blocked", "objectCount", len(repoObjects))
	}

	return result, nil
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
