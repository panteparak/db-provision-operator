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

// Package user provides the DatabaseUser feature module for managing database users.
package user

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/service/drift"
)

// API defines the public interface for the user module.
type API interface {
	// Create creates a new database user with the provided password.
	Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, password string) (*Result, error)

	// Update updates an existing database user.
	Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error)

	// Delete removes a database user.
	Delete(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error

	// Exists checks if a user exists.
	Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error)

	// RotatePassword rotates the user's password.
	RotatePassword(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error

	// GetInstance returns the DatabaseInstance for the given spec.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// GetOwnedObjects retrieves all database objects owned by the specified user.
	// This is used for pre-deletion safety checks to prevent dropping users
	// that own database objects.
	GetOwnedObjects(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*OwnershipCheckResult, error)

	// DetectDrift compares the CR spec to the actual user state and returns any differences.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Result represents the result of a user operation.
type Result struct {
	Created    bool
	Updated    bool
	SecretName string
	Message    string
}

// OwnershipCheckResult represents the result of checking user object ownership.
type OwnershipCheckResult struct {
	// OwnsObjects indicates if the user owns any database objects
	OwnsObjects bool

	// OwnedObjects lists the objects owned by the user
	OwnedObjects []OwnedObject

	// BlocksDeletion indicates if the ownership should block deletion
	BlocksDeletion bool

	// Resolution provides a suggested command to resolve ownership issues
	Resolution string
}

// RepositoryInterface defines the interface for user repository operations.
// This interface enables dependency injection and testing with mocks.
type RepositoryInterface interface {
	// Create creates a new database user.
	Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace, password string) (*Result, error)

	// Exists checks if a user exists.
	Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error)

	// Update updates user settings.
	Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error)

	// Delete deletes a database user.
	Delete(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error

	// SetPassword sets a new password for a user.
	SetPassword(ctx context.Context, username, password string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error

	// GetInstance returns the DatabaseInstance for a given spec.
	GetInstance(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// GetEngine returns the database engine type for a given spec.
	GetEngine(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (string, error)

	// GetOwnedObjects retrieves all database objects owned by the specified user.
	GetOwnedObjects(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) ([]OwnedObject, error)

	// DetectDrift detects configuration drift between the CR spec and actual user state.
	DetectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, allowDestructive bool) (*drift.Result, error)

	// CorrectDrift attempts to correct detected drift by applying necessary changes.
	CorrectDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, driftResult *drift.Result, allowDestructive bool) (*drift.CorrectionResult, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
