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

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// Grant grants privileges to a user or role
func (a *Adapter) Grant(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	for _, opt := range opts {
		if err := a.grantPrivileges(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// Revoke revokes privileges from a user or role
func (a *Adapter) Revoke(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	for _, opt := range opts {
		if err := a.revokePrivileges(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// GrantRole grants role membership to a user or role
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

// RevokeRole revokes role membership from a user or role
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

// SetDefaultPrivileges sets default privileges for new objects
func (a *Adapter) SetDefaultPrivileges(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
	for _, opt := range opts {
		if err := a.setDefaultPrivilege(ctx, grantee, opt); err != nil {
			return err
		}
	}
	return nil
}

// GetGrants retrieves grants for a user or role
func (a *Adapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var grants []types.GrantInfo

	// Get database-level grants
	rows, err := pool.Query(ctx, `
		SELECT d.datname, array_agg(DISTINCT p.privilege_type)
		FROM pg_database d
		CROSS JOIN LATERAL (
			SELECT privilege_type
			FROM aclexplode(d.datacl) acl
			JOIN pg_roles r ON acl.grantee = r.oid
			WHERE r.rolname = $1
		) p
		WHERE p.privilege_type IS NOT NULL
		GROUP BY d.datname`,
		grantee)
	if err != nil {
		return nil, fmt.Errorf("failed to get database grants: %w", err)
	}

	for rows.Next() {
		var grant types.GrantInfo
		var privileges []string
		if err := rows.Scan(&grant.Database, &privileges); err != nil {
			rows.Close()
			return nil, err
		}
		grant.Grantee = grantee
		grant.ObjectType = "database"
		grant.ObjectName = grant.Database
		grant.Privileges = privileges
		grants = append(grants, grant)
	}
	rows.Close()

	return grants, nil
}

// grantPrivileges grants privileges for a single grant option
func (a *Adapter) grantPrivileges(ctx context.Context, grantee string, opt types.GrantOptions) error {
	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var queries []string

	// Handle ALL TABLES IN SCHEMA
	if opt.Schema != "" && len(opt.Tables) == 0 && len(opt.Sequences) == 0 && len(opt.Functions) == 0 {
		// Check if privileges include table privileges
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
			q, err := b.Build()
			if err != nil {
				return fmt.Errorf("failed to build grant query: %w", err)
			}
			queries = append(queries, q)
		}

		// Schema usage grant
		q, err := sqlbuilder.NewPg().Grant("USAGE").OnSchema(opt.Schema).To(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build schema usage grant: %w", err)
		}
		queries = append(queries, q)
	}

	// Specific tables
	for _, table := range opt.Tables {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnTable(opt.Schema, table).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, err := b.Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnSequence(opt.Schema, seq).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, err := b.Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
		}
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		b := sqlbuilder.NewPg().Grant(opt.Privileges...).OnFunction(opt.Schema, fn).To(grantee)
		if opt.WithGrantOption {
			b.WithGrantOption()
		}
		q, err := b.Build()
		if err != nil {
			return fmt.Errorf("failed to build grant query: %w", err)
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

// revokePrivileges revokes privileges for a single grant option
func (a *Adapter) revokePrivileges(ctx context.Context, grantee string, opt types.GrantOptions) error {
	database := opt.Database
	if database == "" {
		database = a.config.Database
	}

	var queries []string

	// Handle ALL TABLES IN SCHEMA
	if opt.Schema != "" && len(opt.Tables) == 0 {
		q, err := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnAllTablesInSchema(opt.Schema).From(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build revoke query: %w", err)
		}
		queries = append(queries, q)
	}

	// Specific tables
	for _, table := range opt.Tables {
		q, err := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnTable(opt.Schema, table).From(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build revoke query: %w", err)
		}
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		q, err := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnSequence(opt.Schema, seq).From(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build revoke query: %w", err)
		}
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		q, err := sqlbuilder.NewPg().Revoke(opt.Privileges...).OnFunction(opt.Schema, fn).From(grantee).Build()
		if err != nil {
			return fmt.Errorf("failed to build revoke query: %w", err)
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

// setDefaultPrivilege sets a single default privilege
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
