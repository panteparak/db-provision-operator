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

package role

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for role operations.
type Handler struct {
	repo     RepositoryInterface
	eventBus eventbus.Bus
	logger   logr.Logger
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository RepositoryInterface
	EventBus   eventbus.Bus
	Logger     logr.Logger
}

// NewHandler creates a new role handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Create creates a new database role.
func (h *Handler) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("role", spec.RoleName, "namespace", namespace)

	if spec.RoleName == "" {
		return nil, fmt.Errorf("role name is required")
	}

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// Check if role already exists
	exists, err := h.repo.Exists(ctx, spec.RoleName, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("check existence: %w", err)
	}

	if exists {
		log.V(1).Info("Role already exists")
		return &Result{Created: false, Message: "role already exists"}, nil
	}

	// Create the role
	log.Info("Creating role")
	createStart := time.Now()

	result, err := h.repo.Create(ctx, spec, namespace)
	if err != nil {
		metrics.RecordRoleOperation(metrics.OperationCreate, engine, namespace, metrics.StatusFailure)
		return nil, fmt.Errorf("create: %w", err)
	}

	if result.Created {
		createDuration := time.Since(createStart).Seconds()
		metrics.RecordRoleOperation(metrics.OperationCreate, engine, namespace, metrics.StatusSuccess)
		log.Info("Role created successfully", "duration", createDuration)
	}

	// Publish event
	if result.Created && h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewRoleCreated(
			spec.RoleName,
			spec.InstanceRef.Name,
			namespace,
		))
	}

	return result, nil
}

// Update updates an existing database role.
func (h *Handler) Update(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("role", roleName, "namespace", namespace)

	result, err := h.repo.Update(ctx, roleName, spec, namespace)
	if err != nil {
		log.Error(err, "Failed to update role")
		return nil, err
	}

	if result.Updated {
		log.Info("Role updated")

		engine, _ := h.repo.GetEngine(ctx, spec, namespace)

		if h.eventBus != nil {
			h.eventBus.PublishAsync(ctx, eventbus.NewRoleUpdated(
				roleName,
				spec.InstanceRef.Name,
				namespace,
				[]string{"settings"},
			))
			metrics.RecordRoleOperation(metrics.OperationUpdate, engine, namespace, metrics.StatusSuccess)
		}
	}

	return result, nil
}

// Delete removes a database role.
func (h *Handler) Delete(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, force bool) error {
	log := logf.FromContext(ctx).WithValues("role", roleName, "namespace", namespace)

	engine, _ := h.repo.GetEngine(ctx, spec, namespace)

	log.Info("Deleting role")
	deleteStart := time.Now()

	if err := h.repo.Delete(ctx, roleName, spec, namespace, force); err != nil {
		metrics.RecordRoleOperation(metrics.OperationDelete, engine, namespace, metrics.StatusFailure)
		return fmt.Errorf("delete: %w", err)
	}

	deleteDuration := time.Since(deleteStart).Seconds()
	metrics.RecordRoleOperation(metrics.OperationDelete, engine, namespace, metrics.StatusSuccess)

	log.Info("Role deleted successfully", "duration", deleteDuration)

	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewRoleDeleted(
			roleName,
			spec.InstanceRef.Name,
			namespace,
		))
	}

	return nil
}

// Exists checks if a role exists.
func (h *Handler) Exists(ctx context.Context, roleName string, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (bool, error) {
	return h.repo.Exists(ctx, roleName, spec, namespace)
}

// OnDatabaseCreated handles the DatabaseCreated event.
func (h *Handler) OnDatabaseCreated(ctx context.Context, event *eventbus.DatabaseCreated) error {
	logf.FromContext(ctx).V(1).Info("Database created, roles can now be granted access",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	return nil
}

// OnDatabaseDeleted handles the DatabaseDeleted event.
func (h *Handler) OnDatabaseDeleted(ctx context.Context, event *eventbus.DatabaseDeleted) error {
	logf.FromContext(ctx).V(1).Info("Database deleted, role grants may need cleanup",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	return nil
}

// UpdateInfoMetric updates the info metric for a role.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(role *dbopsv1alpha1.DatabaseRole) {
	phase := string(role.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	metrics.SetRoleInfo(
		role.Name,
		role.Namespace,
		role.Spec.InstanceRef.Name,
		role.Spec.RoleName,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted role.
func (h *Handler) CleanupInfoMetric(role *dbopsv1alpha1.DatabaseRole) {
	metrics.DeleteRoleInfo(role.Name, role.Namespace)
}

// GetInstance returns the DatabaseInstance for the given spec.
func (h *Handler) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	return h.repo.GetInstance(ctx, spec, namespace)
}

// DetectDrift compares the CR spec to the actual role state and returns any differences.
func (h *Handler) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	log := logf.FromContext(ctx).WithValues("role", spec.RoleName, "namespace", namespace)
	log.V(1).Info("Detecting role drift")

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
func (h *Handler) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	log := logf.FromContext(ctx).WithValues("role", spec.RoleName, "namespace", namespace)
	log.Info("Correcting role drift", "diffs", len(driftResult.Diffs))

	correctionResult, err := h.repo.CorrectDrift(ctx, spec, namespace, driftResult, allowDestructive)
	if err != nil {
		return nil, fmt.Errorf("correct drift: %w", err)
	}

	if correctionResult.HasCorrections() {
		log.Info("Drift corrected", "corrected", len(correctionResult.Corrected))
	}

	return correctionResult, nil
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
