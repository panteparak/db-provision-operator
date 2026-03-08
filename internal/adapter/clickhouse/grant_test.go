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

package clickhouse

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
		mockDB, mock, err = sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if mockDB != nil {
			_ = mockDB.Close()
		}
	})

	Describe("Grant", func() {
		BeforeEach(func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = mockDB
		})

		It("grants global privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT", "INSERT"},
				},
			}

			mock.ExpectExec("GRANT SELECT, INSERT ON \\*\\.\\* TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

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

			mock.ExpectExec("GRANT SELECT, INSERT ON `mydb`\\.\\* TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

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

			mock.ExpectExec("GRANT SELECT, UPDATE ON `mydb`\\.`users` TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

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

			mock.ExpectExec("GRANT SELECT\\(`name`, `email`\\) ON `mydb`\\.`users` TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

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

			mock.ExpectExec("GRANT SELECT ON `mydb`\\.\\* TO `testuser` WITH GRANT OPTION").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("does not flush privileges (unlike MySQL)", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT"},
				},
			}

			// Only expect the GRANT query — no FLUSH PRIVILEGES
			mock.ExpectExec("GRANT SELECT ON \\*\\.\\* TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error when not connected", func() {
			disconnectedAdapter := NewAdapter(defaultConfig())

			opts := []types.GrantOptions{
				{Level: "global", Privileges: []string{"SELECT"}},
			}

			err := disconnectedAdapter.Grant(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("returns error on grant failure", func() {
			opts := []types.GrantOptions{
				{Level: "global", Privileges: []string{"SELECT"}},
			}

			mock.ExpectExec("GRANT SELECT ON \\*\\.\\* TO `testuser`").
				WillReturnError(errors.New("access denied"))

			err := adapter.Grant(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to grant privileges"))
		})
	})

	Describe("Revoke", func() {
		BeforeEach(func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = mockDB
		})

		It("revokes global privileges", func() {
			opts := []types.GrantOptions{
				{
					Level:      "global",
					Privileges: []string{"SELECT", "INSERT"},
				},
			}

			mock.ExpectExec("REVOKE SELECT, INSERT ON \\*\\.\\* FROM `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

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

			mock.ExpectExec("REVOKE DELETE ON `mydb`\\.\\* FROM `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

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

			mock.ExpectExec("REVOKE UPDATE, DELETE ON `mydb`\\.`orders` FROM `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error on revoke failure", func() {
			opts := []types.GrantOptions{
				{Level: "global", Privileges: []string{"SELECT"}},
			}

			mock.ExpectExec("REVOKE SELECT ON \\*\\.\\* FROM `testuser`").
				WillReturnError(errors.New("access denied"))

			err := adapter.Revoke(ctx, "testuser", opts)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to revoke privileges"))
		})
	})

	Describe("GrantRole", func() {
		BeforeEach(func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = mockDB
		})

		It("grants role to user (no host iteration needed)", func() {
			// ClickHouse uses identifier escaping for both role and user
			mock.ExpectExec("GRANT `admin` TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("GRANT `readonly` TO `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.GrantRole(ctx, "testuser", []string{"admin", "readonly"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error when not connected", func() {
			disconnectedAdapter := NewAdapter(defaultConfig())

			err := disconnectedAdapter.GrantRole(ctx, "testuser", []string{"admin"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("returns error on grant role failure", func() {
			mock.ExpectExec("GRANT `admin` TO `testuser`").
				WillReturnError(errors.New("role does not exist"))

			err := adapter.GrantRole(ctx, "testuser", []string{"admin"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to grant role"))
		})
	})

	Describe("RevokeRole", func() {
		BeforeEach(func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = mockDB
		})

		It("revokes role from user", func() {
			mock.ExpectExec("REVOKE `admin` FROM `testuser`").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.RevokeRole(ctx, "testuser", []string{"admin"})
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error on revoke role failure", func() {
			mock.ExpectExec("REVOKE `admin` FROM `testuser`").
				WillReturnError(errors.New("role not granted"))

			err := adapter.RevokeRole(ctx, "testuser", []string{"admin"})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to revoke role"))
		})
	})

	Describe("SetDefaultPrivileges", func() {
		It("is a no-op for ClickHouse", func() {
			adapter = NewAdapter(defaultConfig())

			opts := []types.DefaultPrivilegeGrantOptions{
				{Database: "testdb", Schema: "public", ObjectType: "tables", Privileges: []string{"SELECT"}},
			}

			err := adapter.SetDefaultPrivileges(ctx, "testuser", opts)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetGrants", func() {
		BeforeEach(func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = mockDB
		})

		It("returns grants for user", func() {
			rows := sqlmock.NewRows([]string{"user_name", "role_name", "access_type", "database", "table", "grant_option"}).
				AddRow("testuser", "", "SELECT", "mydb", "", uint8(0)).
				AddRow("testuser", "", "INSERT", "mydb", "users", uint8(1))
			mock.ExpectQuery("SELECT.*FROM system.grants.*WHERE user_name = \\? OR role_name = \\?").
				WithArgs("testuser", "testuser").
				WillReturnRows(rows)

			grants, err := adapter.GetGrants(ctx, "testuser")
			Expect(err).NotTo(HaveOccurred())
			Expect(grants).To(HaveLen(2))
			Expect(grants[0].Database).To(Equal("mydb"))
			Expect(grants[0].Privileges).To(Equal([]string{"SELECT"}))
			Expect(grants[0].ObjectType).To(Equal("database"))
			Expect(grants[1].ObjectType).To(Equal("table"))
			Expect(grants[1].ObjectName).To(Equal("users"))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns empty grants when none exist", func() {
			rows := sqlmock.NewRows([]string{"user_name", "role_name", "access_type", "database", "table", "grant_option"})
			mock.ExpectQuery("SELECT.*FROM system.grants.*WHERE user_name = \\? OR role_name = \\?").
				WithArgs("testuser", "testuser").
				WillReturnRows(rows)

			grants, err := adapter.GetGrants(ctx, "testuser")
			Expect(err).NotTo(HaveOccurred())
			Expect(grants).To(BeEmpty())
		})

		It("returns error when not connected", func() {
			disconnectedAdapter := NewAdapter(defaultConfig())

			grants, err := disconnectedAdapter.GetGrants(ctx, "testuser")
			Expect(err).To(HaveOccurred())
			Expect(grants).To(BeNil())
		})

		It("returns error when query fails", func() {
			mock.ExpectQuery("SELECT.*FROM system.grants.*WHERE user_name = \\? OR role_name = \\?").
				WithArgs("testuser", "testuser").
				WillReturnError(fmt.Errorf("connection error"))

			grants, err := adapter.GetGrants(ctx, "testuser")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get grants"))
			Expect(grants).To(BeNil())
		})
	})

	Describe("buildGrantQuery", func() {
		BeforeEach(func() {
			adapter = NewAdapter(defaultConfig())
		})

		It("builds correct query for each level", func() {
			testCases := []struct {
				name     string
				grantee  string
				opt      types.GrantOptions
				expected string
			}{
				{
					name:     "global level",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "global", Privileges: []string{"SELECT", "INSERT"}},
					expected: "GRANT SELECT, INSERT ON *.* TO `testuser`",
				},
				{
					name:     "database level",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "database", Database: "mydb", Privileges: []string{"ALL"}},
					expected: "GRANT ALL ON `mydb`.* TO `testuser`",
				},
				{
					name:     "table level",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "table", Database: "mydb", Table: "users", Privileges: []string{"SELECT"}},
					expected: "GRANT SELECT ON `mydb`.`users` TO `testuser`",
				},
				{
					name:     "with grant option",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "database", Database: "mydb", Privileges: []string{"SELECT"}, WithGrantOption: true},
					expected: "GRANT SELECT ON `mydb`.* TO `testuser` WITH GRANT OPTION",
				},
				{
					name:     "default to database when database specified",
					grantee:  "testuser",
					opt:      types.GrantOptions{Database: "mydb", Privileges: []string{"SELECT"}},
					expected: "GRANT SELECT ON `mydb`.* TO `testuser`",
				},
				{
					name:     "default to global when no database",
					grantee:  "testuser",
					opt:      types.GrantOptions{Privileges: []string{"SELECT"}},
					expected: "GRANT SELECT ON *.* TO `testuser`",
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
			adapter = NewAdapter(defaultConfig())
		})

		It("builds correct query for each level", func() {
			testCases := []struct {
				name     string
				grantee  string
				opt      types.GrantOptions
				expected string
			}{
				{
					name:     "global level",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "global", Privileges: []string{"SELECT"}},
					expected: "REVOKE SELECT ON *.* FROM `testuser`",
				},
				{
					name:     "database level",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "database", Database: "mydb", Privileges: []string{"ALL PRIVILEGES"}},
					expected: "REVOKE ALL PRIVILEGES ON `mydb`.* FROM `testuser`",
				},
				{
					name:     "table level",
					grantee:  "testuser",
					opt:      types.GrantOptions{Level: "table", Database: "mydb", Table: "users", Privileges: []string{"DELETE"}},
					expected: "REVOKE DELETE ON `mydb`.`users` FROM `testuser`",
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
})
