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

// postgresSpecBuilder builds adapter options from PostgreSQL-specific specs.
type postgresSpecBuilder struct{}

// NewPostgresSpecBuilder creates a new PostgreSQL spec builder.
func NewPostgresSpecBuilder() SpecBuilder {
	return &postgresSpecBuilder{}
}

// BuildDatabaseCreateOptions builds CreateDatabaseOptions from a DatabaseSpec.
func (b *postgresSpecBuilder) BuildDatabaseCreateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.CreateDatabaseOptions {
	opts := types.CreateDatabaseOptions{
		Name: spec.Name,
	}

	if spec.Postgres != nil {
		opts.Encoding = spec.Postgres.Encoding
		opts.LCCollate = spec.Postgres.LCCollate
		opts.LCCtype = spec.Postgres.LCCtype
		opts.Tablespace = spec.Postgres.Tablespace
		opts.Template = spec.Postgres.Template
		opts.ConnectionLimit = spec.Postgres.ConnectionLimit
		opts.IsTemplate = spec.Postgres.IsTemplate
		opts.AllowConnections = spec.Postgres.AllowConnections
	}

	return opts
}

// BuildDatabaseUpdateOptions builds UpdateDatabaseOptions from a DatabaseSpec.
func (b *postgresSpecBuilder) BuildDatabaseUpdateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.UpdateDatabaseOptions {
	opts := types.UpdateDatabaseOptions{}

	if spec.Postgres != nil {
		// Extensions
		for _, ext := range spec.Postgres.Extensions {
			opts.Extensions = append(opts.Extensions, types.ExtensionOptions{
				Name:    ext.Name,
				Schema:  ext.Schema,
				Version: ext.Version,
			})
		}
		// Schemas
		for _, schema := range spec.Postgres.Schemas {
			opts.Schemas = append(opts.Schemas, types.SchemaOptions{
				Name:  schema.Name,
				Owner: schema.Owner,
			})
		}
		// Default privileges
		for _, dp := range spec.Postgres.DefaultPrivileges {
			opts.DefaultPrivileges = append(opts.DefaultPrivileges, types.DefaultPrivilegeOptions{
				Role:       dp.Role,
				Schema:     dp.Schema,
				ObjectType: dp.ObjectType,
				Privileges: dp.Privileges,
			})
		}
	}

	return opts
}

// BuildUserCreateOptions builds CreateUserOptions from a DatabaseUserSpec.
func (b *postgresSpecBuilder) BuildUserCreateOptions(spec *dbopsv1alpha1.DatabaseUserSpec, password string) types.CreateUserOptions {
	opts := types.CreateUserOptions{
		Username: spec.Username,
		Password: password,
		Login:    true, // Default to allowing login
		Inherit:  true, // Default to inheriting privileges
	}

	if spec.Postgres != nil {
		opts.ConnectionLimit = spec.Postgres.ConnectionLimit
		opts.ValidUntil = spec.Postgres.ValidUntil
		opts.Superuser = spec.Postgres.Superuser
		opts.CreateDB = spec.Postgres.CreateDB
		opts.CreateRole = spec.Postgres.CreateRole
		opts.Inherit = spec.Postgres.Inherit
		opts.Login = spec.Postgres.Login
		opts.Replication = spec.Postgres.Replication
		opts.BypassRLS = spec.Postgres.BypassRLS
		opts.InRoles = spec.Postgres.InRoles
		opts.ConfigParams = spec.Postgres.ConfigParameters
	}

	return opts
}

// BuildUserUpdateOptions builds UpdateUserOptions from a DatabaseUserSpec.
func (b *postgresSpecBuilder) BuildUserUpdateOptions(spec *dbopsv1alpha1.DatabaseUserSpec) types.UpdateUserOptions {
	opts := types.UpdateUserOptions{}

	if spec.Postgres != nil {
		if spec.Postgres.ConnectionLimit != 0 {
			opts.ConnectionLimit = &spec.Postgres.ConnectionLimit
		}
		if spec.Postgres.ValidUntil != "" {
			opts.ValidUntil = &spec.Postgres.ValidUntil
		}
		opts.InRoles = spec.Postgres.InRoles
		opts.ConfigParams = spec.Postgres.ConfigParameters
	}

	return opts
}

// BuildRoleCreateOptions builds CreateRoleOptions from a DatabaseRoleSpec.
func (b *postgresSpecBuilder) BuildRoleCreateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.CreateRoleOptions {
	opts := types.CreateRoleOptions{
		RoleName: spec.RoleName,
		Inherit:  true, // Default to inheriting privileges
	}

	if spec.Postgres != nil {
		opts.Login = spec.Postgres.Login
		opts.Inherit = spec.Postgres.Inherit
		opts.CreateDB = spec.Postgres.CreateDB
		opts.CreateRole = spec.Postgres.CreateRole
		opts.Superuser = spec.Postgres.Superuser
		opts.Replication = spec.Postgres.Replication
		opts.BypassRLS = spec.Postgres.BypassRLS
		opts.InRoles = spec.Postgres.InRoles

		// Convert grants
		if len(spec.Postgres.Grants) > 0 {
			opts.Grants = make([]types.GrantOptions, 0, len(spec.Postgres.Grants))
			for _, g := range spec.Postgres.Grants {
				opts.Grants = append(opts.Grants, types.GrantOptions{
					Database:        g.Database,
					Schema:          g.Schema,
					Tables:          g.Tables,
					Sequences:       g.Sequences,
					Functions:       g.Functions,
					Privileges:      g.Privileges,
					WithGrantOption: g.WithGrantOption,
				})
			}
		}
	}

	return opts
}

// BuildRoleUpdateOptions builds UpdateRoleOptions from a DatabaseRoleSpec.
func (b *postgresSpecBuilder) BuildRoleUpdateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.UpdateRoleOptions {
	opts := types.UpdateRoleOptions{}

	if spec.Postgres != nil {
		login := spec.Postgres.Login
		opts.Login = &login
		inherit := spec.Postgres.Inherit
		opts.Inherit = &inherit
		createDB := spec.Postgres.CreateDB
		opts.CreateDB = &createDB
		createRole := spec.Postgres.CreateRole
		opts.CreateRole = &createRole
		superuser := spec.Postgres.Superuser
		opts.Superuser = &superuser
		replication := spec.Postgres.Replication
		opts.Replication = &replication
		bypassRLS := spec.Postgres.BypassRLS
		opts.BypassRLS = &bypassRLS
		opts.InRoles = spec.Postgres.InRoles

		// Convert grants
		if len(spec.Postgres.Grants) > 0 {
			opts.Grants = make([]types.GrantOptions, 0, len(spec.Postgres.Grants))
			for _, g := range spec.Postgres.Grants {
				opts.Grants = append(opts.Grants, types.GrantOptions{
					Database:        g.Database,
					Schema:          g.Schema,
					Tables:          g.Tables,
					Sequences:       g.Sequences,
					Functions:       g.Functions,
					Privileges:      g.Privileges,
					WithGrantOption: g.WithGrantOption,
				})
			}
		}
	}

	return opts
}
