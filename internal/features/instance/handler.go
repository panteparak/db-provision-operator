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

package instance

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

// Handler contains the business logic for instance operations.
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

// NewHandler creates a new instance handler.
func NewHandler(cfg HandlerConfig) *Handler {
	return &Handler{
		repo:     cfg.Repository,
		eventBus: cfg.EventBus,
		logger:   cfg.Logger,
	}
}

// Connect establishes a connection to the database instance.
// Implements API.Connect
func (h *Handler) Connect(ctx context.Context, name, namespace string) (*ConnectResult, error) {
	log := logf.FromContext(ctx).WithValues("instance", name, "namespace", namespace)

	// Get the instance
	instance, err := h.repo.GetInstance(ctx, name, namespace)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}

	engine := string(instance.Spec.Engine)

	// Attempt connection with metrics
	connectStart := time.Now()
	result, err := h.repo.Connect(ctx, instance)
	connectDuration := time.Since(connectStart).Seconds()

	if err != nil {
		metrics.RecordConnectionAttempt(name, engine, namespace, metrics.StatusFailure)
		metrics.SetInstanceHealth(name, engine, namespace, false)
		log.Error(err, "Connection failed")
		return nil, err
	}

	// Record successful connection
	metrics.RecordConnectionAttempt(name, engine, namespace, metrics.StatusSuccess)
	metrics.RecordConnectionLatency(name, engine, namespace, connectDuration)
	metrics.SetInstanceHealth(name, engine, namespace, true)
	metrics.SetInstanceLastHealthCheck(name, engine, namespace, float64(time.Now().Unix()))

	log.Info("Successfully connected",
		"version", result.Version,
		"duration", connectDuration)

	// Publish event for other modules
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceConnected(
			name,
			namespace,
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
func (h *Handler) Ping(ctx context.Context, name, namespace string) error {
	instance, err := h.repo.GetInstance(ctx, name, namespace)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}

	return h.repo.Ping(ctx, instance)
}

// GetVersion returns the database version.
// Implements API.GetVersion
func (h *Handler) GetVersion(ctx context.Context, name, namespace string) (string, error) {
	instance, err := h.repo.GetInstance(ctx, name, namespace)
	if err != nil {
		return "", fmt.Errorf("get instance: %w", err)
	}

	return h.repo.GetVersion(ctx, instance)
}

// IsHealthy checks if the instance is healthy.
// Implements API.IsHealthy
func (h *Handler) IsHealthy(ctx context.Context, name, namespace string) (bool, error) {
	instance, err := h.repo.GetInstance(ctx, name, namespace)
	if err != nil {
		return false, fmt.Errorf("get instance: %w", err)
	}

	if err := h.repo.Ping(ctx, instance); err != nil {
		return false, nil
	}

	return true, nil
}

// HandleDisconnect handles when an instance becomes disconnected.
func (h *Handler) HandleDisconnect(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, reason string) {
	log := logf.FromContext(ctx).WithValues("instance", instance.Name, "namespace", instance.Namespace)
	engine := string(instance.Spec.Engine)

	log.Info("Instance disconnected", "reason", reason)

	// Update metrics
	metrics.SetInstanceHealth(instance.Name, engine, instance.Namespace, false)

	// Publish event for other modules
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceDisconnected(
			instance.Name,
			instance.Namespace,
			reason,
		))
	}
}

// HandleHealthCheckPassed handles when a health check passes.
func (h *Handler) HandleHealthCheckPassed(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) {
	engine := string(instance.Spec.Engine)

	metrics.SetInstanceHealth(instance.Name, engine, instance.Namespace, true)
	metrics.SetInstanceLastHealthCheck(instance.Name, engine, instance.Namespace, float64(time.Now().Unix()))

	// Publish healthy event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceHealthy(
			instance.Name,
			instance.Namespace,
		))
	}
}

// HandleHealthCheckFailed handles when a health check fails.
func (h *Handler) HandleHealthCheckFailed(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, reason string) {
	log := logf.FromContext(ctx).WithValues("instance", instance.Name, "namespace", instance.Namespace)
	engine := string(instance.Spec.Engine)

	log.Info("Health check failed", "reason", reason)

	metrics.SetInstanceHealth(instance.Name, engine, instance.Namespace, false)

	// Publish unhealthy event
	if h.eventBus != nil {
		h.eventBus.PublishAsync(ctx, eventbus.NewInstanceUnhealthy(
			instance.Name,
			instance.Namespace,
			reason,
		))
	}
}

// CleanupMetrics removes metrics for a deleted instance.
func (h *Handler) CleanupMetrics(instance *dbopsv1alpha1.DatabaseInstance) {
	engine := string(instance.Spec.Engine)
	metrics.DeleteInstanceMetrics(instance.Name, engine, instance.Namespace)
	metrics.DeleteInstanceInfo(instance.Name, instance.Namespace)
}

// UpdateInfoMetric updates the info metric for an instance.
// This should be called after status changes to keep Grafana table views current.
func (h *Handler) UpdateInfoMetric(instance *dbopsv1alpha1.DatabaseInstance) {
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

	metrics.SetInstanceInfo(instance.Name, instance.Namespace, engine, version, host, port, phase)
}

// Ensure Handler implements API interface.
var _ API = (*Handler)(nil)
