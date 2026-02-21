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

// DatabaseBuilder builds CREATE DATABASE, ALTER DATABASE, and DROP DATABASE statements.
type DatabaseBuilder struct {
	dialect Dialect

	action string // "CREATE", "ALTER", "DROP"
	name   string

	// CREATE/ALTER options
	owner           string
	encoding        string
	template        string
	lcCollate       string
	lcCtype         string
	tablespace      string
	connectionLimit int
	isTemplate      bool

	// MySQL-specific
	charset   string
	collation string

	// DROP options
	ifExists bool
	cascade  bool
}

// --- PG constructors ---

// PgCreateDatabase starts a CREATE DATABASE statement for PostgreSQL.
func PgCreateDatabase(name string) *DatabaseBuilder {
	return &DatabaseBuilder{dialect: PgDialect{}, action: "CREATE", name: name}
}

// PgDropDatabase starts a DROP DATABASE statement for PostgreSQL.
func PgDropDatabase(name string) *DatabaseBuilder {
	return &DatabaseBuilder{dialect: PgDialect{}, action: "DROP", name: name}
}

// --- MySQL constructors ---

// MySQLCreateDatabase starts a CREATE DATABASE statement for MySQL.
func MySQLCreateDatabase(name string) *DatabaseBuilder {
	return &DatabaseBuilder{dialect: MySQLDialect{}, action: "CREATE", name: name}
}

// MySQLAlterDatabase starts an ALTER DATABASE statement for MySQL.
func MySQLAlterDatabase(name string) *DatabaseBuilder {
	return &DatabaseBuilder{dialect: MySQLDialect{}, action: "ALTER", name: name}
}

// MySQLDropDatabase starts a DROP DATABASE statement for MySQL.
func MySQLDropDatabase(name string) *DatabaseBuilder {
	return &DatabaseBuilder{dialect: MySQLDialect{}, action: "DROP", name: name}
}

// --- Option setters ---

func (b *DatabaseBuilder) Owner(owner string) *DatabaseBuilder    { b.owner = owner; return b }
func (b *DatabaseBuilder) Encoding(enc string) *DatabaseBuilder   { b.encoding = enc; return b }
func (b *DatabaseBuilder) Template(tpl string) *DatabaseBuilder   { b.template = tpl; return b }
func (b *DatabaseBuilder) LCCollate(lc string) *DatabaseBuilder   { b.lcCollate = lc; return b }
func (b *DatabaseBuilder) LCCtype(lc string) *DatabaseBuilder     { b.lcCtype = lc; return b }
func (b *DatabaseBuilder) Tablespace(ts string) *DatabaseBuilder  { b.tablespace = ts; return b }
func (b *DatabaseBuilder) ConnectionLimit(n int) *DatabaseBuilder { b.connectionLimit = n; return b }
func (b *DatabaseBuilder) IsTemplate(v bool) *DatabaseBuilder     { b.isTemplate = v; return b }
func (b *DatabaseBuilder) Charset(cs string) *DatabaseBuilder     { b.charset = cs; return b }
func (b *DatabaseBuilder) Collation(col string) *DatabaseBuilder  { b.collation = col; return b }
func (b *DatabaseBuilder) IfExists() *DatabaseBuilder             { b.ifExists = true; return b }
func (b *DatabaseBuilder) Cascade() *DatabaseBuilder              { b.cascade = true; return b }

// Build assembles and returns the SQL string.
func (b *DatabaseBuilder) Build() string {
	switch b.action {
	case "CREATE":
		return b.buildCreate()
	case "ALTER":
		return b.buildAlter()
	case "DROP":
		return b.buildDrop()
	default:
		return ""
	}
}

func (b *DatabaseBuilder) buildCreate() string {
	var sb strings.Builder

	switch b.dialect.(type) {
	case MySQLDialect:
		sb.WriteString("CREATE DATABASE IF NOT EXISTS ")
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))
		if b.charset != "" {
			sb.WriteString(" CHARACTER SET ")
			sb.WriteString(b.dialect.EscapeIdentifier(b.charset))
		}
		if b.collation != "" {
			sb.WriteString(" COLLATE ")
			sb.WriteString(b.dialect.EscapeIdentifier(b.collation))
		}
	default: // PG
		sb.WriteString("CREATE DATABASE ")
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))
		var opts []string
		if b.owner != "" {
			opts = append(opts, fmt.Sprintf("OWNER = %s", b.dialect.EscapeIdentifier(b.owner)))
		}
		if b.template != "" {
			opts = append(opts, fmt.Sprintf("TEMPLATE = %s", b.dialect.EscapeIdentifier(b.template)))
		}
		if b.encoding != "" {
			opts = append(opts, fmt.Sprintf("ENCODING = %s", b.dialect.EscapeLiteral(b.encoding)))
		}
		if b.lcCollate != "" {
			opts = append(opts, fmt.Sprintf("LC_COLLATE = %s", b.dialect.EscapeLiteral(b.lcCollate)))
		}
		if b.lcCtype != "" {
			opts = append(opts, fmt.Sprintf("LC_CTYPE = %s", b.dialect.EscapeLiteral(b.lcCtype)))
		}
		if b.tablespace != "" {
			opts = append(opts, fmt.Sprintf("TABLESPACE = %s", b.dialect.EscapeIdentifier(b.tablespace)))
		}
		if b.connectionLimit != 0 {
			opts = append(opts, fmt.Sprintf("CONNECTION LIMIT = %d", b.connectionLimit))
		}
		if b.isTemplate {
			opts = append(opts, "IS_TEMPLATE = TRUE")
		}
		if len(opts) > 0 {
			sb.WriteString(" WITH ")
			sb.WriteString(strings.Join(opts, " "))
		}
	}

	return sb.String()
}

func (b *DatabaseBuilder) buildAlter() string {
	var sb strings.Builder

	switch b.dialect.(type) {
	case MySQLDialect:
		sb.WriteString("ALTER DATABASE ")
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))
		if b.charset != "" {
			sb.WriteString(" CHARACTER SET ")
			sb.WriteString(b.dialect.EscapeIdentifier(b.charset))
		}
		if b.collation != "" {
			sb.WriteString(" COLLATE ")
			sb.WriteString(b.dialect.EscapeIdentifier(b.collation))
		}
	default:
		sb.WriteString("ALTER DATABASE ")
		sb.WriteString(b.dialect.EscapeIdentifier(b.name))
		if b.owner != "" {
			sb.WriteString(" OWNER TO ")
			sb.WriteString(b.dialect.EscapeIdentifier(b.owner))
		}
	}

	return sb.String()
}

func (b *DatabaseBuilder) buildDrop() string {
	var sb strings.Builder
	sb.WriteString("DROP DATABASE ")
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	sb.WriteString(b.dialect.EscapeIdentifier(b.name))
	if b.cascade {
		sb.WriteString(" CASCADE")
	}
	return sb.String()
}
