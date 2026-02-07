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

package cockroachdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/db-provision-operator/internal/adapter/cockroachdb/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Database Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(testutil.DefaultConnectionConfig())
	})

	Describe("CreateDatabase", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: "testdb"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation", func() {
			It("should generate CREATE DATABASE with name only", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(Equal(`CREATE DATABASE "testdb"`))
			})

			It("should generate CREATE DATABASE with owner", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            "dbowner",
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`OWNER = "dbowner"`))
			})

			It("should generate CREATE DATABASE with encoding", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Encoding:         "UTF8",
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`ENCODING = 'UTF8'`))
			})

			It("should generate CREATE DATABASE with connection limit", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					ConnectionLimit:  100,
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`CONNECTION LIMIT = 100`))
			})

			It("should generate CREATE DATABASE with all CockroachDB-supported options", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            "dbowner",
					Encoding:         "UTF8",
					ConnectionLimit:  50,
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`OWNER = "dbowner"`))
				Expect(sql).To(ContainSubstring(`ENCODING = 'UTF8'`))
				Expect(sql).To(ContainSubstring(`CONNECTION LIMIT = 50`))
			})

			It("should NOT include TEMPLATE (unsupported in CockroachDB)", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Template:         "template1",
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).NotTo(ContainSubstring("TEMPLATE"))
			})

			It("should NOT include LC_COLLATE (unsupported in CockroachDB)", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					LCCollate:        "en_US.UTF-8",
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).NotTo(ContainSubstring("LC_COLLATE"))
			})

			It("should NOT include TABLESPACE (unsupported in CockroachDB)", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Tablespace:       "pg_default",
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).NotTo(ContainSubstring("TABLESPACE"))
			})

			It("should NOT include IS_TEMPLATE (unsupported in CockroachDB)", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					IsTemplate:       true,
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).NotTo(ContainSubstring("IS_TEMPLATE"))
			})

			It("should escape special characters in database name", func() {
				opts := types.CreateDatabaseOptions{
					Name:             `test"db`,
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`"test""db"`))
			})

			It("should escape special characters in owner name", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            `db"owner`,
					AllowConnections: true,
				}
				sql := buildCRDBCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`OWNER = "db""owner"`))
			})
		})
	})

	Describe("DropDatabase", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.DropDatabase(ctx, "testdb", types.DropDatabaseOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation", func() {
			It("should generate DROP DATABASE IF EXISTS with CASCADE", func() {
				sql := buildCRDBDropDatabaseSQL("testdb")
				Expect(sql).To(Equal(`DROP DATABASE IF EXISTS "testdb" CASCADE`))
			})

			It("should escape special characters in database name", func() {
				sql := buildCRDBDropDatabaseSQL(`test"db`)
				Expect(sql).To(Equal(`DROP DATABASE IF EXISTS "test""db" CASCADE`))
			})
		})
	})

	Describe("DatabaseExists", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				exists, err := adapter.DatabaseExists(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})

		Context("SQL Generation", func() {
			It("should use crdb_internal.databases instead of pg_database", func() {
				sql := buildCRDBDatabaseExistsSQL()
				Expect(sql).To(ContainSubstring("SELECT EXISTS"))
				Expect(sql).To(ContainSubstring("crdb_internal.databases"))
				Expect(sql).To(ContainSubstring("name = $1"))
				Expect(sql).NotTo(ContainSubstring("pg_database"))
			})
		})
	})

	Describe("GetDatabaseInfo", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				info, err := adapter.GetDatabaseInfo(ctx, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})

		Context("SQL Generation", func() {
			It("should use crdb_internal.databases for metadata", func() {
				sql := buildCRDBGetDatabaseInfoSQL()
				Expect(sql).To(ContainSubstring("crdb_internal.databases"))
				Expect(sql).To(ContainSubstring("pg_encoding_to_char"))
				Expect(sql).To(ContainSubstring("pg_catalog.pg_roles"))
			})

			It("should use crdb_internal.ranges for size estimation", func() {
				sql := buildCRDBGetDatabaseSizeSQL()
				Expect(sql).To(ContainSubstring("crdb_internal.ranges"))
				Expect(sql).To(ContainSubstring("range_size_bytes"))
				Expect(sql).To(ContainSubstring("database_name"))
			})
		})
	})

	Describe("UpdateDatabase", func() {
		Context("when not connected", func() {
			It("should return error when creating schema", func() {
				opts := types.UpdateDatabaseOptions{
					Schemas: []types.SchemaOptions{
						{Name: "app"},
					},
				}
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})

			It("should return error when setting default privileges", func() {
				opts := types.UpdateDatabaseOptions{
					DefaultPrivileges: []types.DefaultPrivilegeOptions{
						{
							Role:       "app_role",
							ObjectType: "tables",
							Privileges: []string{"SELECT"},
						},
					},
				}
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should succeed with empty options (no-op)", func() {
			// UpdateDatabase with no schemas or default privileges should succeed
			opts := types.UpdateDatabaseOptions{}
			err := adapter.UpdateDatabase(ctx, "testdb", opts)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("createSchema", func() {
		It("should generate CREATE SCHEMA IF NOT EXISTS", func() {
			schema := types.SchemaOptions{Name: "app"}
			sql := buildCRDBCreateSchemaSQL(schema)
			Expect(sql).To(Equal(`CREATE SCHEMA IF NOT EXISTS "app"`))
		})

		It("should include AUTHORIZATION when owner specified", func() {
			schema := types.SchemaOptions{Name: "app", Owner: "app_owner"}
			sql := buildCRDBCreateSchemaSQL(schema)
			Expect(sql).To(ContainSubstring(`CREATE SCHEMA IF NOT EXISTS "app"`))
			Expect(sql).To(ContainSubstring(`AUTHORIZATION "app_owner"`))
		})

		It("should escape special characters in schema name", func() {
			schema := types.SchemaOptions{Name: `app"schema`}
			sql := buildCRDBCreateSchemaSQL(schema)
			Expect(sql).To(ContainSubstring(`"app""schema"`))
		})

		It("should escape special characters in owner name", func() {
			schema := types.SchemaOptions{Name: "app", Owner: `app"owner`}
			sql := buildCRDBCreateSchemaSQL(schema)
			Expect(sql).To(ContainSubstring(`AUTHORIZATION "app""owner"`))
		})
	})

	Describe("setDefaultPrivileges", func() {
		It("should generate ALTER DEFAULT PRIVILEGES for tables", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "tables",
				Privileges: []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
			}
			sql, err := buildCRDBSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ALTER DEFAULT PRIVILEGES"))
			Expect(sql).To(ContainSubstring(`FOR ROLE "app_role"`))
			Expect(sql).To(ContainSubstring("GRANT SELECT, INSERT, UPDATE, DELETE"))
			Expect(sql).To(ContainSubstring("ON TABLES"))
			Expect(sql).To(ContainSubstring(`TO "app_role"`))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for sequences", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "sequences",
				Privileges: []string{"USAGE", "SELECT"},
			}
			sql, err := buildCRDBSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ON SEQUENCES"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for functions", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "functions",
				Privileges: []string{"EXECUTE"},
			}
			sql, err := buildCRDBSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ON FUNCTIONS"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for types", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "types",
				Privileges: []string{"USAGE"},
			}
			sql, err := buildCRDBSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ON TYPES"))
		})

		It("should include schema when specified", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				Schema:     "app",
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}
			sql, err := buildCRDBSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring(`IN SCHEMA "app"`))
		})

		It("should return error for unsupported object type", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "unsupported",
				Privileges: []string{"SELECT"},
			}
			_, err := buildCRDBSetDefaultPrivilegesSQL(dp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported object type"))
		})
	})

	Describe("With Mock Pool", func() {
		var (
			mock pgxmock.PgxPoolIface
		)

		BeforeEach(func() {
			var err error
			mock, err = pgxmock.NewPool()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mock.Close()
		})

		Describe("CreateDatabase with mock", func() {
			It("should execute CREATE DATABASE query", func() {
				mock.ExpectExec(`CREATE DATABASE "testdb"`).
					WillReturnResult(pgxmock.NewResult("CREATE DATABASE", 0))

				err := createDatabaseWithCRDBMock(ctx, mock, types.CreateDatabaseOptions{
					Name:             "testdb",
					AllowConnections: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should execute CREATE DATABASE with owner and encoding", func() {
				mock.ExpectExec(`CREATE DATABASE "testdb" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE DATABASE", 0))

				err := createDatabaseWithCRDBMock(ctx, mock, types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            "owner",
					Encoding:         "UTF8",
					AllowConnections: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectExec(`CREATE DATABASE "testdb"`).
					WillReturnError(fmt.Errorf("database already exists"))

				err := createDatabaseWithCRDBMock(ctx, mock, types.CreateDatabaseOptions{
					Name:             "testdb",
					AllowConnections: true,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create database"))
			})
		})

		Describe("DropDatabase with mock", func() {
			It("should execute DROP DATABASE CASCADE query", func() {
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb" CASCADE`).
					WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))

				err := dropDatabaseWithCRDBMock(ctx, mock, "testdb", types.DropDatabaseOptions{
					Force: false,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should cancel sessions when force is true", func() {
				// Expect session query
				sessionRows := pgxmock.NewRows([]string{"session_id"}).
					AddRow("session-1").
					AddRow("session-2")
				mock.ExpectQuery(`SELECT session_id FROM crdb_internal.cluster_sessions`).
					WillReturnRows(sessionRows)

				// Expect cancel session calls
				mock.ExpectExec(`CANCEL SESSION`).
					WillReturnResult(pgxmock.NewResult("CANCEL", 0))
				mock.ExpectExec(`CANCEL SESSION`).
					WillReturnResult(pgxmock.NewResult("CANCEL", 0))

				// Expect drop
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb" CASCADE`).
					WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))

				err := dropDatabaseWithCRDBMockForce(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on drop failure", func() {
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb" CASCADE`).
					WillReturnError(fmt.Errorf("database in use"))

				err := dropDatabaseWithCRDBMock(ctx, mock, "testdb", types.DropDatabaseOptions{
					Force: false,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop database"))
			})
		})

		Describe("DatabaseExists with mock", func() {
			It("should return true when database exists", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS.*crdb_internal\.databases`).
					WithArgs("testdb").
					WillReturnRows(rows)

				exists, err := databaseExistsWithCRDBMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return false when database does not exist", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS.*crdb_internal\.databases`).
					WithArgs("testdb").
					WillReturnRows(rows)

				exists, err := databaseExistsWithCRDBMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("connection error"))

				exists, err := databaseExistsWithCRDBMock(ctx, mock, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check database existence"))
				Expect(exists).To(BeFalse())
			})
		})

		Describe("GetDatabaseInfo with mock", func() {
			It("should return database info when database exists", func() {
				// First query: database info from crdb_internal.databases
				dbRows := pgxmock.NewRows([]string{"name", "owner", "encoding"}).
					AddRow("testdb", "root", "UTF8")
				mock.ExpectQuery(`SELECT.*FROM.*crdb_internal\.databases`).
					WithArgs("testdb").
					WillReturnRows(dbRows)

				// Second query: size from crdb_internal.ranges
				sizeRows := pgxmock.NewRows([]string{"size"}).AddRow(int64(8388608))
				mock.ExpectQuery(`SELECT.*FROM.*crdb_internal\.ranges`).
					WithArgs("testdb").
					WillReturnRows(sizeRows)

				info, err := getDatabaseInfoWithCRDBMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testdb"))
				Expect(info.Owner).To(Equal("root"))
				Expect(info.Encoding).To(Equal("UTF8"))
				Expect(info.SizeBytes).To(Equal(int64(8388608)))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error when database does not exist", func() {
				mock.ExpectQuery(`SELECT.*FROM.*crdb_internal\.databases`).
					WithArgs("nonexistent").
					WillReturnError(fmt.Errorf("no rows in result set"))

				info, err := getDatabaseInfoWithCRDBMock(ctx, mock, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get database info"))
				Expect(info).To(BeNil())
			})

			It("should handle size estimation failure gracefully", func() {
				// First query succeeds
				dbRows := pgxmock.NewRows([]string{"name", "owner", "encoding"}).
					AddRow("testdb", "root", "UTF8")
				mock.ExpectQuery(`SELECT.*FROM.*crdb_internal\.databases`).
					WithArgs("testdb").
					WillReturnRows(dbRows)

				// Second query fails
				mock.ExpectQuery(`SELECT.*FROM.*crdb_internal\.ranges`).
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("range info unavailable"))

				info, err := getDatabaseInfoWithCRDBMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testdb"))
				Expect(info.SizeBytes).To(Equal(int64(0))) // Graceful fallback
			})
		})
	})
})

