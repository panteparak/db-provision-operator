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

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/db-provision-operator/internal/adapter"
)

// InstanceService handles database instance operations (connection testing, health checks).
// It extracts business logic from controllers and can be used both by
// Kubernetes controllers and the CLI tool.
type InstanceService struct {
	baseService
	adapter adapter.DatabaseAdapter
	config  *Config
}

// NewInstanceService creates a new InstanceService with the given configuration.
func NewInstanceService(cfg *Config) (*InstanceService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &InstanceService{
		baseService: newBaseService(cfg, "InstanceService"),
		adapter:     dbAdapter,
		config:      cfg,
	}, nil
}

// NewInstanceServiceWithAdapter creates an InstanceService with a pre-created adapter.
func NewInstanceServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *InstanceService {
	return &InstanceService{
		baseService: newBaseService(cfg, "InstanceService"),
		adapter:     adp,
		config:      cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *InstanceService) Connect(ctx context.Context) error {
	op := s.startOp("Connect", s.config.Host)

	ctx, cancel := s.config.Timeouts.WithConnectTimeout(ctx)
	defer cancel()

	if err := s.adapter.Connect(ctx); err != nil {
		op.Error(err, "failed to connect")
		if ctx.Err() == context.DeadlineExceeded {
			return NewTimeoutError("connect", s.config.Host, s.config.Timeouts.ConnectTimeout.String(), err)
		}
		return NewConnectionError(s.config.Host, s.config.Port, err)
	}

	op.Success("connected successfully")
	return nil
}

// Close closes the database connection.
func (s *InstanceService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// Adapter returns the underlying database adapter.
// This is useful for operations that need direct adapter access,
// such as resource discovery.
func (s *InstanceService) Adapter() adapter.DatabaseAdapter {
	return s.adapter
}

// HealthCheckResult contains the result of a health check.
type HealthCheckResult struct {
	Healthy      bool
	Version      string
	Message      string
	Latency      time.Duration
	ErrorMessage string
}

// HealthCheck performs a health check on the database connection.
// This is the main method for the CLI to test connectivity.
func (s *InstanceService) HealthCheck(ctx context.Context) (*HealthCheckResult, error) {
	op := s.startOp("HealthCheck", s.config.Host)

	result := &HealthCheckResult{
		Healthy: false,
	}

	// Apply query timeout
	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	// Measure connection and ping time
	start := time.Now()

	// Try to ping
	if err := s.adapter.Ping(ctx); err != nil {
		op.Error(err, "health check failed - ping error")
		result.ErrorMessage = fmt.Sprintf("Ping failed: %v", err)
		result.Message = "Database is not healthy"
		return result, s.wrapError(ctx, s.config, "ping", s.config.Host, err)
	}

	result.Latency = time.Since(start)

	// Get version
	version, err := s.adapter.GetVersion(ctx)
	if err != nil {
		// Version retrieval failure is not critical
		op.Debug("version retrieval failed", "error", err)
		result.Version = "unknown"
	} else {
		result.Version = version
	}

	result.Healthy = true
	result.Message = fmt.Sprintf("Connected to %s %s", s.config.Engine, result.Version)

	op.Success("health check passed")
	return result, nil
}

// TestConnection tests the database connection.
// Returns a Result with success/failure information.
func (s *InstanceService) TestConnection(ctx context.Context) (*Result, error) {
	op := s.startOp("TestConnection", s.config.Host)

	healthResult, err := s.HealthCheck(ctx)
	if err != nil {
		op.Error(err, "connection test failed")
		return nil, err
	}

	if !healthResult.Healthy {
		op.Debug("connection test unhealthy", "error", healthResult.ErrorMessage)
		return &Result{
			Success: false,
			Message: healthResult.ErrorMessage,
		}, nil
	}

	op.Success("connection test passed")
	return &Result{
		Success: true,
		Message: healthResult.Message,
		Data: map[string]interface{}{
			"version": healthResult.Version,
			"latency": healthResult.Latency.String(),
		},
	}, nil
}

// GetVersion returns the database version.
func (s *InstanceService) GetVersion(ctx context.Context) (string, error) {
	op := s.startOp("GetVersion", s.config.Host)

	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	version, err := s.adapter.GetVersion(ctx)
	if err != nil {
		op.Error(err, "failed to get version")
		return "", s.wrapError(ctx, s.config, "get version", s.config.Host, err)
	}

	op.Success("retrieved version")
	return version, nil
}

// Ping verifies the database connection is alive.
func (s *InstanceService) Ping(ctx context.Context) error {
	op := s.startOp("Ping", s.config.Host)

	ctx, cancel := s.config.Timeouts.WithQueryTimeout(ctx)
	defer cancel()

	if err := s.adapter.Ping(ctx); err != nil {
		op.Error(err, "ping failed")
		return s.wrapError(ctx, s.config, "ping", s.config.Host, err)
	}

	op.Success("ping successful")
	return nil
}
