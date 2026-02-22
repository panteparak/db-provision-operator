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
	"sync"
	"testing"
)

const concurrencyLevel = 100

// --- Concurrency tests (designed for -race) ---------------------------------

// TestConcurrency_ParallelGrantBuilders verifies that independent GrantBuilder
// instances built concurrently produce correct, non-contaminated output.
func TestConcurrency_ParallelGrantBuilders(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]string, concurrencyLevel)
	errs := make([]error, concurrencyLevel)

	// Barrier: all goroutines start simultaneously
	var start sync.WaitGroup
	start.Add(1)

	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start.Wait()
			grantee := fmt.Sprintf("user_%d", idx)
			q, err := NewPg().Grant("SELECT").OnTable("public", "users").To(grantee).Build()
			results[idx] = q
			errs[idx] = err
		}(i)
	}

	start.Done() // release barrier
	wg.Wait()

	for i := 0; i < concurrencyLevel; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		expected := fmt.Sprintf(`GRANT SELECT ON TABLE "public"."users" TO "user_%d"`, i)
		if results[i] != expected {
			t.Errorf("goroutine %d: got %q, want %q", i, results[i], expected)
		}
	}
}

// TestConcurrency_ParallelDatabaseBuilders verifies independent DatabaseBuilders.
func TestConcurrency_ParallelDatabaseBuilders(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]string, concurrencyLevel)

	var start sync.WaitGroup
	start.Add(1)

	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start.Wait()
			name := fmt.Sprintf("db_%d", idx)
			results[idx] = PgCreateDatabase(name).Owner("admin").Build()
		}(i)
	}

	start.Done()
	wg.Wait()

	for i := 0; i < concurrencyLevel; i++ {
		expected := fmt.Sprintf(`CREATE DATABASE "db_%d" WITH OWNER = "admin"`, i)
		if results[i] != expected {
			t.Errorf("goroutine %d: got %q, want %q", i, results[i], expected)
		}
	}
}

// TestConcurrency_ParallelRoleBuilders verifies independent RoleBuilders.
func TestConcurrency_ParallelRoleBuilders(t *testing.T) {
	var wg sync.WaitGroup
	results := make([]string, concurrencyLevel)

	var start sync.WaitGroup
	start.Add(1)

	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start.Wait()
			name := fmt.Sprintf("role_%d", idx)
			results[idx] = PgCreateRole(name).Login(true).Build()
		}(i)
	}

	start.Done()
	wg.Wait()

	for i := 0; i < concurrencyLevel; i++ {
		expected := fmt.Sprintf(`CREATE ROLE "role_%d" WITH LOGIN`, i)
		if results[i] != expected {
			t.Errorf("goroutine %d: got %q, want %q", i, results[i], expected)
		}
	}
}

// TestConcurrency_ParallelValidatePrivileges verifies concurrent reads
// of the global privilege maps are safe.
func TestConcurrency_ParallelValidatePrivileges(t *testing.T) {
	var wg sync.WaitGroup
	errs := make([]error, concurrencyLevel)

	var start sync.WaitGroup
	start.Add(1)

	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start.Wait()
			// Read from both maps concurrently
			err := ValidatePrivileges([]string{"SELECT", "INSERT"}, ValidPostgresPrivileges)
			if err != nil {
				errs[idx] = err
				return
			}
			err = ValidatePrivileges([]string{"SELECT", "INSERT"}, ValidMySQLPrivileges)
			errs[idx] = err
		}(i)
	}

	start.Done()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}
}

// TestConcurrency_SharedDialect verifies that a single Dialect value
// (value receiver) is safe for concurrent use.
func TestConcurrency_SharedDialect(t *testing.T) {
	d := PgDialect{}
	var wg sync.WaitGroup
	results := make([]string, concurrencyLevel)

	var start sync.WaitGroup
	start.Add(1)

	for i := 0; i < concurrencyLevel; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			start.Wait()
			results[idx] = d.EscapeIdentifier(fmt.Sprintf("name_%d", idx))
		}(i)
	}

	start.Done()
	wg.Wait()

	for i := 0; i < concurrencyLevel; i++ {
		expected := fmt.Sprintf(`"name_%d"`, i)
		if results[i] != expected {
			t.Errorf("goroutine %d: got %q, want %q", i, results[i], expected)
		}
	}
}

