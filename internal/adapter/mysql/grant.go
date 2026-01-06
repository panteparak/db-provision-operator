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

package mysql

import (
	"context"
	"fmt"
	"strings"

	"github.com/db-provision-operator/internal/adapter/types"
)

// Grant grants privileges to a MySQL user
func (a *Adapter) Grant(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	for _, opt := range opts {
		query := a.buildGrantQuery(grantee, opt)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to grant privileges to %s: %w", grantee, err)
		}
	}

	// Flush privileges to ensure changes take effect
	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("failed to flush privileges: %w", err)
	}

	return nil
}

// Revoke revokes privileges from a MySQL user
func (a *Adapter) Revoke(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	for _, opt := range opts {
		query := a.buildRevokeQuery(grantee, opt)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to revoke privileges from %s: %w", grantee, err)
		}
	}

	// Flush privileges
	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		return fmt.Errorf("failed to flush privileges: %w", err)
	}

	return nil
}

// GrantRole grants role membership to a user
func (a *Adapter) GrantRole(ctx context.Context, grantee string, roles []string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	for _, role := range roles {
		query := fmt.Sprintf("GRANT %s TO %s",
			escapeLiteral(role),
			escapeLiteral(grantee))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to grant role %s to %s: %w", role, grantee, err)
		}
	}

	return nil
}

// RevokeRole revokes role membership from a user
func (a *Adapter) RevokeRole(ctx context.Context, grantee string, roles []string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	for _, role := range roles {
		query := fmt.Sprintf("REVOKE %s FROM %s",
			escapeLiteral(role),
			escapeLiteral(grantee))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to revoke role %s from %s: %w", role, grantee, err)
		}
	}

	return nil
}

// SetDefaultPrivileges sets default privileges (MySQL doesn't have this concept)
// This is a no-op for MySQL as it doesn't support default privileges like PostgreSQL
func (a *Adapter) SetDefaultPrivileges(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
	// MySQL doesn't have default privileges like PostgreSQL
	// This could be implemented with triggers in the future
	return nil
}

// GetGrants retrieves grants for a MySQL user
func (a *Adapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Get hosts for the user
	hostRows, err := db.QueryContext(ctx,
		"SELECT Host FROM mysql.user WHERE User = ?",
		grantee)
	if err != nil {
		return nil, fmt.Errorf("failed to get user hosts: %w", err)
	}
	defer func() { _ = hostRows.Close() }()

	var hosts []string
	for hostRows.Next() {
		var host string
		if err := hostRows.Scan(&host); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}

	var grants []types.GrantInfo

	for _, host := range hosts {
		// Get grants for this user@host
		rows, err := db.QueryContext(ctx,
			fmt.Sprintf("SHOW GRANTS FOR %s@%s",
				escapeLiteral(grantee),
				escapeLiteral(host)))
		if err != nil {
			continue // Skip if can't get grants
		}

		for rows.Next() {
			var grantStr string
			if err := rows.Scan(&grantStr); err != nil {
				_ = rows.Close()
				continue
			}

			// Parse the grant string
			grant := a.parseGrantString(grantStr, grantee)
			if grant != nil {
				grants = append(grants, *grant)
			}
		}
		_ = rows.Close()
	}

	return grants, nil
}

// buildGrantQuery builds a GRANT query
func (a *Adapter) buildGrantQuery(grantee string, opt types.GrantOptions) string {
	privileges := formatPrivileges(opt.Privileges)

	var target string
	switch opt.Level {
	case "global":
		target = "*.*"
	case "database":
		target = fmt.Sprintf("%s.*", escapeIdentifier(opt.Database))
	case "table":
		target = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Table))
	case "column":
		// Column-level privileges
		if len(opt.Columns) > 0 {
			var cols []string
			for _, col := range opt.Columns {
				cols = append(cols, escapeIdentifier(col))
			}
			privileges = fmt.Sprintf("%s (%s)", privileges, strings.Join(cols, ", "))
		}
		target = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Table))
	case "procedure":
		target = fmt.Sprintf("PROCEDURE %s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Procedure))
	case "function":
		target = fmt.Sprintf("FUNCTION %s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Function))
	default:
		// Default to database level if database is specified
		if opt.Database != "" {
			target = fmt.Sprintf("%s.*", escapeIdentifier(opt.Database))
		} else {
			target = "*.*"
		}
	}

	query := fmt.Sprintf("GRANT %s ON %s TO %s", privileges, target, escapeLiteral(grantee))

	if opt.WithGrantOption {
		query += " WITH GRANT OPTION"
	}

	return query
}

// buildRevokeQuery builds a REVOKE query
func (a *Adapter) buildRevokeQuery(grantee string, opt types.GrantOptions) string {
	privileges := formatPrivileges(opt.Privileges)

	var target string
	switch opt.Level {
	case "global":
		target = "*.*"
	case "database":
		target = fmt.Sprintf("%s.*", escapeIdentifier(opt.Database))
	case "table":
		target = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Table))
	case "column":
		if len(opt.Columns) > 0 {
			var cols []string
			for _, col := range opt.Columns {
				cols = append(cols, escapeIdentifier(col))
			}
			privileges = fmt.Sprintf("%s (%s)", privileges, strings.Join(cols, ", "))
		}
		target = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Table))
	case "procedure":
		target = fmt.Sprintf("PROCEDURE %s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Procedure))
	case "function":
		target = fmt.Sprintf("FUNCTION %s.%s", escapeIdentifier(opt.Database), escapeIdentifier(opt.Function))
	default:
		if opt.Database != "" {
			target = fmt.Sprintf("%s.*", escapeIdentifier(opt.Database))
		} else {
			target = "*.*"
		}
	}

	return fmt.Sprintf("REVOKE %s ON %s FROM %s", privileges, target, escapeLiteral(grantee))
}

// parseGrantString parses a MySQL GRANT string into a GrantInfo
func (a *Adapter) parseGrantString(grantStr, grantee string) *types.GrantInfo {
	// This is a simplified parser - full parsing would be more complex
	grant := &types.GrantInfo{
		Grantee: grantee,
	}

	// Extract privileges (between GRANT and ON)
	grantStr = strings.TrimPrefix(grantStr, "GRANT ")
	parts := strings.SplitN(grantStr, " ON ", 2)
	if len(parts) < 2 {
		return nil
	}

	privs := strings.Split(parts[0], ", ")
	grant.Privileges = privs

	// Extract object
	remaining := parts[1]
	objParts := strings.SplitN(remaining, " TO ", 2)
	if len(objParts) < 1 {
		return nil
	}

	obj := objParts[0]
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
