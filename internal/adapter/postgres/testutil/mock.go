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

// NewMockPool creates a new pgxmock pool for testing PostgreSQL adapter code.
// The returned mock is configured with QueryMatcherRegexp to allow flexible query matching.
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

// ExpectVersion sets up an expectation for querying the PostgreSQL server version.
func ExpectVersion(mock pgxmock.PgxPoolIface, version string) {
	rows := pgxmock.NewRows([]string{"server_version"}).AddRow(version)
	mock.ExpectQuery(`SHOW server_version`).WillReturnRows(rows)
}

// ExpectDatabaseExists sets up an expectation for checking if a database exists.
func ExpectDatabaseExists(mock pgxmock.PgxPoolIface, name string, exists bool) {
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_database WHERE datname = \$1\)`).
		WithArgs(name).
		WillReturnRows(rows)
}

// ExpectUserExists sets up an expectation for checking if a user exists.
func ExpectUserExists(mock pgxmock.PgxPoolIface, username string, exists bool) {
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_roles WHERE rolname = \$1\)`).
		WithArgs(username).
		WillReturnRows(rows)
}

// ExpectRoleExists sets up an expectation for checking if a role exists.
// Note: In PostgreSQL, roles and users are stored in pg_roles.
func ExpectRoleExists(mock pgxmock.PgxPoolIface, rolename string, exists bool) {
	rows := pgxmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM pg_roles WHERE rolname = \$1\)`).
		WithArgs(rolename).
		WillReturnRows(rows)
}

// ExpectExecSuccess sets up a generic expectation for an exec command that succeeds.
// This matches any query and returns a successful result with the specified rows affected.
func ExpectExecSuccess(mock pgxmock.PgxPoolIface) {
	mock.ExpectExec(`.*`).WillReturnResult(pgxmock.NewResult("EXEC", 1))
}

// ExpectExecSuccessWithRowsAffected sets up an expectation for an exec command
// with a specific number of rows affected.
func ExpectExecSuccessWithRowsAffected(mock pgxmock.PgxPoolIface, rowsAffected int64) {
	mock.ExpectExec(`.*`).WillReturnResult(pgxmock.NewResult("EXEC", rowsAffected))
}

// ExpectCreateDatabase sets up an expectation for creating a database.
func ExpectCreateDatabase(mock pgxmock.PgxPoolIface, name string) {
	mock.ExpectExec(`CREATE DATABASE "` + name + `".*`).
		WillReturnResult(pgxmock.NewResult("CREATE DATABASE", 0))
}

// ExpectDropDatabase sets up an expectation for dropping a database.
func ExpectDropDatabase(mock pgxmock.PgxPoolIface, name string) {
	mock.ExpectExec(`DROP DATABASE IF EXISTS "` + name + `"`).
		WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))
}

// ExpectTerminateConnections sets up an expectation for terminating database connections.
func ExpectTerminateConnections(mock pgxmock.PgxPoolIface, dbname string) {
	mock.ExpectExec(`SELECT pg_terminate_backend\(pid\).*WHERE datname = \$1.*`).
		WithArgs(dbname).
		WillReturnResult(pgxmock.NewResult("SELECT", 0))
}

// ExpectCreateUser sets up an expectation for creating a user.
func ExpectCreateUser(mock pgxmock.PgxPoolIface, username string) {
	mock.ExpectExec(`CREATE USER "` + username + `".*`).
		WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))
}

// ExpectDropUser sets up an expectation for dropping a user.
func ExpectDropUser(mock pgxmock.PgxPoolIface, username string) {
	mock.ExpectExec(`DROP USER IF EXISTS "` + username + `"`).
		WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))
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
	mock.ExpectExec(`ALTER (USER|ROLE) "` + username + `" WITH PASSWORD .*`).
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

// ExpectBegin sets up an expectation for beginning a transaction.
func ExpectBegin(mock pgxmock.PgxPoolIface) {
	mock.ExpectBegin()
}

// ExpectCommit sets up an expectation for committing a transaction.
func ExpectCommit(mock pgxmock.PgxPoolIface) {
	mock.ExpectCommit()
}

// ExpectRollback sets up an expectation for rolling back a transaction.
func ExpectRollback(mock pgxmock.PgxPoolIface) {
	mock.ExpectRollback()
}

// ExpectDatabaseInfo sets up expectations for retrieving database information.
func ExpectDatabaseInfo(mock pgxmock.PgxPoolIface, name, owner, encoding, collation string, sizeBytes int64, ownerOid int64) {
	// First query: get database info
	dbRows := pgxmock.NewRows([]string{"datname", "datdba", "size", "encoding", "collation"}).
		AddRow(name, ownerOid, sizeBytes, encoding, collation)
	mock.ExpectQuery(`SELECT d.datname, d.datdba, pg_database_size\(d.datname\).*FROM pg_database d.*WHERE d.datname = \$1`).
		WithArgs(name).
		WillReturnRows(dbRows)

	// Second query: get owner name
	ownerRows := pgxmock.NewRows([]string{"rolname"}).AddRow(owner)
	mock.ExpectQuery(`SELECT rolname FROM pg_roles WHERE oid = \$1`).
		WithArgs(ownerOid).
		WillReturnRows(ownerRows)
}

// ExpectUserInfo sets up expectations for retrieving user information.
func ExpectUserInfo(mock pgxmock.PgxPoolIface, username string, connLimit int32, superuser, createDB, createRole, inherit, login, replication, bypassRLS bool) {
	rows := pgxmock.NewRows([]string{
		"rolname", "rolconnlimit", "rolsuper", "rolcreatedb", "rolcreaterole",
		"rolinherit", "rolcanlogin", "rolreplication", "rolbypassrls",
	}).AddRow(username, connLimit, superuser, createDB, createRole, inherit, login, replication, bypassRLS)

	mock.ExpectQuery(`SELECT.*FROM pg_roles WHERE rolname = \$1`).
		WithArgs(username).
		WillReturnRows(rows)
}

// ExpectRoleInfo sets up expectations for retrieving role information.
func ExpectRoleInfo(mock pgxmock.PgxPoolIface, rolename string, login, inherit, createDB, createRole, superuser, replication, bypassRLS bool) {
	rows := pgxmock.NewRows([]string{
		"rolname", "rolcanlogin", "rolinherit", "rolcreatedb", "rolcreaterole",
		"rolsuper", "rolreplication", "rolbypassrls",
	}).AddRow(rolename, login, inherit, createDB, createRole, superuser, replication, bypassRLS)

	mock.ExpectQuery(`SELECT.*FROM pg_roles WHERE rolname = \$1`).
		WithArgs(rolename).
		WillReturnRows(rows)
}
