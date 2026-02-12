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
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// Handler contains the business logic for database operations.
// It coordinates between the repository and the event bus.
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

// NewHandler creates a new database handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Create creates a new database on the target instance.
// Implements API.Create
func (h *Handler) Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("database", spec.Name, "namespace", namespace)

	// 1. Validation
	if spec.Name == "" {
		return nil, fmt.Errorf("database name is required")
	}

	// Validate that either instanceRef or clusterInstanceRef is set
	if spec.InstanceRef == nil && spec.ClusterInstanceRef == nil {
		return nil, fmt.Errorf("either instanceRef or clusterInstanceRef is required")
	}

	// Validate that the reference has a name
	if spec.InstanceRef != nil && spec.InstanceRef.Name == "" {
		return nil, fmt.Errorf("instanceRef.name is required")
	}
	if spec.ClusterInstanceRef != nil && spec.ClusterInstanceRef.Name == "" {
		return nil, fmt.Errorf("clusterInstanceRef.name is required")
	}

	// Get engine type for metrics
	engine, err := h.repo.GetEngine(ctx, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// 2. Check if database already exists
	exists, err := h.repo.Exists(ctx, spec.Name, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("check existence: %w", err)
	}

	if exists {
		log.V(1).Info("Database already exists")
		return &Result{Created: false, Message: "database already exists"}, nil
	}

	// 3. Create the database
	log.Info("Creating database")
	createStart := time.Now()

	result, err := h.repo.Create(ctx, spec, namespace)
	if err != nil {
		metrics.RecordDatabaseOperation(metrics.OperationCreate, engine, namespace, metrics.StatusFailure)
		return nil, fmt.Errorf("create: %w", err)
	}

	if result.Created {
		createDuration := time.Since(createStart).Seconds()
		metrics.RecordDatabaseOperation(metrics.OperationCreate, engine, namespace, metrics.StatusSuccess)
		metrics.RecordDatabaseOperationDuration(metrics.OperationCreate, engine, namespace, createDuration)
		log.Info("Database created successfully")
	}

	// 4. Publish event for other modules
	if result.Created && h.eventBus != nil {
		instanceName := ""
		if spec.InstanceRef != nil {
			instanceName = spec.InstanceRef.Name
		} else if spec.ClusterInstanceRef != nil {
			instanceName = spec.ClusterInstanceRef.Name
		}
		h.eventBus.PublishAsync(ctx, eventbus.NewDatabaseCreated(
			spec.Name,
			instanceName,
			namespace,
			engine,
		))
	}

	return result, nil
}

// Update updates an existing database with new settings.
// Implements API.Update
func (h *Handler) Update(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("database", name, "namespace", namespace)

	result, err := h.repo.Update(ctx, name, spec, namespace)
	if err != nil {
		log.Error(err, "Failed to update database settings")
		return nil, err
	}

	if result.Updated {
		log.Info("Database settings updated")

		// Get engine for event
		engine, _ := h.repo.GetEngine(ctx, spec, namespace)

		// Get instance name for event
		instanceName := ""
		if spec.InstanceRef != nil {
			instanceName = spec.InstanceRef.Name
		} else if spec.ClusterInstanceRef != nil {
			instanceName = spec.ClusterInstanceRef.Name
		}

		// Publish update event
		if h.eventBus != nil {
			h.eventBus.PublishAsync(ctx, eventbus.NewDatabaseUpdated(
				name,
				instanceName,
				namespace,
				[]string{"settings"},
			))

			// Record metric
			metrics.RecordDatabaseOperation(metrics.OperationUpdate, engine, namespace, metrics.StatusSuccess)
		}
	}

	return result, nil
}

// Delete removes a database from the target instance.
// Implements API.Delete
func (h *Handler) Delete(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error {
	log := logf.FromContext(ctx).WithValues("database", name, "namespace", namespace, "force", force)

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec, namespace)
	if err != nil {
		log.Error(err, "Failed to get engine type")
		// Continue anyway, just won't have accurate metrics
	}

	// Get instance name for metrics cleanup
	resolved, _ := h.repo.ResolveInstance(ctx, spec, namespace)
	instanceName := ""
	if resolved != nil {
		instanceName = resolved.Name
	}

	log.Info("Deleting database")
	deleteStart := time.Now()

	if err := h.repo.Delete(ctx, name, spec, namespace, force); err != nil {
		metrics.RecordDatabaseOperation(metrics.OperationDelete, engine, namespace, metrics.StatusFailure)
		return fmt.Errorf("delete: %w", err)
	}

	deleteDuration := time.Since(deleteStart).Seconds()
	metrics.RecordDatabaseOperation(metrics.OperationDelete, engine, namespace, metrics.StatusSuccess)
	metrics.RecordDatabaseOperationDuration(metrics.OperationDelete, engine, namespace, deleteDuration)

	// Clean up database size metric
	if instanceName != "" {
		metrics.DeleteDatabaseMetrics(name, instanceName, engine, namespace)
	}

	log.Info("Database deleted successfully")

	// Publish event for other modules
	if h.eventBus != nil {
		// Use the instance name we already resolved above
		h.eventBus.PublishAsync(ctx, eventbus.NewDatabaseDeleted(
			name,
			instanceName,
			namespace,
		))
	}

	return nil
}