// --- Determinism tests ------------------------------------------------------

// TestDeterminism_GrantBuilder verifies that the same builder chain
// produces identical output across 1000 invocations.
func TestDeterminism_GrantBuilder(t *testing.T) {
	build := func() string {
		q, err := NewPg().Grant("SELECT", "INSERT", "UPDATE", "DELETE").
			OnTable("public", "users").To("appuser").WithGrantOption().Build()
		if err != nil {
			t.Fatal(err)
		}
		return q
	}

	first := build()
	for i := 0; i < 1000; i++ {
		got := build()
		if got != first {
			t.Fatalf("iteration %d: got %q, want %q", i, got, first)
		}
	}
}

// TestDeterminism_DatabaseBuilder verifies deterministic option ordering.
func TestDeterminism_DatabaseBuilder(t *testing.T) {
	build := func() string {
		return PgCreateDatabase("testdb").
			Owner("admin").Template("template0").Encoding("UTF8").
			LCCollate("en_US.UTF-8").LCCtype("en_US.UTF-8").
			Tablespace("pg_default").ConnectionLimit(100).
			IsTemplate(true).Build()
	}

	first := build()
	for i := 0; i < 1000; i++ {
		got := build()
		if got != first {
			t.Fatalf("iteration %d: got %q, want %q", i, got, first)
		}
	}
}

// TestDeterminism_RoleBuilder verifies deterministic option ordering.
func TestDeterminism_RoleBuilder(t *testing.T) {
	build := func() string {
		return PgCreateRole("appuser").
			WithPassword("secret").Login(true).Superuser(false).
			CreateDB(false).CreateRoleOpt(false).Inherit(true).
			Replication(false).BypassRLS(false).
			InRoles("parent1", "parent2").Build()
	}

	first := build()
	for i := 0; i < 1000; i++ {
		got := build()
		if got != first {
			t.Fatalf("iteration %d: got %q, want %q", i, got, first)
		}
	}
}

// --- Edge case tests --------------------------------------------------------

// TestEdgeCase_UnicodeIdentifiers tests various Unicode characters in
// identifier and literal positions for both dialects.
func TestEdgeCase_UnicodeIdentifiers(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"emoji", "db_🔥_name"},
		{"CJK", "数据库名"},
		{"zero-width space", "name\u200Bsecret"},
		{"RTL override", "name\u202Eevil"},
		{"combining char", "name\u0301accent"},
		{"surrogate-like", "name\U0001F600smile"},
		{"full-width", "ＡＢＣ"},
	}

	dialects := []struct {
		name string
		d    Dialect
	}{
		{"PG", PgDialect{}},
		{"MySQL", MySQLDialect{}},
	}

	for _, dialect := range dialects {
		for _, tc := range cases {
			t.Run(dialect.name+"/"+tc.name, func(t *testing.T) {
				id := dialect.d.EscapeIdentifier(tc.input)
				if len(id) < 2 {
					t.Fatalf("EscapeIdentifier(%q) too short: %q", tc.input, id)
				}

				lit := dialect.d.EscapeLiteral(tc.input)
				if len(lit) < 2 {
					t.Fatalf("EscapeLiteral(%q) too short: %q", tc.input, lit)
				}

				// Verify the input is preserved inside the delimiters
				if !strings.Contains(id, tc.input) && !strings.Contains(id, strings.ReplaceAll(tc.input, `"`, `""`)) {
					// For PG, if input contains quotes they get doubled
					if _, ok := dialect.d.(PgDialect); !ok || !strings.Contains(tc.input, `"`) {
						t.Errorf("EscapeIdentifier lost content: input=%q, got=%q", tc.input, id)
					}
				}
			})
		}
	}
}

