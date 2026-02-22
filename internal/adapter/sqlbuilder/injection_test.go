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
	"strings"
	"testing"
)

// injectionPayloads is the corpus of common SQL injection attack strings.
var injectionPayloads = []string{
	`'; DROP TABLE users; --`,
	`"; DROP TABLE users; --`,
	"` ; DROP TABLE users; --",
	`Robert'); DROP TABLE students;--`,
	`\'; DROP TABLE users; --`,
	"name\x00injection",      // null byte
	"name\u200Binvisible",    // zero-width space
	"name\u202Ertl_override", // RTL override
	`UNION SELECT * FROM passwords`,
	"multi\nline\ninjection", // newline
	`/* comment */ DROP TABLE x`,
}

// TestInjection_IdentifierPositions tests that injection payloads in every
// identifier-escaped position produce properly delimited output.
func TestInjection_IdentifierPositions(t *testing.T) {
	dialects := []struct {
		name  string
		d     Dialect
		open  byte
		close byte
	}{
		{"PG", PgDialect{}, '"', '"'},
		{"MySQL", MySQLDialect{}, '`', '`'},
	}

	for _, dialect := range dialects {
		t.Run(dialect.name, func(t *testing.T) {
			for _, payload := range injectionPayloads {
				// Direct escape function test
				assertIdentifierDelimited(t, dialect.d, payload, dialect.open, dialect.close)

				switch dialect.d.(type) {
				case PgDialect:
					// PG-specific grant builder positions
					q, err := NewPg().Grant("SELECT").OnTable(payload, "t").To("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for schema %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewPg().Grant("SELECT").OnTable("", payload).To("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for table %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewPg().Grant("EXECUTE").OnFunction("", payload).To("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for function %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewPg().Grant("USAGE").OnSequence("", payload).To("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for sequence %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewPg().Grant("SELECT").OnTable("", "t").To(payload).Build()
					if err != nil {
						t.Fatalf("unexpected error for grantee %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewPg().GrantRole(payload).To("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for role %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					// PG database builder positions
					assertContainsDelimited(t, PgCreateDatabase(payload).Build(), dialect.d.EscapeIdentifier(payload))
					assertContainsDelimited(t, PgCreateDatabase("db").Owner(payload).Build(), dialect.d.EscapeIdentifier(payload))
					assertContainsDelimited(t, PgCreateDatabase("db").Template(payload).Build(), dialect.d.EscapeIdentifier(payload))
					assertContainsDelimited(t, PgCreateDatabase("db").Tablespace(payload).Build(), dialect.d.EscapeIdentifier(payload))

					// PG role builder name
					assertContainsDelimited(t, PgCreateRole(payload).Build(), dialect.d.EscapeIdentifier(payload))

				case MySQLDialect:
					// MySQL-specific grant builder positions
					q, err := NewMySQL().Grant("SELECT").OnDatabase(payload).ToLiteral("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for MySQL db %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewMySQL().Grant("SELECT").OnMySQLTable(payload, "t").ToLiteral("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for MySQL table db %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewMySQL().Grant("SELECT").OnMySQLTable("db", payload).ToLiteral("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for MySQL table %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewMySQL().Grant("EXECUTE").OnMySQLFunction("db", payload).ToLiteral("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for MySQL function %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					q, err = NewMySQL().Grant("EXECUTE").OnProcedure("db", payload).ToLiteral("u").Build()
					if err != nil {
						t.Fatalf("unexpected error for MySQL procedure %q: %v", payload, err)
					}
					assertContainsDelimited(t, q, dialect.d.EscapeIdentifier(payload))

					// MySQL database builder positions
					assertContainsDelimited(t, MySQLCreateDatabase(payload).Build(), dialect.d.EscapeIdentifier(payload))
					assertContainsDelimited(t, MySQLCreateDatabase("db").Charset(payload).Build(), dialect.d.EscapeIdentifier(payload))
					assertContainsDelimited(t, MySQLCreateDatabase("db").Collation(payload).Build(), dialect.d.EscapeIdentifier(payload))
				}
			}
		})
	}
}

// TestInjection_LiteralPositions tests injection payloads in literal-escaped positions.
func TestInjection_LiteralPositions(t *testing.T) {
	for _, payload := range injectionPayloads {
		// Password (PG)
		q := PgCreateRole("u").WithPassword(payload).Build()
		assertContainsDelimited(t, q, PgDialect{}.EscapeLiteral(payload))

		// Encoding
		q = PgCreateDatabase("db").Encoding(payload).Build()
		assertContainsDelimited(t, q, PgDialect{}.EscapeLiteral(payload))

		// LC_COLLATE
		q = PgCreateDatabase("db").LCCollate(payload).Build()
		assertContainsDelimited(t, q, PgDialect{}.EscapeLiteral(payload))

		// LC_CTYPE
		q = PgCreateDatabase("db").LCCtype(payload).Build()
		assertContainsDelimited(t, q, PgDialect{}.EscapeLiteral(payload))

		// ValidUntil
		q = PgCreateRole("u").ValidUntil(payload).Build()
		assertContainsDelimited(t, q, PgDialect{}.EscapeLiteral(payload))

		// MySQL user name (literal escaping via ToUser)
		mysqlQ, err := NewMySQL().Grant("SELECT").OnGlobal().ToUser(payload, "%").Build()
		if err != nil {
			t.Fatalf("unexpected error for MySQL user %q: %v", payload, err)
		}
		assertContainsDelimited(t, mysqlQ, MySQLDialect{}.EscapeLiteral(payload))

		// MySQL host (literal escaping via ToUser)
		mysqlQ, err = NewMySQL().Grant("SELECT").OnGlobal().ToUser("u", payload).Build()
		if err != nil {
			t.Fatalf("unexpected error for MySQL host %q: %v", payload, err)
		}
		assertContainsDelimited(t, mysqlQ, MySQLDialect{}.EscapeLiteral(payload))

		// MySQL password
		q = MySQLCreateUser("u", "%").IdentifiedBy(payload).Build()
		assertContainsDelimited(t, q, MySQLDialect{}.EscapeLiteral(payload))

		// MySQL role name (literal)
		q = MySQLCreateRole(payload).Build()
		assertContainsDelimited(t, q, MySQLDialect{}.EscapeLiteral(payload))
	}
}

// TestInjection_PrivilegeAllowlist tests that injection payloads in privilege
// position are rejected by ValidatePrivileges.
func TestInjection_PrivilegeAllowlist(t *testing.T) {
	badPrivileges := []string{
		"SELECT; DROP TABLE users",
		"SELECT\nDROP TABLE users",
		"ALL PRIVILEGES; --",
		"",
		"   ",
		"SELECT\x00DROP",
		"' OR '1'='1",
		"SELECT/**/DROP",
	}
	for _, priv := range badPrivileges {
		// Test with non-empty slice containing the bad privilege
		err := ValidatePrivileges([]string{priv}, ValidPostgresPrivileges)
		if err == nil {
			t.Errorf("expected error for privilege %q, got nil", priv)
		}
		err = ValidatePrivileges([]string{priv}, ValidMySQLPrivileges)
		if err == nil {
			t.Errorf("expected error for MySQL privilege %q, got nil", priv)
		}
	}

	// Empty slice in non-empty: "SELECT" + bad
	for _, priv := range badPrivileges {
		err := ValidatePrivileges([]string{"SELECT", priv}, ValidPostgresPrivileges)
		if err == nil {
			t.Errorf("expected error for mixed privileges with %q", priv)
		}
	}
}

// TestInjection_QuotingInvariant verifies the structural escaping invariant:
// EscapeIdentifier(s) always starts with the opening delimiter and ends with
// the closing delimiter, and between them all delimiter chars are doubled.
func TestInjection_QuotingInvariant(t *testing.T) {
	edgeCases := append(injectionPayloads,
		"",
		"normal",
		"with spaces",
		strings.Repeat("a", 10000),
		"\x00\x01\x02",
		"'\"` \\\n\t\r",
	)

	t.Run("PgIdentifier", func(t *testing.T) {
		d := PgDialect{}
		for _, s := range edgeCases {
			got := d.EscapeIdentifier(s)
			assertQuotingInvariant(t, got, '"', s)
		}
	})

	t.Run("PgLiteral", func(t *testing.T) {
		d := PgDialect{}
		for _, s := range edgeCases {
			got := d.EscapeLiteral(s)
			assertQuotingInvariant(t, got, '\'', s)
		}
	})

	t.Run("MySQLIdentifier", func(t *testing.T) {
		d := MySQLDialect{}
		for _, s := range edgeCases {
			got := d.EscapeIdentifier(s)
			assertQuotingInvariant(t, got, '`', s)
		}
	})

	t.Run("MySQLLiteral", func(t *testing.T) {
		d := MySQLDialect{}
		for _, s := range edgeCases {
			got := d.EscapeLiteral(s)
			if len(got) < 2 {
				t.Errorf("EscapeLiteral(%q) = %q, too short", s, got)
				continue
			}
			if got[0] != '\'' || got[len(got)-1] != '\'' {
				t.Errorf("EscapeLiteral(%q) = %q, not properly delimited", s, got)
			}
		}
	})
}

// TestInjection_AuthPluginUnescaped documents that authPlugin is written
// without escaping (line 318 in role.go). This is intentional because plugin
// names are MySQL keywords (e.g., caching_sha2_password, mysql_native_password).
func TestInjection_AuthPluginUnescaped(t *testing.T) {
	// Normal usage: plugin name appears unescaped
	q := MySQLCreateUser("u", "%").AuthPlugin("caching_sha2_password").IdentifiedBy("pass").Build()
	if !strings.Contains(q, "WITH caching_sha2_password BY") {
		t.Errorf("expected unescaped plugin name, got %q", q)
	}

	// Document: if someone passes a malicious plugin name, it goes unescaped.
	// This is acceptable because plugin names are NOT user-controlled in practice;
	// they come from the CRD spec's enum validation.
	q = MySQLCreateUser("u", "%").AuthPlugin("evil; DROP TABLE x").IdentifiedBy("pass").Build()
	if !strings.Contains(q, "evil; DROP TABLE x") {
		t.Errorf("expected unescaped (intentional) plugin string, got %q", q)
	}
}

// --- helpers ----------------------------------------------------------------

func assertIdentifierDelimited(t *testing.T, d Dialect, input string, open, close byte) {
	t.Helper()
	got := d.EscapeIdentifier(input)
	if len(got) < 2 {
		t.Errorf("EscapeIdentifier(%q) = %q, too short", input, got)
		return
	}
	if got[0] != open || got[len(got)-1] != close {
		t.Errorf("EscapeIdentifier(%q) = %q, not properly delimited with %c...%c", input, got, open, close)
	}
}

func assertContainsDelimited(t *testing.T, sql, escaped string) {
	t.Helper()
	if !strings.Contains(sql, escaped) {
		t.Errorf("SQL %q does not contain escaped %q", sql, escaped)
	}
}

func assertQuotingInvariant(t *testing.T, got string, delim byte, input string) {
	t.Helper()
	if len(got) < 2 {
		t.Errorf("escape(%q) = %q, too short", input, got)
		return
	}
	if got[0] != delim || got[len(got)-1] != delim {
		t.Errorf("escape(%q) = %q, not properly delimited with %c", input, got, delim)
		return
	}
	// Check interior: all delimiter chars must be doubled
	inner := got[1 : len(got)-1]
	i := 0
	for i < len(inner) {
		if inner[i] == delim {
			if i+1 >= len(inner) || inner[i+1] != delim {
				t.Errorf("escape(%q) = %q, lone %c at position %d in interior", input, got, delim, i+1)
				return
			}
			i += 2 // skip the doubled pair
		} else {
			i++
		}
	}
}
