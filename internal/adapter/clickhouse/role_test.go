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
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Role Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
		mock    sqlmock.Sqlmock
		db      *sql.DB
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		db, mock, err = sqlmock.New()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if db != nil {
			_ = db.Close()
		}
	})

	Describe("CreateRole", func() {
		Context("without grants", func() {
			It("should execute CREATE ROLE query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE ROLE IF NOT EXISTS `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with database-level grants", func() {
			It("should create role and apply grants", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE ROLE IF NOT EXISTS `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("GRANT SELECT, INSERT ON `testdb`\\.\\* TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT", "INSERT"},
							Level:      "database",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with global-level grants", func() {
			It("should apply global grants", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE ROLE IF NOT EXISTS `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("GRANT SELECT ON \\*\\.\\* TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
					Grants: []types.GrantOptions{
						{
							Privileges: []string{"SELECT"},
							Level:      "global",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with table-level grants", func() {
			It("should apply table grants", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE ROLE IF NOT EXISTS `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("GRANT SELECT ON `testdb`\\.`users` TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Table:      "users",
							Privileges: []string{"SELECT"},
							Level:      "table",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with GRANT OPTION", func() {
			It("should apply grants with GRANT OPTION", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE ROLE IF NOT EXISTS `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("GRANT SELECT ON `testdb`\\.\\* TO `testrole` WITH GRANT OPTION").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
					Grants: []types.GrantOptions{
						{
							Database:        "testdb",
							Privileges:      []string{"SELECT"},
							Level:           "database",
							WithGrantOption: true,
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when create role fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE ROLE IF NOT EXISTS `testrole`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create role"))
			})
		})
	})

	Describe("DropRole", func() {
		Context("when dropping role", func() {
			It("should execute DROP ROLE query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("DROP ROLE IF EXISTS `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropRole(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.DropRole(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("DROP ROLE IF EXISTS `testrole`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.DropRole(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop role"))
			})
		})
	})

	Describe("RoleExists", func() {
		Context("when role exists", func() {
			It("should return true", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.roles WHERE name = \\?").
					WithArgs("testrole").
					WillReturnRows(rows)

				exists, err := adapter.RoleExists(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when role does not exist", func() {
			It("should return false", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(0))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.roles WHERE name = \\?").
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := adapter.RoleExists(ctx, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				exists, err := adapter.RoleExists(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})
	})

	Describe("UpdateRole", func() {
		Context("with new grants", func() {
			It("should apply grants", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("GRANT SELECT ON `testdb`\\.\\* TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT"},
							Level:      "database",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with add grants", func() {
			It("should add grants", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("GRANT INSERT ON `testdb`\\.\\* TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					AddGrants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"INSERT"},
							Level:      "database",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with remove grants", func() {
			It("should revoke grants", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("REVOKE DELETE ON `testdb`\\.\\* FROM `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					RemoveGrants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"DELETE"},
							Level:      "database",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with combined operations", func() {
			It("should apply, add, and remove in sequence", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("GRANT SELECT ON `testdb`\\.\\* TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("GRANT INSERT ON `testdb`\\.\\* TO `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("REVOKE DELETE ON `testdb`\\.\\* FROM `testrole`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{Database: "testdb", Privileges: []string{"SELECT"}, Level: "database"},
					},
					AddGrants: []types.GrantOptions{
						{Database: "testdb", Privileges: []string{"INSERT"}, Level: "database"},
					},
					RemoveGrants: []types.GrantOptions{
						{Database: "testdb", Privileges: []string{"DELETE"}, Level: "database"},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})

	Describe("GetRoleInfo", func() {
		Context("when role exists", func() {
			It("should return role info", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.roles WHERE name = \\?").
					WithArgs("testrole").
					WillReturnRows(rows)

				info, err := adapter.GetRoleInfo(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testrole"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when role does not exist", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(0))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.roles WHERE name = \\?").
					WithArgs("nonexistent").
					WillReturnRows(rows)

				info, err := adapter.GetRoleInfo(ctx, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
				Expect(info).To(BeNil())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				info, err := adapter.GetRoleInfo(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})
	})

	Describe("applyGrantToRole error handling", func() {
		It("returns error when database is required but not provided", func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = db

			err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
				Privileges: []string{"SELECT"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database is required"))
		})
	})

	Describe("revokeGrantFromRole error handling", func() {
		It("returns error when database is required but not provided", func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = db

			err := adapter.revokeGrantFromRole(ctx, "testrole", types.GrantOptions{
				Privileges: []string{"SELECT"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database is required"))
		})
	})
})
