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

import "strings"

// Dialect abstracts database-specific SQL escaping and privilege validation.
type Dialect interface {
	EscapeIdentifier(s string) string
	EscapeLiteral(s string) string
	ValidPrivileges() map[string]bool
}

// PgDialect implements Dialect for PostgreSQL and CockroachDB.
type PgDialect struct{}

// EscapeIdentifier wraps s in double quotes, doubling any embedded quotes.
func (PgDialect) EscapeIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// EscapeLiteral wraps s in single quotes, doubling any embedded quotes.
func (PgDialect) EscapeLiteral(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `''`) + `'`
}

// ValidPrivileges returns the set of valid PostgreSQL privileges.
func (PgDialect) ValidPrivileges() map[string]bool {
	return ValidPostgresPrivileges
}

// MySQLDialect implements Dialect for MySQL.
type MySQLDialect struct{}

// EscapeIdentifier wraps s in backticks, doubling any embedded backticks.
func (MySQLDialect) EscapeIdentifier(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// EscapeLiteral wraps s in single quotes, escaping embedded quotes and backslashes.
func (MySQLDialect) EscapeLiteral(s string) string {
	escaped := strings.ReplaceAll(s, `'`, `''`)
	escaped = strings.ReplaceAll(escaped, `\`, `\\`)
	return `'` + escaped + `'`
}

// ValidPrivileges returns the set of valid MySQL privileges.
func (MySQLDialect) ValidPrivileges() map[string]bool {
	return ValidMySQLPrivileges
}
