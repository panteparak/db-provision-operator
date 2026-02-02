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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/db-provision-operator/internal/adapter/cockroachdb/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Grant Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(testutil.DefaultConnectionConfig())
	})

	Describe("GrantRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.GrantRole(ctx, "testuser", []string{"admin"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation", func() {
			It("should generate GRANT role TO grantee", func() {
				sql := buildCRDBGrantRoleSQL("admin", "testuser")
				Expect(sql).To(Equal(`GRANT "admin" TO "testuser"`))
			})

			It("should escape special characters", func() {
				sql := buildCRDBGrantRoleSQL(`admin"group`, `test"user`)
				Expect(sql).To(Equal(`GRANT "admin""group" TO "test""user"`))
			})
		})
	})

	Describe("RevokeRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.RevokeRole(ctx, "testuser", []string{"admin"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation", func() {
			It("should generate REVOKE role FROM grantee", func() {
				sql := buildCRDBRevokeRoleSQL("admin", "testuser")
				Expect(sql).To(Equal(`REVOKE "admin" FROM "testuser"`))
			})

			It("should escape special characters", func() {
				sql := buildCRDBRevokeRoleSQL(`admin"group`, `test"user`)
				Expect(sql).To(Equal(`REVOKE "admin""group" FROM "test""user"`))
			})
		})
	})

	Describe("GetGrants", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				grants, err := adapter.GetGrants(ctx, "testuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(grants).To(BeNil())
			})
		})

		It("should use crdb_internal.cluster_database_privileges", func() {
			sql := buildCRDBGetGrantsSQL()
			Expect(sql).To(ContainSubstring("crdb_internal.cluster_database_privileges"))
			Expect(sql).To(ContainSubstring("grantee = $1"))
			Expect(sql).To(ContainSubstring("database_name"))
			Expect(sql).To(ContainSubstring("privilege_type"))
		})
	})

	Describe("SetDefaultPrivileges", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.SetDefaultPrivileges(ctx, "testuser", []types.DefaultPrivilegeGrantOptions{
					{
						Database:   "testdb",
						ObjectType: "tables",
						Privileges: []string{"SELECT"},
					},
				})
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("grantPrivileges SQL Generation", func() {
		Context("ALL TABLES IN SCHEMA", func() {
			It("should generate GRANT ON ALL TABLES IN SCHEMA for table privileges", func() {
				opt := types.GrantOptions{
					Schema:     "public",
					Privileges: []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA "public" TO "testuser"`),
				))
			})

			It("should include schema USAGE grant", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT USAGE ON SCHEMA "app" TO "testuser"`),
				))
			})

			It("should include WITH GRANT OPTION when set", func() {
				opt := types.GrantOptions{
					Schema:          "public",
					Privileges:      []string{"SELECT"},
					WithGrantOption: true,
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				hasGrantOption := false
				for _, q := range queries {
					if strings.Contains(q, "ALL TABLES IN SCHEMA") && strings.Contains(q, "WITH GRANT OPTION") {
						hasGrantOption = true
						break
					}
				}
				Expect(hasGrantOption).To(BeTrue())
			})

			It("should detect table privileges: ALL", func() {
				opt := types.GrantOptions{
					Schema:     "public",
					Privileges: []string{"ALL"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				hasAllTables := false
				for _, q := range queries {
					if strings.Contains(q, "ALL TABLES IN SCHEMA") {
						hasAllTables = true
						break
					}
				}
				Expect(hasAllTables).To(BeTrue())
			})

			It("should detect table privileges: TRUNCATE, REFERENCES, TRIGGER", func() {
				for _, priv := range []string{"TRUNCATE", "REFERENCES", "TRIGGER"} {
					opt := types.GrantOptions{
						Schema:     "public",
						Privileges: []string{priv},
					}
					queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
					hasAllTables := false
					for _, q := range queries {
						if strings.Contains(q, "ALL TABLES IN SCHEMA") {
							hasAllTables = true
							break
						}
					}
					Expect(hasAllTables).To(BeTrue(), "privilege %s should trigger ALL TABLES grant", priv)
				}
			})
		})

		Context("Specific tables", func() {
			It("should generate table-level grant with schema prefix", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Tables:     []string{"users", "orders"},
					Privileges: []string{"SELECT", "INSERT"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT SELECT, INSERT ON TABLE "app"."users" TO "testuser"`),
				))
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT SELECT, INSERT ON TABLE "app"."orders" TO "testuser"`),
				))
			})

			It("should generate table-level grant without schema prefix", func() {
				opt := types.GrantOptions{
					Tables:     []string{"users"},
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT SELECT ON TABLE "users" TO "testuser"`),
				))
			})

			It("should include WITH GRANT OPTION for tables", func() {
				opt := types.GrantOptions{
					Tables:          []string{"users"},
					Privileges:      []string{"SELECT"},
					WithGrantOption: true,
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring("WITH GRANT OPTION"))
			})
		})

		Context("Specific sequences", func() {
			It("should generate sequence-level grant with schema prefix", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Sequences:  []string{"users_id_seq"},
					Privileges: []string{"USAGE", "SELECT"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT USAGE, SELECT ON SEQUENCE "app"."users_id_seq" TO "testuser"`),
				))
			})

			It("should generate sequence-level grant without schema prefix", func() {
				opt := types.GrantOptions{
					Sequences:  []string{"global_seq"},
					Privileges: []string{"USAGE"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT USAGE ON SEQUENCE "global_seq" TO "testuser"`),
				))
			})

			It("should include WITH GRANT OPTION for sequences", func() {
				opt := types.GrantOptions{
					Sequences:       []string{"users_id_seq"},
					Privileges:      []string{"USAGE"},
					WithGrantOption: true,
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring("WITH GRANT OPTION"))
			})
		})

		Context("Specific functions", func() {
			It("should generate function-level grant with schema prefix", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Functions:  []string{"calculate_total()"},
					Privileges: []string{"EXECUTE"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT EXECUTE ON FUNCTION "app".calculate_total() TO "testuser"`),
				))
			})

			It("should generate function-level grant without schema prefix", func() {
				opt := types.GrantOptions{
					Functions:  []string{"my_func()"},
					Privileges: []string{"EXECUTE"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT EXECUTE ON FUNCTION my_func() TO "testuser"`),
				))
			})

			It("should include WITH GRANT OPTION for functions", func() {
				opt := types.GrantOptions{
					Functions:       []string{"my_func()"},
					Privileges:      []string{"EXECUTE"},
					WithGrantOption: true,
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring("WITH GRANT OPTION"))
			})
		})

		Context("SQL injection prevention", func() {
			It("should escape special characters in grantee", func() {
				opt := types.GrantOptions{
					Tables:     []string{"users"},
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBGrantPrivilegesSQL(`user"inject`, opt)
				Expect(queries[0]).To(ContainSubstring(`"user""inject"`))
			})

			It("should escape special characters in schema name", func() {
				opt := types.GrantOptions{
					Schema:     `schema"inject`,
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				hasEscaped := false
				for _, q := range queries {
					if strings.Contains(q, `"schema""inject"`) {
						hasEscaped = true
						break
					}
				}
				Expect(hasEscaped).To(BeTrue())
			})

			It("should escape special characters in table name", func() {
				opt := types.GrantOptions{
					Tables:     []string{`table"inject`},
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBGrantPrivilegesSQL("testuser", opt)
				Expect(queries[0]).To(ContainSubstring(`"table""inject"`))
			})
		})
	})

	Describe("revokePrivileges SQL Generation", func() {
		Context("ALL TABLES IN SCHEMA", func() {
			It("should generate REVOKE ON ALL TABLES IN SCHEMA", func() {
				opt := types.GrantOptions{
					Schema:     "public",
					Privileges: []string{"SELECT", "INSERT"},
				}
				queries := buildCRDBRevokePrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`REVOKE SELECT, INSERT ON ALL TABLES IN SCHEMA "public" FROM "testuser"`),
				))
			})
		})

		Context("Specific tables", func() {
			It("should generate REVOKE on specific tables with schema", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Tables:     []string{"users"},
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBRevokePrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`REVOKE SELECT ON TABLE "app"."users" FROM "testuser"`),
				))
			})

			It("should generate REVOKE on specific tables without schema", func() {
				opt := types.GrantOptions{
					Tables:     []string{"users"},
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBRevokePrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`REVOKE SELECT ON TABLE "users" FROM "testuser"`),
				))
			})
		})

		Context("Specific sequences", func() {
			It("should generate REVOKE on sequences", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Sequences:  []string{"users_id_seq"},
					Privileges: []string{"USAGE"},
				}
				queries := buildCRDBRevokePrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`REVOKE USAGE ON SEQUENCE "app"."users_id_seq" FROM "testuser"`),
				))
			})
		})

		Context("Specific functions", func() {
			It("should generate REVOKE on functions", func() {
				opt := types.GrantOptions{
					Schema:     "app",
					Functions:  []string{"calculate_total()"},
					Privileges: []string{"EXECUTE"},
				}
				queries := buildCRDBRevokePrivilegesSQL("testuser", opt)
				Expect(queries).To(ContainElement(
					ContainSubstring(`REVOKE EXECUTE ON FUNCTION "app".calculate_total() FROM "testuser"`),
				))
			})
		})
	})

	Describe("setDefaultPrivilege SQL Generation", func() {
		It("should generate ALTER DEFAULT PRIVILEGES for tables", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				ObjectType: "tables",
				Privileges: []string{"SELECT", "INSERT"},
				GrantedBy:  "admin",
			}
			sql := buildCRDBSetDefaultPrivilegeSQL("testuser", opt)
			Expect(sql).To(ContainSubstring("ALTER DEFAULT PRIVILEGES"))
			Expect(sql).To(ContainSubstring(`FOR ROLE "admin"`))
			Expect(sql).To(ContainSubstring("GRANT SELECT, INSERT"))
			Expect(sql).To(ContainSubstring("ON TABLES"))
			Expect(sql).To(ContainSubstring(`TO "testuser"`))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for sequences", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				ObjectType: "sequences",
				Privileges: []string{"USAGE", "SELECT"},
			}
			sql := buildCRDBSetDefaultPrivilegeSQL("testuser", opt)
			Expect(sql).To(ContainSubstring("ON SEQUENCES"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for functions", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				ObjectType: "functions",
				Privileges: []string{"EXECUTE"},
			}
			sql := buildCRDBSetDefaultPrivilegeSQL("testuser", opt)
			Expect(sql).To(ContainSubstring("ON FUNCTIONS"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for types", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				ObjectType: "types",
				Privileges: []string{"USAGE"},
			}
			sql := buildCRDBSetDefaultPrivilegeSQL("testuser", opt)
			Expect(sql).To(ContainSubstring("ON TYPES"))
		})

		It("should generate ALTER DEFAULT PRIVILEGES for schemas", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				ObjectType: "schemas",
				Privileges: []string{"USAGE", "CREATE"},
			}
			sql := buildCRDBSetDefaultPrivilegeSQL("testuser", opt)
			Expect(sql).To(ContainSubstring("ON SCHEMAS"))
		})

		It("should include IN SCHEMA when specified", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "app",
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}
			sql := buildCRDBSetDefaultPrivilegeSQL("testuser", opt)
			Expect(sql).To(ContainSubstring(`IN SCHEMA "app"`))
		})

		It("should escape special characters in grantee", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}
			sql := buildCRDBSetDefaultPrivilegeSQL(`user"inject`, opt)
			Expect(sql).To(ContainSubstring(`"user""inject"`))
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

		Describe("GrantRole with mock", func() {
			It("should grant single role", func() {
				mock.ExpectExec(`GRANT "admin" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))

				err := grantRoleWithCRDBMock(ctx, mock, "testuser", []string{"admin"})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should grant multiple roles", func() {
				mock.ExpectExec(`GRANT "admin" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))
				mock.ExpectExec(`GRANT "developers" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))
				mock.ExpectExec(`GRANT "readers" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))

				err := grantRoleWithCRDBMock(ctx, mock, "testuser", []string{"admin", "developers", "readers"})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on failure", func() {
				mock.ExpectExec(`GRANT "admin" TO "testuser"`).
					WillReturnError(fmt.Errorf("role admin does not exist"))

				err := grantRoleWithCRDBMock(ctx, mock, "testuser", []string{"admin"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to grant role"))
			})
		})

		Describe("RevokeRole with mock", func() {
			It("should revoke single role", func() {
				mock.ExpectExec(`REVOKE "admin" FROM "testuser"`).
					WillReturnResult(pgxmock.NewResult("REVOKE", 0))

				err := revokeRoleWithCRDBMock(ctx, mock, "testuser", []string{"admin"})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should revoke multiple roles", func() {
				mock.ExpectExec(`REVOKE "admin" FROM "testuser"`).
					WillReturnResult(pgxmock.NewResult("REVOKE", 0))
				mock.ExpectExec(`REVOKE "developers" FROM "testuser"`).
					WillReturnResult(pgxmock.NewResult("REVOKE", 0))

				err := revokeRoleWithCRDBMock(ctx, mock, "testuser", []string{"admin", "developers"})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on failure", func() {
				mock.ExpectExec(`REVOKE "admin" FROM "testuser"`).
					WillReturnError(fmt.Errorf("role admin is not a member"))

				err := revokeRoleWithCRDBMock(ctx, mock, "testuser", []string{"admin"})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to revoke role"))
			})
		})

		Describe("GetGrants with mock", func() {
			It("should return grants grouped by database", func() {
				rows := pgxmock.NewRows([]string{"database_name", "privilege_type"}).
					AddRow("testdb", "CONNECT").
					AddRow("testdb", "CREATE").
					AddRow("otherdb", "CONNECT")
				mock.ExpectQuery(`SELECT database_name, privilege_type FROM crdb_internal.cluster_database_privileges`).
					WithArgs("testuser").
					WillReturnRows(rows)

				grants, err := getGrantsWithCRDBMock(ctx, mock, "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(grants).To(HaveLen(2))

				// Find grants by database
				grantMap := make(map[string]types.GrantInfo)
				for _, g := range grants {
					grantMap[g.Database] = g
				}

				testdbGrant := grantMap["testdb"]
				Expect(testdbGrant.Grantee).To(Equal("testuser"))
				Expect(testdbGrant.ObjectType).To(Equal("database"))
				Expect(testdbGrant.Privileges).To(ConsistOf("CONNECT", "CREATE"))

				otherdbGrant := grantMap["otherdb"]
				Expect(otherdbGrant.Grantee).To(Equal("testuser"))
				Expect(otherdbGrant.Privileges).To(ConsistOf("CONNECT"))

				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return empty list when no grants", func() {
				rows := pgxmock.NewRows([]string{"database_name", "privilege_type"})
				mock.ExpectQuery(`SELECT database_name, privilege_type FROM crdb_internal.cluster_database_privileges`).
					WithArgs("testuser").
					WillReturnRows(rows)

				grants, err := getGrantsWithCRDBMock(ctx, mock, "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(grants).To(BeEmpty())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectQuery(`SELECT database_name, privilege_type`).
					WithArgs("testuser").
					WillReturnError(fmt.Errorf("permission denied"))

				grants, err := getGrantsWithCRDBMock(ctx, mock, "testuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get database grants"))
				Expect(grants).To(BeNil())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})
})

// SQL builder helper functions for CockroachDB grant operations

func buildCRDBGrantRoleSQL(role, grantee string) string {
	return fmt.Sprintf("GRANT %s TO %s", escapeIdentifier(role), escapeIdentifier(grantee))
}

func buildCRDBRevokeRoleSQL(role, grantee string) string {
	return fmt.Sprintf("REVOKE %s FROM %s", escapeIdentifier(role), escapeIdentifier(grantee))
}

func buildCRDBGetGrantsSQL() string {
	return "SELECT database_name, privilege_type FROM crdb_internal.cluster_database_privileges WHERE grantee = $1"
}

func buildCRDBGrantPrivilegesSQL(grantee string, opt types.GrantOptions) []string {
	var queries []string

	// Handle ALL TABLES IN SCHEMA or schema-level grants
	if opt.Schema != "" && len(opt.Tables) == 0 && len(opt.Sequences) == 0 && len(opt.Functions) == 0 {
		hasTablePrivs := false
		for _, p := range opt.Privileges {
			upper := strings.ToUpper(p)
			if upper == "SELECT" || upper == "INSERT" || upper == "UPDATE" || upper == "DELETE" ||
				upper == "TRUNCATE" || upper == "REFERENCES" || upper == "TRIGGER" || upper == "ALL" {
				hasTablePrivs = true
				break
			}
		}

		if hasTablePrivs {
			q := fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s",
				strings.Join(opt.Privileges, ", "),
				escapeIdentifier(opt.Schema),
				escapeIdentifier(grantee))
			if opt.WithGrantOption {
				q += " WITH GRANT OPTION"
			}
			queries = append(queries, q)
		}

		// Schema usage grant
		queries = append(queries, fmt.Sprintf("GRANT USAGE ON SCHEMA %s TO %s",
			escapeIdentifier(opt.Schema),
			escapeIdentifier(grantee)))
	}

	// Specific tables
	for _, table := range opt.Tables {
		tableName := table
		if opt.Schema != "" {
			tableName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(table))
		} else {
			tableName = escapeIdentifier(table)
		}

		q := fmt.Sprintf("GRANT %s ON TABLE %s TO %s",
			strings.Join(opt.Privileges, ", "),
			tableName,
			escapeIdentifier(grantee))
		if opt.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		seqName := seq
		if opt.Schema != "" {
			seqName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(seq))
		} else {
			seqName = escapeIdentifier(seq)
		}

		q := fmt.Sprintf("GRANT %s ON SEQUENCE %s TO %s",
			strings.Join(opt.Privileges, ", "),
			seqName,
			escapeIdentifier(grantee))
		if opt.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		fnName := fn
		if opt.Schema != "" {
			fnName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), fn)
		}

		q := fmt.Sprintf("GRANT %s ON FUNCTION %s TO %s",
			strings.Join(opt.Privileges, ", "),
			fnName,
			escapeIdentifier(grantee))
		if opt.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	return queries
}