// Exists checks if a database exists on the target instance.
// Implements API.Exists
func (h *Handler) Exists(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error) {
	return h.repo.Exists(ctx, name, spec, namespace)
}

// GetInfo returns information about a database.
// Implements API.GetInfo
func (h *Handler) GetInfo(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error) {
	return h.repo.GetInfo(ctx, name, spec, namespace)
}

// VerifyAccess verifies that the database is accepting connections.
// Implements API.VerifyAccess
func (h *Handler) VerifyAccess(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
	return h.repo.VerifyAccess(ctx, name, spec, namespace)
}

// UpdateDatabaseMetrics updates the database size metrics.
func (h *Handler) UpdateDatabaseMetrics(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error {
	info, err := h.repo.GetInfo(ctx, name, spec, namespace)
	if err != nil {
		return err
	}

	if info != nil {
		engine, _ := h.repo.GetEngine(ctx, spec, namespace)
		resolved, _ := h.repo.ResolveInstance(ctx, spec, namespace)
		instanceName := ""
		if resolved != nil {
			instanceName = resolved.Name
		}
		metrics.SetDatabaseSize(name, instanceName, engine, namespace, float64(info.SizeBytes))
	}

	return nil
}

// OnInstanceConnected handles the InstanceConnected event.
// This is called when a DatabaseInstance successfully connects.
func (h *Handler) OnInstanceConnected(ctx context.Context, event *eventbus.InstanceConnected) error {
	logf.FromContext(ctx).V(1).Info("Instance connected, databases on this instance are now accessible",
		"instance", event.InstanceName,
		"namespace", event.Namespace,
		"engine", event.Engine)
	return nil
}

// OnInstanceDisconnected handles the InstanceDisconnected event.
// This is called when a DatabaseInstance loses connection.
func (h *Handler) OnInstanceDisconnected(ctx context.Context, event *eventbus.InstanceDisconnected) error {
	logf.FromContext(ctx).Info("Instance disconnected, databases on this instance may be inaccessible",
		"instance", event.InstanceName,
		"namespace", event.Namespace,
		"reason", event.Reason)
	return nil
}

// UpdateInfoMetric updates the info metric for a database.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(database *dbopsv1alpha1.Database) {
	phase := string(database.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	// Get instance name from either instanceRef or clusterInstanceRef
	instanceName := ""
	if database.Spec.InstanceRef != nil {
		instanceName = database.Spec.InstanceRef.Name
	} else if database.Spec.ClusterInstanceRef != nil {
		instanceName = database.Spec.ClusterInstanceRef.Name
	}

	metrics.SetDatabaseInfo(
		database.Name,
		database.Namespace,
		instanceName,
		database.Spec.Name,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted database.
func (h *Handler) CleanupInfoMetric(database *dbopsv1alpha1.Database) {
	metrics.DeleteDatabaseInfo(database.Name, database.Namespace)
}

// GetInstance returns the DatabaseInstance for the given spec.
// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
// Implements API.GetInstance
func (h *Handler) GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error) {
	return h.repo.GetInstance(ctx, spec, namespace)
}

// ResolveInstance resolves the instance reference (supports both instanceRef and clusterInstanceRef).
// Implements API.ResolveInstance
func (h *Handler) ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*instanceresolver.ResolvedInstance, error) {
	return h.repo.ResolveInstance(ctx, spec, namespace)
}

// DetectDrift compares the CR spec to the actual database state and returns any differences.
// Implements API.DetectDrift
func (h *Handler) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error) {
	log := logf.FromContext(ctx).WithValues("database", spec.Name, "namespace", namespace)
	log.V(1).Info("Detecting drift for database")

	result, err := h.repo.DetectDrift(ctx, spec, namespace, allowDestructive)
	if err != nil {
		return nil, fmt.Errorf("detect drift: %w", err)
	}

	if result != nil && result.HasDrift() {
		log.Info("Drift detected", "diffs", len(result.Diffs))

		// Record drift metric
		engine, _ := h.repo.GetEngine(ctx, spec, namespace)
		metrics.RecordDatabaseOperation(metrics.OperationDriftDetection, engine, namespace, metrics.StatusSuccess)
	}

	return result, nil
}

// CorrectDrift attempts to correct detected drift by applying necessary changes.
// Implements API.CorrectDrift
func (h *Handler) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	log := logf.FromContext(ctx).WithValues("database", spec.Name, "namespace", namespace)
	log.Info("Correcting drift for database", "diffs", len(driftResult.Diffs))

	correctionResult, err := h.repo.CorrectDrift(ctx, spec, namespace, driftResult, allowDestructive)
	if err != nil {
		return nil, fmt.Errorf("correct drift: %w", err)
	}

	// Log correction summary
	if correctionResult != nil {
		log.Info("Drift correction complete",
			"corrected", len(correctionResult.Corrected),
			"skipped", len(correctionResult.Skipped),
			"failed", len(correctionResult.Failed))

		// Record correction metric
		engine, _ := h.repo.GetEngine(ctx, spec, namespace)
		if len(correctionResult.Corrected) > 0 {
			metrics.RecordDatabaseOperation(metrics.OperationDriftCorrection, engine, namespace, metrics.StatusSuccess)
		}
		if len(correctionResult.Failed) > 0 {
			metrics.RecordDatabaseOperation(metrics.OperationDriftCorrection, engine, namespace, metrics.StatusFailure)
		}
	}

	return correctionResult, nil
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
