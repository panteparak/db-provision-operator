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
}

// Result represents the result of a database operation.
type Result struct {
	// Created indicates if the database was newly created.
	Created bool

	// Updated indicates if the database was updated.
	Updated bool

	// Message provides additional information about the operation.
	Message string
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
