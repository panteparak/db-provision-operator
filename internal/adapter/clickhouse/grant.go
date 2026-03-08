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

package clickhouse

import (
	"context"
	"fmt"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// grantLog is the logger for grant operations
var grantLog = ctrl.Log.WithName("clickhouse-adapter").WithName("grant")

// Grant grants privileges to a ClickHouse user or role
func (a *Adapter) Grant(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	log := grantLog.WithValues("grantee", grantee, "optCount", len(opts))
	log.V(1).Info("Granting privileges")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for i, opt := range opts {
		query, buildErr := a.buildGrantQuery(grantee, opt)
		if buildErr != nil {
			log.Error(buildErr, "Failed to build grant query", "index", i)
			return fmt.Errorf("failed to build grant query: %w", buildErr)
		}
		log.V(2).Info("Executing grant query", "index", i, "query", query)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			log.Error(err, "Failed to execute grant query", "query", query)
			return fmt.Errorf("failed to grant privileges to %s: %w", grantee, err)
		}
	}

	// No FLUSH PRIVILEGES needed — ClickHouse applies changes immediately
	log.V(1).Info("Successfully granted privileges")
	return nil
}

// Revoke revokes privileges from a ClickHouse user or role
func (a *Adapter) Revoke(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	log := grantLog.WithValues("grantee", grantee, "optCount", len(opts))
	log.V(1).Info("Revoking privileges")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for i, opt := range opts {
		query, buildErr := a.buildRevokeQuery(grantee, opt)
		if buildErr != nil {
			log.Error(buildErr, "Failed to build revoke query", "index", i)
			return fmt.Errorf("failed to build revoke query: %w", buildErr)
		}
		log.V(2).Info("Executing revoke query", "index", i, "query", query)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			log.Error(err, "Failed to execute revoke query", "query", query)
			return fmt.Errorf("failed to revoke privileges from %s: %w", grantee, err)
		}
	}

	log.V(1).Info("Successfully revoked privileges")
	return nil
}

// GrantRole grants role membership to a user
func (a *Adapter) GrantRole(ctx context.Context, grantee string, roles []string) error {
	log := grantLog.WithValues("grantee", grantee, "roles", roles)
	log.Info("Granting role membership")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	// ClickHouse users are global — no host-based iteration needed
	for _, role := range roles {
		query, buildErr := sqlbuilder.NewClickHouse().GrantRole(role).To(grantee).Build()
		if buildErr != nil {
			log.Error(buildErr, "Failed to build grant role query", "role", role)
			return fmt.Errorf("failed to build grant role query: %w", buildErr)
		}
		log.Info("Executing role grant", "role", role, "query", query)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			log.Error(err, "Failed to grant role", "role", role, "query", query)
			return fmt.Errorf("failed to grant role %s to %s: %w", role, grantee, err)
		}
		log.Info("Successfully granted role", "role", role)
	}

	log.Info("Successfully granted all role memberships")
	return nil
}

// RevokeRole revokes role membership from a user
func (a *Adapter) RevokeRole(ctx context.Context, grantee string, roles []string) error {
	log := grantLog.WithValues("grantee", grantee, "roles", roles)
	log.Info("Revoking role membership")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for _, role := range roles {
		query, buildErr := sqlbuilder.NewClickHouse().RevokeRole(role).From(grantee).Build()
		if buildErr != nil {
			log.Error(buildErr, "Failed to build revoke role query", "role", role)
			return fmt.Errorf("failed to build revoke role query: %w", buildErr)
		}
		log.Info("Executing role revoke", "role", role, "query", query)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			log.Error(err, "Failed to revoke role", "role", role, "query", query)
			return fmt.Errorf("failed to revoke role %s from %s: %w", role, grantee, err)
		}
		log.Info("Successfully revoked role", "role", role)
	}

	log.Info("Successfully revoked all role memberships")
	return nil
}

// SetDefaultPrivileges is a no-op for ClickHouse as it doesn't support default privileges.
func (a *Adapter) SetDefaultPrivileges(_ context.Context, _ string, _ []types.DefaultPrivilegeGrantOptions) error {
	return nil
}

