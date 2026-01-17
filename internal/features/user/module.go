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

package user

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Module represents the User feature module.
type Module struct {
	handler    *Handler
	controller *Controller
	repository *Repository
	eventBus   eventbus.Bus
	logger     logr.Logger
}

// ModuleConfig holds dependencies for the user module.
type ModuleConfig struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	EventBus      eventbus.Bus
	SecretManager *secret.Manager
	Logger        logr.Logger
}

// NewModule creates and wires the user module.
func NewModule(cfg ModuleConfig) (*Module, error) {
	logger := cfg.Logger.WithName("user")

	// Create repository (database operations)
	repo := NewRepository(RepositoryConfig{
		Client:        cfg.Client,
		SecretManager: cfg.SecretManager,
		Logger:        logger.WithName("repository"),
	})

	// Create handler (business logic)
	handler := NewHandler(HandlerConfig{
		Repository:    repo,
		SecretManager: cfg.SecretManager,
		EventBus:      cfg.EventBus,
		Logger:        logger.WithName("handler"),
	})

	// Create controller (K8s reconciliation)
	controller := NewController(ControllerConfig{
		Client:        cfg.Client,
		Scheme:        cfg.Scheme,
		Handler:       handler,
		SecretManager: cfg.SecretManager,
		Logger:        logger.WithName("controller"),
	})

	m := &Module{
		handler:    handler,
		controller: controller,
		repository: repo,
		eventBus:   cfg.EventBus,
		logger:     logger,
	}

	// Subscribe to events from other modules
	m.subscribeToEvents()

	return m, nil
}

// SetupWithManager registers the controller with the manager.
func (m *Module) SetupWithManager(mgr ctrl.Manager) error {
	return m.controller.SetupWithManager(mgr)
}

// Handler returns the module's handler for external use.
func (m *Module) Handler() API {
	return m.handler
}

// subscribeToEvents registers event handlers for inter-module communication.
func (m *Module) subscribeToEvents() {
	if m.eventBus == nil {
		return
	}

	// React to database creation - users may need database grants
	m.eventBus.Subscribe(eventbus.EventDatabaseCreated, "user.OnDatabaseCreated",
		func(ctx context.Context, event eventbus.Event) error {
			dbEvent, ok := event.(*eventbus.DatabaseCreated)
			if !ok {
				return nil
			}
			return m.handler.OnDatabaseCreated(ctx, dbEvent)
		})

	// React to database deletion - may affect user access
	m.eventBus.Subscribe(eventbus.EventDatabaseDeleted, "user.OnDatabaseDeleted",
		func(ctx context.Context, event eventbus.Event) error {
			dbEvent, ok := event.(*eventbus.DatabaseDeleted)
			if !ok {
				return nil
			}
			return m.handler.OnDatabaseDeleted(ctx, dbEvent)
		})

	// React to instance disconnection - users may need status updates
	m.eventBus.Subscribe(eventbus.EventInstanceDisconnected, "user.OnInstanceDisconnected",
		func(ctx context.Context, event eventbus.Event) error {
			instEvent, ok := event.(*eventbus.InstanceDisconnected)
			if !ok {
				return nil
			}
			m.logger.V(1).Info("Instance disconnected, user operations may fail",
				"instance", instEvent.InstanceName,
				"namespace", instEvent.Namespace,
				"reason", instEvent.Reason)
			return nil
		})

	m.logger.Info("Subscribed to events")
}
