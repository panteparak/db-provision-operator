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

// Package grant provides the DatabaseGrant feature module for managing database grants.
package grant

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// API defines the public interface for the grant module.
type API interface {
	// Apply applies grants to a database user.
	Apply(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error)

	// Revoke revokes grants from a database user.
	Revoke(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error

	// Exists checks if grants have been applied.
	Exists(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error)

	// GetInstance returns the DatabaseInstance for a given spec.
	// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// ResolveInstance resolves the instance reference via the user's instanceRef or clusterInstanceRef.
	ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*instanceresolver.ResolvedInstance, error)

	// ResolveTarget resolves the target of the grant (user or role).
	ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*TargetInfo, error)

	// DetectDrift compares the CR spec to the actual grant state and returns any differences.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Result represents the result of a grant operation.
type Result struct {
	Applied           bool
	Roles             []string
	DirectGrants      int32
	DefaultPrivileges int32
	Message           string
}

// TargetInfo contains resolved information about the grant target.
type TargetInfo struct {
	// Type is "user" or "role"
	Type string

	// Name is the K8s resource name
	Name string

	// Namespace is the K8s resource namespace
	Namespace string

	// DatabaseName is the actual database username/rolename
	DatabaseName string
}

// RepositoryInterface defines the interface for grant repository operations.
// This interface enables dependency injection and testing with mocks.
type RepositoryInterface interface {
	// Apply applies grants to a database user.
	Apply(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error)

	// Revoke revokes grants from a database user.
	Revoke(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error

	// Exists checks if grants have been applied.
	Exists(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error)

	// GetUser returns the DatabaseUser for a given spec.
	GetUser(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseUser, error)

	// GetInstance returns the DatabaseInstance for a given spec.
	// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// ResolveInstance resolves the instance reference via the user's instanceRef or clusterInstanceRef.
	ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*instanceresolver.ResolvedInstance, error)

	// ResolveTarget resolves the target of the grant (user or role).
	ResolveTarget(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*TargetInfo, error)

	// GetEngine returns the database engine type for a given spec.
	GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (string, error)

	// DetectDrift compares the CR spec to the actual grant state and returns any differences.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
