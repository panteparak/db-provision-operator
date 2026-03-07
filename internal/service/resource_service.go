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

	"github.com/db-provision-operator/internal/adapter"
)

// ResourceService provides common infrastructure for all resource services.
// It handles adapter lifecycle, connection management, and timeout wrapping.
//
// Embed this struct in concrete services to eliminate boilerplate:
//
//	type DatabaseService struct {
//	    *ResourceService
//	}
type ResourceService struct {
	baseService
	adapter     adapter.DatabaseAdapter
	config      *Config
	specBuilder SpecBuilder
}

// NewResourceService creates a ResourceService from config.
// It creates the appropriate database adapter based on the engine type.
func NewResourceService(cfg *Config, serviceName string) (*ResourceService, error) {
	if cfg == nil {
		return nil, &ValidationError{Field: "config", Message: "config is required"}
	}

	dbAdapter, err := adapter.NewAdapter(cfg.GetEngineType(), cfg.ToAdapterConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	return &ResourceService{
		baseService: newBaseService(cfg, serviceName),
		adapter:     dbAdapter,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}, nil
}

// NewResourceServiceWithAdapter creates a ResourceService with a pre-created adapter.
// This is useful for testing or when the adapter is already available.
func NewResourceServiceWithAdapter(adp adapter.DatabaseAdapter, cfg *Config, serviceName string) *ResourceService {
	return &ResourceService{
		baseService: newBaseService(cfg, serviceName),
		adapter:     adp,
		config:      cfg,
		specBuilder: GetSpecBuilder(cfg.GetEngineType()),
	}
}

// Connect establishes a connection to the database server with timeout.
func (s *ResourceService) Connect(ctx context.Context) error {
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
func (s *ResourceService) Close() error {
	if s.adapter != nil {
		return s.adapter.Close()
	}
	return nil
}

// Adapter returns the underlying database adapter.
func (s *ResourceService) Adapter() adapter.DatabaseAdapter {
	return s.adapter
}

// Config returns the service configuration.
func (s *ResourceService) Config() *Config {
	return s.config
}

// SpecBuilder returns the engine-specific spec builder.
func (s *ResourceService) SpecBuilder() SpecBuilder {
	return s.specBuilder
}
