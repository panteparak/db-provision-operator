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
	"github.com/go-logr/logr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Module represents the ClusterInstance feature module.
// It owns all components related to ClusterDatabaseInstance resource management.
type Module struct {
	handler    *Handler
	controller *Controller
	repository *Repository
	eventBus   eventbus.Bus
	logger     logr.Logger
}

// Config holds dependencies for the cluster instance module.
type Config struct {
	Manager       ctrl.Manager
	Recorder      record.EventRecorder
	EventBus      eventbus.Bus
	SecretManager *secret.Manager
}

// NewModule creates and wires the cluster instance module.
func NewModule(cfg Config) (*Module, error) {
	logger := cfg.Manager.GetLogger().WithName("clusterinstance")

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
		Client:   cfg.Manager.GetClient(),
		Scheme:   cfg.Manager.GetScheme(),
		Recorder: cfg.Recorder,
		Handler:  handler,
		Logger:   logger.WithName("controller"),
	})

	return &Module{
		handler:    handler,
		controller: controller,
		repository: repo,
		eventBus:   cfg.EventBus,
		logger:     logger,
	}, nil
}

// SetupWithManager registers the controller with the manager.
func (m *Module) SetupWithManager(mgr ctrl.Manager) error {
	return m.controller.SetupWithManager(mgr)
}

// Handler returns the module's handler for use by other modules.
func (m *Module) Handler() API {
	return m.handler
}

// Name returns the module name.
func (m *Module) Name() string {
	return "clusterinstance"
}