// GetGrants retrieves grants for a ClickHouse user or role
func (a *Adapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Query system.grants for the grantee (can be user or role)
	rows, err := db.QueryContext(ctx, `
		SELECT
			user_name,
			role_name,
			access_type,
			database,
			table,
			grant_option
		FROM system.grants
		WHERE user_name = ? OR role_name = ?`,
		grantee, grantee)
	if err != nil {
		return nil, fmt.Errorf("failed to get grants: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []types.GrantInfo
	for rows.Next() {
		var (
			userName    string
			roleName    string
			accessType  string
			database    string
			table       string
			grantOption uint8
		)
		if err := rows.Scan(&userName, &roleName, &accessType, &database, &table, &grantOption); err != nil {
			return nil, fmt.Errorf("failed to scan grant: %w", err)
		}

		grant := types.GrantInfo{
			Grantee:    grantee,
			Database:   database,
			Privileges: []string{accessType},
		}
		if table != "" {
			grant.ObjectType = "table"
			grant.ObjectName = table
		} else if database != "" {
			grant.ObjectType = "database"
			grant.ObjectName = database
		}
		grants = append(grants, grant)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating grants: %w", err)
	}

	return grants, nil
}

// buildGrantQuery builds a GRANT query using the SQL builder.
func (a *Adapter) buildGrantQuery(grantee string, opt types.GrantOptions) (string, error) {
	// Column-level grants: GRANT priv ON db.table COLUMNS(col1, col2) TO user
	if len(opt.Columns) > 0 {
		return a.buildColumnGrantQuery("GRANT", "TO", grantee, opt)
	}

	b := sqlbuilder.NewClickHouse().Grant(opt.Privileges...)
	a.setClickHouseTarget(b, opt)
	b.To(grantee)
	if opt.WithGrantOption {
		b.WithGrantOption()
	}
	return b.Build()
}

// buildRevokeQuery builds a REVOKE query using the SQL builder.
func (a *Adapter) buildRevokeQuery(grantee string, opt types.GrantOptions) (string, error) {
	if len(opt.Columns) > 0 {
		return a.buildColumnGrantQuery("REVOKE", "FROM", grantee, opt)
	}

	b := sqlbuilder.NewClickHouse().Revoke(opt.Privileges...)
	a.setClickHouseTarget(b, opt)
	b.From(grantee)
	return b.Build()
}

// buildColumnGrantQuery builds column-level GRANT/REVOKE for ClickHouse.
func (a *Adapter) buildColumnGrantQuery(verb, preposition, grantee string, opt types.GrantOptions) (string, error) {
	d := sqlbuilder.ClickHouseDialect{}

	if err := sqlbuilder.ValidatePrivileges(opt.Privileges, d.ValidPrivileges()); err != nil {
		return "", err
	}

	privList := make([]string, len(opt.Privileges))
	for i, p := range opt.Privileges {
		privList[i] = strings.ToUpper(strings.TrimSpace(p))
	}

	cols := make([]string, len(opt.Columns))
	for i, c := range opt.Columns {
		cols[i] = d.EscapeIdentifier(c)
	}

	target := fmt.Sprintf("%s.%s",
		d.EscapeIdentifier(opt.Database),
		d.EscapeIdentifier(opt.Table))

	// ClickHouse uses parenthesized column list after privileges
	query := fmt.Sprintf("%s %s(%s) ON %s %s %s",
		verb,
		strings.Join(privList, ", "),
		strings.Join(cols, ", "),
		target,
		preposition,
		d.EscapeIdentifier(grantee))

	if opt.WithGrantOption && verb == "GRANT" {
		query += " WITH GRANT OPTION"
	}

	return query, nil
}

// setClickHouseTarget sets the target object on a GrantBuilder.
func (a *Adapter) setClickHouseTarget(b *sqlbuilder.GrantBuilder, opt types.GrantOptions) {
	switch opt.Level {
	case "global":
		b.OnGlobal()
	case "database":
		b.OnDatabase(opt.Database)
	case "table":
		b.OnMySQLTable(opt.Database, opt.Table) // Uses `db`.`table` format (same as MySQL)
	default:
		if opt.Table != "" && opt.Database != "" {
			b.OnMySQLTable(opt.Database, opt.Table)
		} else if opt.Database != "" {
			b.OnDatabase(opt.Database)
		} else {
			b.OnGlobal()
		}
	}
}
