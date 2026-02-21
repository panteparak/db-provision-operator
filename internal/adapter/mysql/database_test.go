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
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/mysql/testutil"
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
		Context("when creating database with defaults", func() {
			It("should execute CREATE DATABASE query", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.CreateDatabaseOpts("testdb")
				err := adapter.CreateDatabase(ctx, opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when creating database with charset", func() {
			It("should include CHARACTER SET in query", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb` CHARACTER SET `utf8mb4`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.CreateDatabaseOptsWithCharset("testdb", "utf8mb4")
				err := adapter.CreateDatabase(ctx, opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when creating database with collation", func() {
			It("should include COLLATE in query", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb` COLLATE `utf8mb4_unicode_ci`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.CreateDatabaseOptsWithCollation("testdb", "utf8mb4_unicode_ci")
				err := adapter.CreateDatabase(ctx, opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when creating database with charset and collation", func() {
			It("should include both CHARACTER SET and COLLATE in query", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb` CHARACTER SET `utf8mb4` COLLATE `utf8mb4_unicode_ci`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.CreateDatabaseOptsWithCharsetAndCollation("testdb", "utf8mb4", "utf8mb4_unicode_ci")
				err := adapter.CreateDatabase(ctx, opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				// adapter.db is nil (not connected)

				opts := testutil.CreateDatabaseOpts("testdb")
				err := adapter.CreateDatabase(ctx, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error with database name", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("CREATE DATABASE IF NOT EXISTS `testdb`").
					WillReturnError(fmt.Errorf("access denied"))

				opts := testutil.CreateDatabaseOpts("testdb")
				err := adapter.CreateDatabase(ctx, opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create database"))
				Expect(err.Error()).To(ContainSubstring("testdb"))
			})
		})
	})

	Describe("DropDatabase", func() {
		Context("when dropping database", func() {
			It("should execute DROP DATABASE query", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", testutil.DropDatabaseOpts(false))
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when force dropping with killing connections", func() {
			It("should query processlist and kill connections before dropping", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// Expect query for active connections
				rows := sqlmock.NewRows([]string{"kill_cmd"}).
					AddRow("KILL 123;").
					AddRow("KILL 456;")
				mock.ExpectQuery("SELECT CONCAT\\('KILL ', id, ';'\\).*FROM information_schema.processlist.*WHERE db = \\?").
					WithArgs("testdb").
					WillReturnRows(rows)

				// Expect kill commands
				mock.ExpectExec("KILL 123;").
					WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec("KILL 456;").
					WillReturnResult(sqlmock.NewResult(0, 0))

				// Expect drop database
				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", testutil.DropDatabaseOpts(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should continue if processlist query fails", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// Expect query for active connections to fail
				mock.ExpectQuery("SELECT CONCAT\\('KILL ', id, ';'\\).*FROM information_schema.processlist.*WHERE db = \\?").
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("permission denied"))

				// Should still drop database
				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", testutil.DropDatabaseOpts(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should continue if no active connections found", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// Expect empty result set
				rows := sqlmock.NewRows([]string{"kill_cmd"})
				mock.ExpectQuery("SELECT CONCAT\\('KILL ', id, ';'\\).*FROM information_schema.processlist.*WHERE db = \\?").
					WithArgs("testdb").
					WillReturnRows(rows)

				// Expect drop database
				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				err := adapter.DropDatabase(ctx, "testdb", testutil.DropDatabaseOpts(true))
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				// adapter.db is nil (not connected)

				err := adapter.DropDatabase(ctx, "testdb", testutil.DropDatabaseOpts(false))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when drop query fails", func() {
			It("should return error with database name", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
					WillReturnError(fmt.Errorf("access denied"))

				err := adapter.DropDatabase(ctx, "testdb", testutil.DropDatabaseOpts(false))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop database"))
				Expect(err.Error()).To(ContainSubstring("testdb"))
			})
		})
	})

	Describe("DatabaseExists", func() {
		Context("when database exists", func() {
			It("should return true", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM information_schema.schemata WHERE schema_name = \\?\\)").
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
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM information_schema.schemata WHERE schema_name = \\?\\)").
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
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				// adapter.db is nil (not connected)

				exists, err := adapter.DatabaseExists(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})

		Context("when query fails", func() {
			It("should return error", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM information_schema.schemata WHERE schema_name = \\?\\)").
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
			It("should return database info", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// First query: get database schema info
				dbRows := sqlmock.NewRows([]string{"schema_name", "default_character_set_name", "default_collation_name"}).
					AddRow("testdb", "utf8mb4", "utf8mb4_unicode_ci")
				mock.ExpectQuery("SELECT schema_name, default_character_set_name, default_collation_name.*FROM information_schema.schemata.*WHERE schema_name = \\?").
					WithArgs("testdb").
					WillReturnRows(dbRows)

				// Second query: get database size
				sizeRows := sqlmock.NewRows([]string{"size"}).AddRow(int64(1048576))
				mock.ExpectQuery("SELECT COALESCE\\(SUM\\(data_length \\+ index_length\\), 0\\).*FROM information_schema.tables.*WHERE table_schema = \\?").
					WithArgs("testdb").
					WillReturnRows(sizeRows)

				info, err := adapter.GetDatabaseInfo(ctx, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testdb"))
				Expect(info.Charset).To(Equal("utf8mb4"))
				Expect(info.Collation).To(Equal("utf8mb4_unicode_ci"))
				Expect(info.SizeBytes).To(Equal(int64(1048576)))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when database info includes size", func() {
			It("should return size in bytes", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// First query: get database schema info
				dbRows := sqlmock.NewRows([]string{"schema_name", "default_character_set_name", "default_collation_name"}).
					AddRow("largedb", "utf8mb4", "utf8mb4_unicode_ci")
				mock.ExpectQuery("SELECT schema_name, default_character_set_name, default_collation_name.*FROM information_schema.schemata.*WHERE schema_name = \\?").
					WithArgs("largedb").
					WillReturnRows(dbRows)

				// Second query: get database size (10GB)
				sizeRows := sqlmock.NewRows([]string{"size"}).AddRow(int64(10737418240))
				mock.ExpectQuery("SELECT COALESCE\\(SUM\\(data_length \\+ index_length\\), 0\\).*FROM information_schema.tables.*WHERE table_schema = \\?").
					WithArgs("largedb").
					WillReturnRows(sizeRows)

				info, err := adapter.GetDatabaseInfo(ctx, "largedb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.SizeBytes).To(Equal(int64(10737418240)))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should set size to 0 if size query fails", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// First query: get database schema info
				dbRows := sqlmock.NewRows([]string{"schema_name", "default_character_set_name", "default_collation_name"}).
					AddRow("testdb", "utf8mb4", "utf8mb4_unicode_ci")
				mock.ExpectQuery("SELECT schema_name, default_character_set_name, default_collation_name.*FROM information_schema.schemata.*WHERE schema_name = \\?").
					WithArgs("testdb").
					WillReturnRows(dbRows)

				// Second query: size query fails
				mock.ExpectQuery("SELECT COALESCE\\(SUM\\(data_length \\+ index_length\\), 0\\).*FROM information_schema.tables.*WHERE table_schema = \\?").
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
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				// adapter.db is nil (not connected)

				info, err := adapter.GetDatabaseInfo(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})

		Context("when database does not exist", func() {
			It("should return error", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectQuery("SELECT schema_name, default_character_set_name, default_collation_name.*FROM information_schema.schemata.*WHERE schema_name = \\?").
					WithArgs("nonexistent").
					WillReturnError(sql.ErrNoRows)

				info, err := adapter.GetDatabaseInfo(ctx, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get database info"))
				Expect(info).To(BeNil())
			})
		})
	})

	Describe("UpdateDatabase", func() {
		Context("when updating charset", func() {
			It("should execute ALTER DATABASE with CHARACTER SET", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("ALTER DATABASE `testdb` CHARACTER SET `utf8mb4`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.UpdateDatabaseOptsWithCharset("utf8mb4")
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when updating collation", func() {
			It("should execute ALTER DATABASE with COLLATE", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("ALTER DATABASE `testdb` COLLATE `utf8mb4_unicode_ci`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.UpdateDatabaseOptsWithCollation("utf8mb4_unicode_ci")
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when updating both charset and collation", func() {
			It("should execute ALTER DATABASE with both options", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("ALTER DATABASE `testdb` CHARACTER SET `utf8mb4` COLLATE `utf8mb4_unicode_ci`").
					WillReturnResult(sqlmock.NewResult(0, 0))

				opts := testutil.UpdateDatabaseOpts("utf8mb4", "utf8mb4_unicode_ci")
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when no update options provided", func() {
			It("should return nil without executing query", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				// No expectations set - no query should be executed
				opts := types.UpdateDatabaseOptions{}
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Context("when not connected", func() {
			It("should return error", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				// adapter.db is nil (not connected)

				opts := testutil.UpdateDatabaseOptsWithCharset("utf8mb4")
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("when query fails", func() {
			It("should return error with database name", func() {
				adapter = NewAdapter(testutil.DefaultConnectionConfig())
				adapter.db = db

				mock.ExpectExec("ALTER DATABASE `testdb` CHARACTER SET `utf8mb4`").
					WillReturnError(fmt.Errorf("access denied"))

				opts := testutil.UpdateDatabaseOptsWithCharset("utf8mb4")
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update database"))
				Expect(err.Error()).To(ContainSubstring("testdb"))
			})
		})
	})

	Describe("escapeIdentifier", func() {
		It("should wrap identifier in backticks", func() {
			result := escapeIdentifier("tablename")
			Expect(result).To(Equal("`tablename`"))
		})

		It("should escape existing backticks by doubling them", func() {
			result := escapeIdentifier("table`name")
			Expect(result).To(Equal("`table``name`"))
		})

		It("should handle multiple backticks", func() {
			result := escapeIdentifier("ta`ble`name")
			Expect(result).To(Equal("`ta``ble``name`"))
		})

		It("should handle empty string", func() {
			result := escapeIdentifier("")
			Expect(result).To(Equal("``"))
		})

		It("should handle special characters", func() {
			result := escapeIdentifier("test-db_123")
			Expect(result).To(Equal("`test-db_123`"))
		})
	})

	Describe("escapeLiteral", func() {
		It("should wrap literal in single quotes", func() {
			result := escapeLiteral("value")
			Expect(result).To(Equal("'value'"))
		})

		It("should escape existing single quotes by doubling them", func() {
			result := escapeLiteral("val'ue")
			Expect(result).To(Equal("'val''ue'"))
		})

		It("should handle multiple single quotes", func() {
			result := escapeLiteral("va'l'ue")
			Expect(result).To(Equal("'va''l''ue'"))
		})

		It("should handle empty string", func() {
			result := escapeLiteral("")
			Expect(result).To(Equal("''"))
		})

		It("should escape backslashes", func() {
			result := escapeLiteral("path\\to\\file")
			Expect(result).To(Equal("'path\\\\to\\\\file'"))
		})

		It("should escape both single quotes and backslashes", func() {
			result := escapeLiteral("it's a \\path")
			Expect(result).To(Equal("'it''s a \\\\path'"))
		})
	})
})
