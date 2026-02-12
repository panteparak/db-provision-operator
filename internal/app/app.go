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

// Package app provides the application bootstrap for wiring all feature modules together.
package app

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/features/backup"
	"github.com/db-provision-operator/internal/features/backupschedule"
	"github.com/db-provision-operator/internal/features/clustergrant"
	"github.com/db-provision-operator/internal/features/clusterinstance"
	"github.com/db-provision-operator/internal/features/clusterrole"
	"github.com/db-provision-operator/internal/features/database"
	"github.com/db-provision-operator/internal/features/grant"
	"github.com/db-provision-operator/internal/features/instance"
	"github.com/db-provision-operator/internal/features/restore"
	"github.com/db-provision-operator/internal/features/role"
	"github.com/db-provision-operator/internal/features/user"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Module represents a feature module that can be registered with the manager.
type Module interface {
	SetupWithManager(ctrl.Manager) error
}

// Application represents the main application with all feature modules.
type Application struct {
	eventBus      eventbus.Bus
	secretManager *secret.Manager
	modules       []Module
	logger        logr.Logger
}

// NewApplication creates a new Application with all feature modules wired together.
func NewApplication(mgr ctrl.Manager) (*Application, error) {
	logger := mgr.GetLogger().WithName("app")

	// Create shared infrastructure
	eventBus := eventbus.NewInMemoryBus(
		eventbus.WithLogger(logger.WithName("eventbus")),
		eventbus.WithMiddleware(
			eventbus.LoggingMiddleware(logger.WithName("events")),
			eventbus.RecoveryMiddleware(logger.WithName("recovery")),
		),
	)

	// Create secret manager
	secretManager := secret.NewManager(mgr.GetClient())

	// Create feature modules
	var modules []Module

	// Instance module (foundation - other modules depend on instances)
	instanceMod, err := instance.NewModule(instance.Config{
		Manager:       mgr,
		Recorder:      mgr.GetEventRecorderFor("databaseinstance-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, instanceMod)

	// ClusterInstance module (cluster-scoped instances - parallel to Instance module)
	clusterInstanceMod, err := clusterinstance.NewModule(clusterinstance.Config{
		Manager:       mgr,
		Recorder:      mgr.GetEventRecorderFor("clusterdatabaseinstance-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, clusterInstanceMod)

	// Database module (depends on Instance)
	databaseMod, err := database.NewModule(database.Config{
		Manager:       mgr,
		EventBus:      eventBus,
		SecretManager: secretManager,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, databaseMod)

	// User module (depends on Instance)
	userMod, err := user.NewModule(user.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("databaseuser-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, userMod)

	// Role module (depends on Instance)
	roleMod, err := role.NewModule(role.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("databaserole-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, roleMod)

	// ClusterRole module (cluster-scoped roles - depends on ClusterInstance)
	clusterRoleMod, err := clusterrole.NewModule(clusterrole.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("clusterdatabaserole-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, clusterRoleMod)

	// Grant module (depends on User and Database)
	grantMod, err := grant.NewModule(grant.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("databasegrant-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, grantMod)

	// ClusterGrant module (cluster-scoped grants - depends on ClusterInstance, User, and ClusterRole)
	clusterGrantMod, err := clustergrant.NewModule(clustergrant.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("clusterdatabasegrant-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, clusterGrantMod)

	// Backup module (depends on Database and Instance)
	backupMod, err := backup.NewModule(backup.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("databasebackup-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, backupMod)

	// BackupSchedule module (depends on Backup)
	backupScheduleMod, err := backupschedule.NewModule(backupschedule.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("databasebackupschedule-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, backupScheduleMod)

	// Restore module (depends on Backup and Instance)
	restoreMod, err := restore.NewModule(restore.ModuleConfig{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor("databaserestore-controller"),
		EventBus:      eventBus,
		SecretManager: secretManager,
		Logger:        logger,
	})
	if err != nil {
		return nil, err
	}
	modules = append(modules, restoreMod)

	logger.Info("Application created with feature modules",
		"moduleCount", len(modules))

	return &Application{
		eventBus:      eventBus,
		secretManager: secretManager,
		modules:       modules,
		logger:        logger,
	}, nil
}

// SetupWithManager registers all feature modules with the controller manager.
func (a *Application) SetupWithManager(mgr ctrl.Manager) error {
	for _, mod := range a.modules {
		if err := mod.SetupWithManager(mgr); err != nil {
			return err
		}
	}
	a.logger.Info("All feature modules registered with manager")
	return nil
}

// EventBus returns the application's event bus for external use.
func (a *Application) EventBus() eventbus.Bus {
	return a.eventBus
}

// SecretManager returns the application's secret manager for external use.
func (a *Application) SecretManager() *secret.Manager {
	return a.secretManager
}
