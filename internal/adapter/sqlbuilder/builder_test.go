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
	"testing"
)

// --- Dialect tests -------------------------------------------------------

func TestPgEscapeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mydb", `"mydb"`},
		{`my"db`, `"my""db"`},
		{`my""db`, `"my""""db"`},
		{"", `""`},
		{`Robert"; DROP TABLE students;--`, `"Robert""; DROP TABLE students;--"`},
	}
	d := PgDialect{}
	for _, tt := range tests {
		got := d.EscapeIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("PgEscapeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPgEscapeLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", `'hello'`},
		{"it's", `'it''s'`},
		{`it's a "test"`, `'it''s a "test"'`},
		{"", `''`},
		{`'; DROP TABLE students;--`, `'''; DROP TABLE students;--'`},
	}
	d := PgDialect{}
	for _, tt := range tests {
		got := d.EscapeLiteral(tt.input)
		if got != tt.want {
			t.Errorf("PgEscapeLiteral(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMySQLEscapeIdentifier(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"mydb", "`mydb`"},
		{"my`db", "`my``db`"},
		{"", "``"},
		{"`; DROP TABLE students;--", "```; DROP TABLE students;--`"},
	}
	d := MySQLDialect{}
	for _, tt := range tests {
		got := d.EscapeIdentifier(tt.input)
		if got != tt.want {
			t.Errorf("MySQLEscapeIdentifier(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMySQLEscapeLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", `'hello'`},
		{"it's", `'it''s'`},
		{`back\slash`, `'back\\slash'`},
		{`'; DROP TABLE students;--`, `'''; DROP TABLE students;--'`},
	}
	d := MySQLDialect{}
	for _, tt := range tests {
		got := d.EscapeLiteral(tt.input)
		if got != tt.want {
			t.Errorf("MySQLEscapeLiteral(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- Privilege validation tests ------------------------------------------

func TestValidatePrivileges(t *testing.T) {
	tests := []struct {
		name      string
		privs     []string
		allowlist map[string]bool
		wantErr   bool
	}{
		{"valid pg", []string{"SELECT", "INSERT"}, ValidPostgresPrivileges, false},
		{"valid pg all", []string{"ALL PRIVILEGES"}, ValidPostgresPrivileges, false},
		{"case insensitive", []string{"select", "insert"}, ValidPostgresPrivileges, false},
		{"whitespace trimmed", []string{" SELECT ", " INSERT "}, ValidPostgresPrivileges, false},
		{"injection payload", []string{"SELECT; DROP TABLE users"}, ValidPostgresPrivileges, true},
		{"empty", []string{}, ValidPostgresPrivileges, true},
		{"mixed valid invalid", []string{"SELECT", "INVALID_PRIV"}, ValidPostgresPrivileges, true},
		{"valid mysql", []string{"CREATE VIEW", "SHOW DATABASES"}, ValidMySQLPrivileges, false},
		{"mysql injection", []string{"SELECT, DROP"}, ValidMySQLPrivileges, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePrivileges(tt.privs, tt.allowlist)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePrivileges() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- Grant builder tests -------------------------------------------------

func TestPgGrantOnTable(t *testing.T) {
	q, err := NewPg().Grant("SELECT", "INSERT").OnTable("public", "users").To("myuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT SELECT, INSERT ON TABLE "public"."users" TO "myuser"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantWithGrantOption(t *testing.T) {
	q, err := NewPg().Grant("SELECT").OnTable("", "users").To("myuser").WithGrantOption().Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT SELECT ON TABLE "users" TO "myuser" WITH GRANT OPTION`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgRevokeAllTablesInSchema(t *testing.T) {
	q, err := NewPg().Revoke("ALL").OnAllTablesInSchema("public").From("myuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `REVOKE ALL ON ALL TABLES IN SCHEMA "public" FROM "myuser"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantDatabase(t *testing.T) {
	q, err := NewPg().Grant("CONNECT", "TEMPORARY").OnDatabase("mydb").To("appuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT CONNECT, TEMPORARY ON DATABASE "mydb" TO "appuser"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantSchema(t *testing.T) {
	q, err := NewPg().Grant("USAGE").OnSchema("public").To("reader").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT USAGE ON SCHEMA "public" TO "reader"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantSequence(t *testing.T) {
	q, err := NewPg().Grant("USAGE", "SELECT").OnSequence("public", "my_seq").To("reader").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT USAGE, SELECT ON SEQUENCE "public"."my_seq" TO "reader"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantFunction(t *testing.T) {
	q, err := NewPg().Grant("EXECUTE").OnFunction("public", "my_func()").To("reader").Build()
	if err != nil {
		t.Fatal(err)
	}
	// The function name is now properly escaped via EscapeIdentifier
	want := `GRANT EXECUTE ON FUNCTION "public"."my_func()" TO "reader"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantRole(t *testing.T) {
	q, err := NewPg().GrantRole("admin").To("myuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT "admin" TO "myuser"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgRevokeRole(t *testing.T) {
	q, err := NewPg().RevokeRole("admin").From("myuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `REVOKE "admin" FROM "myuser"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgAlterDefaultPrivileges(t *testing.T) {
	q, err := NewPg().AlterDefaultPrivileges("owner", "public").
		Grant("SELECT").OnTables().To("reader").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER DEFAULT PRIVILEGES FOR ROLE "owner" IN SCHEMA "public" GRANT SELECT ON TABLES TO "reader"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgAlterDefaultPrivilegesSequences(t *testing.T) {
	q, err := NewPg().AlterDefaultPrivileges("", "myschema").
		Grant("USAGE", "SELECT").OnSequences().To("appuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER DEFAULT PRIVILEGES IN SCHEMA "myschema" GRANT USAGE, SELECT ON SEQUENCES TO "appuser"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgGrantRejectsInvalidPrivilege(t *testing.T) {
	_, err := NewPg().Grant("SELECT; DROP TABLE users").OnTable("public", "users").To("evil").Build()
	if err == nil {
		t.Fatal("expected error for injection payload")
	}
}

func TestPgGrantRejectsNoTarget(t *testing.T) {
	_, err := NewPg().Grant("SELECT").To("user").Build()
	if err == nil {
		t.Fatal("expected error for missing target")
	}
}

func TestPgGrantRejectsNoGrantee(t *testing.T) {
	_, err := NewPg().Grant("SELECT").OnTable("public", "t").Build()
	if err == nil {
		t.Fatal("expected error for missing grantee")
	}
}

func TestPgGrantIdentifierWithQuotes(t *testing.T) {
	q, err := NewPg().Grant("SELECT").OnTable("public", `my"table`).To(`my"user`).Build()
	if err != nil {
		t.Fatal(err)
	}
	want := `GRANT SELECT ON TABLE "public"."my""table" TO "my""user"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

// --- MySQL grant tests ---------------------------------------------------

func TestMySQLGrantDatabase(t *testing.T) {
	q, err := NewMySQL().Grant("SELECT", "INSERT").OnDatabase("mydb").ToLiteral("appuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT SELECT, INSERT ON `mydb`.* TO 'appuser'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantGlobal(t *testing.T) {
	q, err := NewMySQL().Grant("ALL PRIVILEGES").OnGlobal().ToLiteral("root").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT ALL PRIVILEGES ON *.* TO 'root'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantTable(t *testing.T) {
	q, err := NewMySQL().Grant("SELECT", "UPDATE").OnMySQLTable("mydb", "users").ToLiteral("appuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT SELECT, UPDATE ON `mydb`.`users` TO 'appuser'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantProcedure(t *testing.T) {
	q, err := NewMySQL().Grant("EXECUTE").OnProcedure("mydb", "myproc").ToLiteral("appuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT EXECUTE ON PROCEDURE `mydb`.`myproc` TO 'appuser'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantUserHost(t *testing.T) {
	q, err := NewMySQL().Grant("SELECT").OnDatabase("mydb").ToUser("appuser", "%").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT SELECT ON `mydb`.* TO 'appuser'@'%'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLRevokeDatabase(t *testing.T) {
	q, err := NewMySQL().Revoke("SELECT", "INSERT").OnDatabase("mydb").FromLiteral("appuser").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "REVOKE SELECT, INSERT ON `mydb`.* FROM 'appuser'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantRole(t *testing.T) {
	q, err := NewMySQL().GrantRole("admin_role").ToUser("appuser", "%").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT `admin_role` TO 'appuser'@'%'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLRevokeRole(t *testing.T) {
	q, err := NewMySQL().RevokeRole("admin_role").FromUser("appuser", "%").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "REVOKE `admin_role` FROM 'appuser'@'%'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantWithGrantOption(t *testing.T) {
	q, err := NewMySQL().Grant("SELECT").OnGlobal().ToLiteral("admin").WithGrantOption().Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT SELECT ON *.* TO 'admin' WITH GRANT OPTION"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLGrantRejectsInvalidPrivilege(t *testing.T) {
	_, err := NewMySQL().Grant("SELECT; DROP TABLE users").OnGlobal().ToLiteral("evil").Build()
	if err == nil {
		t.Fatal("expected error for injection payload")
	}
}

func TestMySQLGrantEscapesBacktick(t *testing.T) {
	q, err := NewMySQL().Grant("SELECT").OnDatabase("my`db").ToLiteral("user").Build()
	if err != nil {
		t.Fatal(err)
	}
	want := "GRANT SELECT ON `my``db`.* TO 'user'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

// --- Database builder tests ----------------------------------------------

func TestPgCreateDatabase(t *testing.T) {
	q := PgCreateDatabase("mydb").Owner("admin").Encoding("UTF8").
		Template("template0").LCCollate("en_US.UTF-8").ConnectionLimit(100).Build()
	want := `CREATE DATABASE "mydb" WITH OWNER = "admin" TEMPLATE = "template0" ENCODING = 'UTF8' LC_COLLATE = 'en_US.UTF-8' CONNECTION LIMIT = 100`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateDatabaseMinimal(t *testing.T) {
	q := PgCreateDatabase("mydb").Build()
	want := `CREATE DATABASE "mydb"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgDropDatabase(t *testing.T) {
	q := PgDropDatabase("mydb").IfExists().Build()
	want := `DROP DATABASE IF EXISTS "mydb"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgDropDatabaseCascade(t *testing.T) {
	q := PgDropDatabase("mydb").IfExists().Cascade().Build()
	want := `DROP DATABASE IF EXISTS "mydb" CASCADE`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateDatabase(t *testing.T) {
	q := MySQLCreateDatabase("mydb").Charset("utf8mb4").Collation("utf8mb4_unicode_ci").Build()
	want := "CREATE DATABASE IF NOT EXISTS `mydb` CHARACTER SET `utf8mb4` COLLATE `utf8mb4_unicode_ci`"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLAlterDatabase(t *testing.T) {
	q := MySQLAlterDatabase("mydb").Charset("utf8mb4").Collation("utf8mb4_unicode_ci").Build()
	want := "ALTER DATABASE `mydb` CHARACTER SET `utf8mb4` COLLATE `utf8mb4_unicode_ci`"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLDropDatabase(t *testing.T) {
	q := MySQLDropDatabase("mydb").IfExists().Build()
	want := "DROP DATABASE IF EXISTS `mydb`"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateDatabaseEscapesCharset(t *testing.T) {
	// This is the injection that was possible before: charset = "utf8; DROP TABLE users"
	q := MySQLCreateDatabase("mydb").Charset("utf8; DROP TABLE users").Build()
	want := "CREATE DATABASE IF NOT EXISTS `mydb` CHARACTER SET `utf8; DROP TABLE users`"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateDatabaseEscapesOwner(t *testing.T) {
	q := PgCreateDatabase(`my"db`).Owner(`evil"owner`).Build()
	want := `CREATE DATABASE "my""db" WITH OWNER = "evil""owner"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

// --- Role builder tests --------------------------------------------------

func TestPgCreateRole(t *testing.T) {
	q := PgCreateRole("admin").Login(false).CreateDB(true).
		CreateRoleOpt(false).Inherit(true).InRoles("parent_role").Build()
	want := `CREATE ROLE "admin" WITH NOLOGIN CREATEDB NOCREATEROLE INHERIT IN ROLE "parent_role"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateRoleWithPassword(t *testing.T) {
	q := PgCreateRole("appuser").Login(true).WithPassword("s3cret").Build()
	want := `CREATE ROLE "appuser" WITH PASSWORD 's3cret' LOGIN`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgAlterRole(t *testing.T) {
	q := PgAlterRole("admin").Login(true).CreateDB(false).Build()
	want := `ALTER ROLE "admin" WITH LOGIN NOCREATEDB`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgAlterRolePassword(t *testing.T) {
	q := PgAlterRole("admin").WithPassword("newpass").Build()
	want := `ALTER ROLE "admin" WITH PASSWORD 'newpass'`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgAlterRoleSet(t *testing.T) {
	q := PgAlterRole("admin").Set("search_path", "public").Build()
	want := `ALTER ROLE "admin" SET "search_path" = 'public'`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgDropRole(t *testing.T) {
	q := PgDropRole("admin").IfExists().Build()
	want := `DROP ROLE IF EXISTS "admin"`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateRoleEscapesName(t *testing.T) {
	q := PgCreateRole(`evil"role`).Login(false).Build()
	want := `CREATE ROLE "evil""role" WITH NOLOGIN`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgAlterRolePasswordEscapesQuotes(t *testing.T) {
	q := PgAlterRole("admin").WithPassword("pass'word").Build()
	want := `ALTER ROLE "admin" WITH PASSWORD 'pass''word'`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateRoleWithValidUntil(t *testing.T) {
	q := PgCreateRole("temp_user").Login(true).ValidUntil("2026-12-31").Build()
	want := `CREATE ROLE "temp_user" WITH LOGIN VALID UNTIL '2026-12-31'`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateRoleWithConnectionLimit(t *testing.T) {
	q := PgCreateRole("limited").Login(true).ConnectionLimit(10).Build()
	want := `CREATE ROLE "limited" WITH LOGIN CONNECTION LIMIT 10`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestPgCreateRoleFullOptions(t *testing.T) {
	q := PgCreateRole("full").Login(true).Superuser(false).CreateDB(false).
		CreateRoleOpt(false).Inherit(true).Replication(false).BypassRLS(false).Build()
	want := `CREATE ROLE "full" WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT NOREPLICATION NOBYPASSRLS`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

// --- MySQL role/user builder tests ----------------------------------------

func TestMySQLCreateRole(t *testing.T) {
	q := MySQLCreateRole("admin").Build()
	want := "CREATE ROLE IF NOT EXISTS 'admin'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLDropRole(t *testing.T) {
	q := MySQLDropRole("admin").IfExists().Build()
	want := "DROP ROLE IF EXISTS 'admin'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateUser(t *testing.T) {
	q := MySQLCreateUser("appuser", "%").IdentifiedBy("s3cret").Build()
	want := "CREATE USER IF NOT EXISTS 'appuser'@'%' IDENTIFIED BY 's3cret'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateUserWithPlugin(t *testing.T) {
	q := MySQLCreateUser("appuser", "%").IdentifiedBy("s3cret").
		AuthPlugin("caching_sha2_password").Build()
	want := "CREATE USER IF NOT EXISTS 'appuser'@'%' IDENTIFIED WITH caching_sha2_password BY 's3cret'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateUserAccountLock(t *testing.T) {
	q := MySQLCreateUser("emulated_role", "%").AccountLock().Build()
	want := "CREATE USER IF NOT EXISTS 'emulated_role'@'%' ACCOUNT LOCK"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLDropUser(t *testing.T) {
	q := MySQLDropUser("appuser", "%").IfExists().Build()
	want := "DROP USER IF EXISTS 'appuser'@'%'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLAlterUserPassword(t *testing.T) {
	q := MySQLAlterUser("appuser", "%").IdentifiedBy("newpass").Build()
	want := "ALTER USER 'appuser'@'%' IDENTIFIED BY 'newpass'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLAlterUserResourceLimits(t *testing.T) {
	q := MySQLAlterUser("appuser", "%").
		ResourceOption("MAX_QUERIES_PER_HOUR 100").
		ResourceOption("MAX_CONNECTIONS_PER_HOUR 50").Build()
	want := "ALTER USER 'appuser'@'%' MAX_QUERIES_PER_HOUR 100 MAX_CONNECTIONS_PER_HOUR 50"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateUserEscapesQuotesInPassword(t *testing.T) {
	q := MySQLCreateUser("user", "%").IdentifiedBy("pass'word").Build()
	want := "CREATE USER IF NOT EXISTS 'user'@'%' IDENTIFIED BY 'pass''word'"
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}

func TestMySQLCreateUserEscapesBackslashInPassword(t *testing.T) {
	q := MySQLCreateUser("user", "%").IdentifiedBy(`pass\word`).Build()
	want := `CREATE USER IF NOT EXISTS 'user'@'%' IDENTIFIED BY 'pass\\word'`
	if q != want {
		t.Errorf("got  %q\nwant %q", q, want)
	}
}
