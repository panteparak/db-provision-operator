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

package service

import (
	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
)

// SpecBuilder abstracts engine-specific option building.
// This pattern allows adding new engines without modifying existing service code.
//
// To add a new engine:
// 1. Create a new spec builder file (e.g., spec_builder_cockroachdb.go)
// 2. Implement all SpecBuilder methods
// 3. Add a case to GetSpecBuilder factory
type SpecBuilder interface {
	// Database options
	BuildDatabaseCreateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.CreateDatabaseOptions
	BuildDatabaseUpdateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.UpdateDatabaseOptions

	// User options
	BuildUserCreateOptions(spec *dbopsv1alpha1.DatabaseUserSpec, password string) types.CreateUserOptions
	BuildUserUpdateOptions(spec *dbopsv1alpha1.DatabaseUserSpec) types.UpdateUserOptions

	// Role options
	BuildRoleCreateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.CreateRoleOptions
	BuildRoleUpdateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.UpdateRoleOptions
}

// GetSpecBuilder returns the appropriate spec builder for the engine type.
// This is the factory function for creating engine-specific builders.
func GetSpecBuilder(engine dbopsv1alpha1.EngineType) SpecBuilder {
	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		return NewPostgresSpecBuilder()
	case dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.EngineTypeMariaDB:
		// MariaDB is MySQL-compatible and uses the same CRD fields (spec.MySQL)
		return NewMySQLSpecBuilder()
	case dbopsv1alpha1.EngineTypeCockroachDB:
		// CockroachDB is PostgreSQL wire-compatible and uses spec.Postgres fields.
		// The CockroachDB adapter silently ignores unsupported PG attributes
		// (SUPERUSER, REPLICATION, BYPASSRLS).
		return NewPostgresSpecBuilder()
	default:
		// Return empty builder for unknown engines
		// This provides safe defaults without panicking
		return &emptySpecBuilder{}
	}
}

// emptySpecBuilder provides minimal options for unknown engines.
// All methods return options with only the required fields populated.
type emptySpecBuilder struct{}

func (b *emptySpecBuilder) BuildDatabaseCreateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.CreateDatabaseOptions {
	return types.CreateDatabaseOptions{Name: spec.Name}
}

func (b *emptySpecBuilder) BuildDatabaseUpdateOptions(_ *dbopsv1alpha1.DatabaseSpec) types.UpdateDatabaseOptions {
	return types.UpdateDatabaseOptions{}
}

func (b *emptySpecBuilder) BuildUserCreateOptions(spec *dbopsv1alpha1.DatabaseUserSpec, password string) types.CreateUserOptions {
	return types.CreateUserOptions{
		Username: spec.Username,
		Password: password,
		Login:    true,
		Inherit:  true,
	}
}

func (b *emptySpecBuilder) BuildUserUpdateOptions(_ *dbopsv1alpha1.DatabaseUserSpec) types.UpdateUserOptions {
	return types.UpdateUserOptions{}
}

func (b *emptySpecBuilder) BuildRoleCreateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.CreateRoleOptions {
	return types.CreateRoleOptions{
		RoleName: spec.RoleName,
		Inherit:  true,
	}
}

func (b *emptySpecBuilder) BuildRoleUpdateOptions(_ *dbopsv1alpha1.DatabaseRoleSpec) types.UpdateRoleOptions {
	return types.UpdateRoleOptions{}
}