// SQL builder helper functions for CockroachDB database operations

func buildCRDBCreateDatabaseSQL(opts types.CreateDatabaseOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE DATABASE ")
	sb.WriteString(escapeIdentifier(opts.Name))

	var options []string

	if opts.Owner != "" {
		options = append(options, fmt.Sprintf("OWNER = %s", escapeIdentifier(opts.Owner)))
	}
	if opts.Encoding != "" {
		options = append(options, fmt.Sprintf("ENCODING = %s", escapeLiteral(opts.Encoding)))
	}
	if opts.ConnectionLimit != 0 {
		options = append(options, fmt.Sprintf("CONNECTION LIMIT = %d", opts.ConnectionLimit))
	}
	// CockroachDB does NOT support: TEMPLATE, LC_COLLATE, LC_CTYPE, TABLESPACE, IS_TEMPLATE

	if len(options) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(options, " "))
	}

	return sb.String()
}

func buildCRDBDropDatabaseSQL(name string) string {
	return fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", escapeIdentifier(name))
}

func buildCRDBDatabaseExistsSQL() string {
	return "SELECT EXISTS(SELECT 1 FROM crdb_internal.databases WHERE name = $1)"
}

func buildCRDBGetDatabaseInfoSQL() string {
	return `
		SELECT d.name, r.rolname, pg_encoding_to_char(e.encoding)
		FROM crdb_internal.databases d
		LEFT JOIN pg_catalog.pg_database e ON e.datname = d.name
		LEFT JOIN pg_catalog.pg_roles r ON r.oid = e.datdba
		WHERE d.name = $1`
}

