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

// mysqlGrantStrategy applies/revokes grants against MySQL-protocol engines.
// MariaDB shares this strategy because it accepts the same grant syntax.
type mysqlGrantStrategy struct {
	svc *GrantService
}

func (st *mysqlGrantStrategy) apply(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (*GrantResult, error) {
	result := &GrantResult{AppliedRoles: []string{}}
	if spec.MySQL == nil {
		return result, nil
	}

	if len(spec.MySQL.Roles) > 0 {
		op.Debug("granting roles", "roles", spec.MySQL.Roles)
		if err := st.svc.adapter.GrantRole(ctx, username, spec.MySQL.Roles); err != nil {
			op.Error(err, "failed to grant roles")
			return nil, st.svc.wrapError(ctx, st.svc.config, "grant roles", username, err)
		}
		result.AppliedRoles = append(result.AppliedRoles, spec.MySQL.Roles...)
	}

	if len(spec.MySQL.Grants) > 0 {
		op.Debug("applying direct grants", "count", len(spec.MySQL.Grants))
		grantOpts := buildMySQLGrantOptions(spec.MySQL.Grants)
		if err := st.svc.adapter.Grant(ctx, username, grantOpts); err != nil {
			op.Error(err, "failed to apply grants")
			return nil, st.svc.wrapError(ctx, st.svc.config, "apply grants", username, err)
		}
		result.AppliedDirectGrants = len(spec.MySQL.Grants)
	}

	return result, nil
}

func (st *mysqlGrantStrategy) revoke(ctx context.Context, username string, spec *dbopsv1alpha1.DatabaseGrantSpec, op *operationLogger) (int, error) {
	if spec.MySQL == nil {
		return 0, nil
	}

	var revoked int

	if len(spec.MySQL.Roles) > 0 {
		op.Debug("revoking roles", "roles", spec.MySQL.Roles)
		if err := st.svc.adapter.RevokeRole(ctx, username, spec.MySQL.Roles); err != nil {
			op.Error(err, "failed to revoke roles")
			return 0, st.svc.wrapError(ctx, st.svc.config, "revoke roles", username, err)
		}
		revoked += len(spec.MySQL.Roles)
	}

	if len(spec.MySQL.Grants) > 0 {
		op.Debug("revoking direct grants", "count", len(spec.MySQL.Grants))
		grantOpts := buildMySQLGrantOptions(spec.MySQL.Grants)
		if err := st.svc.adapter.Revoke(ctx, username, grantOpts); err != nil {
			op.Error(err, "failed to revoke grants")
			return 0, st.svc.wrapError(ctx, st.svc.config, "revoke grants", username, err)
		}
		revoked += len(spec.MySQL.Grants)
	}

	return revoked, nil
}

// buildMySQLGrantOptions converts MySQLGrant specs to adapter GrantOptions.
func buildMySQLGrantOptions(grants []dbopsv1alpha1.MySQLGrant) []types.GrantOptions {
	opts := make([]types.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, types.GrantOptions{
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
	return opts
}