func buildCRDBRevokePrivilegesSQL(grantee string, opt types.GrantOptions) []string {
	var queries []string

	// Handle ALL TABLES IN SCHEMA
	if opt.Schema != "" && len(opt.Tables) == 0 {
		q := fmt.Sprintf("REVOKE %s ON ALL TABLES IN SCHEMA %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			escapeIdentifier(opt.Schema),
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Specific tables
	for _, table := range opt.Tables {
		tableName := table
		if opt.Schema != "" {
			tableName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(table))
		} else {
			tableName = escapeIdentifier(table)
		}

		q := fmt.Sprintf("REVOKE %s ON TABLE %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			tableName,
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Specific sequences
	for _, seq := range opt.Sequences {
		seqName := seq
		if opt.Schema != "" {
			seqName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), escapeIdentifier(seq))
		} else {
			seqName = escapeIdentifier(seq)
		}

		q := fmt.Sprintf("REVOKE %s ON SEQUENCE %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			seqName,
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	// Specific functions
	for _, fn := range opt.Functions {
		fnName := fn
		if opt.Schema != "" {
			fnName = fmt.Sprintf("%s.%s", escapeIdentifier(opt.Schema), fn)
		}

		q := fmt.Sprintf("REVOKE %s ON FUNCTION %s FROM %s",
			strings.Join(opt.Privileges, ", "),
			fnName,
			escapeIdentifier(grantee))
		queries = append(queries, q)
	}

	return queries
}

func buildCRDBSetDefaultPrivilegeSQL(grantee string, opt types.DefaultPrivilegeGrantOptions) string {
	var sb strings.Builder
	sb.WriteString("ALTER DEFAULT PRIVILEGES")

	if opt.GrantedBy != "" {
		sb.WriteString(" FOR ROLE ")
		sb.WriteString(escapeIdentifier(opt.GrantedBy))
	}

	if opt.Schema != "" {
		sb.WriteString(" IN SCHEMA ")
		sb.WriteString(escapeIdentifier(opt.Schema))
	}

	sb.WriteString(" GRANT ")
	sb.WriteString(strings.Join(opt.Privileges, ", "))
	sb.WriteString(" ON ")

	switch strings.ToLower(opt.ObjectType) {
	case "tables":
		sb.WriteString("TABLES")
	case "sequences":
		sb.WriteString("SEQUENCES")
	case "functions":
		sb.WriteString("FUNCTIONS")
	case "types":
		sb.WriteString("TYPES")
	case "schemas":
		sb.WriteString("SCHEMAS")
	}

	sb.WriteString(" TO ")
	sb.WriteString(escapeIdentifier(grantee))

	return sb.String()
}

// Mock-based helper functions for CockroachDB grant operations

func grantRoleWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, grantee string, roles []string) error {
	for _, role := range roles {
		query := fmt.Sprintf("GRANT %s TO %s",
			escapeIdentifier(role),
			escapeIdentifier(grantee))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to grant role %s to %s: %w", role, grantee, err)
		}
	}
	return nil
}

func revokeRoleWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, grantee string, roles []string) error {
	for _, role := range roles {
		query := fmt.Sprintf("REVOKE %s FROM %s",
			escapeIdentifier(role),
			escapeIdentifier(grantee))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to revoke role %s from %s: %w", role, grantee, err)
		}
	}
	return nil
}

func getGrantsWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, grantee string) ([]types.GrantInfo, error) {
	rows, err := pool.Query(ctx,
		"SELECT database_name, privilege_type FROM crdb_internal.cluster_database_privileges WHERE grantee = $1",
		grantee)
	if err != nil {
		return nil, fmt.Errorf("failed to get database grants: %w", err)
	}

	// Group privileges by database
	dbPrivileges := make(map[string][]string)
	for rows.Next() {
		var dbName, privilege string
		if err := rows.Scan(&dbName, &privilege); err != nil {
			rows.Close()
			return nil, err
		}
		dbPrivileges[dbName] = append(dbPrivileges[dbName], privilege)
	}
	rows.Close()

	var grants []types.GrantInfo
	for dbName, privs := range dbPrivileges {
		grants = append(grants, types.GrantInfo{
			Grantee:    grantee,
			Database:   dbName,
			ObjectType: "database",
			ObjectName: dbName,
			Privileges: privs,
		})
	}

	return grants, nil
}
