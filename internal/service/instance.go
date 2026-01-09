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
		adapter: dbAdapter,
		config:  cfg,
	}, nil
}

// NewInstanceServiceWithAdapter creates an InstanceService with a pre-created adapter.
func NewInstanceServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config) *InstanceService {
	return &InstanceService{
		adapter: adp,
		config:  cfg,
	}
}

// Connect establishes a connection to the database server.
func (s *InstanceService) Connect(ctx context.Context) error {
	if err := s.adapter.Connect(ctx); err != nil {
		return NewConnectionError(s.config.Host, s.config.Port, err)
	}
	return nil
}

// Close closes the database connection.
func (s *InstanceService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
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
	result := &HealthCheckResult{
		Healthy: false,
	}

	// Measure connection and ping time
	start := time.Now()

	// Try to ping
	if err := s.adapter.Ping(ctx); err != nil {
		result.ErrorMessage = fmt.Sprintf("Ping failed: %v", err)
		result.Message = "Database is not healthy"
		return result, NewDatabaseError("ping", s.config.Host, err)
	}

	result.Latency = time.Since(start)

	// Get version
	version, err := s.adapter.GetVersion(ctx)
	if err != nil {
		// Version retrieval failure is not critical
		result.Version = "unknown"
	} else {
		result.Version = version
	}

	result.Healthy = true
	result.Message = fmt.Sprintf("Connected to %s %s", s.config.Engine, result.Version)

	return result, nil
}

// TestConnection tests the database connection.
// Returns a Result with success/failure information.
func (s *InstanceService) TestConnection(ctx context.Context) (*Result, error) {
	healthResult, err := s.HealthCheck(ctx)
	if err != nil {
		return nil, err
	}

	if !healthResult.Healthy {
		return &Result{
			Success: false,
			Message: healthResult.ErrorMessage,
		}, nil
	}

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
	version, err := s.adapter.GetVersion(ctx)
	if err != nil {
		return "", NewDatabaseError("get version", s.config.Host, err)
	}
	return version, nil
}

// Ping verifies the database connection is alive.
func (s *InstanceService) Ping(ctx context.Context) error {
	if err := s.adapter.Ping(ctx); err != nil {
		return NewDatabaseError("ping", s.config.Host, err)
	}
	return nil
}
