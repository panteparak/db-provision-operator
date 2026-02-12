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

// mysqlSpecBuilder builds adapter options from MySQL-specific specs.
type mysqlSpecBuilder struct{}

// NewMySQLSpecBuilder creates a new MySQL spec builder.
func NewMySQLSpecBuilder() SpecBuilder {
	return &mysqlSpecBuilder{}
}

// BuildDatabaseCreateOptions builds CreateDatabaseOptions from a DatabaseSpec.
func (b *mysqlSpecBuilder) BuildDatabaseCreateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.CreateDatabaseOptions {
	opts := types.CreateDatabaseOptions{
		Name:  spec.Name,
		Owner: spec.Owner, // MySQL doesn't have native ownership, but tracked for metadata
	}

	if spec.MySQL != nil {
		opts.Charset = spec.MySQL.Charset
		opts.Collation = spec.MySQL.Collation
	}

	return opts
}

// BuildDatabaseUpdateOptions builds UpdateDatabaseOptions from a DatabaseSpec.
func (b *mysqlSpecBuilder) BuildDatabaseUpdateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.UpdateDatabaseOptions {
	opts := types.UpdateDatabaseOptions{}

	if spec.MySQL != nil {
		opts.Charset = spec.MySQL.Charset
		opts.Collation = spec.MySQL.Collation
	}

	return opts
}

// BuildUserCreateOptions builds CreateUserOptions from a DatabaseUserSpec.
func (b *mysqlSpecBuilder) BuildUserCreateOptions(spec *dbopsv1alpha1.DatabaseUserSpec, password string) types.CreateUserOptions {
	opts := types.CreateUserOptions{
		Username: spec.Username,
		Password: password,
		Login:    true, // Default to allowing login
		Inherit:  true, // Default to inheriting privileges
	}

	if spec.MySQL != nil {
		opts.MaxQueriesPerHour = spec.MySQL.MaxQueriesPerHour
		opts.MaxUpdatesPerHour = spec.MySQL.MaxUpdatesPerHour
		opts.MaxConnectionsPerHour = spec.MySQL.MaxConnectionsPerHour
		opts.MaxUserConnections = spec.MySQL.MaxUserConnections
		opts.AuthPlugin = string(spec.MySQL.AuthPlugin)
		opts.RequireSSL = spec.MySQL.RequireSSL
		opts.RequireX509 = spec.MySQL.RequireX509
		opts.AllowedHosts = spec.MySQL.AllowedHosts
		opts.AccountLocked = spec.MySQL.AccountLocked
	}

	return opts
}

// BuildUserUpdateOptions builds UpdateUserOptions from a DatabaseUserSpec.
func (b *mysqlSpecBuilder) BuildUserUpdateOptions(spec *dbopsv1alpha1.DatabaseUserSpec) types.UpdateUserOptions {
	opts := types.UpdateUserOptions{}

	if spec.MySQL != nil {
		if spec.MySQL.MaxQueriesPerHour != 0 {
			opts.MaxQueriesPerHour = &spec.MySQL.MaxQueriesPerHour
		}
		if spec.MySQL.MaxUpdatesPerHour != 0 {
			opts.MaxUpdatesPerHour = &spec.MySQL.MaxUpdatesPerHour
		}
		if spec.MySQL.MaxConnectionsPerHour != 0 {
			opts.MaxConnectionsPerHour = &spec.MySQL.MaxConnectionsPerHour
		}
		if spec.MySQL.MaxUserConnections != 0 {
			opts.MaxUserConnections = &spec.MySQL.MaxUserConnections
		}
	}

	return opts
}

// BuildRoleCreateOptions builds CreateRoleOptions from a DatabaseRoleSpec.
func (b *mysqlSpecBuilder) BuildRoleCreateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.CreateRoleOptions {
	opts := types.CreateRoleOptions{
		RoleName: spec.RoleName,
		Inherit:  true, // Default to inheriting privileges
	}

	if spec.MySQL != nil {
		opts.UseNativeRoles = spec.MySQL.UseNativeRoles

		// Convert grants
		if len(spec.MySQL.Grants) > 0 {
			opts.Grants = make([]types.GrantOptions, 0, len(spec.MySQL.Grants))
			for _, g := range spec.MySQL.Grants {
				opts.Grants = append(opts.Grants, types.GrantOptions{
					Level:           string(g.Level),
					Database:        g.Database,
					Table:           g.Table,
					Columns:         g.Columns,
					Procedure:       g.Procedure,
					Function:        g.Function,
					Privileges:      g.Privileges,
					WithGrantOption: g.WithGrantOption,
				})
			}
		}
	}

	return opts
}

// BuildRoleUpdateOptions builds UpdateRoleOptions from a DatabaseRoleSpec.
func (b *mysqlSpecBuilder) BuildRoleUpdateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.UpdateRoleOptions {
	opts := types.UpdateRoleOptions{}

	if spec.MySQL != nil {
		// Convert grants for add
		if len(spec.MySQL.Grants) > 0 {
			opts.AddGrants = make([]types.GrantOptions, 0, len(spec.MySQL.Grants))
			for _, g := range spec.MySQL.Grants {
				opts.AddGrants = append(opts.AddGrants, types.GrantOptions{
					Level:           string(g.Level),
					Database:        g.Database,
					Table:           g.Table,
					Columns:         g.Columns,
					Procedure:       g.Procedure,
					Function:        g.Function,
					Privileges:      g.Privileges,
					WithGrantOption: g.WithGrantOption,
				})
			}
		}
	}

	return opts
}
