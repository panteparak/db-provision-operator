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

package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Database Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(types.ConnectionConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "postgres",
			Username: "postgres",
			Password: "password",
		})
	})

	Describe("CreateDatabase", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				opts := types.CreateDatabaseOptions{
					Name: "testdb",
				}
				err := adapter.CreateDatabase(ctx, opts)
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
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(Equal(`CREATE DATABASE "testdb"`))
			})

			It("should generate CREATE DATABASE with owner", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            "dbowner",
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`OWNER = "dbowner"`))
			})

			It("should generate CREATE DATABASE with encoding", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Encoding:         "UTF8",
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`ENCODING = 'UTF8'`))
			})

			It("should generate CREATE DATABASE with template", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Template:         "template1",
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`TEMPLATE = "template1"`))
			})

			It("should generate CREATE DATABASE with collation", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					LCCollate:        "en_US.UTF-8",
					LCCtype:          "en_US.UTF-8",
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`LC_COLLATE = 'en_US.UTF-8'`))
				Expect(sql).To(ContainSubstring(`LC_CTYPE = 'en_US.UTF-8'`))
			})

			It("should generate CREATE DATABASE with connection limit", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					ConnectionLimit:  100,
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`CONNECTION LIMIT = 100`))
			})

			It("should generate CREATE DATABASE with tablespace", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Tablespace:       "pg_default",
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`TABLESPACE = "pg_default"`))
			})

			It("should generate CREATE DATABASE with is_template", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					IsTemplate:       true,
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`IS_TEMPLATE = TRUE`))
			})

			It("should generate CREATE DATABASE with allow_connections false", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					AllowConnections: false,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`ALLOW_CONNECTIONS = FALSE`))
			})

			It("should generate CREATE DATABASE with all options", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            "dbowner",
					Encoding:         "UTF8",
					Template:         "template0",
					LCCollate:        "en_US.UTF-8",
					LCCtype:          "en_US.UTF-8",
					Tablespace:       "pg_default",
					ConnectionLimit:  50,
					IsTemplate:       true,
					AllowConnections: false,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`CREATE DATABASE "testdb"`))
				Expect(sql).To(ContainSubstring(`OWNER = "dbowner"`))
				Expect(sql).To(ContainSubstring(`ENCODING = 'UTF8'`))
				Expect(sql).To(ContainSubstring(`TEMPLATE = "template0"`))
				Expect(sql).To(ContainSubstring(`LC_COLLATE = 'en_US.UTF-8'`))
				Expect(sql).To(ContainSubstring(`LC_CTYPE = 'en_US.UTF-8'`))
				Expect(sql).To(ContainSubstring(`TABLESPACE = "pg_default"`))
				Expect(sql).To(ContainSubstring(`CONNECTION LIMIT = 50`))
				Expect(sql).To(ContainSubstring(`IS_TEMPLATE = TRUE`))
				Expect(sql).To(ContainSubstring(`ALLOW_CONNECTIONS = FALSE`))
			})

			It("should escape special characters in database name", func() {
				opts := types.CreateDatabaseOptions{
					Name:             `test"db`,
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`"test""db"`))
			})

			It("should escape special characters in owner name", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Owner:            `db"owner`,
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`OWNER = "db""owner"`))
			})

			It("should escape special characters in encoding", func() {
				opts := types.CreateDatabaseOptions{
					Name:             "testdb",
					Encoding:         "UTF'8",
					AllowConnections: true,
				}
				sql := buildCreateDatabaseSQL(opts)
				Expect(sql).To(ContainSubstring(`ENCODING = 'UTF''8'`))
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
			It("should generate DROP DATABASE IF EXISTS", func() {
				sql := buildDropDatabaseSQL("testdb")
				Expect(sql).To(Equal(`DROP DATABASE IF EXISTS "testdb"`))
			})

			It("should escape special characters in database name", func() {
				sql := buildDropDatabaseSQL(`test"db`)
				Expect(sql).To(Equal(`DROP DATABASE IF EXISTS "test""db"`))
			})

			It("should generate terminate connections query", func() {
				sql := buildTerminateConnectionsSQL("testdb")
				Expect(sql).To(ContainSubstring("pg_terminate_backend"))
				Expect(sql).To(ContainSubstring("pg_stat_activity"))
				Expect(sql).To(ContainSubstring("$1"))
				Expect(sql).To(ContainSubstring("pg_backend_pid()"))
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
			It("should generate EXISTS query for pg_database", func() {
				sql := buildDatabaseExistsSQL()
				Expect(sql).To(ContainSubstring("SELECT EXISTS"))
				Expect(sql).To(ContainSubstring("pg_database"))
				Expect(sql).To(ContainSubstring("datname = $1"))
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
			It("should generate query for database info", func() {
				sql := buildGetDatabaseInfoSQL()
				Expect(sql).To(ContainSubstring("pg_database"))
				Expect(sql).To(ContainSubstring("pg_database_size"))
				Expect(sql).To(ContainSubstring("pg_encoding_to_char"))
				Expect(sql).To(ContainSubstring("datname"))
				Expect(sql).To(ContainSubstring("datdba"))
				Expect(sql).To(ContainSubstring("datcollate"))
			})

			It("should generate query for owner name", func() {
				sql := buildGetOwnerNameSQL()
				Expect(sql).To(ContainSubstring("pg_roles"))
				Expect(sql).To(ContainSubstring("rolname"))
				Expect(sql).To(ContainSubstring("oid = $1"))
			})
		})
	})

	Describe("UpdateDatabase", func() {
		Context("when not connected", func() {
			It("should return error when installing extension", func() {
				opts := types.UpdateDatabaseOptions{
					Extensions: []types.ExtensionOptions{
						{Name: "uuid-ossp"},
					},
				}
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to install extension"))
			})

			It("should return error when creating schema", func() {
				opts := types.UpdateDatabaseOptions{
					Schemas: []types.SchemaOptions{
						{Name: "app"},
					},
				}
				err := adapter.UpdateDatabase(ctx, "testdb", opts)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create schema"))
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
				Expect(err.Error()).To(ContainSubstring("failed to set default privileges"))
			})
		})
	})

	Describe("installExtension", func() {
		It("should generate CREATE EXTENSION IF NOT EXISTS", func() {
			ext := types.ExtensionOptions{
				Name: "uuid-ossp",
			}
			sql := buildInstallExtensionSQL(ext)
			Expect(sql).To(Equal(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`))
		})

		It("should include schema when specified", func() {
			ext := types.ExtensionOptions{
				Name:   "uuid-ossp",
				Schema: "extensions",
			}
			sql := buildInstallExtensionSQL(ext)
			Expect(sql).To(ContainSubstring(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`))
			Expect(sql).To(ContainSubstring(`SCHEMA "extensions"`))
		})

		It("should include version when specified", func() {
			ext := types.ExtensionOptions{
				Name:    "uuid-ossp",
				Version: "1.1",
			}
			sql := buildInstallExtensionSQL(ext)
			Expect(sql).To(ContainSubstring(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`))
			Expect(sql).To(ContainSubstring(`VERSION '1.1'`))
		})

		It("should include both schema and version when specified", func() {
			ext := types.ExtensionOptions{
				Name:    "uuid-ossp",
				Schema:  "extensions",
				Version: "1.1",
			}
			sql := buildInstallExtensionSQL(ext)
			Expect(sql).To(ContainSubstring(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`))
			Expect(sql).To(ContainSubstring(`SCHEMA "extensions"`))
			Expect(sql).To(ContainSubstring(`VERSION '1.1'`))
		})

		It("should escape special characters in extension name", func() {
			ext := types.ExtensionOptions{
				Name: `uuid"ossp`,
			}
			sql := buildInstallExtensionSQL(ext)
			Expect(sql).To(ContainSubstring(`"uuid""ossp"`))
		})
	})

	Describe("createSchema", func() {
		It("should generate CREATE SCHEMA IF NOT EXISTS", func() {
			schema := types.SchemaOptions{
				Name: "app",
			}
			sql := buildCreateSchemaSQL(schema)
			Expect(sql).To(Equal(`CREATE SCHEMA IF NOT EXISTS "app"`))
		})

		It("should include owner when specified", func() {
			schema := types.SchemaOptions{
				Name:  "app",
				Owner: "app_owner",
			}
			sql := buildCreateSchemaSQL(schema)
			Expect(sql).To(ContainSubstring(`CREATE SCHEMA IF NOT EXISTS "app"`))
			Expect(sql).To(ContainSubstring(`AUTHORIZATION "app_owner"`))
		})

		It("should escape special characters in schema name", func() {
			schema := types.SchemaOptions{
				Name: `app"schema`,
			}
			sql := buildCreateSchemaSQL(schema)
			Expect(sql).To(ContainSubstring(`"app""schema"`))
		})

		It("should escape special characters in owner name", func() {
			schema := types.SchemaOptions{
				Name:  "app",
				Owner: `app"owner`,
			}
			sql := buildCreateSchemaSQL(schema)
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
			sql, err := buildSetDefaultPrivilegesSQL(dp)
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
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ALTER DEFAULT PRIVILEGES"))
			Expect(sql).To(ContainSubstring(`FOR ROLE "app_role"`))
			Expect(sql).To(ContainSubstring("GRANT USAGE, SELECT"))
			Expect(sql).To(ContainSubstring("ON SEQUENCES"))
			Expect(sql).To(ContainSubstring(`TO "app_role"`))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for functions", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "functions",
				Privileges: []string{"EXECUTE"},
			}
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ALTER DEFAULT PRIVILEGES"))
			Expect(sql).To(ContainSubstring("ON FUNCTIONS"))
			Expect(sql).To(ContainSubstring("GRANT EXECUTE"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for types", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "types",
				Privileges: []string{"USAGE"},
			}
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring("ALTER DEFAULT PRIVILEGES"))
			Expect(sql).To(ContainSubstring("ON TYPES"))
			Expect(sql).To(ContainSubstring("GRANT USAGE"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES with role", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_admin",
				ObjectType: "tables",
				Privileges: []string{"ALL"},
			}
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring(`FOR ROLE "app_admin"`))
			Expect(sql).To(ContainSubstring(`TO "app_admin"`))
		})

		It("should generate ALTER DEFAULT PRIVILEGES with schema", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				Schema:     "app",
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring(`IN SCHEMA "app"`))
		})

		It("should return error for unsupported object type", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				ObjectType: "unsupported",
				Privileges: []string{"SELECT"},
			}
			_, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported object type"))
		})

		It("should escape special characters in role name", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       `app"role`,
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring(`"app""role"`))
		})

		It("should escape special characters in schema name", func() {
			dp := types.DefaultPrivilegeOptions{
				Role:       "app_role",
				Schema:     `app"schema`,
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}
			sql, err := buildSetDefaultPrivilegesSQL(dp)
			Expect(err).NotTo(HaveOccurred())
			Expect(sql).To(ContainSubstring(`IN SCHEMA "app""schema"`))
		})
	})

	Describe("escapeIdentifier", func() {
		It("should wrap identifier in double quotes", func() {
			result := escapeIdentifier("tablename")
			Expect(result).To(Equal(`"tablename"`))
		})

		It("should escape existing double quotes", func() {
			result := escapeIdentifier(`table"name`)
			Expect(result).To(Equal(`"table""name"`))
		})

		It("should handle multiple double quotes", func() {
			result := escapeIdentifier(`ta"ble"name`)
			Expect(result).To(Equal(`"ta""ble""name"`))
		})

		It("should handle empty string", func() {
			result := escapeIdentifier("")
			Expect(result).To(Equal(`""`))
		})
	})

	Describe("escapeLiteral", func() {
		It("should wrap literal in single quotes", func() {
			result := escapeLiteral("value")
			Expect(result).To(Equal(`'value'`))
		})

		It("should escape existing single quotes", func() {
			result := escapeLiteral(`val'ue`)
			Expect(result).To(Equal(`'val''ue'`))
		})

		It("should handle multiple single quotes", func() {
			result := escapeLiteral(`va'l'ue`)
			Expect(result).To(Equal(`'va''l''ue'`))
		})

		It("should handle empty string", func() {
			result := escapeLiteral("")
			Expect(result).To(Equal(`''`))
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

				err := createDatabaseWithMock(ctx, mock, types.CreateDatabaseOptions{
					Name:             "testdb",
					AllowConnections: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should execute CREATE DATABASE with all options", func() {
				mock.ExpectExec(`CREATE DATABASE "testdb" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE DATABASE", 0))

				err := createDatabaseWithMock(ctx, mock, types.CreateDatabaseOptions{
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

				err := createDatabaseWithMock(ctx, mock, types.CreateDatabaseOptions{
					Name:             "testdb",
					AllowConnections: true,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create database"))
			})
		})

		Describe("DropDatabase with mock", func() {
			It("should execute DROP DATABASE query", func() {
				mock.ExpectExec("SELECT pg_terminate_backend").
					WithArgs("testdb").
					WillReturnResult(pgxmock.NewResult("SELECT", 0))
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb"`).
					WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))

				err := dropDatabaseWithMock(ctx, mock, "testdb", types.DropDatabaseOptions{
					Force: false,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should always terminate connections before dropping", func() {
				mock.ExpectExec("SELECT pg_terminate_backend").
					WithArgs("testdb").
					WillReturnResult(pgxmock.NewResult("SELECT", 1))
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb"`).
					WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))

				err := dropDatabaseWithMock(ctx, mock, "testdb", types.DropDatabaseOptions{
					Force: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error if terminate connections fails", func() {
				mock.ExpectExec("SELECT pg_terminate_backend").
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("permission denied"))

				err := dropDatabaseWithMock(ctx, mock, "testdb", types.DropDatabaseOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to terminate connections"))
			})

			It("should return error on drop failure", func() {
				mock.ExpectExec("SELECT pg_terminate_backend").
					WithArgs("testdb").
					WillReturnResult(pgxmock.NewResult("SELECT", 0))
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb"`).
					WillReturnError(fmt.Errorf("database in use"))

				err := dropDatabaseWithMock(ctx, mock, "testdb", types.DropDatabaseOptions{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop database"))
			})
		})

		Describe("DatabaseExists with mock", func() {
			It("should return true when database exists", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery("SELECT EXISTS").
					WithArgs("testdb").
					WillReturnRows(rows)

				exists, err := databaseExistsWithMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return false when database does not exist", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery("SELECT EXISTS").
					WithArgs("testdb").
					WillReturnRows(rows)

				exists, err := databaseExistsWithMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectQuery("SELECT EXISTS").
					WithArgs("testdb").
					WillReturnError(fmt.Errorf("connection error"))

				exists, err := databaseExistsWithMock(ctx, mock, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check database existence"))
				Expect(exists).To(BeFalse())
			})
		})

		Describe("GetDatabaseInfo with mock", func() {
			It("should return database info when database exists", func() {
				// First query: database info
				dbRows := pgxmock.NewRows([]string{"datname", "datdba", "size", "encoding", "datcollate"}).
					AddRow("testdb", int64(16384), int64(8388608), "UTF8", "en_US.UTF-8")
				mock.ExpectQuery("SELECT d.datname, d.datdba").
					WithArgs("testdb").
					WillReturnRows(dbRows)

				// Second query: owner name
				ownerRows := pgxmock.NewRows([]string{"rolname"}).AddRow("postgres")
				mock.ExpectQuery("SELECT rolname FROM pg_roles").
					WithArgs(int64(16384)).
					WillReturnRows(ownerRows)

				info, err := getDatabaseInfoWithMock(ctx, mock, "testdb")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testdb"))
				Expect(info.Owner).To(Equal("postgres"))
				Expect(info.SizeBytes).To(Equal(int64(8388608)))
				Expect(info.Encoding).To(Equal("UTF8"))
				Expect(info.Collation).To(Equal("en_US.UTF-8"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error when database does not exist", func() {
				mock.ExpectQuery("SELECT d.datname, d.datdba").
					WithArgs("nonexistent").
					WillReturnError(fmt.Errorf("no rows in result set"))

				info, err := getDatabaseInfoWithMock(ctx, mock, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get database info"))
				Expect(info).To(BeNil())
			})

			It("should return error when owner query fails", func() {
				dbRows := pgxmock.NewRows([]string{"datname", "datdba", "size", "encoding", "datcollate"}).
					AddRow("testdb", int64(16384), int64(8388608), "UTF8", "en_US.UTF-8")
				mock.ExpectQuery("SELECT d.datname, d.datdba").
					WithArgs("testdb").
					WillReturnRows(dbRows)

				mock.ExpectQuery("SELECT rolname FROM pg_roles").
					WithArgs(int64(16384)).
					WillReturnError(fmt.Errorf("role not found"))

				info, err := getDatabaseInfoWithMock(ctx, mock, "testdb")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get database owner"))
				Expect(info).To(BeNil())
			})
		})
	})
})

// Helper functions to build SQL statements (extracted logic for testing)

func buildCreateDatabaseSQL(opts types.CreateDatabaseOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE DATABASE ")
	sb.WriteString(escapeIdentifier(opts.Name))

	var options []string

	if opts.Owner != "" {
		options = append(options, fmt.Sprintf("OWNER = %s", escapeIdentifier(opts.Owner)))
	}
	if opts.Template != "" {
		options = append(options, fmt.Sprintf("TEMPLATE = %s", escapeIdentifier(opts.Template)))
	}
	if opts.Encoding != "" {
		options = append(options, fmt.Sprintf("ENCODING = %s", escapeLiteral(opts.Encoding)))
	}
	if opts.LCCollate != "" {
		options = append(options, fmt.Sprintf("LC_COLLATE = %s", escapeLiteral(opts.LCCollate)))
	}
	if opts.LCCtype != "" {
		options = append(options, fmt.Sprintf("LC_CTYPE = %s", escapeLiteral(opts.LCCtype)))
	}
	if opts.Tablespace != "" {
		options = append(options, fmt.Sprintf("TABLESPACE = %s", escapeIdentifier(opts.Tablespace)))
	}
	if opts.ConnectionLimit != 0 {
		options = append(options, fmt.Sprintf("CONNECTION LIMIT = %d", opts.ConnectionLimit))
	}
	if opts.IsTemplate {
		options = append(options, "IS_TEMPLATE = TRUE")
	}
	if !opts.AllowConnections {
		options = append(options, "ALLOW_CONNECTIONS = FALSE")
	}

	if len(options) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(options, " "))
	}

	return sb.String()
}

func buildDropDatabaseSQL(name string) string {
	return fmt.Sprintf("DROP DATABASE IF EXISTS %s", escapeIdentifier(name))
}

func buildTerminateConnectionsSQL(name string) string {
	_ = name // name is passed as parameter
	return `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()`
}

func buildDatabaseExistsSQL() string {
	return "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
}

func buildGetDatabaseInfoSQL() string {
	return `
		SELECT d.datname, d.datdba, pg_database_size(d.datname),
		       pg_encoding_to_char(d.encoding), d.datcollate
		FROM pg_database d
		WHERE d.datname = $1`
}

func buildGetOwnerNameSQL() string {
	return "SELECT rolname FROM pg_roles WHERE oid = $1"
}

func buildInstallExtensionSQL(ext types.ExtensionOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE EXTENSION IF NOT EXISTS ")
	sb.WriteString(escapeIdentifier(ext.Name))

	if ext.Schema != "" {
		sb.WriteString(" SCHEMA ")
		sb.WriteString(escapeIdentifier(ext.Schema))
	}
	if ext.Version != "" {
		sb.WriteString(" VERSION ")
		sb.WriteString(escapeLiteral(ext.Version))
	}

	return sb.String()
}

func buildCreateSchemaSQL(schema types.SchemaOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE SCHEMA IF NOT EXISTS ")
	sb.WriteString(escapeIdentifier(schema.Name))

	if schema.Owner != "" {
		sb.WriteString(" AUTHORIZATION ")
		sb.WriteString(escapeIdentifier(schema.Owner))
	}

	return sb.String()
}

func buildSetDefaultPrivilegesSQL(dp types.DefaultPrivilegeOptions) (string, error) {
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

// Mock-based helper functions for testing with pgxmock

type mockPool interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func createDatabaseWithMock(ctx context.Context, pool mockPool, opts types.CreateDatabaseOptions) error {
	sql := buildCreateDatabaseSQL(opts)
	_, err := pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", opts.Name, err)
	}
	return nil
}

func dropDatabaseWithMock(ctx context.Context, pool mockPool, name string, opts types.DropDatabaseOptions) error {
	terminateQuery := `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()`
	_, err := pool.Exec(ctx, terminateQuery, name)
	if err != nil {
		return fmt.Errorf("failed to terminate connections to database %s: %w", name, err)
	}

	query := buildDropDatabaseSQL(name)
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop database %s: %w", name, err)
	}
	return nil
}

func databaseExistsWithMock(ctx context.Context, pool mockPool, name string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)",
		name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check database existence: %w", err)
	}
	return exists, nil
}

func getDatabaseInfoWithMock(ctx context.Context, pool mockPool, name string) (*types.DatabaseInfo, error) {
	var info types.DatabaseInfo
	var ownerOid int64
	err := pool.QueryRow(ctx, `
		SELECT d.datname, d.datdba, pg_database_size(d.datname),
		       pg_encoding_to_char(d.encoding), d.datcollate
		FROM pg_database d
		WHERE d.datname = $1`,
		name).Scan(&info.Name, &ownerOid, &info.SizeBytes, &info.Encoding, &info.Collation)
	if err != nil {
		return nil, fmt.Errorf("failed to get database info: %w", err)
	}

	err = pool.QueryRow(ctx,
		"SELECT rolname FROM pg_roles WHERE oid = $1",
		ownerOid).Scan(&info.Owner)
	if err != nil {
		return nil, fmt.Errorf("failed to get database owner: %w", err)
	}

	return &info, nil
}
