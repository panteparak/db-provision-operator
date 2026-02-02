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

package cockroachdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/db-provision-operator/internal/adapter/types"
)

// Grant grants privileges to a user or role in CockroachDB.
func (a *Adapter) Grant(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	for _, opt := range opts {
		if err := a.grantPrivileges(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// Revoke revokes privileges from a user or role in CockroachDB.
func (a *Adapter) Revoke(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	for _, opt := range opts {
		if err := a.revokePrivileges(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// GrantRole grants role membership to a user or role.
func (a *Adapter) GrantRole(ctx context.Context, grantee string, roles []string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	for _, role := range roles {
		query := fmt.Sprintf("GRANT %s TO %s",
			escapeIdentifier(role),
			escapeIdentifier(grantee))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to grant role %s to %s: %w", role, grantee, err)
		}
	}

	return nil
}

// RevokeRole revokes role membership from a user or role.
func (a *Adapter) RevokeRole(ctx context.Context, grantee string, roles []string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	for _, role := range roles {
		query := fmt.Sprintf("REVOKE %s FROM %s",
			escapeIdentifier(role),
			escapeIdentifier(grantee))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to revoke role %s from %s: %w", role, grantee, err)
		}
	}

	return nil
}

// SetDefaultPrivileges sets default privileges for new objects in CockroachDB.
// CockroachDB supports ALTER DEFAULT PRIVILEGES with the same syntax as PostgreSQL.
func (a *Adapter) SetDefaultPrivileges(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
	for _, opt := range opts {
		if err := a.setDefaultPrivilege(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// GetGrants retrieves grants for a user or role in CockroachDB.
// CockroachDB provides SHOW GRANTS which is more ergonomic than PostgreSQL's aclexplode().
func (a *Adapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var grants []types.GrantInfo

	// Query database-level grants using SHOW GRANTS
	rows, err := pool.Query(ctx,
		"SELECT database_name, privilege_type FROM crdb_internal.cluster_database_privileges WHERE grantee = $1",
		grantee)
	if err != nil {
		return nil, fmt.Errorf("failed to get database grants: %w", err)
	}

	// Group privileges by database
	dbPrivileges := make(map[string][]string)
	for rows.Next() {
		var dbName, privilege string
		if err := rows.Scan(&dbName, &privilege); err != nil {
			rows.Close()
			return nil, err
		}
		dbPrivileges[dbName] = append(dbPrivileges[dbName], privilege)
	}
	rows.Close()

	for dbName, privs := range dbPrivileges {
		grants = append(grants, types.GrantInfo{
			Grantee:    grantee,
			Database:   dbName,
			ObjectType: "database",
			ObjectName: dbName,
			Privileges: privs,
		})
	}

	return grants, nil
}

// grantPrivileges grants privileges for a single grant option.
func (a *Adapter) grantPrivileges(ctx context.Context, grantee string, opt types.GrantOptions) error {
	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var queries []string

	// Handle ALL TABLES IN SCHEMA or schema-level grants
	if opt.Schema != "" && len(opt.Tables) == 0 && len(opt.Sequences) == 0 && len(opt.Functions) == 0 {
		hasTablePrivs := false
		for _, p := range opt.Privileges {
			upper := strings.ToUpper(p)
			if upper == "SELECT" || upper == "INSERT" || upper == "UPDATE" || upper == "DELETE" ||
				upper == "TRUNCATE" || upper == "REFERENCES" || upper == "TRIGGER" || upper == "ALL" {
				hasTablePrivs = true
				break
			}
		}

		if hasTablePrivs {
			q := fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s",
				strings.Join(opt.Privileges, ", "),
				escapeIdentifier(opt.Schema),
				escapeIdentifier(grantee))
			if opt.WithGrantOption {
				q += " WITH GRANT OPTION"
			}
			queries = append(queries, q)
		}

		// Schema usage grant
		queries = append(queries, fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s",
			escapeIdentifier(opt.Schema),
			escapeIdentifier(grantee)))
	}

	// Specific tables
	for _, table := range opt.Tables {
		tableName := table
		if opt.Schema != "" {
			tableName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(table))
		} else {
			tableName = escapeIdentifier(table)
		}

		q := fmt.Sprintf("GRANT %s ON TABLE %s TO %s",
			strings.Join(opt.Privileges, ", "),
			tableName,
			escapeIdentifier(grantee))
		if opt.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		seqName := seq
		if opt.Schema != "" {
			seqName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(seq))
		} else {
			seqName = escapeIdentifier(seq)
		}

		q := fmt.Sprintf("GRANT %s ON SEQUENCE %s TO %s",
			strings.Join(opt.Privileges, ", "),
			seqName,
			escapeIdentifier(grantee))
		if opt.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		fnName := fn
		if opt.Schema != "" {
			fnName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), fn)
		}

		q := fmt.Sprintf("GRANT %s ON FUNCTION %s TO %s",
			strings.Join(opt.Privileges, ", "),
			fnName,
			escapeIdentifier(grantee))
		if opt.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Execute all queries
	for _, query := range queries {
		if err := a.execWithNewConnection(ctx, database, query); err != nil {
			return fmt.Errorf("failed to execute grant: %w", err)
		}
	}

	return nil
}

// revokePrivileges revokes privileges for a single grant option.
func (a *Adapter) revokePrivileges(ctx context.Context, grantee string, opt types.GrantOptions) error {
	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var queries []string

	// Handle ALL TABLES IN SCHEMA
	if opt.Schema != "" && len(opt.Tables) == 0 {
		q := fmt.Sprintf("REVOKE %s ON ALL TABLES IN SCHEMA %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			escapeIdentifier(opt.Schema),
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Specific tables
	for _, table := range opt.Tables {
		tableName := table
		if opt.Schema != "" {
			tableName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(table))
		} else {
			tableName = escapeIdentifier(table)
		}

		q := fmt.Sprintf("REVOKE %s ON TABLE %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			tableName,
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		seqName := seq
		if opt.Schema != "" {
			seqName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(seq))
		} else {
			seqName = escapeIdentifier(seq)
		}

		q := fmt.Sprintf("REVOKE %s ON SEQUENCE %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			seqName,
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		fnName := fn
		if opt.Schema != "" {
			fnName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), fn)
		}

		q := fmt.Sprintf("REVOKE %s ON FUNCTION %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			fnName,
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Execute all queries
	for _, query := range queries {
		if err := a.execWithNewConnection(ctx, database, query); err != nil {
			return fmt.Errorf("failed to execute revoke: %w", err)
		}
	}

	return nil
}

// setDefaultPrivilege sets a single default privilege.
func (a *Adapter) setDefaultPrivilege(ctx context.Context, grantee string, opt types.DefaultPrivilegeGrantOptions) error {
	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var sb strings.Builder
	sb.WriteString("ALTER DEFAULT PRIVILEGES")

	if opt.GrantedBy != "" {
		sb.WriteString(" FOR ROLE ")
		sb.WriteString(escapeIdentifier(opt.GrantedBy))
	}

	if opt.Schema != "" {
		sb.WriteString(" IN SCHEMA ")
		sb.WriteString(escapeIdentifier(opt.Schema))
	}

	sb.WriteString(" GRANT ")
	sb.WriteString(strings.Join(opt.Privileges, ", "))
	sb.WriteString(" ON ")

	switch strings.ToLower(opt.ObjectType) {
	case "tables":
		sb.WriteString("TABLES")
	case "sequences":
		sb.WriteString("SEQUENCES")
	case "functions":
		sb.WriteString("FUNCTIONS")
	case "types":
		sb.WriteString("TYPES")
	case "schemas":
		sb.WriteString("SCHEMAS")
	default:
		return fmt.Errorf("unsupported object type: %s", opt.ObjectType)
	}

	sb.WriteString(" TO ")
	sb.WriteString(escapeIdentifier(grantee))

	return a.execWithNewConnection(ctx, database, sb.String())
}
