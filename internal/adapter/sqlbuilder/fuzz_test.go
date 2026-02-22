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

// FuzzPgEscapeIdentifier verifies that PgDialect.EscapeIdentifier never panics
// and always produces a properly delimited string.
func FuzzPgEscapeIdentifier(f *testing.F) {
	seeds := []string{
		"", "normal", `with"quote`, "\x00", "emoji_🔥",
		strings.Repeat("a", 1000),
		`"`, `""`, `"""`, "a\"b\"c",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	d := PgDialect{}
	f.Fuzz(func(t *testing.T, s string) {
		got := d.EscapeIdentifier(s)
		assertFuzzIdentifier(t, got, '"', s)
	})
}

// FuzzPgEscapeLiteral verifies that PgDialect.EscapeLiteral never panics
// and always produces a properly delimited string.
func FuzzPgEscapeLiteral(f *testing.F) {
	seeds := []string{
		"", "normal", "with'quote", `with\backslash`, "\x00",
		"emoji_🔥", strings.Repeat("a", 1000),
		`'`, `''`, `'''`, "a'b'c",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	d := PgDialect{}
	f.Fuzz(func(t *testing.T, s string) {
		got := d.EscapeLiteral(s)
		assertFuzzLiteral(t, got, '\'', s)
	})
}

// FuzzMySQLEscapeIdentifier verifies that MySQLDialect.EscapeIdentifier
// never panics and always produces a properly delimited string.
func FuzzMySQLEscapeIdentifier(f *testing.F) {
	seeds := []string{
		"", "normal", "with`backtick", "\x00", "emoji_🔥",
		strings.Repeat("a", 1000),
		"`", "``", "```", "a`b`c",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	d := MySQLDialect{}
	f.Fuzz(func(t *testing.T, s string) {
		got := d.EscapeIdentifier(s)
		assertFuzzIdentifier(t, got, '`', s)
	})
}

// FuzzMySQLEscapeLiteral verifies that MySQLDialect.EscapeLiteral never panics
// and always produces a properly delimited string with properly escaped backslashes.
func FuzzMySQLEscapeLiteral(f *testing.F) {
	seeds := []string{
		"", "normal", "with'quote", `with\backslash`, "\x00",
		"emoji_🔥", strings.Repeat("a", 1000),
		`'`, `''`, `\`, `\\`, `\'`, `'\'`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	d := MySQLDialect{}
	f.Fuzz(func(t *testing.T, s string) {
		got := d.EscapeLiteral(s)
		if len(got) < 2 {
			t.Fatalf("EscapeLiteral(%q) = %q, too short", s, got)
		}
		if got[0] != '\'' || got[len(got)-1] != '\'' {
			t.Fatalf("EscapeLiteral(%q) = %q, not properly delimited", s, got)
		}
		// Backslash count in output must be >= input
		inBS := strings.Count(s, `\`)
		outBS := strings.Count(got[1:len(got)-1], `\`)
		if outBS < inBS {
			t.Fatalf("EscapeLiteral(%q): output has %d backslashes, input has %d", s, outBS, inBS)
		}
	})
}

// FuzzValidatePrivileges verifies that ValidatePrivileges never panics
// and returns nil for known-valid privileges.
func FuzzValidatePrivileges(f *testing.F) {
	seeds := []string{
		"SELECT", "INSERT", "ALL PRIVILEGES", "USAGE",
		"", "   ", "invalid",
		"SELECT; DROP TABLE users",
		"\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		// Must not panic
		err := ValidatePrivileges([]string{s}, ValidPostgresPrivileges)

		// If it's a known valid PG privilege, err must be nil
		upper := strings.ToUpper(strings.TrimSpace(s))
		if ValidPostgresPrivileges[upper] && err != nil {
			t.Fatalf("ValidatePrivileges(%q) returned error for valid privilege: %v", s, err)
		}
	})
}

// FuzzGrantBuilderFullChain fuzzes the complete Grant builder chain.
func FuzzGrantBuilderFullChain(f *testing.F) {
	seeds := []struct {
		priv, target, grantee string
	}{
		{"SELECT", "users", "appuser"},
		{"INSERT", "my\"table", "my\"user"},
		{"ALL PRIVILEGES", "\x00", "evil'; --"},
		{"USAGE", "emoji_🔥", "normal"},
	}
	for _, s := range seeds {
		f.Add(s.priv, s.target, s.grantee)
	}

	f.Fuzz(func(t *testing.T, priv, target, grantee string) {
		q, err := NewPg().Grant(priv).OnTable("", target).To(grantee).Build()
		// Must not panic
		if err != nil {
			// On error, output must be empty
			if q != "" {
				t.Fatalf("Build() returned error with non-empty string: %q", q)
			}
			return
		}
		// On success, output must be non-empty
		if q == "" {
			t.Fatal("Build() returned nil error with empty string")
		}
	})
}

// FuzzDatabaseBuilderBuild fuzzes the PgCreateDatabase builder.
func FuzzDatabaseBuilderBuild(f *testing.F) {
	seeds := []string{
		"mydb", `my"db`, "\x00", "emoji_🔥",
		strings.Repeat("x", 1000),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		q := PgCreateDatabase(name).Build()
		// Must not panic; output is always non-empty
		if q == "" {
			t.Fatal("PgCreateDatabase.Build() returned empty string")
		}
	})
}

// FuzzRoleBuilderBuild fuzzes the PgCreateRole builder with name and password.
func FuzzRoleBuilderBuild(f *testing.F) {
	seeds := []struct {
		name, password string
	}{
		{"admin", "secret"},
		{`evil"role`, `pass'word`},
		{"\x00", `\`},
		{"emoji_🔥", ""},
	}
	for _, s := range seeds {
		f.Add(s.name, s.password)
	}

	f.Fuzz(func(t *testing.T, name, password string) {
		b := PgCreateRole(name)
		if password != "" {
			b.WithPassword(password)
		}
		q := b.Login(true).Build()
		// Must not panic; output is always non-empty
		if q == "" {
			t.Fatal("PgCreateRole.Build() returned empty string")
		}
	})
}

// --- fuzz helpers -----------------------------------------------------------

func assertFuzzIdentifier(t *testing.T, got string, delim byte, input string) {
	t.Helper()
	if len(got) < 2 {
		t.Fatalf("EscapeIdentifier(%q) = %q, too short", input, got)
	}
	if got[0] != delim || got[len(got)-1] != delim {
		t.Fatalf("EscapeIdentifier(%q) = %q, not properly delimited with %c", input, got, delim)
	}
	// Interior: no lone delimiter characters
	inner := got[1 : len(got)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] == delim {
			if i+1 >= len(inner) || inner[i+1] != delim {
				t.Fatalf("EscapeIdentifier(%q) = %q, lone %c at interior pos %d", input, got, delim, i)
			}
			i++ // skip pair
		}
	}
}

func assertFuzzLiteral(t *testing.T, got string, delim byte, input string) {
	t.Helper()
	if len(got) < 2 {
		t.Fatalf("EscapeLiteral(%q) = %q, too short", input, got)
	}
	if got[0] != delim || got[len(got)-1] != delim {
		t.Fatalf("EscapeLiteral(%q) = %q, not properly delimited with %c", input, got, delim)
	}
	inner := got[1 : len(got)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] == delim {
			if i+1 >= len(inner) || inner[i+1] != delim {
				t.Fatalf("EscapeLiteral(%q) = %q, lone %c at interior pos %d", input, got, delim, i)
			}
			i++
		}
	}
}