func buildCRDBGetDatabaseSizeSQL() string {
	return `
		SELECT COALESCE(SUM(range_size_bytes), 0)::INT8
		FROM crdb_internal.ranges
		WHERE database_name = $1`
}

func buildCRDBCreateSchemaSQL(schema types.SchemaOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE SCHEMA IF NOT EXISTS ")
	sb.WriteString(escapeIdentifier(schema.Name))

	if schema.Owner != "" {
		sb.WriteString(" AUTHORIZATION ")
		sb.WriteString(escapeIdentifier(schema.Owner))
	}

	return sb.String()
}

func buildCRDBSetDefaultPrivilegesSQL(dp types.DefaultPrivilegeOptions) (string, error) {
	var sb strings.Builder
	sb.WriteString("ALTER DEFAULT PRIVILEGES")

	if dp.Role != "" {
		sb.WriteString(" FOR ROLE ")
		sb.WriteString(escapeIdentifier(dp.Role))
	}

	if dp.Schema != "" {
		sb.WriteString(" IN SCHEMA ")
		sb.WriteString(escapeIdentifier(dp.Schema))
	}

	sb.WriteString(" GRANT ")
	sb.WriteString(strings.Join(dp.Privileges, ", "))
	sb.WriteString(" ON ")

	switch dp.ObjectType {
	case "tables":
		sb.WriteString("TABLES")
	case "sequences":
		sb.WriteString("SEQUENCES")
	case "functions":
		sb.WriteString("FUNCTIONS")
	case "types":
		sb.WriteString("TYPES")
	default:
		return "", fmt.Errorf("unsupported object type: %s", dp.ObjectType)
	}

	sb.WriteString(" TO ")
	sb.WriteString(escapeIdentifier(dp.Role))

	return sb.String(), nil
}

