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

var grantLog = ctrl.Log.WithName("clickhouse-adapter").WithName("grant")

// Grant grants privileges to a ClickHouse user or role.
func (a *Adapter) Grant(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	log := grantLog.WithValues("grantee", grantee, "optCount", len(opts))
	log.V(1).Info("Granting privileges")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for i, opt := range opts {
		if err := sqlbuilder.ValidatePrivileges(opt.Privileges, sqlbuilder.ClickHouseDialect{}.ValidPrivileges()); err != nil {
			log.Error(err, "Invalid privileges", "index", i)
			return fmt.Errorf("failed to build grant query: %w", err)
		}

		privList := make([]string, len(opt.Privileges))
		for j, p := range opt.Privileges {
			privList[j] = strings.ToUpper(strings.TrimSpace(p))
		}

		target := buildClickHouseTarget(opt)
		query := fmt.Sprintf("GRANT %s ON %s TO %s",
			strings.Join(privList, ", "),
			target,
			escapeIdentifier(grantee))

		log.V(2).Info("Executing grant query", "index", i, "query", query, "level", opt.Level, "database", opt.Database)
		if _, execErr := db.ExecContext(ctx, query); execErr != nil {
			log.Error(execErr, "Failed to execute grant query", "query", query)
			return fmt.Errorf("failed to grant privileges to %s: %w", grantee, execErr)
		}
	}

	log.V(1).Info("Successfully granted privileges")
	return nil
}

// Revoke revokes privileges from a ClickHouse user or role.
func (a *Adapter) Revoke(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	log := grantLog.WithValues("grantee", grantee, "optCount", len(opts))
	log.V(1).Info("Revoking privileges")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for i, opt := range opts {
		if err := sqlbuilder.ValidatePrivileges(opt.Privileges, sqlbuilder.ClickHouseDialect{}.ValidPrivileges()); err != nil {
			log.Error(err, "Invalid privileges", "index", i)
			return fmt.Errorf("failed to build revoke query: %w", err)
		}

		privList := make([]string, len(opt.Privileges))
		for j, p := range opt.Privileges {
			privList[j] = strings.ToUpper(strings.TrimSpace(p))
		}

		target := buildClickHouseTarget(opt)
		query := fmt.Sprintf("REVOKE %s ON %s FROM %s",
			strings.Join(privList, ", "),
			target,
			escapeIdentifier(grantee))

		log.V(2).Info("Executing revoke query", "index", i, "query", query, "level", opt.Level, "database", opt.Database)
		if _, execErr := db.ExecContext(ctx, query); execErr != nil {
			log.Error(execErr, "Failed to execute revoke query", "query", query)
			return fmt.Errorf("failed to revoke privileges from %s: %w", grantee, execErr)
		}
	}

	log.V(1).Info("Successfully revoked privileges")
	return nil
}

// GrantRole grants role membership to a ClickHouse user.
func (a *Adapter) GrantRole(ctx context.Context, grantee string, roles []string) error {
	log := grantLog.WithValues("grantee", grantee, "roles", roles)
	log.Info("Granting role membership")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for _, role := range roles {
		query := fmt.Sprintf("GRANT %s TO %s",
			escapeIdentifier(role),
			escapeIdentifier(grantee))

		log.Info("Executing role grant", "role", role, "query", query)
		if _, execErr := db.ExecContext(ctx, query); execErr != nil {
			log.Error(execErr, "Failed to grant role", "role", role, "query", query)
			return fmt.Errorf("failed to grant role %s to %s: %w", role, grantee, execErr)
		}
		log.Info("Successfully granted role", "role", role)
	}

	log.Info("Successfully granted all role memberships")
	return nil
}

// RevokeRole revokes role membership from a ClickHouse user.
func (a *Adapter) RevokeRole(ctx context.Context, grantee string, roles []string) error {
	log := grantLog.WithValues("grantee", grantee, "roles", roles)
	log.Info("Revoking role membership")

	db, err := a.getDB()
	if err != nil {
		log.Error(err, "Failed to get database connection")
		return err
	}

	for _, role := range roles {
		query := fmt.Sprintf("REVOKE %s FROM %s",
			escapeIdentifier(role),
			escapeIdentifier(grantee))

		log.Info("Executing role revoke", "role", role, "query", query)
		if _, execErr := db.ExecContext(ctx, query); execErr != nil {
			log.Error(execErr, "Failed to revoke role", "role", role, "query", query)
			return fmt.Errorf("failed to revoke role %s from %s: %w", role, grantee, execErr)
		}
		log.Info("Successfully revoked role", "role", role)
	}

	log.Info("Successfully revoked all role memberships")
	return nil
}

// SetDefaultPrivileges is a no-op for ClickHouse as it does not support default privileges.
func (a *Adapter) SetDefaultPrivileges(_ context.Context, _ string, _ []types.DefaultPrivilegeGrantOptions) error {
	// ClickHouse does not support default privileges like PostgreSQL.
	return nil
}

// GetGrants retrieves grants for a ClickHouse user or role.
func (a *Adapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SHOW GRANTS FOR %s", escapeIdentifier(grantee))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get grants for %s: %w", grantee, err)
	}
	defer func() { _ = rows.Close() }()

	var grants []types.GrantInfo
	for rows.Next() {
		var grantStr string
		if err := rows.Scan(&grantStr); err != nil {
			return nil, fmt.Errorf("failed to scan grant row: %w", err)
		}

		grant := parseClickHouseGrantString(grantStr, grantee)
		if grant != nil {
			grants = append(grants, *grant)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating grant rows: %w", err)
	}

	return grants, nil
}

// buildClickHouseTarget returns the ON target string for a GRANT/REVOKE query based on the grant level.
func buildClickHouseTarget(opt types.GrantOptions) string {
	switch opt.Level {
	case "global":
		return "*.*"
	case "database":
		return escapeIdentifier(opt.Database) + ".*"
	case "table":
		return escapeIdentifier(opt.Database) + "." + escapeIdentifier(opt.Table)
	default:
		if opt.Table != "" && opt.Database != "" {
			return escapeIdentifier(opt.Database) + "." + escapeIdentifier(opt.Table)
		}
		if opt.Database != "" {
			return escapeIdentifier(opt.Database) + ".*"
		}
		return "*.*"
	}
}

// parseClickHouseGrantString parses a ClickHouse SHOW GRANTS row into a GrantInfo.
func parseClickHouseGrantString(grantStr, grantee string) *types.GrantInfo {
	grant := &types.GrantInfo{
		Grantee: grantee,
	}

	// Example: GRANT SELECT, INSERT ON db.table TO user
	grantStr = strings.TrimPrefix(grantStr, "GRANT ")
	parts := strings.SplitN(grantStr, " ON ", 2)
	if len(parts) < 2 {
		return nil
	}

	privs := strings.Split(parts[0], ", ")
	grant.Privileges = privs

	remaining := parts[1]
	objParts := strings.SplitN(remaining, " TO ", 2)
	if len(objParts) < 1 {
		return nil
	}

	obj := strings.TrimSpace(objParts[0])
	if strings.Contains(obj, ".") {
		dbTable := strings.SplitN(obj, ".", 2)
		grant.Database = strings.Trim(dbTable[0], "`")
		if len(dbTable) > 1 {
			tableName := strings.Trim(dbTable[1], "`")
			if tableName != "*" {
				grant.ObjectType = "table"
				grant.ObjectName = tableName
			} else {
				grant.ObjectType = "database"
				grant.ObjectName = grant.Database
			}
		}
	}

	return grant
}
