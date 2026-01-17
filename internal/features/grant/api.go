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
)

// API defines the public interface for the grant module.
type API interface {
	// Apply applies grants to a database user.
	Apply(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (*Result, error)

	// Revoke revokes grants from a database user.
	Revoke(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) error

	// Exists checks if grants have been applied.
	Exists(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, namespace string) (bool, error)
}

// Result represents the result of a grant operation.
type Result struct {
	Applied           bool
	Roles             []string
	DirectGrants      int32
	DefaultPrivileges int32
	Message           string
}
