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

var _ = Describe("Database Operations", func() {
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

	Describe("CreateDatabase", func() {
		Context("with defaults", func() {
			It("should execute CREATE DATABASE query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: "testdb"})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with engine", func() {
			It("should include ENGINE in query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb` ENGINE = Atomic").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
					Name:   "testdb",
					Engine: "Atomic",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("with comment", func() {
			It("should include COMMENT in query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb` COMMENT 'managed by operator'").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
					Name:    "testdb",
					Comment: "managed by operator",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: "testdb"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error with database name", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: "testdb"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create database"))
				Expect(err.Error()).To(ContainSubstring("testdb"))
			})
		})
	})

	Describe("DropDatabase", func() {
		Context("when dropping database", func() {
			It("should execute DROP DATABASE query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", types.DropDatabaseOptions{Force: false})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when force dropping", func() {
			It("should kill queries before dropping", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("KILL QUERY WHERE current_database").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", types.DropDatabaseOptions{Force: true})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should continue if kill query fails", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("KILL QUERY WHERE current_database").
					WillReturnError(fmt.Errorf("permission denied"))
				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", types.DropDatabaseOptions{Force: true})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.DropDatabase(ctx, "testdb", types.DropDatabaseOptions{Force: false})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when drop query fails", func() {
			It("should return error with database name", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.DropDatabase(ctx, "testdb", types.DropDatabaseOptions{Force: false})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop database"))
				Expect(err.Error()).To(ContainSubstring("testdb"))
			})
		})
	})

	Describe("DatabaseExists", func() {
		Context("when database exists", func() {
			It("should return true", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.databases WHERE name = \\?").
					WithArgs("testdb").
					WillReturnRows(rows)

				exists, err := adapter.DatabaseExists(ctx, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when database does not exist", func() {
			It("should return false", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(0))
				mock.ExpectQuery("SELECT count\\(\\) FROM system.databases WHERE name = \\?").
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := adapter.DatabaseExists(ctx, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				exists, err := adapter.DatabaseExists(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})

		Context("when query fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectQuery("SELECT count\\(\\) FROM system.databases WHERE name = \\?").
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("connection error"))

				exists, err := adapter.DatabaseExists(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check database existence"))
				Expect(exists).To(BeFalse())
			})
		})
	})

	Describe("GetDatabaseInfo", func() {
		Context("when database exists", func() {
			It("should return database info with size", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				dbRows := sqlmock.NewRows([]string{"name", "engine", "comment"}).
					AddRow("testdb", "Atomic", "test database")
				mock.ExpectQuery("SELECT name, engine, comment.*FROM system.databases.*WHERE name = \\?").
					WithArgs("testdb").
					WillReturnRows(dbRows)

				sizeRows := sqlmock.NewRows([]string{"size"}).AddRow(int64(1048576))
				mock.ExpectQuery("SELECT COALESCE\\(sum\\(bytes_on_disk\\), 0\\).*FROM system.parts.*WHERE database = \\?").
					WithArgs("testdb").
					WillReturnRows(sizeRows)

				info, err := adapter.GetDatabaseInfo(ctx, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testdb"))
				Expect(info.Engine).To(Equal("Atomic"))
				Expect(info.Comment).To(Equal("test database"))
				Expect(info.SizeBytes).To(Equal(int64(1048576)))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should set size to 0 if size query fails", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				dbRows := sqlmock.NewRows([]string{"name", "engine", "comment"}).
					AddRow("testdb", "Atomic", "")
				mock.ExpectQuery("SELECT name, engine, comment.*FROM system.databases.*WHERE name = \\?").
					WithArgs("testdb").
					WillReturnRows(dbRows)

				mock.ExpectQuery("SELECT COALESCE\\(sum\\(bytes_on_disk\\), 0\\).*FROM system.parts.*WHERE database = \\?").
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("access denied"))

				info, err := adapter.GetDatabaseInfo(ctx, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.SizeBytes).To(Equal(int64(0)))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				info, err := adapter.GetDatabaseInfo(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})

		Context("when database does not exist", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectQuery("SELECT name, engine, comment.*FROM system.databases.*WHERE name = \\?").
					WithArgs("nonexistent").
					WillReturnError(sql.ErrNoRows)

				info, err := adapter.GetDatabaseInfo(ctx, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get database info"))
				Expect(info).To(BeNil())
			})
		})
	})

	Describe("TransferDatabaseOwnership", func() {
		It("is a no-op", func() {
			adapter = NewAdapter(defaultConfig())
			err := adapter.TransferDatabaseOwnership(ctx, "testdb", "newowner")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("UpdateDatabase", func() {
		Context("when updating comment", func() {
			It("should execute ALTER DATABASE with COMMENT", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("ALTER DATABASE `testdb` MODIFY COMMENT 'updated comment'").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.UpdateDatabase(ctx, "testdb", types.UpdateDatabaseOptions{
					Comment: "updated comment",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when no update options provided", func() {
			It("should return nil without executing query", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				err := adapter.UpdateDatabase(ctx, "testdb", types.UpdateDatabaseOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())

				err := adapter.UpdateDatabase(ctx, "testdb", types.UpdateDatabaseOptions{Comment: "test"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(defaultConfig())
				adapter.db = db

				mock.ExpectExec("ALTER DATABASE `testdb` MODIFY COMMENT 'test'").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.UpdateDatabase(ctx, "testdb", types.UpdateDatabaseOptions{Comment: "test"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update database"))
			})
		})
	})
})
