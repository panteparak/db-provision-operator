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

import "testing"

// --- DatabaseBuilder coverage gaps ------------------------------------------

func TestDatabaseBuilder_CoverageGaps(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "LCCtype",
			sql:  PgCreateDatabase("mydb").LCCtype("en_US.UTF-8").Build(),
			want: `CREATE DATABASE "mydb" WITH LC_CTYPE = 'en_US.UTF-8'`,
		},
		{
			name: "Tablespace",
			sql:  PgCreateDatabase("mydb").Tablespace("fast_ssd").Build(),
			want: `CREATE DATABASE "mydb" WITH TABLESPACE = "fast_ssd"`,
		},
		{
			name: "IsTemplate true",
			sql:  PgCreateDatabase("tmpl").IsTemplate(true).Build(),
			want: `CREATE DATABASE "tmpl" WITH IS_TEMPLATE = TRUE`,
		},
		{
			name: "IsTemplate false produces no clause",
			sql:  PgCreateDatabase("mydb").IsTemplate(false).Build(),
			want: `CREATE DATABASE "mydb"`,
		},
		{
			name: "PgDropDatabase without IfExists",
			sql:  PgDropDatabase("mydb").Build(),
			want: `DROP DATABASE "mydb"`,
		},
		{
			name: "Cascade without IfExists",
			sql:  PgDropDatabase("mydb").Cascade().Build(),
			want: `DROP DATABASE "mydb" CASCADE`,
		},
		{
			name: "ConnectionLimit negative",
			sql:  PgCreateDatabase("mydb").ConnectionLimit(-1).Build(),
			want: `CREATE DATABASE "mydb" WITH CONNECTION LIMIT = -1`,
		},
		{
			name: "ConnectionLimit zero produces no clause",
			sql:  PgCreateDatabase("mydb").ConnectionLimit(0).Build(),
			want: `CREATE DATABASE "mydb"`,
		},
		{
			name: "MySQLCreateDatabase minimal",
			sql:  MySQLCreateDatabase("mydb").Build(),
			want: "CREATE DATABASE IF NOT EXISTS `mydb`",
		},
		{
			name: "MySQLDropDatabase without IfExists",
			sql:  MySQLDropDatabase("mydb").Build(),
			want: "DROP DATABASE `mydb`",
		},
		{
			name: "MySQLAlterDatabase charset only",
			sql:  MySQLAlterDatabase("mydb").Charset("utf8mb4").Build(),
			want: "ALTER DATABASE `mydb` CHARACTER SET `utf8mb4`",
		},
		{
			name: "MySQLAlterDatabase collation only",
			sql:  MySQLAlterDatabase("mydb").Collation("utf8mb4_bin").Build(),
			want: "ALTER DATABASE `mydb` COLLATE `utf8mb4_bin`",
		},
		{
			name: "unknown action returns empty",
			sql:  (&DatabaseBuilder{dialect: PgDialect{}, action: "UNKNOWN", name: "x"}).Build(),
			want: "",
		},
		{
			name: "PG ALTER with Owner",
			sql:  (&DatabaseBuilder{dialect: PgDialect{}, action: "ALTER", name: "mydb", owner: "newowner"}).Build(),
			want: `ALTER DATABASE "mydb" OWNER TO "newowner"`,
		},
		{
			name: "PG ALTER without Owner",
			sql:  (&DatabaseBuilder{dialect: PgDialect{}, action: "ALTER", name: "mydb"}).Build(),
			want: `ALTER DATABASE "mydb"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sql != tt.want {
				t.Errorf("got  %q\nwant %q", tt.sql, tt.want)
			}
		})
	}
}

// TestDatabaseBuilder_FullOptionOrdering verifies the WITH clause option
// order is deterministic: OWNER, TEMPLATE, ENCODING, LC_COLLATE, LC_CTYPE,
// TABLESPACE, CONNECTION LIMIT, IS_TEMPLATE.
func TestDatabaseBuilder_FullOptionOrdering(t *testing.T) {
	q := PgCreateDatabase("testdb").
		Owner("admin").
		Template("template0").
		Encoding("UTF8").
		LCCollate("en_US.UTF-8").
		LCCtype("en_US.UTF-8").
		Tablespace("pg_default").
		ConnectionLimit(100).
		IsTemplate(true).
		Build()

	want := `CREATE DATABASE "testdb" WITH ` +
		`OWNER = "admin" ` +
		`TEMPLATE = "template0" ` +
		`ENCODING = 'UTF8' ` +
		`LC_COLLATE = 'en_US.UTF-8' ` +
		`LC_CTYPE = 'en_US.UTF-8' ` +
		`TABLESPACE = "pg_default" ` +
		`CONNECTION LIMIT = 100 ` +
		`IS_TEMPLATE = TRUE`

	if q != want {
		t.Errorf("option ordering mismatch\ngot  %q\nwant %q", q, want)
	}
}

// --- RoleBuilder coverage gaps ----------------------------------------------

func TestRoleBuilder_CoverageGaps(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{
			name: "Superuser true",
			sql:  PgCreateRole("admin").Superuser(true).Build(),
			want: `CREATE ROLE "admin" WITH SUPERUSER`,
		},
		{
			name: "Replication true",
			sql:  PgCreateRole("repl").Replication(true).Build(),
			want: `CREATE ROLE "repl" WITH REPLICATION`,
		},
		{
			name: "BypassRLS true",
			sql:  PgCreateRole("byp").BypassRLS(true).Build(),
			want: `CREATE ROLE "byp" WITH BYPASSRLS`,
		},
		{
			name: "Login true then false appends both",
			sql:  PgCreateRole("u").Login(true).Login(false).Build(),
			want: `CREATE ROLE "u" WITH LOGIN NOLOGIN`,
		},
		{
			name: "PgDropRole without IfExists",
			sql:  PgDropRole("admin").Build(),
			want: `DROP ROLE "admin"`,
		},
		{
			name: "PgAlterRole minimal",
			sql:  PgAlterRole("admin").Build(),
			want: `ALTER ROLE "admin"`,
		},
		{
			name: "InRoles with multiple roles",
			sql:  PgCreateRole("u").InRoles("role1", "role2", "role3").Build(),
			want: `CREATE ROLE "u" WITH IN ROLE "role1", "role2", "role3"`,
		},
		{
			name: "WithPassword + Login + InRoles ordering",
			sql:  PgCreateRole("u").WithPassword("secret").Login(true).InRoles("parent").Build(),
			want: `CREATE ROLE "u" WITH PASSWORD 'secret' LOGIN IN ROLE "parent"`,
		},
		{
			name: "MySQLCreateUser with AuthPlugin but no password",
			sql:  MySQLCreateUser("u", "%").AuthPlugin("caching_sha2_password").Build(),
			want: "CREATE USER IF NOT EXISTS 'u'@'%' IDENTIFIED WITH caching_sha2_password",
		},
		{
			name: "MySQLAlterUser minimal",
			sql:  MySQLAlterUser("u", "%").Build(),
			want: "ALTER USER 'u'@'%'",
		},
		{
			name: "RequireSSL",
			sql:  MySQLCreateUser("u", "%").RequireSSL().Build(),
			want: "CREATE USER IF NOT EXISTS 'u'@'%' REQUIRE SSL",
		},
		{
			name: "RequireX509",
			sql:  MySQLCreateUser("u", "%").RequireX509().Build(),
			want: "CREATE USER IF NOT EXISTS 'u'@'%' REQUIRE X509",
		},
		{
			name: "RequireNone",
			sql:  MySQLCreateUser("u", "%").RequireNone().Build(),
			want: "CREATE USER IF NOT EXISTS 'u'@'%' REQUIRE NONE",
		},
		{
			name: "AccountUnlock",
			sql:  MySQLCreateUser("u", "%").AccountUnlock().Build(),
			want: "CREATE USER IF NOT EXISTS 'u'@'%' ACCOUNT UNLOCK",
		},
		{
			name: "MySQLDropRole without IfExists",
			sql:  MySQLDropRole("admin").Build(),
			want: "DROP ROLE 'admin'",
		},
		{
			name: "MySQLDropUser without IfExists",
			sql:  MySQLDropUser("u", "%").Build(),
			want: "DROP USER 'u'@'%'",
		},
		{
			name: "MySQLCreateUser combined options",
			sql:  MySQLCreateUser("u", "%").IdentifiedBy("pass").RequireSSL().AccountLock().Build(),
			want: "CREATE USER IF NOT EXISTS 'u'@'%' IDENTIFIED BY 'pass' REQUIRE SSL ACCOUNT LOCK",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sql != tt.want {
				t.Errorf("got  %q\nwant %q", tt.sql, tt.want)
			}
		})
	}
}
