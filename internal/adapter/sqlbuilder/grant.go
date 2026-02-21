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

package sqlbuilder

import (
	"fmt"
	"strings"
)

// grantAction distinguishes GRANT from REVOKE.
type grantAction int

const (
	actionGrant grantAction = iota
	actionRevoke
	actionGrantRole
	actionRevokeRole
	actionAlterDefaultGrant
)

// GrantBuilder builds GRANT, REVOKE, GRANT role, and ALTER DEFAULT PRIVILEGES statements.
type GrantBuilder struct {
	dialect Dialect
	action  grantAction

	// privileges (validated against allowlist on Build)
	privileges []string

	// object target
	target string // fully-built ON ... clause

	// grantee / from
	grantee string

	// role grant/revoke
	role string

	// options
	withGrantOption bool

	// ALTER DEFAULT PRIVILEGES fields
	adpForRole string
	adpSchema  string
	adpObjType string // TABLES, SEQUENCES, etc.

	err error // sticky first error
}

// --- Constructors --------------------------------------------------------

// NewPg returns a fresh GrantBuilder for PostgreSQL / CockroachDB.
func NewPg() *GrantBuilder {
	return &GrantBuilder{dialect: PgDialect{}}
}

// NewMySQL returns a fresh GrantBuilder for MySQL.
func NewMySQL() *GrantBuilder {
	return &GrantBuilder{dialect: MySQLDialect{}}
}

// --- Action setters ------------------------------------------------------

// Grant starts a GRANT <privileges> statement.
// If called after AlterDefaultPrivileges, it sets the privileges
// without changing the action type.
func (b *GrantBuilder) Grant(privs ...string) *GrantBuilder {
	if b.action != actionAlterDefaultGrant {
		b.action = actionGrant
	}
	b.privileges = privs
	return b
}

// Revoke starts a REVOKE <privileges> statement.
func (b *GrantBuilder) Revoke(privs ...string) *GrantBuilder {
	b.action = actionRevoke
	b.privileges = privs
	return b
}

// GrantRole starts a GRANT <role> TO <grantee> statement.
func (b *GrantBuilder) GrantRole(role string) *GrantBuilder {
	b.action = actionGrantRole
	b.role = role
	return b
}

// RevokeRole starts a REVOKE <role> FROM <grantee> statement.
func (b *GrantBuilder) RevokeRole(role string) *GrantBuilder {
	b.action = actionRevokeRole
	b.role = role
	return b
}

// AlterDefaultPrivileges starts an ALTER DEFAULT PRIVILEGES statement.
func (b *GrantBuilder) AlterDefaultPrivileges(forRole, schema string) *GrantBuilder {
	b.action = actionAlterDefaultGrant
	b.adpForRole = forRole
	b.adpSchema = schema
	return b
}

// --- Object target setters -----------------------------------------------

// OnDatabase targets a database.
// PG: ON DATABASE "db"  |  MySQL: ON `db`.*
func (b *GrantBuilder) OnDatabase(db string) *GrantBuilder {
	switch b.dialect.(type) {
	case MySQLDialect:
		b.target = fmt.Sprintf("ON %s.*", b.dialect.EscapeIdentifier(db))
	default:
		b.target = fmt.Sprintf("ON DATABASE %s", b.dialect.EscapeIdentifier(db))
	}
	return b
}

// OnSchema targets a schema.
func (b *GrantBuilder) OnSchema(schema string) *GrantBuilder {
	b.target = fmt.Sprintf("ON SCHEMA %s", b.dialect.EscapeIdentifier(schema))
	return b
}

// OnTable targets a specific table.
func (b *GrantBuilder) OnTable(schema, table string) *GrantBuilder {
	if schema != "" {
		b.target = fmt.Sprintf("ON TABLE %s.%s",
			b.dialect.EscapeIdentifier(schema),
			b.dialect.EscapeIdentifier(table))
	} else {
		b.target = fmt.Sprintf("ON TABLE %s", b.dialect.EscapeIdentifier(table))
	}
	return b
}

// OnSequence targets a specific sequence.
func (b *GrantBuilder) OnSequence(schema, seq string) *GrantBuilder {
	if schema != "" {
		b.target = fmt.Sprintf("ON SEQUENCE %s.%s",
			b.dialect.EscapeIdentifier(schema),
			b.dialect.EscapeIdentifier(seq))
	} else {
		b.target = fmt.Sprintf("ON SEQUENCE %s", b.dialect.EscapeIdentifier(seq))
	}
	return b
}

