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

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// grantLog is the logger for grant operations
var grantLog = ctrl.Log.WithName("mysql-adapter").WithName("grant")

// Grant grants privileges to a MySQL user
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
		log.V(2).Info("Executing grant query", "index", i, "query", query, "level", opt.Level, "database", opt.Database)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			log.Error(err, "Failed to execute grant query", "query", query)
			return fmt.Errorf("failed to grant privileges to %s: %w", grantee, err)
		}
	}

	// Flush privileges to ensure changes take effect
	log.V(2).Info("Flushing privileges")
	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		log.Error(err, "Failed to flush privileges")
		return fmt.Errorf("failed to flush privileges: %w", err)
	}

	log.V(1).Info("Successfully granted privileges")
	return nil
}

// Revoke revokes privileges from a MySQL user
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
		log.V(2).Info("Executing revoke query", "index", i, "query", query, "level", opt.Level, "database", opt.Database)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			log.Error(err, "Failed to execute revoke query", "query", query)
			return fmt.Errorf("failed to revoke privileges from %s: %w", grantee, err)
		}
	}

	// Flush privileges
	log.V(2).Info("Flushing privileges")
	_, err = db.ExecContext(ctx, "FLUSH PRIVILEGES")
	if err != nil {
		log.Error(err, "Failed to flush privileges")
		return fmt.Errorf("failed to flush privileges: %w", err)
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

	// Get all hosts for this user - MySQL/MariaDB requires user@host format
	hosts, err := a.getUserHosts(ctx, grantee)
	if err != nil {
		log.Error(err, "Failed to get hosts for user")
		return fmt.Errorf("failed to get hosts for user %s: %w", grantee, err)
	}
	if len(hosts) == 0 {
		log.Error(nil, "User not found - no hosts returned")
		return fmt.Errorf("user %s not found", grantee)
	}
	log.V(1).Info("Found user hosts", "hosts", hosts)

	for _, role := range roles {
		for _, host := range hosts {
			query, buildErr := sqlbuilder.NewMySQL().GrantRole(role).ToUser(grantee, host).Build()
			if buildErr != nil {
				log.Error(buildErr, "Failed to build grant role query", "role", role, "host", host)
				return fmt.Errorf("failed to build grant role query: %w", buildErr)
			}
			log.Info("Executing role grant", "role", role, "host", host, "query", query)
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				log.Error(err, "Failed to grant role", "role", role, "host", host, "query", query)
				return fmt.Errorf("failed to grant role %s to %s@%s: %w", role, grantee, host, err)
			}
			log.Info("Successfully granted role", "role", role, "host", host)
		}
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

	// Get all hosts for this user - MySQL/MariaDB requires user@host format
	hosts, err := a.getUserHosts(ctx, grantee)
	if err != nil {
		log.Error(err, "Failed to get hosts for user")
		return fmt.Errorf("failed to get hosts for user %s: %w", grantee, err)
	}
	if len(hosts) == 0 {
		log.Error(nil, "User not found - no hosts returned")
		return fmt.Errorf("user %s not found", grantee)
	}
	log.V(1).Info("Found user hosts", "hosts", hosts)

	for _, role := range roles {
		for _, host := range hosts {
			query, buildErr := sqlbuilder.NewMySQL().RevokeRole(role).FromUser(grantee, host).Build()
			if buildErr != nil {
				log.Error(buildErr, "Failed to build revoke role query", "role", role, "host", host)
				return fmt.Errorf("failed to build revoke role query: %w", buildErr)
			}
			log.Info("Executing role revoke", "role", role, "host", host, "query", query)
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				log.Error(err, "Failed to revoke role", "role", role, "host", host, "query", query)
				return fmt.Errorf("failed to revoke role %s from %s@%s: %w", role, grantee, host, err)
			}
			log.Info("Successfully revoked role", "role", role, "host", host)
		}
	}

	log.Info("Successfully revoked all role memberships")
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

// buildGrantQuery builds a GRANT query using the SQL builder.
func (a *Adapter) buildGrantQuery(grantee string, opt types.GrantOptions) (string, error) {
	// Column-level grants need special handling: GRANT priv (col1, col2) ON db.table TO user
	if opt.Level == "column" && len(opt.Columns) > 0 {
		return a.buildColumnGrantQuery("GRANT", "TO", grantee, opt)
	}

	b := sqlbuilder.NewMySQL().Grant(opt.Privileges...)
	a.setMySQLTarget(b, opt)
	b.ToLiteral(grantee)
	if opt.WithGrantOption {
		b.WithGrantOption()
	}
	return b.Build()
}

// buildRevokeQuery builds a REVOKE query using the SQL builder.
func (a *Adapter) buildRevokeQuery(grantee string, opt types.GrantOptions) (string, error) {
	// Column-level revokes need special handling
	if opt.Level == "column" && len(opt.Columns) > 0 {
		return a.buildColumnGrantQuery("REVOKE", "FROM", grantee, opt)
	}

	b := sqlbuilder.NewMySQL().Revoke(opt.Privileges...)
	a.setMySQLTarget(b, opt)
	b.FromLiteral(grantee)
	return b.Build()
}

// buildColumnGrantQuery builds column-level GRANT/REVOKE with escaped columns.
func (a *Adapter) buildColumnGrantQuery(verb, preposition, grantee string, opt types.GrantOptions) (string, error) {
	d := sqlbuilder.MySQLDialect{}

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

	query := fmt.Sprintf("%s %s (%s) ON %s %s %s",
		verb,
		strings.Join(privList, ", "),
		strings.Join(cols, ", "),
		target,
		preposition,
		d.EscapeLiteral(grantee))

	if opt.WithGrantOption && verb == "GRANT" {
		query += " WITH GRANT OPTION"
	}

	return query, nil
}

// setMySQLTarget sets the target object on a GrantBuilder based on GrantOptions.
func (a *Adapter) setMySQLTarget(b *sqlbuilder.GrantBuilder, opt types.GrantOptions) {
	switch opt.Level {
	case "global":
		b.OnGlobal()
	case "database":
		b.OnDatabase(opt.Database)
	case "table":
		b.OnMySQLTable(opt.Database, opt.Table)
	case "procedure":
		b.OnProcedure(opt.Database, opt.Procedure)
	case "function":
		b.OnMySQLFunction(opt.Database, opt.Function)
	default:
		if opt.Database != "" {
			b.OnDatabase(opt.Database)
		} else {
			b.OnGlobal()
		}
	}
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

// getUserHosts retrieves all hosts associated with a MySQL/MariaDB user
func (a *Adapter) getUserHosts(ctx context.Context, username string) ([]string, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx,
		"SELECT Host FROM mysql.user WHERE User = ?",
		username)
	if err != nil {
		return nil, fmt.Errorf("failed to query user hosts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hosts []string
	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}

	return hosts, nil
}
