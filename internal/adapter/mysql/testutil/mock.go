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
	"database/sql"
	"database/sql/driver"

	"github.com/DATA-DOG/go-sqlmock"
)

// NewMockDB creates a new sqlmock database for testing MySQL adapter code.
// The returned mock is configured with QueryMatcherRegexp to allow flexible query matching.
func NewMockDB() (*sql.DB, sqlmock.Sqlmock, error) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		return nil, nil, err
	}
	return db, mock, nil
}

// ExpectPing sets up an expectation for a Ping operation on the mock database.
func ExpectPing(mock sqlmock.Sqlmock) {
	mock.ExpectPing()
}

// ExpectVersion sets up an expectation for querying the MySQL server version.
func ExpectVersion(mock sqlmock.Sqlmock, version string) {
	rows := sqlmock.NewRows([]string{"VERSION()"}).AddRow(version)
	mock.ExpectQuery(`SELECT VERSION\(\)`).WillReturnRows(rows)
}

// ExpectDatabaseExists sets up an expectation for checking if a database exists.
func ExpectDatabaseExists(mock sqlmock.Sqlmock, name string, exists bool) {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM INFORMATION_SCHEMA\.SCHEMATA WHERE SCHEMA_NAME = \?\)`).
		WithArgs(name).
		WillReturnRows(rows)
}

// ExpectUserExists sets up an expectation for checking if a user exists.
func ExpectUserExists(mock sqlmock.Sqlmock, username string, exists bool) {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \?\)`).
		WithArgs(username).
		WillReturnRows(rows)
}

// ExpectRoleExists sets up an expectation for checking if a role exists.
// Note: In MySQL 8.0+, roles are stored in mysql.user with account_locked='Y'.
func ExpectRoleExists(mock sqlmock.Sqlmock, roleName string, exists bool) {
	rows := sqlmock.NewRows([]string{"exists"}).AddRow(exists)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \? AND Host = '%' AND account_locked = 'Y'\)`).
		WithArgs(roleName).
		WillReturnRows(rows)
}

// ExpectUserHosts sets up an expectation for getting all hosts for a user.
func ExpectUserHosts(mock sqlmock.Sqlmock, username string, hosts []string) {
	rows := sqlmock.NewRows([]string{"Host"})
	for _, host := range hosts {
		rows.AddRow(host)
	}
	mock.ExpectQuery(`SELECT Host FROM mysql\.user WHERE User = \?`).
		WithArgs(username).
		WillReturnRows(rows)
}

// ExpectExecSuccess sets up a generic expectation for an exec command that succeeds.
func ExpectExecSuccess(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`.*`).WillReturnResult(sqlmock.NewResult(0, 1))
}

// ExpectExecSuccessWithRowsAffected sets up an expectation for an exec command
// with a specific number of rows affected.
func ExpectExecSuccessWithRowsAffected(mock sqlmock.Sqlmock, rowsAffected int64) {
	mock.ExpectExec(`.*`).WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}

// ExpectQueryRows sets up a generic query expectation that returns the provided rows.
// columns specifies the column names, and rows contains the data as a slice of slices.
func ExpectQueryRows(mock sqlmock.Sqlmock, columns []string, rows ...[]interface{}) {
	mockRows := sqlmock.NewRows(columns)
	for _, row := range rows {
		// Convert []interface{} to []driver.Value for AddRow
		driverValues := make([]driver.Value, len(row))
		for i, v := range row {
			driverValues[i] = v
		}
		mockRows.AddRow(driverValues...)
	}
	mock.ExpectQuery(`.*`).WillReturnRows(mockRows)
}

// ExpectCreateDatabase sets up an expectation for creating a database.
func ExpectCreateDatabase(mock sqlmock.Sqlmock, name string) {
	mock.ExpectExec(`CREATE DATABASE.*` + name + `.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectDropDatabase sets up an expectation for dropping a database.
func ExpectDropDatabase(mock sqlmock.Sqlmock, name string) {
	mock.ExpectExec(`DROP DATABASE IF EXISTS.*` + name + `.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectCreateUser sets up an expectation for creating a user.
func ExpectCreateUser(mock sqlmock.Sqlmock, username string) {
	mock.ExpectExec(`CREATE USER.*` + username + `.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectDropUser sets up an expectation for dropping a user.
func ExpectDropUser(mock sqlmock.Sqlmock, username string) {
	mock.ExpectExec(`DROP USER IF EXISTS.*` + username + `.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectGrant sets up an expectation for a GRANT statement.
func ExpectGrant(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`GRANT .* ON .* TO .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectGrantWithPattern sets up an expectation for a GRANT statement with a specific pattern.
func ExpectGrantWithPattern(mock sqlmock.Sqlmock, pattern string) {
	mock.ExpectExec(pattern).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectRevoke sets up an expectation for a REVOKE statement.
func ExpectRevoke(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`REVOKE .* ON .* FROM .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectRevokeWithPattern sets up an expectation for a REVOKE statement with a specific pattern.
func ExpectRevokeWithPattern(mock sqlmock.Sqlmock, pattern string) {
	mock.ExpectExec(pattern).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectFlushPrivileges sets up an expectation for FLUSH PRIVILEGES.
func ExpectFlushPrivileges(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`FLUSH PRIVILEGES`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectGrantRole sets up an expectation for granting a role to a user.
func ExpectGrantRole(mock sqlmock.Sqlmock, role, user string) {
	mock.ExpectExec(`GRANT .* TO .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectRevokeRole sets up an expectation for revoking a role from a user.
func ExpectRevokeRole(mock sqlmock.Sqlmock, role, user string) {
	mock.ExpectExec(`REVOKE .* FROM .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// ExpectShowGrants sets up an expectation for SHOW GRANTS.
func ExpectShowGrants(mock sqlmock.Sqlmock, username, host string, grants []string) {
	rows := sqlmock.NewRows([]string{"Grants for " + username + "@" + host})
	for _, grant := range grants {
		rows.AddRow(grant)
	}
	mock.ExpectQuery(`SHOW GRANTS FOR.*`).WillReturnRows(rows)
}

// ExpectQueryError sets up an expectation for a query that returns an error.
func ExpectQueryError(mock sqlmock.Sqlmock, queryPattern string, err error) {
	mock.ExpectQuery(queryPattern).WillReturnError(err)
}

// ExpectExecError sets up an expectation for an exec command that returns an error.
func ExpectExecError(mock sqlmock.Sqlmock, queryPattern string, err error) {
	mock.ExpectExec(queryPattern).WillReturnError(err)
}

// ExpectBegin sets up an expectation for beginning a transaction.
func ExpectBegin(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
}

// ExpectCommit sets up an expectation for committing a transaction.
func ExpectCommit(mock sqlmock.Sqlmock) {
	mock.ExpectCommit()
}

// ExpectRollback sets up an expectation for rolling back a transaction.
func ExpectRollback(mock sqlmock.Sqlmock) {
	mock.ExpectRollback()
}

// ExpectAlterPassword sets up an expectation for altering a user's password.
func ExpectAlterPassword(mock sqlmock.Sqlmock, username string) {
	mock.ExpectExec(`ALTER USER.*` + username + `.*IDENTIFIED BY.*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
}
