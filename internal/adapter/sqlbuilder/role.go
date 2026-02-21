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

// RoleBuilder builds CREATE ROLE, ALTER ROLE, DROP ROLE, CREATE USER, ALTER USER,
// and DROP USER statements for PostgreSQL, CockroachDB, and MySQL.
type RoleBuilder struct {
	dialect Dialect
	action  string // "CREATE ROLE", "ALTER ROLE", "DROP ROLE", "CREATE USER", "DROP USER", "ALTER USER"
	name    string

	// Role/user options (PostgreSQL/CockroachDB)
	roleOpts []string

	// Password (PG)
	password string

	// IN ROLE list (PG)
	inRoles []string

	// SET key = value (PG ALTER ROLE SET)
	setParam string
	setValue string

	// MySQL user-specific
	host       string // MySQL user@host
	authPlugin string

	// DROP options
	ifExists bool
}

// --- PG constructors ---

// PgCreateRole starts a CREATE ROLE statement for PostgreSQL.
func PgCreateRole(name string) *RoleBuilder {
	return &RoleBuilder{dialect: PgDialect{}, action: "CREATE ROLE", name: name}
}

// PgAlterRole starts an ALTER ROLE statement for PostgreSQL.
func PgAlterRole(name string) *RoleBuilder {
	return &RoleBuilder{dialect: PgDialect{}, action: "ALTER ROLE", name: name}
}

// PgDropRole starts a DROP ROLE statement for PostgreSQL.
func PgDropRole(name string) *RoleBuilder {
	return &RoleBuilder{dialect: PgDialect{}, action: "DROP ROLE", name: name}
}

// --- MySQL constructors ---

// MySQLCreateRole starts a CREATE ROLE statement for MySQL.
func MySQLCreateRole(name string) *RoleBuilder {
	return &RoleBuilder{dialect: MySQLDialect{}, action: "CREATE ROLE", name: name}
}

// MySQLDropRole starts a DROP ROLE statement for MySQL.
func MySQLDropRole(name string) *RoleBuilder {
	return &RoleBuilder{dialect: MySQLDialect{}, action: "DROP ROLE", name: name}
}

// MySQLCreateUser starts a CREATE USER statement for MySQL.
func MySQLCreateUser(name, host string) *RoleBuilder {
	return &RoleBuilder{dialect: MySQLDialect{}, action: "CREATE USER", name: name, host: host}
}

// MySQLAlterUser starts an ALTER USER statement for MySQL.
func MySQLAlterUser(name, host string) *RoleBuilder {
	return &RoleBuilder{dialect: MySQLDialect{}, action: "ALTER USER", name: name, host: host}
}

// MySQLDropUser starts a DROP USER statement for MySQL.
func MySQLDropUser(name, host string) *RoleBuilder {
	return &RoleBuilder{dialect: MySQLDialect{}, action: "DROP USER", name: name, host: host}
}

// --- Option setters ---

// Login adds LOGIN or NOLOGIN.
func (b *RoleBuilder) Login(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "LOGIN")
	} else {
		b.roleOpts = append(b.roleOpts, "NOLOGIN")
	}
	return b
}

// Superuser adds SUPERUSER or NOSUPERUSER.
func (b *RoleBuilder) Superuser(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "SUPERUSER")
	} else {
		b.roleOpts = append(b.roleOpts, "NOSUPERUSER")
	}
	return b
}

// CreateDB adds CREATEDB or NOCREATEDB.
func (b *RoleBuilder) CreateDB(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "CREATEDB")
	} else {
		b.roleOpts = append(b.roleOpts, "NOCREATEDB")
	}
	return b
}

// CreateRoleOpt adds CREATEROLE or NOCREATEROLE.
func (b *RoleBuilder) CreateRoleOpt(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "CREATEROLE")
	} else {
		b.roleOpts = append(b.roleOpts, "NOCREATEROLE")
	}
	return b
}

// Inherit adds INHERIT or NOINHERIT.
func (b *RoleBuilder) Inherit(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "INHERIT")
	} else {
		b.roleOpts = append(b.roleOpts, "NOINHERIT")
	}
	return b
}