// OnFunction targets a specific function (fixes the unescaped-fn-name bug).
func (b *GrantBuilder) OnFunction(schema, fn string) *GrantBuilder {
	if schema != "" {
		b.target = fmt.Sprintf("ON FUNCTION %s.%s",
			b.dialect.EscapeIdentifier(schema),
			b.dialect.EscapeIdentifier(fn))
	} else {
		b.target = fmt.Sprintf("ON FUNCTION %s", b.dialect.EscapeIdentifier(fn))
	}
	return b
}

// OnAllTablesInSchema targets all tables in a schema.
func (b *GrantBuilder) OnAllTablesInSchema(schema string) *GrantBuilder {
	b.target = fmt.Sprintf("ON ALL TABLES IN SCHEMA %s", b.dialect.EscapeIdentifier(schema))
	return b
}

// OnAllSequencesInSchema targets all sequences in a schema.
func (b *GrantBuilder) OnAllSequencesInSchema(schema string) *GrantBuilder {
	b.target = fmt.Sprintf("ON ALL SEQUENCES IN SCHEMA %s", b.dialect.EscapeIdentifier(schema))
	return b
}

// OnGlobal targets *.* (MySQL only).
func (b *GrantBuilder) OnGlobal() *GrantBuilder {
	b.target = "ON *.*"
	return b
}

// OnMySQLTable targets `db`.`table` (MySQL).
func (b *GrantBuilder) OnMySQLTable(db, table string) *GrantBuilder {
	b.target = fmt.Sprintf("ON %s.%s",
		b.dialect.EscapeIdentifier(db),
		b.dialect.EscapeIdentifier(table))
	return b
}

// OnProcedure targets PROCEDURE `db`.`proc` (MySQL).
func (b *GrantBuilder) OnProcedure(db, proc string) *GrantBuilder {
	b.target = fmt.Sprintf("ON PROCEDURE %s.%s",
		b.dialect.EscapeIdentifier(db),
		b.dialect.EscapeIdentifier(proc))
	return b
}

// OnMySQLFunction targets FUNCTION `db`.`fn` (MySQL).
func (b *GrantBuilder) OnMySQLFunction(db, fn string) *GrantBuilder {
	b.target = fmt.Sprintf("ON FUNCTION %s.%s",
		b.dialect.EscapeIdentifier(db),
		b.dialect.EscapeIdentifier(fn))
	return b
}

// OnTables is for ALTER DEFAULT PRIVILEGES ... ON TABLES.
func (b *GrantBuilder) OnTables() *GrantBuilder {
	b.adpObjType = "TABLES"
	return b
}

// OnSequences is for ALTER DEFAULT PRIVILEGES ... ON SEQUENCES.
func (b *GrantBuilder) OnSequences() *GrantBuilder {
	b.adpObjType = "SEQUENCES"
	return b
}

// OnFunctions is for ALTER DEFAULT PRIVILEGES ... ON FUNCTIONS.
func (b *GrantBuilder) OnFunctions() *GrantBuilder {
	b.adpObjType = "FUNCTIONS"
	return b
}

// OnTypes is for ALTER DEFAULT PRIVILEGES ... ON TYPES.
func (b *GrantBuilder) OnTypes() *GrantBuilder {
	b.adpObjType = "TYPES"
	return b
}

// OnSchemas is for ALTER DEFAULT PRIVILEGES ... ON SCHEMAS.
func (b *GrantBuilder) OnSchemas() *GrantBuilder {
	b.adpObjType = "SCHEMAS"
	return b
}

// --- Grantee setters -----------------------------------------------------

// To sets the grantee (PG identifier escaping).
func (b *GrantBuilder) To(grantee string) *GrantBuilder {
	b.grantee = b.dialect.EscapeIdentifier(grantee)
	return b
}

// From sets the revokee (PG identifier escaping).
func (b *GrantBuilder) From(grantee string) *GrantBuilder {
	b.grantee = b.dialect.EscapeIdentifier(grantee)
	return b
}

// ToUser sets the grantee with user@host format (MySQL literal escaping).
func (b *GrantBuilder) ToUser(user, host string) *GrantBuilder {
	b.grantee = fmt.Sprintf("%s@%s",
		b.dialect.EscapeLiteral(user),
		b.dialect.EscapeLiteral(host))
	return b
}

// FromUser sets the revokee with user@host format (MySQL literal escaping).
func (b *GrantBuilder) FromUser(user, host string) *GrantBuilder {
	b.grantee = fmt.Sprintf("%s@%s",
		b.dialect.EscapeLiteral(user),
		b.dialect.EscapeLiteral(host))
	return b
}

