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

// clickhouseGrantStrategy applies/revokes grants against ClickHouse.
type clickhouseGrantStrategy struct {
	svc *GrantService
}

func (st *clickhouseGrantStrategy) apply(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (*GrantResult, error) {
	result := &GrantResult{AppliedRoles: []string{}}
	if spec.ClickHouse == nil {
		return result, nil
	}

	if len(spec.ClickHouse.Roles) > 0 {
		op.Debug("granting roles", "roles", spec.ClickHouse.Roles)
		if err := st.svc.adapter.GrantRole(ctx, username, spec.ClickHouse.Roles); err != nil {
			op.Error(err, "failed to grant roles")
			return nil, st.svc.wrapError(ctx, st.svc.config, "grant roles", username, err)
		}
		result.AppliedRoles = append(result.AppliedRoles, spec.ClickHouse.Roles...)
	}

	if len(spec.ClickHouse.Grants) > 0 {
		op.Debug("applying direct grants", "count", len(spec.ClickHouse.Grants))
		grantOpts := buildClickHouseGrantOptions(spec.ClickHouse.Grants)
		if err := st.svc.adapter.Grant(ctx, username, grantOpts); err != nil {
			op.Error(err, "failed to apply grants")
			return nil, st.svc.wrapError(ctx, st.svc.config, "apply grants", username, err)
		}
		result.AppliedDirectGrants = len(spec.ClickHouse.Grants)
	}

	return result, nil
}

func (st *clickhouseGrantStrategy) revoke(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (int, error) {
	if spec.ClickHouse == nil {
		return 0, nil
	}

	var revoked int

	if len(spec.ClickHouse.Roles) > 0 {
		op.Debug("revoking roles", "roles", spec.ClickHouse.Roles)
		if err := st.svc.adapter.RevokeRole(ctx, username, spec.ClickHouse.Roles); err != nil {
			op.Error(err, "failed to revoke roles")
			return 0, st.svc.wrapError(ctx, st.svc.config, "revoke roles", username, err)
		}
		revoked += len(spec.ClickHouse.Roles)
	}

	if len(spec.ClickHouse.Grants) > 0 {
		op.Debug("revoking direct grants", "count", len(spec.ClickHouse.Grants))
		grantOpts := buildClickHouseGrantOptions(spec.ClickHouse.Grants)
		if err := st.svc.adapter.Revoke(ctx, username, grantOpts); err != nil {
			op.Error(err, "failed to revoke grants")
			return 0, st.svc.wrapError(ctx, st.svc.config, "revoke grants", username, err)
		}
		revoked += len(spec.ClickHouse.Grants)
	}

	return revoked, nil
}

// buildClickHouseGrantOptions converts ClickHouseGrant specs to adapter GrantOptions.
func buildClickHouseGrantOptions(grants []dbopsv1alpha1.ClickHouseGrant) []types.GrantOptions {
	opts := make([]types.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, types.GrantOptions{
			Level:           string(g.Level),
			Database:        g.Database,
			Table:           g.Table,
			Privileges:      g.Privileges,
			WithGrantOption: g.WithGrantOption,
		})
	}
	return opts
}