// Replication adds REPLICATION or NOREPLICATION.
func (b *RoleBuilder) Replication(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "REPLICATION")
	} else {
		b.roleOpts = append(b.roleOpts, "NOREPLICATION")
	}
	return b
}

// BypassRLS adds BYPASSRLS or NOBYPASSRLS.
func (b *RoleBuilder) BypassRLS(v bool) *RoleBuilder {
	if v {
		b.roleOpts = append(b.roleOpts, "BYPASSRLS")
	} else {
		b.roleOpts = append(b.roleOpts, "NOBYPASSRLS")
	}
	return b
}

// WithPassword adds PASSWORD '<password>'.
func (b *RoleBuilder) WithPassword(password string) *RoleBuilder {
	b.password = password
	return b
}

// ConnectionLimit adds CONNECTION LIMIT <n>.
func (b *RoleBuilder) ConnectionLimit(n int) *RoleBuilder {
	b.roleOpts = append(b.roleOpts, fmt.Sprintf("CONNECTION LIMIT %d", n))
	return b
}

// ValidUntil adds VALID UNTIL '<timestamp>'.
func (b *RoleBuilder) ValidUntil(ts string) *RoleBuilder {
	b.roleOpts = append(b.roleOpts, fmt.Sprintf("VALID UNTIL %s", b.dialect.EscapeLiteral(ts)))
	return b
}

// InRoles adds IN ROLE <role1>, <role2>, ...
func (b *RoleBuilder) InRoles(roles ...string) *RoleBuilder {
	b.inRoles = append(b.inRoles, roles...)
	return b
}

// Set starts an ALTER ROLE ... SET <param> = <value>.
func (b *RoleBuilder) Set(param, value string) *RoleBuilder {
	b.setParam = param
	b.setValue = value
	return b
}

// IdentifiedBy adds IDENTIFIED BY '<password>' (MySQL).
func (b *RoleBuilder) IdentifiedBy(password string) *RoleBuilder {
	b.password = password
	return b
}

// AuthPlugin sets WITH <plugin> (MySQL).
func (b *RoleBuilder) AuthPlugin(plugin string) *RoleBuilder {
	b.authPlugin = plugin
	return b
}

// IfExists adds IF EXISTS (for DROP statements).
func (b *RoleBuilder) IfExists() *RoleBuilder {
	b.ifExists = true
	return b
}

// AccountLock adds ACCOUNT LOCK (MySQL).
func (b *RoleBuilder) AccountLock() *RoleBuilder {
	b.roleOpts = append(b.roleOpts, "ACCOUNT LOCK")
	return b
}

// AccountUnlock adds ACCOUNT UNLOCK (MySQL).
func (b *RoleBuilder) AccountUnlock() *RoleBuilder {
	b.roleOpts = append(b.roleOpts, "ACCOUNT UNLOCK")
	return b
}

// RequireSSL adds REQUIRE SSL (MySQL).
func (b *RoleBuilder) RequireSSL() *RoleBuilder {
	b.roleOpts = append(b.roleOpts, "REQUIRE SSL")
	return b
}

// RequireX509 adds REQUIRE X509 (MySQL).
func (b *RoleBuilder) RequireX509() *RoleBuilder {
	b.roleOpts = append(b.roleOpts, "REQUIRE X509")
	return b
}

// RequireNone adds REQUIRE NONE (MySQL).
func (b *RoleBuilder) RequireNone() *RoleBuilder {
	b.roleOpts = append(b.roleOpts, "REQUIRE NONE")
	return b
}

// ResourceOption adds a WITH resource option (MySQL ALTER USER).
func (b *RoleBuilder) ResourceOption(opt string) *RoleBuilder {
	b.roleOpts = append(b.roleOpts, opt)
	return b
}

// Build assembles and returns the SQL string.
func (b *RoleBuilder) Build() string {
	switch {
	case b.action == "DROP ROLE":
		return b.buildDropRole()
	case b.action == "DROP USER":
		return b.buildDropUser()
	case b.action == "ALTER ROLE" && b.setParam != "":
		return b.buildAlterRoleSet()
	default:
		return b.buildCreateOrAlter()
	}
}

