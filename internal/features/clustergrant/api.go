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

// Package clustergrant provides the ClusterDatabaseGrant feature module for managing
// cluster-scoped grants with cross-namespace access control.
package clustergrant

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
)

// API defines the public interface for the cluster grant module.
type API interface {
	// Apply applies grants to a database user or role.
	Apply(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error)

	// Revoke revokes grants from a database user or role.
	Revoke(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error

	// Exists checks if grants have been applied.
	Exists(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error)

	// GetInstance returns the ClusterDatabaseInstance for a given spec.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error)

	// GetEngine returns the database engine type for a given spec.
	GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error)

	// ResolveTarget resolves the target of the grant (user or role).
	ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error)

	// DetectDrift compares the CR spec to the actual grant state and returns any differences.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Result represents the result of a grant operation.
type Result struct {
	Applied           bool
	Roles             []string
	DirectGrants      int32
	DefaultPrivileges int32
	GrantDetails      []GrantDetailInfo
	Message           string
}

// GrantDetailInfo contains information about a specific grant operation.
type GrantDetailInfo struct {
	Database   string
	Schema     string
	Privileges []string
	Status     string
	Message    string
}

// TargetInfo contains resolved information about the grant target.
type TargetInfo struct {
	// Type is "user" or "role"
	Type string

	// Name is the K8s resource name
	Name string

	// Namespace is the K8s resource namespace (empty for cluster-scoped roles)
	Namespace string

	// DatabaseName is the actual database username/rolename
	DatabaseName string

	// IsClusterRole indicates if the target is a ClusterDatabaseRole
	IsClusterRole bool
}

// RepositoryInterface defines the interface for cluster grant repository operations.
// This interface enables dependency injection and testing with mocks.
type RepositoryInterface interface {
	// Apply applies grants to a database user or role.
	Apply(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*Result, error)

	// Revoke revokes grants from a database user or role.
	Revoke(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) error

	// Exists checks if grants have been applied.
	Exists(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (bool, error)

	// GetInstance returns the ClusterDatabaseInstance for a given spec.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*dbopsv1alpha1.ClusterDatabaseInstance, error)

	// GetEngine returns the database engine type for a given spec.
	GetEngine(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (string, error)

	// ResolveTarget resolves the target of the grant (user or role).
	ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec) (*TargetInfo, error)

	// DetectDrift compares the CR spec to the actual grant state and returns any differences.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.ClusterDatabaseGrantSpec, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
