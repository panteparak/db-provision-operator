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
)

// API defines the public interface for the user module.
type API interface {
	// Create creates a new database user.
	Create(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error)

	// Update updates an existing database user.
	Update(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (*Result, error)

	// Delete removes a database user.
	Delete(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string, force bool) error

	// Exists checks if a user exists.
	Exists(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) (bool, error)

	// RotatePassword rotates the user's password.
	RotatePassword(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseUserSpec, namespace string) error
}

// Result represents the result of a user operation.
type Result struct {
	Created    bool
	Updated    bool
	SecretName string
	Message    string
}
