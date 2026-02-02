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
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Handler contains the business logic for grant operations.
type Handler struct {
	repo     *Repository
	eventBus eventbus.Bus
	logger   logr.Logger
}

// HandlerConfig holds dependencies for the handler.
type HandlerConfig struct {
	Repository *Repository
	EventBus   eventbus.Bus
	Logger     logr.Logger
}

// NewHandler creates a new grant handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Apply applies grants to a database user.
func (h *Handler) Apply(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error) {
	log := logf.FromContext(ctx).WithValues("userRef", spec.UserRef.Name, "namespace", namespace)

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec, namespace)
	if err != nil {
		return nil, fmt.Errorf("get engine: %w", err)
	}

	// Apply grants
	log.Info("Applying grants")
	applyStart := time.Now()

	result, err := h.repo.Apply(ctx, spec, namespace)
	if err != nil {
		metrics.RecordGrantOperation(metrics.OperationCreate, engine, namespace, metrics.StatusFailure)
		return nil, fmt.Errorf("apply: %w", err)
	}

	applyDuration := time.Since(applyStart).Seconds()
	metrics.RecordGrantOperation(metrics.OperationCreate, engine, namespace, metrics.StatusSuccess)
	log.Info("Grants applied successfully",
		"roles", result.Roles,
		"directGrants", result.DirectGrants,
		"defaultPrivileges", result.DefaultPrivileges,
		"duration", applyDuration)

	// Publish event
	if h.eventBus != nil {
		databaseRef := ""
		if spec.DatabaseRef != nil {
			databaseRef = spec.DatabaseRef.Name
		}
		privileges := h.collectPrivileges(spec)
		h.eventBus.PublishAsync(ctx, eventbus.NewGrantApplied(
			spec.UserRef.Name,
			spec.UserRef.Name,
			databaseRef,
			namespace,
			privileges,
		))
	}

	return result, nil
}

// Revoke revokes grants from a database user.
func (h *Handler) Revoke(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error {
	log := logf.FromContext(ctx).WithValues("userRef", spec.UserRef.Name, "namespace", namespace)

	// Get engine for metrics
	engine, err := h.repo.GetEngine(ctx, spec, namespace)
	if err != nil {
		// Log but continue - we want to try revoke even if we can't get engine
		log.Error(err, "Failed to get engine for metrics")
		engine = "unknown"
	}

	log.Info("Revoking grants")
	revokeStart := time.Now()

	if err := h.repo.Revoke(ctx, spec, namespace); err != nil {
		metrics.RecordGrantOperation(metrics.OperationDelete, engine, namespace, metrics.StatusFailure)
		return fmt.Errorf("revoke: %w", err)
	}

	revokeDuration := time.Since(revokeStart).Seconds()
	metrics.RecordGrantOperation(metrics.OperationDelete, engine, namespace, metrics.StatusSuccess)
	log.Info("Grants revoked successfully", "duration", revokeDuration)

	// Publish event
	if h.eventBus != nil {
		databaseRef := ""
		if spec.DatabaseRef != nil {
			databaseRef = spec.DatabaseRef.Name
		}
		privileges := h.collectPrivileges(spec)
		h.eventBus.PublishAsync(ctx, eventbus.NewGrantRevoked(
			spec.UserRef.Name,
			spec.UserRef.Name,
			databaseRef,
			namespace,
			privileges,
		))
	}

	return nil
}

// Exists checks if grants have been applied.
func (h *Handler) Exists(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error) {
	return h.repo.Exists(ctx, spec, namespace)
}

// OnUserCreated handles the UserCreated event to apply pending grants.
func (h *Handler) OnUserCreated(ctx context.Context, event *eventbus.UserCreated) error {
	logf.FromContext(ctx).V(1).Info("User created, checking for pending grants",
		"username", event.Username,
		"namespace", event.Namespace)
	// In a full implementation, this would query for DatabaseGrant resources
	// that reference this user and trigger reconciliation
	return nil
}

// OnDatabaseCreated handles the DatabaseCreated event to apply pending grants.
func (h *Handler) OnDatabaseCreated(ctx context.Context, event *eventbus.DatabaseCreated) error {
	logf.FromContext(ctx).V(1).Info("Database created, checking for pending grants",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	// In a full implementation, this would query for DatabaseGrant resources
	// that reference this database and trigger reconciliation
	return nil
}

// OnDatabaseDeleted handles the DatabaseDeleted event to handle grant cleanup.
func (h *Handler) OnDatabaseDeleted(ctx context.Context, event *eventbus.DatabaseDeleted) error {
	logf.FromContext(ctx).V(1).Info("Database deleted, grants may need cleanup",
		"database", event.DatabaseName,
		"namespace", event.Namespace)
	// In a full implementation, this would notify or update grants
	// that reference the deleted database
	return nil
}

// collectPrivileges collects all privileges from the grant spec for event publishing.
func (h *Handler) collectPrivileges(spec *dbopsv1alpha1.DatabaseGrantSpec) []string {
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

// UpdateInfoMetric updates the info metric for a grant.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(grant *dbopsv1alpha1.DatabaseGrant) {
	phase := string(grant.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	databaseRef := ""
	if grant.Spec.DatabaseRef != nil {
		databaseRef = grant.Spec.DatabaseRef.Name
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

	metrics.SetGrantInfo(
		grant.Name,
		grant.Namespace,
		grant.Spec.UserRef.Name,
		databaseRef,
		privilegesStr,
		phase,
	)
}

// CleanupInfoMetric removes the info metric for a deleted grant.
func (h *Handler) CleanupInfoMetric(grant *dbopsv1alpha1.DatabaseGrant) {
	metrics.DeleteGrantInfo(grant.Name, grant.Namespace)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
