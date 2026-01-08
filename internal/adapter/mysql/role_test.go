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
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/mysql/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Role Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(testutil.DefaultConnectionConfig())
	})

	Describe("CreateRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when connected", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("creates native role (MySQL 8.0+)", func() {
				mock.ExpectExec(`CREATE ROLE IF NOT EXISTS 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("creates role as locked user (legacy)", func() {
				mock.ExpectExec(`CREATE USER IF NOT EXISTS 'testrole'@'%' ACCOUNT LOCK`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: false,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("applies grants to native role", func() {
				mock.ExpectExec(`CREATE ROLE IF NOT EXISTS 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`GRANT SELECT, INSERT ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: true,
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

			It("applies grants to legacy role", func() {
				mock.ExpectExec(`CREATE USER IF NOT EXISTS 'testrole'@'%' ACCOUNT LOCK`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`GRANT SELECT ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: false,
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

			It("applies global grants to role", func() {
				mock.ExpectExec(`CREATE ROLE IF NOT EXISTS 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`GRANT SELECT, INSERT ON \*\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: true,
					Grants: []types.GrantOptions{
						{
							Privileges: []string{"SELECT", "INSERT"},
							Level:      "global",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("applies table grants to role", func() {
				mock.ExpectExec(`CREATE ROLE IF NOT EXISTS 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`GRANT SELECT ON ` + "`testdb`" + `\.` + "`users`" + ` TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: true,
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

			It("applies grants with GRANT OPTION", func() {
				mock.ExpectExec(`CREATE ROLE IF NOT EXISTS 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(`GRANT SELECT ON ` + "`testdb`" + `\.\* TO 'testrole' WITH GRANT OPTION`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: true,
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

			It("handles create role failure", func() {
				mock.ExpectExec(`CREATE ROLE IF NOT EXISTS 'testrole'`).
					WillReturnError(fmt.Errorf("role already exists"))

				err := adapter.CreateRole(ctx, types.CreateRoleOptions{
					RoleName:       "testrole",
					UseNativeRoles: true,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create role"))
			})
		})
	})

	Describe("DropRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.DropRole(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when connected", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("drops native role", func() {
				mock.ExpectExec(`DROP ROLE IF EXISTS 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropRole(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("falls back to dropping user when DROP ROLE fails", func() {
				mock.ExpectExec(`DROP ROLE IF EXISTS 'testrole'`).
					WillReturnError(fmt.Errorf("unknown command"))
				mock.ExpectExec(`DROP USER IF EXISTS 'testrole'@'%'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropRole(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("returns error when both DROP ROLE and DROP USER fail", func() {
				mock.ExpectExec(`DROP ROLE IF EXISTS 'testrole'`).
					WillReturnError(fmt.Errorf("unknown command"))
				mock.ExpectExec(`DROP USER IF EXISTS 'testrole'@'%'`).
					WillReturnError(fmt.Errorf("user does not exist"))

				err := adapter.DropRole(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop role"))
			})
		})
	})

	Describe("RoleExists", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				exists, err := adapter.RoleExists(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})

		Context("when connected", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("returns true when role exists (locked user)", func() {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \? AND account_locked = 'Y'\)`).
					WithArgs("testrole").
					WillReturnRows(rows)

				exists, err := adapter.RoleExists(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("returns false when role does not exist", func() {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \? AND account_locked = 'Y'\)`).
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := adapter.RoleExists(ctx, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("falls back to alternative query on error", func() {
				// First query fails
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \? AND account_locked = 'Y'\)`).
					WithArgs("testrole").
					WillReturnError(fmt.Errorf("column not found"))

				// Fallback query succeeds
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \?\)`).
					WithArgs("testrole").
					WillReturnRows(rows)

				exists, err := adapter.RoleExists(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("returns error when both queries fail", func() {
				// First query fails
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \? AND account_locked = 'Y'\)`).
					WithArgs("testrole").
					WillReturnError(fmt.Errorf("column not found"))

				// Fallback query also fails
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \?\)`).
					WithArgs("testrole").
					WillReturnError(fmt.Errorf("database error"))

				exists, err := adapter.RoleExists(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check role existence"))
				Expect(exists).To(BeFalse())
			})
		})
	})

	Describe("UpdateRole", func() {
		Context("when not connected", func() {
			It("should return error when applying grants", func() {
				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT"},
							Level:      "database",
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when connected", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("applies new grants", func() {
				mock.ExpectExec(`GRANT SELECT, INSERT ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
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

			It("adds grants", func() {
				mock.ExpectExec(`GRANT UPDATE ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					AddGrants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"UPDATE"},
							Level:      "database",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("removes grants", func() {
				mock.ExpectExec(`REVOKE DELETE ON ` + "`testdb`" + `\.\* FROM 'testrole'`).
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

			It("applies grants, adds, and removes in sequence", func() {
				// Apply new grants
				mock.ExpectExec(`GRANT SELECT ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				// Add grants
				mock.ExpectExec(`GRANT INSERT ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))
				// Remove grants
				mock.ExpectExec(`REVOKE DELETE ON ` + "`testdb`" + `\.\* FROM 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT"},
							Level:      "database",
						},
					},
					AddGrants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"INSERT"},
							Level:      "database",
						},
					},
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

			It("applies global level grants", func() {
				mock.ExpectExec(`GRANT PROCESS, REPLICATION CLIENT ON \*\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{
							Privileges: []string{"PROCESS", "REPLICATION CLIENT"},
							Level:      "global",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("applies table level grants", func() {
				mock.ExpectExec(`GRANT SELECT, UPDATE ON ` + "`testdb`" + `\.` + "`users`" + ` TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Table:      "users",
							Privileges: []string{"SELECT", "UPDATE"},
							Level:      "table",
						},
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("handles grant failure", func() {
				mock.ExpectExec(`GRANT SELECT ON ` + "`testdb`" + `\.\* TO 'testrole'`).
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Grants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT"},
							Level:      "database",
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to grant"))
			})

			It("handles revoke failure", func() {
				mock.ExpectExec(`REVOKE SELECT ON ` + "`testdb`" + `\.\* FROM 'testrole'`).
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					RemoveGrants: []types.GrantOptions{
						{
							Database:   "testdb",
							Privileges: []string{"SELECT"},
							Level:      "database",
						},
					},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to revoke"))
			})
		})
	})

	Describe("GetRoleInfo", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				info, err := adapter.GetRoleInfo(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})

		Context("when connected", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("returns role info", func() {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \?\)`).
					WithArgs("testrole").
					WillReturnRows(rows)

				info, err := adapter.GetRoleInfo(ctx, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testrole"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("returns error when role not found", func() {
				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \?\)`).
					WithArgs("nonexistent").
					WillReturnRows(rows)

				info, err := adapter.GetRoleInfo(ctx, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
				Expect(info).To(BeNil())
			})

			It("returns error on query failure", func() {
				mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM mysql\.user WHERE User = \?\)`).
					WithArgs("testrole").
					WillReturnError(fmt.Errorf("database error"))

				info, err := adapter.GetRoleInfo(ctx, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
				Expect(info).To(BeNil())
			})
		})
	})

	Describe("applyGrantToRole", func() {
		Context("grant levels", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("should handle global level grants", func() {
				mock.ExpectExec(`GRANT SELECT, INSERT ON \*\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
					Privileges: []string{"SELECT", "INSERT"},
					Level:      "global",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should handle database level grants", func() {
				mock.ExpectExec(`GRANT ALL PRIVILEGES ON ` + "`mydb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
					Database:   "mydb",
					Privileges: []string{"ALL PRIVILEGES"},
					Level:      "database",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should handle table level grants", func() {
				mock.ExpectExec(`GRANT SELECT, UPDATE ON ` + "`mydb`" + `\.` + "`mytable`" + ` TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
					Database:   "mydb",
					Table:      "mytable",
					Privileges: []string{"SELECT", "UPDATE"},
					Level:      "table",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should default to database level when no level specified", func() {
				mock.ExpectExec(`GRANT SELECT ON ` + "`defaultdb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
					Database:   "defaultdb",
					Privileges: []string{"SELECT"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error when database is required but not provided", func() {
				err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
					Privileges: []string{"SELECT"},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database is required"))
			})

			It("should handle empty privileges with USAGE", func() {
				mock.ExpectExec(`GRANT USAGE ON ` + "`mydb`" + `\.\* TO 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.applyGrantToRole(ctx, "testrole", types.GrantOptions{
					Database:   "mydb",
					Privileges: []string{},
					Level:      "database",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})

	Describe("revokeGrantFromRole", func() {
		Context("revoke levels", func() {
			var (
				mock sqlmock.Sqlmock
			)

			BeforeEach(func() {
				db, m, err := testutil.NewMockDB()
				Expect(err).NotTo(HaveOccurred())
				mock = m
				adapter.db = db
			})

			AfterEach(func() {
				if adapter.db != nil {
					_ = adapter.db.Close()
				}
			})

			It("should handle global level revokes", func() {
				mock.ExpectExec(`REVOKE SELECT ON \*\.\* FROM 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.revokeGrantFromRole(ctx, "testrole", types.GrantOptions{
					Privileges: []string{"SELECT"},
					Level:      "global",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should handle database level revokes", func() {
				mock.ExpectExec(`REVOKE INSERT ON ` + "`mydb`" + `\.\* FROM 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.revokeGrantFromRole(ctx, "testrole", types.GrantOptions{
					Database:   "mydb",
					Privileges: []string{"INSERT"},
					Level:      "database",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should handle table level revokes", func() {
				mock.ExpectExec(`REVOKE DELETE ON ` + "`mydb`" + `\.` + "`mytable`" + ` FROM 'testrole'`).
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.revokeGrantFromRole(ctx, "testrole", types.GrantOptions{
					Database:   "mydb",
					Table:      "mytable",
					Privileges: []string{"DELETE"},
					Level:      "table",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error when database is required but not provided", func() {
				err := adapter.revokeGrantFromRole(ctx, "testrole", types.GrantOptions{
					Privileges: []string{"SELECT"},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("database is required"))
			})
		})
	})

	Describe("formatPrivileges", func() {
		It("should return USAGE for empty privileges", func() {
			result := formatPrivileges([]string{})
			Expect(result).To(Equal("USAGE"))
		})

		It("should return USAGE for nil privileges", func() {
			result := formatPrivileges(nil)
			Expect(result).To(Equal("USAGE"))
		})

		It("should join multiple privileges", func() {
			result := formatPrivileges([]string{"SELECT", "INSERT", "UPDATE"})
			Expect(result).To(Equal("SELECT, INSERT, UPDATE"))
		})

		It("should return single privilege", func() {
			result := formatPrivileges([]string{"ALL PRIVILEGES"})
			Expect(result).To(Equal("ALL PRIVILEGES"))
		})
	})
})
