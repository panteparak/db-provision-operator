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

package testutil

import (
	"github.com/pashagolub/pgxmock/v4"
)

// NewMockPool creates a new pgxmock pool for testing CockroachDB adapter code.
// CockroachDB uses the PostgreSQL wire protocol, so pgxmock works for both.
func NewMockPool() (pgxmock.PgxPoolIface, error) {
	mock, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherRegexp))
	if err != nil {
		return nil, err
	}
	return mock, nil
}

// ExpectPing sets up an expectation for a Ping operation on the mock pool.
func ExpectPing(mock pgxmock.PgxPoolIface) {
	mock.ExpectPing()
}

// ExpectVersion sets up an expectation for querying the CockroachDB server version.
func ExpectVersion(mock pgxmock.PgxPoolIface, version string) {
	rows := pgxmock.NewRows([]string{"version"}).AddRow(version)
	mock.ExpectQuery(`SELECT version\(\)`).WillReturnRows(rows)
}

// ExpectDatabaseExists sets up an expectation for checking if a database exists.
func ExpectDatabaseExists(mock pgxmock.PgxPoolIface, name string, exists bool) {
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM crdb_internal\.databases WHERE name = \$1\)`).
		WithArgs(name).
		WillReturnRows(rows)
}

// ExpectUserExists sets up an expectation for checking if a user exists.
func ExpectUserExists(mock pgxmock.PgxPoolIface, username string, exists bool) {
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM crdb_internal\.pg_catalog_table_is_implemented.*`).
		WithArgs(username).
		WillReturnRows(rows)
}

// ExpectRoleExists sets up an expectation for checking if a role exists.
// In CockroachDB, users and roles are stored in system.users (exposed via pg_roles).
func ExpectRoleExists(mock pgxmock.PgxPoolIface, rolename string, exists bool) {
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM.*WHERE.*= \$1\)`).
		WithArgs(rolename).
		WillReturnRows(rows)
}

// ExpectExecSuccess sets up a generic expectation for an exec command that succeeds.
func ExpectExecSuccess(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(`.*`).WillReturnResult(pgxmock.NewResult("EXEC", 1))
}

// ExpectCreateDatabase sets up an expectation for creating a database.
func ExpectCreateDatabase(mock pgxmock.PgxPoolIface, name string) {
	mock.ExpectExec(`CREATE DATABASE "` + name + `".*`).
		WillReturnResult(pgxmock.NewResult("CREATE DATABASE", 0))
}

// ExpectDropDatabase sets up an expectation for dropping a database.
func ExpectDropDatabase(mock pgxmock.PgxPoolIface, name string) {
	mock.ExpectExec(`DROP DATABASE IF EXISTS "` + name + `".*`).
		WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))
}

// ExpectCreateRole sets up an expectation for creating a role.
func ExpectCreateRole(mock pgxmock.PgxPoolIface, rolename string) {
	mock.ExpectExec(`CREATE ROLE "` + rolename + `".*`).
		WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))
}

// ExpectDropRole sets up an expectation for dropping a role.
func ExpectDropRole(mock pgxmock.PgxPoolIface, rolename string) {
	mock.ExpectExec(`DROP ROLE IF EXISTS "` + rolename + `"`).
		WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))
}

// ExpectGrant sets up an expectation for a GRANT statement.
func ExpectGrant(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(`GRANT .* TO .*`).
		WillReturnResult(pgxmock.NewResult("GRANT", 0))
}

// ExpectRevoke sets up an expectation for a REVOKE statement.
func ExpectRevoke(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(`REVOKE .* FROM .*`).
		WillReturnResult(pgxmock.NewResult("REVOKE", 0))
}

// ExpectAlterPassword sets up an expectation for altering a user's password.
func ExpectAlterPassword(mock pgxmock.PgxPoolIface, username string) {
	mock.ExpectExec(`ALTER ROLE "` + username + `" WITH PASSWORD .*`).
		WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))
}

// ExpectQueryError sets up an expectation for a query that returns an error.
func ExpectQueryError(mock pgxmock.PgxPoolIface, queryPattern string, err error) {
	mock.ExpectQuery(queryPattern).WillReturnError(err)
}

// ExpectExecError sets up an expectation for an exec command that returns an error.
func ExpectExecError(mock pgxmock.PgxPoolIface, queryPattern string, err error) {
	mock.ExpectExec(queryPattern).WillReturnError(err)
}

// ExpectDatabaseInfo sets up expectations for retrieving database information.
func ExpectDatabaseInfo(mock pgxmock.PgxPoolIface, name, owner, encoding string, sizeBytes int64) {
	dbRows := pgxmock.NewRows([]string{"database_name", "owner", "encoding"}).
		AddRow(name, owner, encoding)
	mock.ExpectQuery(`SELECT.*FROM.*crdb_internal\.databases.*WHERE.*name = \$1`).
		WithArgs(name).
		WillReturnRows(dbRows)

	sizeRows := pgxmock.NewRows([]string{"range_size_mb"}).AddRow(sizeBytes)
	mock.ExpectQuery(`SELECT.*range_size_mb.*`).
		WillReturnRows(sizeRows)
}

// ExpectUserInfo sets up expectations for retrieving user information from CockroachDB.
func ExpectUserInfo(mock pgxmock.PgxPoolIface, username string, login, createDB, createRole bool) {
	rows := pgxmock.NewRows([]string{
		"rolname", "rolcanlogin", "rolcreatedb", "rolcreaterole",
	}).AddRow(username, login, createDB, createRole)

	mock.ExpectQuery(`SELECT.*FROM.*pg_roles.*WHERE.*rolname = \$1`).
		WithArgs(username).
		WillReturnRows(rows)
}

// ExpectRoleInfo sets up expectations for retrieving role information.
func ExpectRoleInfo(mock pgxmock.PgxPoolIface, rolename string, login, createDB, createRole bool) {
	rows := pgxmock.NewRows([]string{
		"rolname", "rolcanlogin", "rolcreatedb", "rolcreaterole",
	}).AddRow(rolename, login, createDB, createRole)

	mock.ExpectQuery(`SELECT.*FROM.*pg_roles.*WHERE.*rolname = \$1`).
		WithArgs(rolename).
		WillReturnRows(rows)
}
