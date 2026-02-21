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

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
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
		query, buildErr := sqlbuilder.NewPg().GrantRole(role).To(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build grant role query: %w", buildErr)
		}
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
		query, buildErr := sqlbuilder.NewPg().RevokeRole(role).From(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build revoke role query: %w", buildErr)
		}
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to revoke role %s from %s: %w", role, grantee, err)
		}
	}

	return nil
}

// SetDefaultPrivileges sets default privileges for new objects in CockroachDB.
// CockroachDB supports ALTER DEFAULT PRIVILEGES with the same syntax as PostgreSQL.
func (a *Adapter) SetDefaultPrivileges(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
	// Verify connection before proceeding
	if _, err := a.getPool(); err != nil {
		return err
	}

	for _, opt := range opts {
		if err := a.setDefaultPrivilege(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// GetGrants retrieves grants for a user or role in CockroachDB.
// Queries each database for grants to the specified grantee.
func (a *Adapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var grants []types.GrantInfo

	// First, get list of all databases
	dbRows, err := pool.Query(ctx, "SELECT name FROM crdb_internal.databases WHERE name NOT IN ('system')")
	if err != nil {
		return nil, fmt.Errorf("failed to list databases: %w", err)
	}

	var databases []string
	for dbRows.Next() {
		var dbName string
		if err := dbRows.Scan(&dbName); err != nil {
			dbRows.Close()
			return nil, err
		}
		databases = append(databases, dbName)
	}
	dbRows.Close()

	// For each database, check grants for this grantee
	for _, dbName := range databases {
		query := fmt.Sprintf("SHOW GRANTS ON DATABASE %s", escapeIdentifier(dbName))
		rows, err := pool.Query(ctx, query)
		if err != nil {
			continue // Skip databases we can't access
		}

		var privileges []string
		for rows.Next() {
			// SHOW GRANTS ON DATABASE returns: database_name, grantee, privilege_type, is_grantable
			var dbNameResult, granteeResult, privilege string
			var isGrantable bool
			if err := rows.Scan(&dbNameResult, &granteeResult, &privilege, &isGrantable); err != nil {
				rows.Close()
				continue
			}
			if granteeResult == grantee {
				privileges = append(privileges, privilege)
			}
		}
		rows.Close()

		if len(privileges) > 0 {
			grants = append(grants, types.GrantInfo{
				Grantee:    grantee,
				Database:   dbName,
				ObjectType: "database",
				ObjectName: dbName,
				Privileges: privileges,
			})
		}
	}

	return grants, nil
}

// grantPrivileges grants privileges for a single grant option.
func (a *Adapter) grantPrivileges(ctx context.Context, grantee string, opt types.GrantOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var queries []string

	// Handle database-level grants when no schema/table/sequence/function is specified
	if opt.Schema == "" && len(opt.Tables) == 0 && len(opt.Sequences) == 0 && len(opt.Functions) == 0 && len(opt.Privileges) > 0 {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnDatabase(database).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, buildErr := b.Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build grant query: %w", buildErr)
		}
		// Database-level grants are executed on the default database
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to grant database privileges: %w", err)
		}
		return nil
	}

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
			b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnAllTablesInSchema(opt.Schema).To(grantee)
			if opt.WithGrantOption {
				b.WithGrantOption()
			}
			q, buildErr := b.Build()
			if buildErr != nil {
				return fmt.Errorf("failed to build grant query: %w", buildErr)
			}
			queries = append(queries, q)
		}

		// Schema usage grant
		q, buildErr := sqlbuilder.NewPg().Grant("USAGE").OnSchema(opt.Schema).To(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build schema usage grant: %w", buildErr)
		}
		queries = append(queries, q)
	}

	// Specific tables
	for _, table := range opt.Tables {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnTable(opt.Schema, table).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, buildErr := b.Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build grant query: %w", buildErr)
		}
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnSequence(opt.Schema, seq).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, buildErr := b.Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build grant query: %w", buildErr)
		}
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnFunction(opt.Schema, fn).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, buildErr := b.Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build grant query: %w", buildErr)
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
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var queries []string

	// Handle database-level revokes when no schema/table/sequence/function is specified
	if opt.Schema == "" && len(opt.Tables) == 0 && len(opt.Sequences) == 0 && len(opt.Functions) == 0 && len(opt.Privileges) > 0 {
		q, buildErr := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnDatabase(database).From(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build revoke query: %w", buildErr)
		}
		// Database-level revokes are executed on the default database
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("failed to revoke database privileges: %w", err)
		}
		return nil
	}

	// Handle ALL TABLES IN SCHEMA
	if opt.Schema != "" && len(opt.Tables) == 0 {
		q, buildErr := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnAllTablesInSchema(opt.Schema).From(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build revoke query: %w", buildErr)
		}
		queries = append(queries, q)
	}

	// Specific tables
	for _, table := range opt.Tables {
		q, buildErr := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnTable(opt.Schema, table).From(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build revoke query: %w", buildErr)
		}
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		q, buildErr := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnSequence(opt.Schema, seq).From(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build revoke query: %w", buildErr)
		}
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		q, buildErr := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnFunction(opt.Schema, fn).From(grantee).Build()
		if buildErr != nil {
			return fmt.Errorf("failed to build revoke query: %w", buildErr)
		}
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

	b := sqlbuilder.NewPg().AlterDefaultPrivileges(opt.GrantedBy, opt.Schema).
		Grant(opt.Privileges...).To(grantee)

	switch strings.ToLower(opt.ObjectType) {
	case "tables":
		b.OnTables()
	case "sequences":
		b.OnSequences()
	case "functions":
		b.OnFunctions()
	case "types":
		b.OnTypes()
	case "schemas":
		b.OnSchemas()
	default:
		return fmt.Errorf("unsupported object type: %s", opt.ObjectType)
	}

	q, err := b.Build()
	if err != nil {
		return fmt.Errorf("failed to build alter default privileges: %w", err)
	}

	return a.execWithNewConnection(ctx, database, q)
}
