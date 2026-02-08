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

package backupschedule

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Module represents the BackupSchedule feature module.
type Module struct {
	handler    *Handler
	controller *Controller
	repository *Repository
	eventBus   eventbus.Bus
	logger     logr.Logger
}

// ModuleConfig holds dependencies for the backupschedule module.
type ModuleConfig struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	Recorder      record.EventRecorder
	EventBus      eventbus.Bus
	SecretManager *secret.Manager
	Logger        logr.Logger
}

// NewModule creates and wires the backupschedule module.
func NewModule(cfg ModuleConfig) (*Module, error) {
	logger := cfg.Logger.WithName("backupschedule")

	// Create repository (K8s operations)
	repo := NewRepository(RepositoryConfig{
		Client: cfg.Client,
		Logger: logger.WithName("repository"),
	})

	// Create handler (business logic)
	handler := NewHandler(HandlerConfig{
		Repository: repo,
		EventBus:   cfg.EventBus,
		Logger:     logger.WithName("handler"),
	})

	// Create controller (K8s reconciliation)
	controller := NewController(ControllerConfig{
		Client:   cfg.Client,
		Scheme:   cfg.Scheme,
		Recorder: cfg.Recorder,
		Handler:  handler,
		Logger:   logger.WithName("controller"),
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

// Name returns the module name.
func (m *Module) Name() string {
	return "backupschedule"
}

// subscribeToEvents registers event handlers for inter-module communication.
func (m *Module) subscribeToEvents() {
	if m.eventBus == nil {
		return
	}

	// React to backup completion - update schedule statistics
	m.eventBus.Subscribe(eventbus.EventBackupCompleted, "backupschedule.OnBackupCompleted",
		func(ctx context.Context, event eventbus.Event) error {
			backupEvent, ok := event.(*eventbus.BackupCompleted)
			if !ok {
				return nil
			}
			return m.handler.OnBackupCompleted(ctx, backupEvent)
		})

	// React to backup failure - update schedule statistics
	m.eventBus.Subscribe(eventbus.EventBackupFailed, "backupschedule.OnBackupFailed",
		func(ctx context.Context, event eventbus.Event) error {
			backupEvent, ok := event.(*eventbus.BackupFailed)
			if !ok {
				return nil
			}
			return m.handler.OnBackupFailed(ctx, backupEvent)
		})

	m.logger.Info("Subscribed to events")
}
