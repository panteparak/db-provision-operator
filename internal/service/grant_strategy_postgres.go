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
	"context"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
)

// postgresGrantStrategy applies/revokes grants against Postgres-protocol engines.
// CockroachDB shares this strategy because it accepts the same grant syntax.
type postgresGrantStrategy struct {
	svc *GrantService
}

func (st *postgresGrantStrategy) apply(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (*GrantResult, error) {
	result := &GrantResult{AppliedRoles: []string{}}
	if spec.Postgres == nil {
		return result, nil
	}

	if len(spec.Postgres.Roles) > 0 {
		op.Debug("granting roles", "roles", spec.Postgres.Roles)
		if err := st.svc.adapter.GrantRole(ctx, username, spec.Postgres.Roles); err != nil {
			op.Error(err, "failed to grant roles")
			return nil, st.svc.wrapError(ctx, st.svc.config, "grant roles", username, err)
		}
		result.AppliedRoles = append(result.AppliedRoles, spec.Postgres.Roles...)
	}

	if len(spec.Postgres.Grants) > 0 {
		op.Debug("applying direct grants", "count", len(spec.Postgres.Grants))
		grantOpts := buildPostgresGrantOptions(spec.Postgres.Grants)
		if err := st.svc.adapter.Grant(ctx, username, grantOpts); err != nil {
			op.Error(err, "failed to apply grants")
			return nil, st.svc.wrapError(ctx, st.svc.config, "apply grants", username, err)
		}
		result.AppliedDirectGrants = len(spec.Postgres.Grants)
	}

	if len(spec.Postgres.DefaultPrivileges) > 0 {
		op.Debug("setting default privileges", "count", len(spec.Postgres.DefaultPrivileges))
		defPrivOpts := buildDefaultPrivilegeOptions(spec.Postgres.DefaultPrivileges)
		if err := st.svc.adapter.SetDefaultPrivileges(ctx, username, defPrivOpts); err != nil {
			op.Error(err, "failed to set default privileges")
			return nil, st.svc.wrapError(ctx, st.svc.config, "set default privileges", username, err)
		}
		result.AppliedDefaultPrivileges = len(spec.Postgres.DefaultPrivileges)
	}

	return result, nil
}

func (st *postgresGrantStrategy) revoke(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (int, error) {
	if spec.Postgres == nil {
		return 0, nil
	}

	var revoked int

	if len(spec.Postgres.Roles) > 0 {
		op.Debug("revoking roles", "roles", spec.Postgres.Roles)
		if err := st.svc.adapter.RevokeRole(ctx, username, spec.Postgres.Roles); err != nil {
			op.Error(err, "failed to revoke roles")
			return 0, st.svc.wrapError(ctx, st.svc.config, "revoke roles", username, err)
		}
		revoked += len(spec.Postgres.Roles)
	}

	if len(spec.Postgres.Grants) > 0 {
		op.Debug("revoking direct grants", "count", len(spec.Postgres.Grants))
		grantOpts := buildPostgresGrantOptions(spec.Postgres.Grants)
		if err := st.svc.adapter.Revoke(ctx, username, grantOpts); err != nil {
			op.Error(err, "failed to revoke grants")
			return 0, st.svc.wrapError(ctx, st.svc.config, "revoke grants", username, err)
		}
		revoked += len(spec.Postgres.Grants)
	}

	return revoked, nil
}

// buildPostgresGrantOptions converts PostgresGrant specs to adapter GrantOptions.
func buildPostgresGrantOptions(grants []dbopsv1alpha1.PostgresGrant) []types.GrantOptions {
	opts := make([]types.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, types.GrantOptions{
			Database:        g.Database,
			Schema:          g.Schema,
			Tables:          g.Tables,
			Sequences:       g.Sequences,
			Functions:       g.Functions,
			Privileges:      g.Privileges,
			WithGrantOption: g.WithGrantOption,
		})
	}
	return opts
}

// buildDefaultPrivilegeOptions converts PostgresDefaultPrivilegeGrant specs to adapter options.
func buildDefaultPrivilegeOptions(defPrivs []dbopsv1alpha1.PostgresDefaultPrivilegeGrant) []types.DefaultPrivilegeGrantOptions {
	opts := make([]types.DefaultPrivilegeGrantOptions, 0, len(defPrivs))
	for _, dp := range defPrivs {
		opts = append(opts, types.DefaultPrivilegeGrantOptions{
			Database:   dp.Database,
			Schema:     dp.Schema,
			GrantedBy:  dp.GrantedBy,
			ObjectType: dp.ObjectType,
			Privileges: dp.Privileges,
		})
	}
	return opts
}
