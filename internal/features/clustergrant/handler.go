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
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for cluster grant operations.
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

// NewHandler creates a new cluster grant handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Apply applies grants to a database user or role.
func (h *Handler) Apply(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterInstance", spec.ClusterInstanceRef.Name)

	// Resolve target to get target info for logging
	target, err := h.repo.ResolveTarget(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}
	log = log.WithValues("targetType", target.Type, "targetName", target.DatabaseName)

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// Apply grants
	log.Info("Applying cluster grants")
	applyStart := time.Now()

	result, err := h.repo.Apply(ctx, spec)
	if err != nil {
		// Use empty namespace for cluster-scoped resources
		metrics.RecordGrantOperation(metrics.OperationCreate, engine, "", metrics.StatusFailure)
		return nil, fmt.Errorf("apply: %w", err)
	}

	applyDuration := time.Since(applyStart).Seconds()
	metrics.RecordGrantOperation(metrics.OperationCreate, engine, "", metrics.StatusSuccess)
	log.Info("Cluster grants applied successfully",
		"roles", result.Roles,
		"directGrants", result.DirectGrants,
		"defaultPrivileges", result.DefaultPrivileges,
		"duration", applyDuration)

	// Publish event
	if h.eventBus != nil {
		privileges := h.collectPrivileges(spec)
		h.eventBus.PublishAsync(ctx, eventbus.NewClusterGrantApplied(
			target.Name,
			target.DatabaseName,
			target.Namespace,
			target.Type,
			spec.ClusterInstanceRef.Name,
			privileges,
		))
	}

	return result, nil
}

// Revoke revokes grants from a database user or role.
func (h *Handler) Revoke(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error {
	log := logf.FromContext(ctx).WithValues("clusterInstance", spec.ClusterInstanceRef.Name)

	// Resolve target for logging and event
	target, err := h.repo.ResolveTarget(ctx, spec)
	if err != nil {
		// Log but continue - we want to try revoke even if we can't resolve target
		log.Error(err, "Failed to resolve target for logging")
	}

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec)
	if err != nil {
		// Log but continue - we want to try revoke even if we can't get engine
		log.Error(err, "Failed to get engine for metrics")
		engine = "unknown"
	}

	log.Info("Revoking cluster grants")
	revokeStart := time.Now()

	if err := h.repo.Revoke(ctx, spec); err != nil {
		metrics.RecordGrantOperation(metrics.OperationDelete, engine, "", metrics.StatusFailure)
		return fmt.Errorf("revoke: %w", err)
	}

	revokeDuration := time.Since(revokeStart).Seconds()
	metrics.RecordGrantOperation(metrics.OperationDelete, engine, "", metrics.StatusSuccess)
	log.Info("Cluster grants revoked successfully", "duration", revokeDuration)

	// Publish event
	if h.eventBus != nil && target != nil {
		privileges := h.collectPrivileges(spec)
		h.eventBus.PublishAsync(ctx, eventbus.NewClusterGrantRevoked(
			target.Name,
			target.DatabaseName,
			target.Namespace,
			target.Type,
			spec.ClusterInstanceRef.Name,
			privileges,
		))
	}

	return nil
}

// Exists checks if grants have been applied.
func (h *Handler) Exists(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error) {
	return h.repo.Exists(ctx, spec)
}

// ResolveTarget resolves the target of the grant (user or role).
func (h *Handler) ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error) {
	return h.repo.ResolveTarget(ctx, spec)
}

// OnClusterRoleCreated handles the ClusterRoleCreated event to apply pending grants.
func (h *Handler) OnClusterRoleCreated(ctx context.Context, event *eventbus.ClusterRoleCreated) error {
	logf.FromContext(ctx).V(1).Info("Cluster role created, checking for pending cluster grants",
		"role", event.RoleName,
		"clusterInstance", event.ClusterInstanceRef)
	// In a full implementation, this would query for ClusterDatabaseGrant resources
	// that reference this role and trigger reconciliation
	return nil
}

// OnUserCreated handles the UserCreated event to apply pending grants.
func (h *Handler) OnUserCreated(ctx context.Context, event *eventbus.UserCreated) error {
	logf.FromContext(ctx).V(1).Info("User created, checking for pending cluster grants",
		"username", event.Username,
		"namespace", event.Namespace)
	// In a full implementation, this would query for ClusterDatabaseGrant resources
	// that reference this user and trigger reconciliation
	return nil
}

// collectPrivileges collects all privileges from the grant spec for event publishing.
func (h *Handler) collectPrivileges(spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) []string {
	var privileges []string

	if spec.Postgres != nil {
		privileges = append(privileges, spec.Postgres.Roles...)
		for _, g := range spec.Postgres.Grants {
			privileges = append(privileges, g.Privileges...)
		}
	}

	if spec.MySQL != nil {
		privileges = append(privileges, spec.MySQL.Roles...)
		for _, g := range spec.MySQL.Grants {
			privileges = append(privileges, g.Privileges...)
		}
	}

	return privileges
}

// UpdateInfoMetric updates the info metric for a cluster grant.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(grant *dbopsv1alpha1.ClusterDatabaseGrant) {
	phase := string(grant.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	// Get target info
	targetRef := ""
	if grant.Spec.UserRef != nil {
		targetRef = fmt.Sprintf("%s/%s", grant.Spec.UserRef.Namespace, grant.Spec.UserRef.Name)
	} else if grant.Spec.RoleRef != nil {
		if grant.Spec.RoleRef.Namespace == "" {
			targetRef = grant.Spec.RoleRef.Name // ClusterDatabaseRole
		} else {
			targetRef = fmt.Sprintf("%s/%s", grant.Spec.RoleRef.Namespace, grant.Spec.RoleRef.Name)
		}
	}

	// Collect privileges as comma-separated string
	privileges := h.collectPrivileges(&grant.Spec)
	privilegesStr := ""
	if len(privileges) > 0 {
		for i, p := range privileges {
			if i > 0 {
				privilegesStr += ","
			}
			privilegesStr += p
		}
	}

	// Use empty namespace for cluster-scoped resources
	metrics.SetGrantInfo(
		grant.Name,
		"", // cluster-scoped, no namespace
		targetRef,
		"", // no direct database ref in cluster grants
		privilegesStr,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted cluster grant.
func (h *Handler) CleanupInfoMetric(grant *dbopsv1alpha1.ClusterDatabaseGrant) {
	metrics.DeleteGrantInfo(grant.Name, "") // cluster-scoped, no namespace
}

// GetInstance returns the ClusterDatabaseInstance for the given spec.
func (h *Handler) GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error) {
	return h.repo.GetInstance(ctx, spec)
}

// GetEngine returns the database engine type for a given spec.
func (h *Handler) GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error) {
	return h.repo.GetEngine(ctx, spec)
}

// DetectDrift compares the CR spec to the actual grant state and returns any differences.
func (h *Handler) DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterInstance", spec.ClusterInstanceRef.Name)
	log.V(1).Info("Detecting cluster grant drift")

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
func (h *Handler) CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error) {
	log := logf.FromContext(ctx).WithValues("clusterInstance", spec.ClusterInstanceRef.Name)
	log.Info("Correcting cluster grant drift", "diffs", len(driftResult.Diffs))

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