// ToLiteral sets the grantee using literal escaping (for MySQL role grants to users).
func (b *GrantBuilder) ToLiteral(grantee string) *GrantBuilder {
	b.grantee = b.dialect.EscapeLiteral(grantee)
	return b
}

// FromLiteral sets the revokee using literal escaping (for MySQL role revokes from users).
func (b *GrantBuilder) FromLiteral(grantee string) *GrantBuilder {
	b.grantee = b.dialect.EscapeLiteral(grantee)
	return b
}

// --- Options -------------------------------------------------------------

// WithGrantOption adds WITH GRANT OPTION to the statement.
func (b *GrantBuilder) WithGrantOption() *GrantBuilder {
	b.withGrantOption = true
	return b
}

// --- Build ---------------------------------------------------------------

// Build assembles and returns the SQL statement.
// Returns an error if privilege validation fails or required fields are missing.
func (b *GrantBuilder) Build() (string, error) {
	if b.err != nil {
		return "", b.err
	}

	switch b.action {
	case actionGrant:
		return b.buildGrantRevoke("GRANT", "TO")
	case actionRevoke:
		return b.buildGrantRevoke("REVOKE", "FROM")
	case actionGrantRole:
		return b.buildRoleGrant("GRANT", "TO")
	case actionRevokeRole:
		return b.buildRoleGrant("REVOKE", "FROM")
	case actionAlterDefaultGrant:
		return b.buildAlterDefaultPrivileges()
	default:
		return "", fmt.Errorf("sqlbuilder: no action specified")
	}
}

func (b *GrantBuilder) buildGrantRevoke(verb, preposition string) (string, error) {
	if err := ValidatePrivileges(b.privileges, b.dialect.ValidPrivileges()); err != nil {
		return "", err
	}
	if b.target == "" {
		return "", fmt.Errorf("sqlbuilder: no target object specified")
	}
	if b.grantee == "" {
		return "", fmt.Errorf("sqlbuilder: no grantee specified")
	}

	privList := normalizePrivileges(b.privileges)
	var sb strings.Builder
	sb.WriteString(verb)
	sb.WriteByte(' ')
	sb.WriteString(strings.Join(privList, ", "))
	sb.WriteByte(' ')
	sb.WriteString(b.target)
	sb.WriteByte(' ')
	sb.WriteString(preposition)
	sb.WriteByte(' ')
	sb.WriteString(b.grantee)
	if b.withGrantOption && verb == "GRANT" {
		sb.WriteString(" WITH GRANT OPTION")
	}
	return sb.String(), nil
}

func (b *GrantBuilder) buildRoleGrant(verb, preposition string) (string, error) {
	if b.role == "" {
		return "", fmt.Errorf("sqlbuilder: no role specified")
	}
	if b.grantee == "" {
		return "", fmt.Errorf("sqlbuilder: no grantee specified")
	}
	return fmt.Sprintf("%s %s %s %s",
		verb,
		b.dialect.EscapeIdentifier(b.role),
		preposition,
		b.grantee), nil
}

func (b *GrantBuilder) buildAlterDefaultPrivileges() (string, error) {
	if err := ValidatePrivileges(b.privileges, b.dialect.ValidPrivileges()); err != nil {
		return "", err
	}
	if b.adpObjType == "" {
		return "", fmt.Errorf("sqlbuilder: no object type specified for ALTER DEFAULT PRIVILEGES")
	}
	if b.grantee == "" {
		return "", fmt.Errorf("sqlbuilder: no grantee specified")
	}

	privList := normalizePrivileges(b.privileges)
	var sb strings.Builder
	sb.WriteString("ALTER DEFAULT PRIVILEGES")
	if b.adpForRole != "" {
		sb.WriteString(" FOR ROLE ")
		sb.WriteString(b.dialect.EscapeIdentifier(b.adpForRole))
	}
	if b.adpSchema != "" {
		sb.WriteString(" IN SCHEMA ")
		sb.WriteString(b.dialect.EscapeIdentifier(b.adpSchema))
	}
	sb.WriteString(" GRANT ")
	sb.WriteString(strings.Join(privList, ", "))
	sb.WriteString(" ON ")
	sb.WriteString(b.adpObjType)
	sb.WriteString(" TO ")
	sb.WriteString(b.grantee)
	return sb.String(), nil
}

// normalizePrivileges upper-cases and trims privileges.
func normalizePrivileges(privs []string) []string {
	out := make([]string, len(privs))
	for i, p := range privs {
		out[i] = strings.ToUpper(strings.TrimSpace(p))
	}
	return out
}
