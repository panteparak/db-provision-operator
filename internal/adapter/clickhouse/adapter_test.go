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

func defaultConfig() types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:     "localhost",
		Port:     9000,
		Database: "default",
		Username: "admin",
		Password: "admin_password",
	}
}

var _ = Describe("Adapter", func() {
	Describe("NewAdapter", func() {
		It("creates a new adapter with config", func() {
			config := defaultConfig()
			adapter := NewAdapter(config)
			Expect(adapter).NotTo(BeNil())
			Expect(adapter.config.Host).To(Equal("localhost"))
			Expect(adapter.config.Port).To(Equal(int32(9000)))
		})
	})

	Describe("buildDSN", func() {
		It("builds basic DSN", func() {
			adapter := NewAdapter(defaultConfig())
			dsn := adapter.buildDSN()
			Expect(dsn).To(Equal("clickhouse://admin:admin_password@localhost:9000/default"))
		})

		It("includes dial timeout", func() {
			config := defaultConfig()
			config.ClickHouseDialTimeout = "10s"
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("dial_timeout=10s"))
		})

		It("includes read timeout", func() {
			config := defaultConfig()
			config.ClickHouseReadTimeout = "30s"
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("read_timeout=30s"))
		})

		It("includes debug mode", func() {
			config := defaultConfig()
			config.ClickHouseDebug = true
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("debug=true"))
		})

		It("uses default database when empty", func() {
			config := defaultConfig()
			config.Database = ""
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("/default"))
		})

		It("includes TLS params when enabled", func() {
			config := defaultConfig()
			config.TLSEnabled = true
			config.TLSMode = "verify-full"
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("secure=true"))
			Expect(dsn).To(ContainSubstring("skip_verify=false"))
		})

		It("sets skip_verify for require mode", func() {
			config := defaultConfig()
			config.TLSEnabled = true
			config.TLSMode = "require"
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("skip_verify=true"))
		})
	})

	Describe("Ping", func() {
		It("returns error when not connected", func() {
			adapter := NewAdapter(defaultConfig())
			err := adapter.Ping(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})
	})

	Describe("GetVersion", func() {
		It("returns error when not connected", func() {
			adapter := NewAdapter(defaultConfig())
			_, err := adapter.GetVersion(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("returns version when connected", func() {
			db, mock, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = db.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = db

			rows := sqlmock.NewRows([]string{"version()"}).AddRow("24.3.1.1234")
			mock.ExpectQuery("SELECT version\\(\\)").WillReturnRows(rows)

			version, err := adapter.GetVersion(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal("24.3.1.1234"))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})
	})

	Describe("Close", func() {
		It("does nothing when not connected", func() {
			adapter := NewAdapter(defaultConfig())
			err := adapter.Close()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("escapeIdentifier", func() {
		It("wraps identifier in backticks", func() {
			result := escapeIdentifier("tablename")
			Expect(result).To(Equal("`tablename`"))
		})

		It("escapes existing backticks by doubling them", func() {
			result := escapeIdentifier("table`name")
			Expect(result).To(Equal("`table``name`"))
		})

		It("handles empty string", func() {
			result := escapeIdentifier("")
			Expect(result).To(Equal("``"))
		})
	})

	Describe("escapeLiteral", func() {
		It("wraps literal in single quotes", func() {
			result := escapeLiteral("value")
			Expect(result).To(Equal("'value'"))
		})

		It("escapes single quotes by doubling them", func() {
			result := escapeLiteral("val'ue")
			Expect(result).To(Equal("'val''ue'"))
		})

		It("escapes backslashes", func() {
			result := escapeLiteral("path\\to\\file")
			Expect(result).To(Equal("'path\\\\to\\\\file'"))
		})

		It("escapes both quotes and backslashes", func() {
			result := escapeLiteral("it's a \\path")
			Expect(result).To(Equal("'it''s a \\\\path'"))
		})
	})

	Describe("SetUserAttribute", func() {
		It("is a no-op", func() {
			adapter := NewAdapter(defaultConfig())
			err := adapter.SetUserAttribute(context.Background(), "user", "key", "val")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("GetUserAttribute", func() {
		It("returns empty string", func() {
			adapter := NewAdapter(defaultConfig())
			val, err := adapter.GetUserAttribute(context.Background(), "user", "key")
			Expect(err).NotTo(HaveOccurred())
			Expect(val).To(Equal(""))
		})
	})

	Describe("SetResourceComment", func() {
		It("returns error when not connected", func() {
			adapter := NewAdapter(defaultConfig())
			err := adapter.SetResourceComment(context.Background(), "database", "mydb", "managed")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("creates metadata table and inserts entry", func() {
			db, mock, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = db.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = db

			// Expect CREATE TABLE
			mock.ExpectExec("CREATE TABLE IF NOT EXISTS _dbops_metadata").
				WillReturnResult(sqlmock.NewResult(0, 0))
			// Expect DELETE (upsert pattern)
			mock.ExpectExec("ALTER TABLE _dbops_metadata DELETE").
				WillReturnResult(sqlmock.NewResult(0, 0))
			// Expect INSERT
			mock.ExpectExec("INSERT INTO _dbops_metadata").
				WillReturnResult(sqlmock.NewResult(0, 0))

			err = adapter.SetResourceComment(context.Background(), "database", "mydb", "managed")
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})
	})

	Describe("GetResourceComment", func() {
		It("returns error when not connected", func() {
			adapter := NewAdapter(defaultConfig())
			_, err := adapter.GetResourceComment(context.Background(), "database", "mydb")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
		})

		It("returns empty when metadata table does not exist", func() {
			db, mock, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = db.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = db

			rows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(0))
			mock.ExpectQuery("SELECT count\\(\\) FROM system.tables").
				WillReturnRows(rows)

			comment, err := adapter.GetResourceComment(context.Background(), "database", "mydb")
			Expect(err).NotTo(HaveOccurred())
			Expect(comment).To(Equal(""))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns metadata when table exists and entry found", func() {
			db, mock, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = db.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = db

			// Table exists
			countRows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
			mock.ExpectQuery("SELECT count\\(\\) FROM system.tables").
				WillReturnRows(countRows)

			// Entry found
			metadataRows := sqlmock.NewRows([]string{"metadata"}).AddRow("managed-by-operator")
			mock.ExpectQuery("SELECT metadata FROM _dbops_metadata FINAL").
				WillReturnRows(metadataRows)

			comment, err := adapter.GetResourceComment(context.Background(), "database", "mydb")
			Expect(err).NotTo(HaveOccurred())
			Expect(comment).To(Equal("managed-by-operator"))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns empty when entry not found", func() {
			db, mock, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = db.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = db

			// Table exists
			countRows := sqlmock.NewRows([]string{"count"}).AddRow(uint64(1))
			mock.ExpectQuery("SELECT count\\(\\) FROM system.tables").
				WillReturnRows(countRows)

			// No entry
			mock.ExpectQuery("SELECT metadata FROM _dbops_metadata FINAL").
				WillReturnError(sql.ErrNoRows)

			comment, err := adapter.GetResourceComment(context.Background(), "database", "nonexistent")
			Expect(err).NotTo(HaveOccurred())
			Expect(comment).To(Equal(""))
		})
	})

	Describe("getDB", func() {
		It("returns error when not connected", func() {
			adapter := NewAdapter(defaultConfig())
			db, err := adapter.getDB()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
			Expect(db).To(BeNil())
		})

		It("returns db when connected", func() {
			mockDB, _, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = mockDB.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = mockDB

			db, err := adapter.getDB()
			Expect(err).NotTo(HaveOccurred())
			Expect(db).NotTo(BeNil())
		})
	})

	Describe("Connection pool configuration", func() {
		It("uses custom max open conns from config", func() {
			config := defaultConfig()
			config.ClickHouseMaxOpenConns = 50
			adapter := NewAdapter(config)
			Expect(adapter.config.ClickHouseMaxOpenConns).To(Equal(int32(50)))
		})
	})

	Describe("buildDSN with multiple params", func() {
		It("joins params with &", func() {
			config := defaultConfig()
			config.ClickHouseDialTimeout = "5s"
			config.ClickHouseReadTimeout = "10s"
			config.ClickHouseDebug = true
			adapter := NewAdapter(config)
			dsn := adapter.buildDSN()
			Expect(dsn).To(ContainSubstring("?"))
			Expect(dsn).To(ContainSubstring("dial_timeout=5s"))
			Expect(dsn).To(ContainSubstring("read_timeout=10s"))
			Expect(dsn).To(ContainSubstring("debug=true"))
			Expect(dsn).To(ContainSubstring("&"))
		})
	})

	Describe("GetVersion error handling", func() {
		It("returns error when query fails", func() {
			db, mock, err := sqlmock.New()
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = db.Close() }()

			adapter := NewAdapter(defaultConfig())
			adapter.db = db

			mock.ExpectQuery("SELECT version\\(\\)").
				WillReturnError(fmt.Errorf("connection lost"))

			_, vErr := adapter.GetVersion(context.Background())
			Expect(vErr).To(HaveOccurred())
			Expect(vErr.Error()).To(ContainSubstring("failed to get version"))
		})
	})
})
