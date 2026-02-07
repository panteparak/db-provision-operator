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

package drift

import (
	"github.com/go-logr/logr"

	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/service"
)

// Service handles drift detection and correction operations.
// It contains the business logic for detecting differences between
// Kubernetes CR specs and actual database state, as well as correcting
// those differences when configured to do so.
//
// The service follows the same patterns as other services in this package:
// - Uses structured logging via logr
// - Returns typed results for controllers to process
// - Does not directly update Kubernetes resources (that's the controller's job)
type Service struct {
	adapter adapter.DatabaseAdapter
	config  *Config
	log     logr.Logger
}

// Config contains configuration for drift detection and correction.
type Config struct {
	// AllowDestructive allows destructive corrections (owner change, drops, etc.)
	// This should be set based on the CR's annotation.
	AllowDestructive bool

	// Logger is the logger to use for drift operations
	Logger logr.Logger
}

// NewService creates a new drift detection service.
func NewService(adp adapter.DatabaseAdapter, cfg *Config) *Service {
	log := cfg.Logger
	if log.GetSink() == nil {
		log = logr.Discard()
	}

	return &Service{
		adapter: adp,
		config:  cfg,
		log:     log.WithName("DriftService"),
	}
}

// NewConfig creates a new drift service config with defaults.
func NewConfig(cfg *service.Config) *Config {
	return &Config{
		AllowDestructive: false,
		Logger:           cfg.GetLogger(),
	}
}
