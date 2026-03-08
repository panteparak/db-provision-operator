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
	"fmt"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
)

// clickhouseSpecBuilder builds adapter options from ClickHouse-specific specs.
type clickhouseSpecBuilder struct{}

// NewClickHouseSpecBuilder creates a new ClickHouse spec builder.
func NewClickHouseSpecBuilder() SpecBuilder {
	return &clickhouseSpecBuilder{}
}

// BuildDatabaseCreateOptions builds CreateDatabaseOptions from a DatabaseSpec.
func (b *clickhouseSpecBuilder) BuildDatabaseCreateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.CreateDatabaseOptions {
	opts := types.CreateDatabaseOptions{
		Name:  spec.Name,
		Owner: spec.Owner, // ClickHouse doesn't have native ownership, but tracked for metadata
	}

	if spec.ClickHouse != nil {
		opts.Engine = spec.ClickHouse.Engine
		opts.Comment = spec.ClickHouse.Comment
	}

	return opts
}

// BuildDatabaseUpdateOptions builds UpdateDatabaseOptions from a DatabaseSpec.
func (b *clickhouseSpecBuilder) BuildDatabaseUpdateOptions(spec *dbopsv1alpha1.DatabaseSpec) types.UpdateDatabaseOptions {
	opts := types.UpdateDatabaseOptions{}

	if spec.ClickHouse != nil {
		opts.Comment = spec.ClickHouse.Comment
	}

	return opts
}

// BuildUserCreateOptions builds CreateUserOptions from a DatabaseUserSpec.
func (b *clickhouseSpecBuilder) BuildUserCreateOptions(spec *dbopsv1alpha1.DatabaseUserSpec, password string) types.CreateUserOptions {
	opts := types.CreateUserOptions{
		Username: spec.Username,
		Password: password,
		Login:    true,
		Inherit:  true,
	}

	if spec.ClickHouse != nil {
		// Convert host restrictions to string format
		if len(spec.ClickHouse.HostRestrictions) > 0 {
			for _, hr := range spec.ClickHouse.HostRestrictions {
				var restriction string
				switch hr.Type {
				case "ANY", "LOCAL", "NONE":
					restriction = string(hr.Type)
				default:
					restriction = fmt.Sprintf("%s %s", hr.Type, hr.Value)
				}
				opts.HostRestrictions = append(opts.HostRestrictions, restriction)
			}
		}
		opts.DefaultDatabase = spec.ClickHouse.DefaultDatabase
		opts.DefaultRole = spec.ClickHouse.DefaultRole
	}

	return opts
}

// BuildUserUpdateOptions builds UpdateUserOptions from a DatabaseUserSpec.
func (b *clickhouseSpecBuilder) BuildUserUpdateOptions(_ *dbopsv1alpha1.DatabaseUserSpec) types.UpdateUserOptions {
	// ClickHouse users have minimal updatable attributes
	return types.UpdateUserOptions{}
}

// BuildRoleCreateOptions builds CreateRoleOptions from a DatabaseRoleSpec.
// ClickHouse roles are pure privilege containers — they have no attributes.
func (b *clickhouseSpecBuilder) BuildRoleCreateOptions(spec *dbopsv1alpha1.DatabaseRoleSpec) types.CreateRoleOptions {
	return types.CreateRoleOptions{
		RoleName: spec.RoleName,
		Inherit:  true,
	}
}

// BuildRoleUpdateOptions builds UpdateRoleOptions from a DatabaseRoleSpec.
func (b *clickhouseSpecBuilder) BuildRoleUpdateOptions(_ *dbopsv1alpha1.DatabaseRoleSpec) types.UpdateRoleOptions {
	return types.UpdateRoleOptions{}
}
