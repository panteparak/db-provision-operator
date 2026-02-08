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

// Package instance provides the DatabaseInstance feature module for managing database connections.
// This module is responsible for establishing and maintaining connections to database instances.
package instance

import (
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// API defines the public interface for the instance module.
// Other modules should only use this interface, not internal types.
type API interface {
	// Connect establishes a connection to the database instance.
	Connect(ctx context.Context, name, namespace string) (*ConnectResult, error)

	// Ping verifies the connection is still active.
	Ping(ctx context.Context, name, namespace string) error

	// GetVersion returns the database version.
	GetVersion(ctx context.Context, name, namespace string) (string, error)

	// IsHealthy checks if the instance is healthy.
	IsHealthy(ctx context.Context, name, namespace string) (bool, error)
}

// ConnectResult represents the result of a connection attempt.
type ConnectResult struct {
	// Connected indicates if the connection was successful.
	Connected bool

	// Version is the database version string.
	Version string

	// Message provides additional information.
	Message string
}

// InstanceInfo contains information about a database instance.
type InstanceInfo struct {
	// Name is the instance name.
	Name string

	// Namespace is the K8s namespace.
	Namespace string

	// Engine is the database engine type (mysql, postgres).
	Engine string

	// Version is the database version.
	Version string

	// Host is the database host.
	Host string

	// Port is the database port.
	Port int

	// Connected indicates if currently connected.
	Connected bool

	// Healthy indicates if health check passed.
	Healthy bool
}

// RepositoryInterface defines the repository operations for instance.
type RepositoryInterface interface {
	// GetInstance retrieves a DatabaseInstance by name and namespace.
	GetInstance(ctx context.Context, name, namespace string) (*dbopsv1alpha1.DatabaseInstance, error)

	// Connect establishes a connection and verifies it with a ping.
	Connect(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (*ConnectResult, error)

	// Ping verifies the connection is still active.
	Ping(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) error

	// GetVersion returns the database version.
	GetVersion(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (string, error)
}

// Ensure Repository implements RepositoryInterface.
var _ RepositoryInterface = (*Repository)(nil)
