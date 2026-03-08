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

var _ = Describe("User Operations", func() {
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

	Describe("CreateUser", func() {
		Context("with basic options", func() {
			It("should execute CREATE USER query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE USER IF NOT EXISTS `appuser` IDENTIFIED BY").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username: "appuser",
					Password: "s3cret",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with host restrictions", func() {
			It("should apply HOST clause via ALTER USER", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE USER IF NOT EXISTS `appuser` IDENTIFIED BY").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("ALTER USER.*HOST").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username:         "appuser",
					Password:         "s3cret",
					HostRestrictions: []string{"IP '10.0.0.0/8'"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with default database", func() {
			It("should set default database via ALTER USER", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE USER IF NOT EXISTS `appuser` IDENTIFIED BY").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("ALTER USER.*DEFAULT DATABASE").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username:        "appuser",
					Password:        "s3cret",
					DefaultDatabase: "mydb",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with default role", func() {
			It("should set default role via ALTER USER", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE USER IF NOT EXISTS `appuser` IDENTIFIED BY").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("ALTER USER.*DEFAULT ROLE").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username:    "appuser",
					Password:    "s3cret",
					DefaultRole: "reader",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username: "appuser",
					Password: "s3cret",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE USER IF NOT EXISTS `appuser`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username: "appuser",
					Password: "s3cret",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create user"))
			})
		})
	})

	Describe("DropUser", func() {
		Context("when dropping user", func() {
			It("should execute DROP USER query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("DROP USER IF EXISTS `appuser`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropUser(ctx, "appuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.DropUser(ctx, "appuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("DROP USER IF EXISTS `appuser`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.DropUser(ctx, "appuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop user"))
			})
		})
	})

	Describe("UserExists", func() {
		Context("when user exists", func() {
			It("should return true", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.users WHERE name = \\?").
					WithArgs("appuser").
					WillReturnRows(rows)

				exists, err := adapter.UserExists(ctx, "appuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when user does not exist", func() {
			It("should return false", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(0))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.users WHERE name = \\?").
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := adapter.UserExists(ctx, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				exists, err := adapter.UserExists(ctx, "appuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})
	})

	Describe("UpdatePassword", func() {
		It("should execute ALTER USER with password", func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = db

			mock.ExpectExec("ALTER USER `appuser` IDENTIFIED BY").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err := adapter.UpdatePassword(ctx, "appuser", "newpassword")
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error when not connected", func() {
			adapter = NewAdapter(defaultConfig())

			err := adapter.UpdatePassword(ctx, "appuser", "newpassword")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("returns error when query fails", func() {
			adapter = NewAdapter(defaultConfig())
			adapter.db = db

			mock.ExpectExec("ALTER USER `appuser` IDENTIFIED BY").
				WillReturnError(fmt.Errorf("access denied"))

			err := adapter.UpdatePassword(ctx, "appuser", "newpassword")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update password"))
		})
	})

	Describe("GetUserInfo", func() {
		Context("when user exists", func() {
			It("should return user info", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.users WHERE name = \\?").
					WithArgs("appuser").
					WillReturnRows(rows)

				info, err := adapter.GetUserInfo(ctx, "appuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Username).To(Equal("appuser"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when user does not exist", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(0))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.users WHERE name = \\?").
					WithArgs("nonexistent").
					WillReturnRows(rows)

				info, err := adapter.GetUserInfo(ctx, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
				Expect(info).To(BeNil())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				info, err := adapter.GetUserInfo(ctx, "appuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})
	})

	Describe("UpdateUser", func() {
		It("is a no-op", func() {
			adapter = NewAdapter(defaultConfig())
			err := adapter.UpdateUser(ctx, "appuser", types.UpdateUserOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetOwnedObjects", func() {
		It("returns nil (no ownership model)", func() {
			adapter = NewAdapter(defaultConfig())
			objects, err := adapter.GetOwnedObjects(ctx, "appuser")
			Expect(err).NotTo(HaveOccurred())
			Expect(objects).To(BeNil())
		})
	})
})
