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

// --- Escape benchmarks ------------------------------------------------------

func BenchmarkEscapeIdentifier(b *testing.B) {
	short := "mydb"
	medium := strings.Repeat("abcde", 10)        // 50 chars
	long := strings.Repeat("a", 1000)            // 1000 chars
	withQuotes := strings.Repeat(`abc"d`, 10)    // 50 chars with 10 quotes
	withBackticks := strings.Repeat("abc`d", 10) // 50 chars with 10 backticks

	b.Run("PG/Short", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(short)
		}
	})
	b.Run("PG/Medium", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(medium)
		}
	})
	b.Run("PG/Long", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(long)
		}
	})
	b.Run("PG/WithQuotes", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(withQuotes)
		}
	})
	b.Run("MySQL/Short", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(short)
		}
	})
	b.Run("MySQL/Medium", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(medium)
		}
	})
	b.Run("MySQL/Long", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(long)
		}
	})
	b.Run("MySQL/WithBackticks", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeIdentifier(withBackticks)
		}
	})
}

func BenchmarkEscapeLiteral(b *testing.B) {
	short := "hello"
	medium := strings.Repeat("abcde", 10)
	long := strings.Repeat("a", 1000)
	withQuotes := strings.Repeat("abc'd", 10)
	withBackslash := strings.Repeat(`abc\d`, 10)

	b.Run("PG/Short", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(short)
		}
	})
	b.Run("PG/Medium", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(medium)
		}
	})
	b.Run("PG/Long", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(long)
		}
	})
	b.Run("PG/WithQuotes", func(b *testing.B) {
		d := PgDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(withQuotes)
		}
	})
	b.Run("MySQL/Short", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(short)
		}
	})
	b.Run("MySQL/Medium", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(medium)
		}
	})
	b.Run("MySQL/Long", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(long)
		}
	})
	b.Run("MySQL/WithBackslash", func(b *testing.B) {
		d := MySQLDialect{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = d.EscapeLiteral(withBackslash)
		}
	})
}

// --- Privilege validation benchmarks ----------------------------------------

func BenchmarkValidatePrivileges(b *testing.B) {
	single := []string{"SELECT"}
	five := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE"}

	allPg := make([]string, 0, len(ValidPostgresPrivileges))
	for k := range ValidPostgresPrivileges {
		allPg = append(allPg, k)
	}

	firstInvalid := []string{"INVALID_PRIV"}
	lastInvalid := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "INVALID_AT_END"}

	b.Run("Single", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidatePrivileges(single, ValidPostgresPrivileges)
		}
	})
	b.Run("Five", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidatePrivileges(five, ValidPostgresPrivileges)
		}
	})
	b.Run("AllPg", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidatePrivileges(allPg, ValidPostgresPrivileges)
		}
	})
	b.Run("FirstInvalid", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidatePrivileges(firstInvalid, ValidPostgresPrivileges)
		}
	})
	b.Run("LastInvalid", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = ValidatePrivileges(lastInvalid, ValidPostgresPrivileges)
		}
	})
}

// --- Grant builder benchmarks -----------------------------------------------

func BenchmarkGrantBuilder(b *testing.B) {
	b.Run("Simple", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = NewPg().Grant("SELECT").OnTable("", "t").To("u").Build()
		}
	})
	b.Run("Complex", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = NewPg().Grant("SELECT", "INSERT", "UPDATE", "DELETE").
				OnTable("public", "users").To("appuser").WithGrantOption().Build()
		}
	})
	b.Run("AlterDefaultPrivileges", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = NewPg().AlterDefaultPrivileges("owner", "public").
				Grant("SELECT", "INSERT").OnTables().To("reader").Build()
		}
	})
	b.Run("MySQL", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = NewMySQL().Grant("SELECT", "INSERT").OnDatabase("mydb").
				ToUser("appuser", "%").Build()
		}
	})
}

// --- Database builder benchmarks --------------------------------------------

func BenchmarkDatabaseBuilder(b *testing.B) {
	b.Run("PgMinimal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = PgCreateDatabase("mydb").Build()
		}
	})
	b.Run("PgFullOptions", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = PgCreateDatabase("mydb").
				Owner("admin").Template("template0").Encoding("UTF8").
				LCCollate("en_US.UTF-8").LCCtype("en_US.UTF-8").
				Tablespace("pg_default").ConnectionLimit(100).
				IsTemplate(true).Build()
		}
	})
	b.Run("MySQLCreate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = MySQLCreateDatabase("mydb").Charset("utf8mb4").
				Collation("utf8mb4_unicode_ci").Build()
		}
	})
	b.Run("Drop", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = PgDropDatabase("mydb").IfExists().Cascade().Build()
		}
	})
}

// --- Role builder benchmarks ------------------------------------------------

func BenchmarkRoleBuilder(b *testing.B) {
	b.Run("PgMinimal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = PgCreateRole("myuser").Build()
		}
	})
	b.Run("PgFullOptions", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = PgCreateRole("myuser").
				WithPassword("secret").Login(true).Superuser(false).
				CreateDB(false).CreateRoleOpt(false).Inherit(true).
				Replication(false).BypassRLS(false).
				InRoles("parent1", "parent2").Build()
		}
	})
	b.Run("MySQLCreateUser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = MySQLCreateUser("appuser", "%").
				IdentifiedBy("secret").
				AuthPlugin("caching_sha2_password").Build()
		}
	})
	b.Run("MySQLAlterUser", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = MySQLAlterUser("appuser", "%").IdentifiedBy("newpass").Build()
		}
	})
}
