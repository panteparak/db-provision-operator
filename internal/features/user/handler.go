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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for user operations.
type Handler struct {
	repo          *Repository
	secretManager *secret.Manager
	eventBus      eventbus.Bus
	logger        logr.Logger
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository    *Repository
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
			spec.InstanceRef.Name,
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
				spec.InstanceRef.Name,
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
			spec.InstanceRef.Name,
			namespace,
		))
	}

	return nil
}

// Exists checks if a user exists.
func (h *Handler) Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error) {
	return h.repo.Exists(ctx, username, spec, namespace)
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
		user.Spec.InstanceRef.Name,
		user.Spec.Username,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted user.
func (h *Handler) CleanupInfoMetric(user *dbopsv1alpha1.DatabaseUser) {
	metrics.DeleteUserInfo(user.Name, user.Namespace)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
