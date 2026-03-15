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

// Package database provides the Database feature module for managing database resources.
// This module is responsible for creating, updating, and deleting databases on DatabaseInstances.
package database

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// API defines the public interface for the database module.
// Other modules should only use this interface, not internal types.
// This interface is the contract that the database module exposes to the rest of the system.
type API interface {
	// Create creates a new database on the target instance.
	// Returns the result of the operation and any error that occurred.
	Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error)

	// Update updates an existing database with new settings.
	// Returns the result of the operation and any error that occurred.
	Update(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error)

	// Delete removes a database from the target instance.
	// If force is true, it will drop connections before deleting.
	Delete(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error

	// Exists checks if a database exists on the target instance.
	Exists(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error)

	// GetInfo returns information about a database.
	GetInfo(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error)

	// VerifyAccess verifies that the database is accepting connections.
	VerifyAccess(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error

	// GetInstance returns the DatabaseInstance for the given spec.
	// Deprecated: Use ResolveInstance instead, which supports both DatabaseInstance and ClusterDatabaseInstance.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// ResolveInstance resolves the instance reference (supports both instanceRef and clusterInstanceRef).
	// Returns the resolved instance information needed for operations.
	ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*instanceresolver.ResolvedInstance, error)

	// DetectDrift compares the CR spec to the actual database state and returns any differences.
	// This is used for drift detection to identify configuration drift.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	// Only non-destructive corrections are applied unless allowDestructive is true.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)

	// ApplyInitSQL resolves and executes init SQL statements on a database.
	// Returns nil result if no initSQL is configured.
	ApplyInitSQL(ctx context.Context, database *dbopsv1alpha1.Database) (*InitSQLResult, error)
}

// Result represents the result of a database operation.
type Result struct {
	// Created indicates if the database was newly created.
	Created bool

	// Updated indicates if the database was updated.
	Updated bool

	// Message provides additional information about the operation.
	Message string

	// Ownership contains the result of ownership provisioning (nil if not applicable).
	Ownership *OwnershipResult
}

// OwnershipResult contains the result of per-database ownership provisioning.
type OwnershipResult struct {
	// RoleName is the auto-created group role name.
	RoleName string

	// UserName is the auto-created login user name.
	UserName string
}

// InitSQLResult holds the outcome of init SQL execution.
type InitSQLResult struct {
	// Applied indicates whether init SQL executed successfully.
	Applied bool

	// Skipped is true when hash matches (already applied).
	Skipped bool

	// Hash is the SHA-256 of the resolved SQL content.
	Hash string

	// StatementsExecuted is the number of statements successfully executed.
	StatementsExecuted int32

	// Error contains the execution error, if any.
	Error error
}

// Info contains information about a database.
type Info struct {
	// Name is the name of the database.
	Name string

	// Owner is the owner of the database.
	Owner string

	// SizeBytes is the size of the database in bytes.
	SizeBytes int64

	// Encoding is the character encoding of the database (PostgreSQL).
	Encoding string

	// Collation is the collation setting of the database.
	Collation string

	// Charset is the character set of the database (MySQL).
	Charset string
}

// RepositoryInterface defines the interface for database repository operations.
// This interface enables dependency injection and testing with mocks.
type RepositoryInterface interface {
	// Create creates a new database.
	// When auto-ownership is enabled, this also provisions the owner role and app user.
	Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error)

	// Exists checks if a database exists.
	Exists(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (bool, error)

	// Update updates database settings.
	Update(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Result, error)

	// Delete deletes a database.
	// When auto-ownership with dropOnDelete is enabled, this also drops the owner role and app user.
	Delete(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string, force bool) error

	// VerifyAccess verifies that a database is accepting connections.
	VerifyAccess(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) error

	// GetInfo gets information about a database.
	GetInfo(ctx context.Context, name string, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*Info, error)

	// GetInstance returns the DatabaseInstance for a given spec.
	// Deprecated: Use ResolveInstance instead.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// ResolveInstance resolves the instance reference (supports both instanceRef and clusterInstanceRef).
	ResolveInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (*instanceresolver.ResolvedInstance, error)

	// GetEngine returns the database engine type for a given spec.
	GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string) (string, error)

	// DetectDrift detects configuration drift between the CR spec and actual database state.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)

	// ResolveInitSQL resolves SQL from any source (inline, configMapRef, secretRef).
	// Returns the statements, a content hash, and any error.
	ResolveInitSQL(ctx context.Context, initSQL *dbopsv1alpha1.InitSQLConfig, namespace string) ([]string, string, error)

	// ExecInitSQL executes init SQL statements on the named database.
	// Returns the count of successfully executed statements and the first error.
	ExecInitSQL(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, namespace string, statements []string) (int, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
