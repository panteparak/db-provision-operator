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

package clusterinstance

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

// Handler contains the business logic for cluster instance operations.
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

// NewHandler creates a new cluster instance handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Connect establishes a connection to the cluster database instance.
// Implements API.Connect
func (h *Handler) Connect(ctx context.Context, name string) (*ConnectResult, error) {
	log := logf.FromContext(ctx).WithValues("clusterinstance", name)

	// Get the instance
	instance, err := h.repo.GetInstance(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get cluster instance: %w", err)
	}

	engine := string(instance.Spec.Engine)

	// Attempt connection with metrics
	connectStart := time.Now()
	result, err := h.repo.Connect(ctx, instance)
	connectDuration := time.Since(connectStart).Seconds()

	if err != nil {
		metrics.RecordConnectionAttempt(name, engine, ClusterNamespaceLabel, metrics.StatusFailure)
		metrics.SetInstanceHealth(name, engine, ClusterNamespaceLabel, false)
		log.Error(err, "Connection failed")
		return nil, err
	}

	// Record successful connection
	metrics.RecordConnectionAttempt(name, engine, ClusterNamespaceLabel, metrics.StatusSuccess)
	metrics.RecordConnectionLatency(name, engine, ClusterNamespaceLabel, connectDuration)
	metrics.SetInstanceHealth(name, engine, ClusterNamespaceLabel, true)
	metrics.SetInstanceLastHealthCheck(name, engine, ClusterNamespaceLabel, float64(time.Now().Unix()))

	log.Info("Successfully connected",
		"version", result.Version,
		"duration", connectDuration)

	// Publish event for other modules
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceConnected(
			name,
			ClusterNamespaceLabel, // Use special cluster label for namespace
			engine,
			result.Version,
			instance.Spec.Connection.Host,
			int(instance.Spec.Connection.Port),
		))
	}

	return result, nil
}

// Ping verifies the connection is still active.
// Implements API.Ping
func (h *Handler) Ping(ctx context.Context, name string) error {
	instance, err := h.repo.GetInstance(ctx, name)
	if err != nil {
		return fmt.Errorf("get cluster instance: %w", err)
	}

	return h.repo.Ping(ctx, instance)
}

// GetVersion returns the database version.
// Implements API.GetVersion
func (h *Handler) GetVersion(ctx context.Context, name string) (string, error) {
	instance, err := h.repo.GetInstance(ctx, name)
	if err != nil {
		return "", fmt.Errorf("get cluster instance: %w", err)
	}

	return h.repo.GetVersion(ctx, instance)
}

// IsHealthy checks if the instance is healthy.
// Implements API.IsHealthy
func (h *Handler) IsHealthy(ctx context.Context, name string) (bool, error) {
	instance, err := h.repo.GetInstance(ctx, name)
	if err != nil {
		return false, fmt.Errorf("get cluster instance: %w", err)
	}

	if err := h.repo.Ping(ctx, instance); err != nil {
		return false, nil
	}

	return true, nil
}

// HandleDisconnect handles when a cluster instance becomes disconnected.
func (h *Handler) HandleDisconnect(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance, reason string) {
	log := logf.FromContext(ctx).WithValues("clusterinstance", instance.Name)
	engine := string(instance.Spec.Engine)

	log.Info("Cluster instance disconnected", "reason", reason)

	// Update metrics
	metrics.SetInstanceHealth(instance.Name, engine, ClusterNamespaceLabel, false)

	// Publish event for other modules
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceDisconnected(
			instance.Name,
			ClusterNamespaceLabel,
			reason,
		))
	}
}

// HandleHealthCheckPassed handles when a health check passes.
func (h *Handler) HandleHealthCheckPassed(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) {
	engine := string(instance.Spec.Engine)

	metrics.SetInstanceHealth(instance.Name, engine, ClusterNamespaceLabel, true)
	metrics.SetInstanceLastHealthCheck(instance.Name, engine, ClusterNamespaceLabel, float64(time.Now().Unix()))

	// Publish healthy event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceHealthy(
			instance.Name,
			ClusterNamespaceLabel,
		))
	}
}

// HandleHealthCheckFailed handles when a health check fails.
func (h *Handler) HandleHealthCheckFailed(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance, reason string) {
	log := logf.FromContext(ctx).WithValues("clusterinstance", instance.Name)
	engine := string(instance.Spec.Engine)

	log.Info("Health check failed", "reason", reason)

	metrics.SetInstanceHealth(instance.Name, engine, ClusterNamespaceLabel, false)

	// Publish unhealthy event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceUnhealthy(
			instance.Name,
			ClusterNamespaceLabel,
			reason,
		))
	}
}

// CleanupMetrics removes metrics for a deleted cluster instance.
func (h *Handler) CleanupMetrics(instance *dbopsv1alpha1.ClusterDatabaseInstance) {
	engine := string(instance.Spec.Engine)
	metrics.DeleteInstanceMetrics(instance.Name, engine, ClusterNamespaceLabel)
	metrics.DeleteInstanceInfo(instance.Name, ClusterNamespaceLabel)
}

// UpdateInfoMetric updates the info metric for a cluster instance.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(instance *dbopsv1alpha1.ClusterDatabaseInstance) {
	engine := string(instance.Spec.Engine)
	version := instance.Status.Version
	if version == "" {
		version = "unknown"
	}
	host := instance.Spec.Connection.Host
	port := fmt.Sprintf("%d", instance.Spec.Connection.Port)
	phase := string(instance.Status.Phase)
	if phase == "" {
		phase = "Pending"
	}

	metrics.SetInstanceInfo(instance.Name, ClusterNamespaceLabel, engine, version, host, port, phase)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
