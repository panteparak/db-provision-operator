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
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for cluster role operations.
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

// NewHandler creates a new cluster role handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Create creates a new cluster-scoped database role.
func (h *Handler) Create(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("role", spec.RoleName, "clusterInstance", spec.ClusterInstanceRef.Name)

	if spec.RoleName == "" {
		return nil, fmt.Errorf("role name is required")
	}

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// Check if role already exists
	exists, err := h.repo.Exists(ctx, spec.RoleName, spec)
	if err != nil {
		return nil, fmt.Errorf("check existence: %w", err)
	}

	if exists {
		log.V(1).Info("Role already exists")
		return &Result{Created: false, Message: "role already exists"}, nil
	}

	// Create the role
	log.Info("Creating cluster role")
	createStart := time.Now()

	result, err := h.repo.Create(ctx, spec)
	if err != nil {
		// Use empty namespace for cluster-scoped resources in metrics
		metrics.RecordRoleOperation(metrics.OperationCreate, engine, "", metrics.StatusFailure)
		return nil, fmt.Errorf("create: %w", err)
	}

	if result.Created {
		createDuration := time.Since(createStart).Seconds()
		metrics.RecordRoleOperation(metrics.OperationCreate, engine, "", metrics.StatusSuccess)
		log.Info("Cluster role created successfully", "duration", createDuration)
	}

	// Publish event for cluster-scoped role
	if result.Created && h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewClusterRoleCreated(
			spec.RoleName,
			spec.ClusterInstanceRef.Name,
		))
	}

	return result, nil
}

// Update updates an existing cluster-scoped database role.
func (h *Handler) Update(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("role", roleName, "clusterInstance", spec.ClusterInstanceRef.Name)

	result, err := h.repo.Update(ctx, roleName, spec)
	if err != nil {
		log.Error(err, "Failed to update cluster role")
		return nil, err
	}

	if result.Updated {
		log.Info("Cluster role updated")

		engine, _ := h.repo.GetEngine(ctx, spec)

		if h.eventBus != nil {
			h.eventBus.PublishAsync(ctx, eventbus.NewClusterRoleUpdated(
				roleName,
				spec.ClusterInstanceRef.Name,
				[]string{"settings"},
			))
			metrics.RecordRoleOperation(metrics.OperationUpdate, engine, "", metrics.StatusSuccess)
		}
	}

	return result, nil
}

// Delete removes a cluster-scoped database role.
func (h *Handler) Delete(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error {
	log := logf.FromContext(ctx).WithValues("role", roleName, "clusterInstance", spec.ClusterInstanceRef.Name)

	engine, _ := h.repo.GetEngine(ctx, spec)

	log.Info("Deleting cluster role")
	deleteStart := time.Now()

	if err := h.repo.Delete(ctx, roleName, spec, force); err != nil {
		metrics.RecordRoleOperation(metrics.OperationDelete, engine, "", metrics.StatusFailure)
		return fmt.Errorf("delete: %w", err)
	}

	deleteDuration := time.Since(deleteStart).Seconds()
	metrics.RecordRoleOperation(metrics.OperationDelete, engine, "", metrics.StatusSuccess)

	log.Info("Cluster role deleted successfully", "duration", deleteDuration)

	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewClusterRoleDeleted(
			roleName,
			spec.ClusterInstanceRef.Name,
		))
	}

	return nil
}

// Exists checks if a role exists.
func (h *Handler) Exists(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error) {
	return h.repo.Exists(ctx, roleName, spec)
}

// OnClusterDatabaseCreated handles the ClusterDatabaseCreated event (future use).
func (h *Handler) OnClusterDatabaseCreated(ctx context.Context, event *eventbus.DatabaseCreated) error {
	logf.FromContext(ctx).V(1).Info("Database created, cluster roles can now be granted access",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	return nil
}

// UpdateInfoMetric updates the info metric for a cluster role.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(role *dbopsv1alpha1.ClusterDatabaseRole) {
	phase := string(role.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	// Use empty namespace for cluster-scoped resources
	metrics.SetRoleInfo(
		role.Name,
		"", // cluster-scoped, no namespace
		role.Spec.ClusterInstanceRef.Name,
		role.Spec.RoleName,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted cluster role.
func (h *Handler) CleanupInfoMetric(role *dbopsv1alpha1.ClusterDatabaseRole) {
	metrics.DeleteRoleInfo(role.Name, "") // cluster-scoped, no namespace
}

// GetInstance returns the ClusterDatabaseInstance for the given spec.
func (h *Handler) GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
	return h.repo.GetInstance(ctx, spec)
}

// DetectDrift compares the CR spec to the actual role state and returns any differences.
func (h *Handler) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error) {
	log := logf.FromContext(ctx).WithValues("role", spec.RoleName, "clusterInstance", spec.ClusterInstanceRef.Name)
	log.V(1).Info("Detecting cluster role drift")

	result, err := h.repo.DetectDrift(ctx, spec, allowDestructive)
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
func (h *Handler) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	log := logf.FromContext(ctx).WithValues("role", spec.RoleName, "clusterInstance", spec.ClusterInstanceRef.Name)
	log.Info("Correcting cluster role drift", "diffs", len(driftResult.Diffs))

	correctionResult, err := h.repo.CorrectDrift(ctx, spec, driftResult, allowDestructive)
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