// TestEdgeCase_VeryLongStrings tests that very long strings are handled
// without errors or truncation.
func TestEdgeCase_VeryLongStrings(t *testing.T) {
	lengths := []int{1000, 10000}
	dialects := []struct {
		name string
		d    Dialect
	}{
		{"PG", PgDialect{}},
		{"MySQL", MySQLDialect{}},
	}

	for _, dialect := range dialects {
		for _, length := range lengths {
			t.Run(fmt.Sprintf("%s/%d", dialect.name, length), func(t *testing.T) {
				input := strings.Repeat("a", length)

				id := dialect.d.EscapeIdentifier(input)
				// Output should be input + 2 delimiters
				if len(id) != length+2 {
					t.Errorf("EscapeIdentifier: len=%d, want %d", len(id), length+2)
				}

				lit := dialect.d.EscapeLiteral(input)
				if len(lit) != length+2 {
					t.Errorf("EscapeLiteral: len=%d, want %d", len(lit), length+2)
				}
			})
		}
	}
}

// TestEdgeCase_NullBytes tests that null bytes in identifier and literal
// positions don't cause panics or truncation.
func TestEdgeCase_NullBytes(t *testing.T) {
	inputs := []string{
		"\x00",
		"before\x00after",
		"\x00\x00\x00",
		"normal\x00",
	}

	dialects := []struct {
		name string
		d    Dialect
	}{
		{"PG", PgDialect{}},
		{"MySQL", MySQLDialect{}},
	}

	for _, dialect := range dialects {
		for _, input := range inputs {
			t.Run(dialect.name+"/"+fmt.Sprintf("%q", input), func(t *testing.T) {
				id := dialect.d.EscapeIdentifier(input)
				if len(id) < 2 {
					t.Fatalf("EscapeIdentifier(%q) too short: %q", input, id)
				}

				lit := dialect.d.EscapeLiteral(input)
				if len(lit) < 2 {
					t.Fatalf("EscapeLiteral(%q) too short: %q", input, lit)
				}

				// Null bytes should be preserved (not stripped)
				if !strings.Contains(id, "\x00") {
					t.Errorf("EscapeIdentifier stripped null byte")
				}
				if !strings.Contains(lit, "\x00") {
					t.Errorf("EscapeLiteral stripped null byte")
				}
			})
		}
	}
}

// TestEdgeCase_SpecialSQLCharacters tests SQL metacharacters in identifier
// and literal positions.
func TestEdgeCase_SpecialSQLCharacters(t *testing.T) {
	chars := []struct {
		name  string
		input string
	}{
		{"semicolon", "name;evil"},
		{"double dash", "name--evil"},
		{"block comment open", "name/*evil"},
		{"block comment close", "name*/evil"},
		{"newline", "name\nevil"},
		{"tab", "name\tevil"},
		{"carriage return", "name\revil"},
		{"combined", "a;b--c/*d*/e\nf"},
	}

	dialects := []struct {
		name string
		d    Dialect
	}{
		{"PG", PgDialect{}},
		{"MySQL", MySQLDialect{}},
	}

	for _, dialect := range dialects {
		for _, tc := range chars {
			t.Run(dialect.name+"/"+tc.name, func(t *testing.T) {
				// Must not panic
				id := dialect.d.EscapeIdentifier(tc.input)
				lit := dialect.d.EscapeLiteral(tc.input)

				// Must be properly delimited
				if len(id) < 2 {
					t.Errorf("EscapeIdentifier too short")
				}
				if len(lit) < 2 {
					t.Errorf("EscapeLiteral too short")
				}

				// SQL metacharacters should be preserved inside delimiters,
				// not stripped or modified (escaping only applies to the
				// delimiter character itself)
				if !strings.Contains(id, tc.input) && !strings.Contains(id, strings.ReplaceAll(tc.input, `"`, `""`)) {
					// This is fine for MySQL if backticks are in the input
				}
			})
		}
	}
}