// Mock-based helper functions for testing CockroachDB SQL execution

type crdbMockPool interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

func createDatabaseWithCRDBMock(ctx context.Context, pool crdbMockPool, opts types.CreateDatabaseOptions) error {
	sql := buildCRDBCreateDatabaseSQL(opts)
	_, err := pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", opts.Name, err)
	}
	return nil
}

func dropDatabaseWithCRDBMock(ctx context.Context, pool crdbMockPool, name string, opts types.DropDatabaseOptions) error {
	query := buildCRDBDropDatabaseSQL(name)
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}
	return nil
}

func dropDatabaseWithCRDBMockForce(ctx context.Context, pool crdbMockPool, name string) error {
	// Cancel active sessions (CockroachDB uses CANCEL SESSION, not pg_terminate_backend)
	rows, err := pool.Query(ctx,
		"SELECT session_id FROM crdb_internal.cluster_sessions WHERE active_queries != '' AND session_id != crdb_internal.node_id()::STRING",
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sessionID string
			if err := rows.Scan(&sessionID); err == nil {
				_, _ = pool.Exec(ctx, fmt.Sprintf("CANCEL SESSION '%s'", sessionID))
			}
		}
	}

	query := buildCRDBDropDatabaseSQL(name)
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}
	return nil
}

func databaseExistsWithCRDBMock(ctx context.Context, pool crdbMockPool, name string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM crdb_internal.databases WHERE name = $1)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}
	return exists, nil
}

func getDatabaseInfoWithCRDBMock(ctx context.Context, pool crdbMockPool, name string) (*types.DatabaseInfo, error) {
	var info types.DatabaseInfo
	err := pool.QueryRow(ctx, `
		SELECT d.name, r.rolname, pg_encoding_to_char(e.encoding)
		FROM crdb_internal.databases d
		LEFT JOIN pg_catalog.pg_database e ON e.datname = d.name
		LEFT JOIN pg_catalog.pg_roles r ON r.oid = e.datdba
		WHERE d.name = $1`,
		name).Scan(&info.Name, &info.Owner, &info.Encoding)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	// Get size (graceful fallback on error)
	var sizeBytes int64
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(range_size_bytes), 0)::INT8
		FROM crdb_internal.ranges
		WHERE database_name = $1`,
		name).Scan(&sizeBytes)
	if err != nil {
		sizeBytes = 0
	}
	info.SizeBytes = sizeBytes

	return &info, nil
}
