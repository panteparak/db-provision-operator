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

// Package clusterrole provides the ClusterDatabaseRole feature module for managing
// cluster-scoped database roles for shared service accounts and cross-namespace access.
package clusterrole

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
)

// API defines the public interface for the clusterrole module.
type API interface {
	// Create creates a new cluster-scoped database role.
	Create(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error)

	// Update updates an existing cluster-scoped database role.
	Update(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error)

	// Delete removes a cluster-scoped database role.
	Delete(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error

	// Exists checks if a role exists.
	Exists(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error)

	// GetInstance returns the ClusterDatabaseInstance for a given spec.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error)

	// DetectDrift compares the CR spec to the actual role state and returns any differences.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Result represents the result of a role operation.
type Result struct {
	Created bool
	Updated bool
	Message string
}

// RepositoryInterface defines the interface for cluster role repository operations.
// This interface enables dependency injection and testing with mocks.
type RepositoryInterface interface {
	// Create creates a new cluster-scoped database role.
	Create(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error)

	// Exists checks if a role exists.
	Exists(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (bool, error)

	// Update updates role settings.
	Update(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*Result, error)

	// Delete deletes a cluster-scoped database role.
	Delete(ctx context.Context, roleName string, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, force bool) error

	// GetInstance returns the ClusterDatabaseInstance for a given spec.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error)

	// GetEngine returns the database engine type for a given spec.
	GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec) (string, error)

	// DetectDrift detects configuration drift between the CR spec and actual role state.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseRoleSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
