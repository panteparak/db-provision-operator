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
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Module represents the Database feature module.
// It owns all components related to Database resource management.
type Module struct {
	handler    *Handler
	controller *Controller
	repository *Repository
	eventBus   eventbus.Bus
	logger     logr.Logger
}

// Config holds dependencies for the database module.
type Config struct {
	Manager              ctrl.Manager
	EventBus             eventbus.Bus
	SecretManager        *secret.Manager
	DefaultDriftInterval time.Duration
	Predicates           []predicate.Predicate
}

// NewModule creates and wires the database module.
func NewModule(cfg Config) (*Module, error) {
	logger := cfg.Manager.GetLogger().WithName("database")

	// Create secret manager if not provided
	secretMgr := cfg.SecretManager
	if secretMgr == nil {
		secretMgr = secret.NewManager(cfg.Manager.GetClient())
	}

	// Create repository (database operations)
	repo := NewRepository(RepositoryConfig{
		Client:        cfg.Manager.GetClient(),
		SecretManager: secretMgr,
		Logger:        logger.WithName("repository"),
	})

	// Create handler (business logic)
	handler := NewHandler(HandlerConfig{
		Repository: repo,
		EventBus:   cfg.EventBus,
		Logger:     logger.WithName("handler"),
	})

	// Create controller (K8s reconciliation)
	controller := NewController(ControllerConfig{
		Client:               cfg.Manager.GetClient(),
		Scheme:               cfg.Manager.GetScheme(),
		Recorder:             cfg.Manager.GetEventRecorderFor("database-controller"),
		Handler:              handler,
		Logger:               logger.WithName("controller"),
		DefaultDriftInterval: cfg.DefaultDriftInterval,
		Predicates:           cfg.Predicates,
	})

	// Subscribe to events from other modules
	if cfg.EventBus != nil {
		subscribeToEvents(cfg.EventBus, handler)
	}

	return &Module{
		handler:    handler,
		controller: controller,
		repository: repo,
		eventBus:   cfg.EventBus,
		logger:     logger,
	}, nil
}

// subscribeToEvents registers event handlers for events from other modules.
func subscribeToEvents(bus eventbus.Bus, handler *Handler) {
	// React to instance connection events
	bus.Subscribe(eventbus.EventInstanceConnected, "database.OnInstanceConnected", func(ctx context.Context, e eventbus.Event) error {
		event, ok := e.(*eventbus.InstanceConnected)
		if !ok {
			return nil
		}
		return handler.OnInstanceConnected(ctx, event)
	})

	// React to instance disconnection
	bus.Subscribe(eventbus.EventInstanceDisconnected, "database.OnInstanceDisconnected", func(ctx context.Context, e eventbus.Event) error {
		event, ok := e.(*eventbus.InstanceDisconnected)
		if !ok {
			return nil
		}
		return handler.OnInstanceDisconnected(ctx, event)
	})
}

// SetupWithManager registers the controller with the manager.
func (m *Module) SetupWithManager(mgr ctrl.Manager) error {
	return m.controller.SetupWithManager(mgr)
}

// Handler returns the module's handler for use by other modules.
// This is the recommended way to interact with the database module.
func (m *Module) Handler() API {
	return m.handler
}

// Name returns the module name.
func (m *Module) Name() string {
	return "database"
}