func (b *RoleBuilder) buildCreateOrAlter() string {
	var sb strings.Builder

	switch b.dialect.(type) {
	case MySQLDialect:
		return b.buildMySQL()
	default:
		// PostgreSQL / CockroachDB
		sb.WriteString(b.action)
		sb.WriteByte(' ')
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))

		allOpts := make([]string, 0, len(b.roleOpts)+2)
		if b.password != "" {
			allOpts = append(allOpts, fmt.Sprintf("PASSWORD %s", b.dialect.EscapeLiteral(b.password)))
		}
		allOpts = append(allOpts, b.roleOpts...)
		if len(b.inRoles) > 0 {
			escaped := make([]string, len(b.inRoles))
			for i, r := range b.inRoles {
				escaped[i] = b.dialect.EscapeIdentifier(r)
			}
			allOpts = append(allOpts, fmt.Sprintf("IN ROLE %s", strings.Join(escaped, ", ")))
		}

		if len(allOpts) > 0 {
			sb.WriteString(" WITH ")
			sb.WriteString(strings.Join(allOpts, " "))
		}
	}

	return sb.String()
}

func (b *RoleBuilder) buildMySQL() string {
	var sb strings.Builder

	switch b.action {
	case "CREATE ROLE":
		sb.WriteString("CREATE ROLE IF NOT EXISTS ")
		sb.WriteString(b.dialect.EscapeLiteral(b.name))

	case "CREATE USER":
		sb.WriteString("CREATE USER IF NOT EXISTS ")
		sb.WriteString(b.dialect.EscapeLiteral(b.name))
		sb.WriteByte('@')
		sb.WriteString(b.dialect.EscapeLiteral(b.host))
		if b.password != "" {
			sb.WriteString(" IDENTIFIED")
			if b.authPlugin != "" {
				sb.WriteString(" WITH ")
				sb.WriteString(b.authPlugin) // plugin names are keywords, not identifiers
			}
			sb.WriteString(" BY ")
			sb.WriteString(b.dialect.EscapeLiteral(b.password))
		} else if b.authPlugin != "" {
			sb.WriteString(" IDENTIFIED WITH ")
			sb.WriteString(b.authPlugin)
		}

	case "ALTER USER":
		sb.WriteString("ALTER USER ")
		sb.WriteString(b.dialect.EscapeLiteral(b.name))
		sb.WriteByte('@')
		sb.WriteString(b.dialect.EscapeLiteral(b.host))
		if b.password != "" {
			sb.WriteString(" IDENTIFIED BY ")
			sb.WriteString(b.dialect.EscapeLiteral(b.password))
		}
	}

	if len(b.roleOpts) > 0 {
		sb.WriteByte(' ')
		sb.WriteString(strings.Join(b.roleOpts, " "))
	}

	return sb.String()
}

func (b *RoleBuilder) buildDropRole() string {
	var sb strings.Builder
	sb.WriteString("DROP ROLE ")
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	switch b.dialect.(type) {
	case MySQLDialect:
		sb.WriteString(b.dialect.EscapeLiteral(b.name))
	default:
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))
	}
	return sb.String()
}

func (b *RoleBuilder) buildDropUser() string {
	var sb strings.Builder
	sb.WriteString("DROP USER ")
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	switch b.dialect.(type) {
	case MySQLDialect:
		sb.WriteString(b.dialect.EscapeLiteral(b.name))
		sb.WriteByte('@')
		sb.WriteString(b.dialect.EscapeLiteral(b.host))
	default:
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))
	}
	return sb.String()
}

func (b *RoleBuilder) buildAlterRoleSet() string {
	return fmt.Sprintf("ALTER ROLE %s SET %s = %s",
		b.dialect.EscapeIdentifier(b.name),
		b.dialect.EscapeIdentifier(b.setParam),
		b.dialect.EscapeLiteral(b.setValue))
}
