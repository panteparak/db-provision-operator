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

package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/mysql/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Grant Operations", func() {
	var (
		adapter *Adapter
		ctx     context.Context
		mockDB  *sql.DB
		mock    sqlmock.Sqlmock
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		mockDB, mock, err = testutil.NewMockDB()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if mockDB != nil {
			_ = mockDB.Close()
		}
	})

	Describe("Grant", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("grants global privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT", "INSERT", "UPDATE"},
				},
			}

			mock.ExpectExec(`GRANT SELECT, INSERT, UPDATE ON \*\.\* TO 'testuser'`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("grants database privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "database",
					Database:   "mydb",
					Privileges: []string{"SELECT", "INSERT"},
				},
			}

			mock.ExpectExec("GRANT SELECT, INSERT ON `mydb`.\\* TO 'testuser'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("grants table privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "table",
					Database:   "mydb",
					Table:      "users",
					Privileges: []string{"SELECT", "UPDATE"},
				},
			}

			mock.ExpectExec("GRANT SELECT, UPDATE ON `mydb`.`users` TO 'testuser'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("grants column privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "column",
					Database:   "mydb",
					Table:      "users",
					Columns:    []string{"name", "email"},
					Privileges: []string{"SELECT"},
				},
			}

			mock.ExpectExec("GRANT SELECT \\(`name`, `email`\\) ON `mydb`.`users` TO 'testuser'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("grants procedure privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "procedure",
					Database:   "mydb",
					Procedure:  "my_proc",
					Privileges: []string{"EXECUTE"},
				},
			}

			mock.ExpectExec("GRANT EXECUTE ON PROCEDURE `mydb`.`my_proc` TO 'testuser'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("grants with grant option", func() {
			opts := []types.GrantOptions{
				{
					Level:           "database",
					Database:        "mydb",
					Privileges:      []string{"SELECT"},
					WithGrantOption: true,
				},
			}

			mock.ExpectExec("GRANT SELECT ON `mydb`.\\* TO 'testuser' WITH GRANT OPTION").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("flushes privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			mock.ExpectExec(`GRANT SELECT ON \*\.\* TO 'testuser'`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`FLUSH PRIVILEGES`).
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("not connected returns error", func() {
			disconnectedAdapter := NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			// db is nil - not connected

			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			err := disconnectedAdapter.Grant(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})
	})

	Describe("Revoke", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("revokes global privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT", "INSERT"},
				},
			}

			mock.ExpectExec(`REVOKE SELECT, INSERT ON \*\.\* FROM 'testuser'`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("revokes database privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "database",
					Database:   "mydb",
					Privileges: []string{"DELETE"},
				},
			}

			mock.ExpectExec("REVOKE DELETE ON `mydb`.\\* FROM 'testuser'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("revokes table privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "table",
					Database:   "mydb",
					Table:      "orders",
					Privileges: []string{"UPDATE", "DELETE"},
				},
			}

			mock.ExpectExec("REVOKE UPDATE, DELETE ON `mydb`.`orders` FROM 'testuser'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			testutil.ExpectFlushPrivileges(mock)

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("flushes privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			mock.ExpectExec(`REVOKE SELECT ON \*\.\* FROM 'testuser'`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`FLUSH PRIVILEGES`).
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})
	})

	Describe("GrantRole", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("grants role to user", func() {
			// First, expect query for user hosts (only once)
			rows := sqlmock.NewRows([]string{"Host"}).AddRow("%")
			mock.ExpectQuery(`SELECT Host FROM mysql.user WHERE User = \?`).
				WithArgs("testuser").
				WillReturnRows(rows)

			// Then expect grants for each role to each host
			// Note: role names use backticks (identifier), user@host uses single quotes (literal)
			mock.ExpectExec("GRANT `admin` TO 'testuser'@'%'").
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("GRANT `readonly` TO 'testuser'@'%'").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.GrantRole(ctx, "testuser", []string{"admin", "readonly"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("not connected returns error", func() {
			disconnectedAdapter := NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			// db is nil - not connected

			err := disconnectedAdapter.GrantRole(ctx, "testuser", []string{"admin"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})
	})

	Describe("RevokeRole", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("revokes role from user", func() {
			// First, expect query for user hosts
			rows := sqlmock.NewRows([]string{"Host"}).AddRow("%")
			mock.ExpectQuery(`SELECT Host FROM mysql.user WHERE User = \?`).
				WithArgs("testuser").
				WillReturnRows(rows)

			// Note: role names use backticks (identifier), user@host uses single quotes (literal)
			mock.ExpectExec("REVOKE `admin` FROM 'testuser'@'%'").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.RevokeRole(ctx, "testuser", []string{"admin"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})
	})

	Describe("SetDefaultPrivileges", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("is no-op for MySQL", func() {
			opts := []types.DefaultPrivilegeGrantOptions{
				{
					Database:   "testdb",
					Schema:     "public",
					ObjectType: "tables",
					Privileges: []string{"SELECT"},
				},
			}

			// SetDefaultPrivileges is a no-op for MySQL, so no mock expectations needed
			err := adapter.SetDefaultPrivileges(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetGrants", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("returns grants for user", func() {
			// First, expect query for user hosts
			hostRows := sqlmock.NewRows([]string{"Host"}).
				AddRow("%").
				AddRow("localhost")
			mock.ExpectQuery(`SELECT Host FROM mysql\.user WHERE User = \?`).
				WithArgs("testuser").
				WillReturnRows(hostRows)

			// Then expect SHOW GRANTS for each host
			grantRows1 := sqlmock.NewRows([]string{"Grants for testuser@%"}).
				AddRow("GRANT SELECT, INSERT ON `mydb`.* TO 'testuser'@'%'")
			mock.ExpectQuery(`SHOW GRANTS FOR 'testuser'@'%'`).
				WillReturnRows(grantRows1)

			grantRows2 := sqlmock.NewRows([]string{"Grants for testuser@localhost"}).
				AddRow("GRANT ALL PRIVILEGES ON *.* TO 'testuser'@'localhost'")
			mock.ExpectQuery(`SHOW GRANTS FOR 'testuser'@'localhost'`).
				WillReturnRows(grantRows2)

			grants, err := adapter.GetGrants(ctx, "testuser")
			Expect(err).NotTo(HaveOccurred())
			Expect(grants).To(HaveLen(2))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})
	})

	Describe("buildGrantQuery", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
		})

		It("builds correct query for each level", func() {
			testCases := []struct {
				name     string
				grantee  string
				opt      types.GrantOptions
				expected string
			}{
				{
					name:    "global level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "global",
						Privileges: []string{"SELECT", "INSERT"},
					},
					expected: "GRANT SELECT, INSERT ON *.* TO 'testuser'",
				},
				{
					name:    "database level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "database",
						Database:   "mydb",
						Privileges: []string{"ALL PRIVILEGES"},
					},
					expected: "GRANT ALL PRIVILEGES ON `mydb`.* TO 'testuser'",
				},
				{
					name:    "table level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "table",
						Database:   "mydb",
						Table:      "users",
						Privileges: []string{"SELECT", "UPDATE"},
					},
					expected: "GRANT SELECT, UPDATE ON `mydb`.`users` TO 'testuser'",
				},
				{
					name:    "column level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "column",
						Database:   "mydb",
						Table:      "users",
						Columns:    []string{"name", "email"},
						Privileges: []string{"SELECT"},
					},
					expected: "GRANT SELECT (`name`, `email`) ON `mydb`.`users` TO 'testuser'",
				},
				{
					name:    "procedure level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "procedure",
						Database:   "mydb",
						Procedure:  "my_procedure",
						Privileges: []string{"EXECUTE"},
					},
					expected: "GRANT EXECUTE ON PROCEDURE `mydb`.`my_procedure` TO 'testuser'",
				},
				{
					name:    "function level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "function",
						Database:   "mydb",
						Function:   "my_function",
						Privileges: []string{"EXECUTE"},
					},
					expected: "GRANT EXECUTE ON FUNCTION `mydb`.`my_function` TO 'testuser'",
				},
				{
					name:    "with grant option",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:           "database",
						Database:        "mydb",
						Privileges:      []string{"SELECT"},
						WithGrantOption: true,
					},
					expected: "GRANT SELECT ON `mydb`.* TO 'testuser' WITH GRANT OPTION",
				},
				{
					name:    "default to database when database specified",
					grantee: "testuser",
					opt: types.GrantOptions{
						Database:   "mydb",
						Privileges: []string{"SELECT"},
					},
					expected: "GRANT SELECT ON `mydb`.* TO 'testuser'",
				},
				{
					name:    "default to global when no database",
					grantee: "testuser",
					opt: types.GrantOptions{
						Privileges: []string{"SELECT"},
					},
					expected: "GRANT SELECT ON *.* TO 'testuser'",
				},
			}

			for _, tc := range testCases {
				By(tc.name)
				result, err := adapter.buildGrantQuery(tc.grantee, tc.opt)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(tc.expected))
			}
		})
	})

	Describe("buildRevokeQuery", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
		})

		It("builds correct query for each level", func() {
			testCases := []struct {
				name     string
				grantee  string
				opt      types.GrantOptions
				expected string
			}{
				{
					name:    "global level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "global",
						Privileges: []string{"SELECT", "INSERT"},
					},
					expected: "REVOKE SELECT, INSERT ON *.* FROM 'testuser'",
				},
				{
					name:    "database level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "database",
						Database:   "mydb",
						Privileges: []string{"ALL PRIVILEGES"},
					},
					expected: "REVOKE ALL PRIVILEGES ON `mydb`.* FROM 'testuser'",
				},
				{
					name:    "table level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "table",
						Database:   "mydb",
						Table:      "users",
						Privileges: []string{"DELETE"},
					},
					expected: "REVOKE DELETE ON `mydb`.`users` FROM 'testuser'",
				},
				{
					name:    "column level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "column",
						Database:   "mydb",
						Table:      "users",
						Columns:    []string{"salary"},
						Privileges: []string{"UPDATE"},
					},
					expected: "REVOKE UPDATE (`salary`) ON `mydb`.`users` FROM 'testuser'",
				},
				{
					name:    "procedure level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "procedure",
						Database:   "mydb",
						Procedure:  "my_proc",
						Privileges: []string{"EXECUTE"},
					},
					expected: "REVOKE EXECUTE ON PROCEDURE `mydb`.`my_proc` FROM 'testuser'",
				},
				{
					name:    "function level",
					grantee: "testuser",
					opt: types.GrantOptions{
						Level:      "function",
						Database:   "mydb",
						Function:   "my_func",
						Privileges: []string{"EXECUTE"},
					},
					expected: "REVOKE EXECUTE ON FUNCTION `mydb`.`my_func` FROM 'testuser'",
				},
			}

			for _, tc := range testCases {
				By(tc.name)
				result, err := adapter.buildRevokeQuery(tc.grantee, tc.opt)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(tc.expected))
			}
		})
	})

	Describe("parseGrantString", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
		})

		It("parses grant string correctly", func() {
			testCases := []struct {
				name        string
				grantStr    string
				grantee     string
				wantPrivs   []string
				wantDB      string
				wantObjType string
				wantObjName string
			}{
				{
					name:        "global grant",
					grantStr:    "GRANT SELECT, INSERT ON *.* TO 'testuser'@'%'",
					grantee:     "testuser",
					wantPrivs:   []string{"SELECT", "INSERT"},
					wantDB:      "*",
					wantObjType: "database",
					wantObjName: "*",
				},
				{
					name:        "database grant",
					grantStr:    "GRANT ALL PRIVILEGES ON `mydb`.* TO 'testuser'@'%'",
					grantee:     "testuser",
					wantPrivs:   []string{"ALL PRIVILEGES"},
					wantDB:      "mydb",
					wantObjType: "database",
					wantObjName: "mydb",
				},
				{
					name:        "table grant",
					grantStr:    "GRANT SELECT, UPDATE ON `mydb`.`users` TO 'testuser'@'%'",
					grantee:     "testuser",
					wantPrivs:   []string{"SELECT", "UPDATE"},
					wantDB:      "mydb",
					wantObjType: "table",
					wantObjName: "users",
				},
			}

			for _, tc := range testCases {
				By(tc.name)
				result := adapter.parseGrantString(tc.grantStr, tc.grantee)
				Expect(result).NotTo(BeNil())
				Expect(result.Grantee).To(Equal(tc.grantee))
				Expect(result.Privileges).To(Equal(tc.wantPrivs))
				Expect(result.Database).To(Equal(tc.wantDB))
				Expect(result.ObjectType).To(Equal(tc.wantObjType))
				Expect(result.ObjectName).To(Equal(tc.wantObjName))
			}
		})

		It("returns nil for invalid grant string", func() {
			result := adapter.parseGrantString("INVALID GRANT STRING", "testuser")
			Expect(result).To(BeNil())
		})
	})

	Describe("Grant error handling", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("returns error on grant failure", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			mock.ExpectExec(`GRANT SELECT ON \*\.\* TO 'testuser'`).
				WillReturnError(errors.New("access denied"))

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to grant privileges"))
		})

		It("returns error on flush privileges failure", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			mock.ExpectExec(`GRANT SELECT ON \*\.\* TO 'testuser'`).
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec(`FLUSH PRIVILEGES`).
				WillReturnError(errors.New("flush failed"))

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to flush privileges"))
		})
	})

	Describe("Revoke error handling", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("returns error on revoke failure", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			mock.ExpectExec(`REVOKE SELECT ON \*\.\* FROM 'testuser'`).
				WillReturnError(errors.New("access denied"))

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to revoke privileges"))
		})
	})

	Describe("GrantRole error handling", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("returns error on grant role failure", func() {
			// First, expect query for user hosts
			rows := sqlmock.NewRows([]string{"Host"}).AddRow("%")
			mock.ExpectQuery(`SELECT Host FROM mysql.user WHERE User = \?`).
				WithArgs("testuser").
				WillReturnRows(rows)

			// Note: role names use backticks (identifier)
			mock.ExpectExec("GRANT `admin` TO 'testuser'@'%'").
				WillReturnError(errors.New("role does not exist"))

			err := adapter.GrantRole(ctx, "testuser", []string{"admin"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to grant role"))
		})
	})

	Describe("RevokeRole error handling", func() {
		BeforeEach(func() {
			adapter = NewAdapter(types.ConnectionConfig{
				Host:     "localhost",
				Port:     3306,
				Database: "testdb",
				Username: "root",
				Password: "password",
			})
			adapter.db = mockDB
		})

		It("returns error on revoke role failure", func() {
			// First, expect query for user hosts
			rows := sqlmock.NewRows([]string{"Host"}).AddRow("%")
			mock.ExpectQuery(`SELECT Host FROM mysql.user WHERE User = \?`).
				WithArgs("testuser").
				WillReturnRows(rows)

			// Note: role names use backticks (identifier)
			mock.ExpectExec("REVOKE `admin` FROM 'testuser'@'%'").
				WillReturnError(errors.New("role not granted"))

			err := adapter.RevokeRole(ctx, "testuser", []string{"admin"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to revoke role"))
		})
	})
})
