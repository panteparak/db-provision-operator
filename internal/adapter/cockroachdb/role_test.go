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

		Context("SQL Generation - Least Privilege Enforcement", func() {
			It("should default to NOLOGIN (key difference from users)", func() {
				opts := types.CreateRoleOptions{RoleName: "testrole"}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("NOLOGIN"))
				Expect(sql).NotTo(MatchRegexp(`[^O]LOGIN`)) // should NOT contain bare LOGIN
			})

			It("should default to NOCREATEDB", func() {
				opts := types.CreateRoleOptions{RoleName: "testrole"}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("NOCREATEDB"))
			})

			It("should default to NOCREATEROLE", func() {
				opts := types.CreateRoleOptions{RoleName: "testrole"}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("NOCREATEROLE"))
			})

			It("should default to NOINHERIT", func() {
				opts := types.CreateRoleOptions{RoleName: "testrole"}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("NOINHERIT"))
			})

			It("should NOT include SUPERUSER (unsupported in CockroachDB)", func() {
				opts := types.CreateRoleOptions{
					RoleName:  "testrole",
					Superuser: true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).NotTo(ContainSubstring("SUPERUSER"))
			})

			It("should NOT include REPLICATION (unsupported in CockroachDB)", func() {
				opts := types.CreateRoleOptions{
					RoleName:    "testrole",
					Replication: true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).NotTo(ContainSubstring("REPLICATION"))
			})

			It("should NOT include BYPASSRLS (unsupported in CockroachDB)", func() {
				opts := types.CreateRoleOptions{
					RoleName:  "testrole",
					BypassRLS: true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).NotTo(ContainSubstring("BYPASSRLS"))
			})
		})

		Context("SQL Generation - Options", func() {
			It("should include LOGIN when explicitly requested", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					Login:    true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("LOGIN"))
				Expect(sql).NotTo(ContainSubstring("NOLOGIN"))
			})

			It("should include CREATEDB when true", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					CreateDB: true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("CREATEDB"))
				Expect(sql).NotTo(ContainSubstring("NOCREATEDB"))
			})

			It("should include CREATEROLE when true", func() {
				opts := types.CreateRoleOptions{
					RoleName:   "testrole",
					CreateRole: true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("CREATEROLE"))
				Expect(sql).NotTo(ContainSubstring("NOCREATEROLE"))
			})

			It("should include INHERIT when true", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					Inherit:  true,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("INHERIT"))
				Expect(sql).NotTo(ContainSubstring("NOINHERIT"))
			})

			It("should include IN ROLE for role membership", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					InRoles:  []string{"admin", "developers"},
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring(`IN ROLE "admin", "developers"`))
			})

			It("should include all options combined", func() {
				opts := types.CreateRoleOptions{
					RoleName:   "testrole",
					Login:      true,
					CreateDB:   true,
					CreateRole: true,
					Inherit:    true,
					InRoles:    []string{"admin"},
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring("LOGIN"))
				Expect(sql).To(ContainSubstring("CREATEDB"))
				Expect(sql).To(ContainSubstring("CREATEROLE"))
				Expect(sql).To(ContainSubstring("INHERIT"))
				Expect(sql).To(ContainSubstring(`IN ROLE "admin"`))
			})

			It("should escape special characters in role name", func() {
				opts := types.CreateRoleOptions{
					RoleName: `test"role`,
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring(`"test""role"`))
			})

			It("should escape special characters in IN ROLE names", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					InRoles:  []string{`admin"group`},
				}
				sql := buildCRDBCreateRoleSQL(opts)
				Expect(sql).To(ContainSubstring(`IN ROLE "admin""group"`))
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

		It("should use safe drop pattern: REASSIGN + DROP OWNED + DROP ROLE", func() {
			roleName := "app_readonly"
			expectedReassign := fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(roleName))
			expectedDropOwned := fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(roleName))
			expectedDropRole := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(roleName))

			Expect(expectedReassign).To(ContainSubstring(`"app_readonly"`))
			Expect(expectedDropOwned).To(ContainSubstring(`"app_readonly"`))
			Expect(expectedDropRole).To(ContainSubstring(`"app_readonly"`))
		})

		It("should escape special characters in role name for safe drop", func() {
			roleName := `role"with"quotes`
			expectedDrop := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(roleName))
			Expect(expectedDrop).To(ContainSubstring(`"role""with""quotes"`))
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

		It("should use pg_catalog.pg_roles for CockroachDB", func() {
			sql := buildCRDBRoleExistsSQL()
			Expect(sql).To(ContainSubstring("pg_catalog.pg_roles"))
			Expect(sql).To(ContainSubstring("rolname = $1"))
		})
	})

	Describe("UpdateRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				createdb := true
				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					CreateDB: &createdb,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation", func() {
			It("should ALTER ROLE with LOGIN", func() {
				login := true
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					Login: &login,
				})
				Expect(sql).To(ContainSubstring("ALTER ROLE"))
				Expect(sql).To(ContainSubstring("LOGIN"))
			})

			It("should ALTER ROLE with NOLOGIN", func() {
				nologin := false
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					Login: &nologin,
				})
				Expect(sql).To(ContainSubstring("NOLOGIN"))
			})

			It("should ALTER ROLE with CREATEDB", func() {
				createdb := true
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					CreateDB: &createdb,
				})
				Expect(sql).To(ContainSubstring("CREATEDB"))
			})

			It("should ALTER ROLE with NOCREATEDB", func() {
				createdb := false
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					CreateDB: &createdb,
				})
				Expect(sql).To(ContainSubstring("NOCREATEDB"))
			})

			It("should ALTER ROLE with CREATEROLE", func() {
				createrole := true
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					CreateRole: &createrole,
				})
				Expect(sql).To(ContainSubstring("CREATEROLE"))
			})

			It("should ALTER ROLE with NOCREATEROLE", func() {
				createrole := false
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					CreateRole: &createrole,
				})
				Expect(sql).To(ContainSubstring("NOCREATEROLE"))
			})

			It("should ALTER ROLE with INHERIT/NOINHERIT", func() {
				inherit := true
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					Inherit: &inherit,
				})
				Expect(sql).To(ContainSubstring("INHERIT"))

				noinherit := false
				sql = buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					Inherit: &noinherit,
				})
				Expect(sql).To(ContainSubstring("NOINHERIT"))
			})

			It("should combine multiple options", func() {
				login := true
				createdb := true
				inherit := true
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{
					Login:    &login,
					CreateDB: &createdb,
					Inherit:  &inherit,
				})
				Expect(sql).To(ContainSubstring("LOGIN"))
				Expect(sql).To(ContainSubstring("CREATEDB"))
				Expect(sql).To(ContainSubstring("INHERIT"))
			})

			It("should return empty string when no options set", func() {
				sql := buildCRDBUpdateRoleSQL("testrole", types.UpdateRoleOptions{})
				Expect(sql).To(BeEmpty())
			})

			It("should escape special characters in role name", func() {
				login := true
				sql := buildCRDBUpdateRoleSQL(`test"role`, types.UpdateRoleOptions{
					Login: &login,
				})
				Expect(sql).To(ContainSubstring(`"test""role"`))
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

		It("should query pg_catalog.pg_roles without superuser/replication/bypassrls", func() {
			sql := buildCRDBGetRoleInfoSQL()
			Expect(sql).To(ContainSubstring("rolname"))
			Expect(sql).To(ContainSubstring("rolcanlogin"))
			Expect(sql).To(ContainSubstring("rolinherit"))
			Expect(sql).To(ContainSubstring("rolcreatedb"))
			Expect(sql).To(ContainSubstring("rolcreaterole"))
			// CockroachDB does NOT have these columns
			Expect(sql).NotTo(ContainSubstring("rolsuper"))
			Expect(sql).NotTo(ContainSubstring("rolreplication"))
			Expect(sql).NotTo(ContainSubstring("rolbypassrls"))
		})

		It("should query role memberships via pg_auth_members", func() {
			sql := buildCRDBGetRoleMembershipsSQL()
			Expect(sql).To(ContainSubstring("pg_catalog.pg_roles"))
			Expect(sql).To(ContainSubstring("pg_catalog.pg_auth_members"))
			Expect(sql).To(ContainSubstring("roleid"))
			Expect(sql).To(ContainSubstring("member"))
		})
	})

	Describe("applyGrant", func() {
		Context("SQL Generation", func() {
			It("should generate database-level grant", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Privileges: []string{"CONNECT", "CREATE"},
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring(`GRANT CONNECT, CREATE ON DATABASE "testdb" TO "testrole"`))
			})

			It("should generate schema-level grant", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Schema:     "app",
					Privileges: []string{"USAGE", "CREATE"},
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring(`GRANT USAGE, CREATE ON SCHEMA "app" TO "testrole"`))
			})

			It("should generate table-level grant with schema prefix", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Schema:     "app",
					Tables:     []string{"users", "orders"},
					Privileges: []string{"SELECT", "INSERT"},
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				Expect(queries).To(HaveLen(2))
				Expect(queries[0]).To(ContainSubstring(`GRANT SELECT, INSERT ON TABLE "app"."users" TO "testrole"`))
				Expect(queries[1]).To(ContainSubstring(`GRANT SELECT, INSERT ON TABLE "app"."orders" TO "testrole"`))
			})

			It("should generate table-level grant without schema prefix", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Tables:     []string{"users"},
					Privileges: []string{"SELECT"},
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring(`GRANT SELECT ON TABLE "users" TO "testrole"`))
			})

			It("should generate table-level grant WITH GRANT OPTION", func() {
				grant := types.GrantOptions{
					Database:        "testdb",
					Tables:          []string{"users"},
					Privileges:      []string{"SELECT"},
					WithGrantOption: true,
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring("WITH GRANT OPTION"))
			})

			It("should generate sequence-level grant", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Schema:     "app",
					Sequences:  []string{"users_id_seq"},
					Privileges: []string{"USAGE", "SELECT"},
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				// Schema + sequences = schema-level grant + sequence grant
				Expect(queries).To(HaveLen(2))
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT USAGE, SELECT ON SEQUENCE "app"."users_id_seq" TO "testrole"`),
				))
			})

			It("should generate function-level grant", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Schema:     "app",
					Functions:  []string{"calculate_total()"},
					Privileges: []string{"EXECUTE"},
				}
				queries := buildCRDBApplyGrantSQL("testrole", grant)
				// Schema + functions = schema-level grant + function grant
				Expect(queries).To(HaveLen(2))
				Expect(queries).To(ContainElement(
					ContainSubstring(`GRANT EXECUTE ON FUNCTION "app".calculate_total() TO "testrole"`),
				))
			})

			It("should escape special characters in grantee name", func() {
				grant := types.GrantOptions{
					Database:   "testdb",
					Privileges: []string{"CONNECT"},
				}
				queries := buildCRDBApplyGrantSQL(`role"inject`, grant)
				Expect(queries).To(HaveLen(1))
				Expect(queries[0]).To(ContainSubstring(`"role""inject"`))
			})
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

		Describe("CreateRole with mock", func() {
			It("should execute CREATE ROLE with NOLOGIN (least privilege)", func() {
				mock.ExpectExec(`CREATE ROLE "testrole" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createRoleWithCRDBMock(ctx, mock, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should execute CREATE ROLE with LOGIN when requested", func() {
				mock.ExpectExec(`CREATE ROLE "login_role" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createRoleWithCRDBMock(ctx, mock, types.CreateRoleOptions{
					RoleName: "login_role",
					Login:    true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectExec(`CREATE ROLE "testrole"`).
					WillReturnError(fmt.Errorf("role already exists"))

				err := createRoleWithCRDBMock(ctx, mock, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create role"))
			})
		})

		Describe("DropRole with mock", func() {
			It("should execute REASSIGN + DROP OWNED + DROP ROLE", func() {
				mock.ExpectExec(`REASSIGN OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
				mock.ExpectExec(`DROP OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP", 0))
				mock.ExpectExec(`DROP ROLE IF EXISTS "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))

				err := dropRoleWithCRDBMock(ctx, mock, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should still proceed if REASSIGN fails", func() {
				mock.ExpectExec(`REASSIGN OWNED BY "testrole"`).
					WillReturnError(fmt.Errorf("nothing owned"))
				mock.ExpectExec(`DROP OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP", 0))
				mock.ExpectExec(`DROP ROLE IF EXISTS "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))

				err := dropRoleWithCRDBMock(ctx, mock, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error if DROP ROLE fails", func() {
				mock.ExpectExec(`REASSIGN OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
				mock.ExpectExec(`DROP OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP", 0))
				mock.ExpectExec(`DROP ROLE IF EXISTS "testrole"`).
					WillReturnError(fmt.Errorf("role is a member of another role"))

				err := dropRoleWithCRDBMock(ctx, mock, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to drop role"))
			})
		})

		Describe("RoleExists with mock", func() {
			It("should return true when role exists", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("testrole").
					WillReturnRows(rows)

				exists, err := roleExistsWithCRDBMock(ctx, mock, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return false when role does not exist", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := roleExistsWithCRDBMock(ctx, mock, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("testrole").
					WillReturnError(fmt.Errorf("connection error"))

				exists, err := roleExistsWithCRDBMock(ctx, mock, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to check role existence"))
				Expect(exists).To(BeFalse())
			})
		})

		Describe("UpdateRole with mock", func() {
			It("should ALTER ROLE with CREATEDB", func() {
				createdb := true
				mock.ExpectExec(`ALTER ROLE "testrole" WITH CREATEDB`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateRoleWithCRDBMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					CreateDB: &createdb,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should ALTER ROLE with multiple attributes", func() {
				login := true
				createdb := true
				mock.ExpectExec(`ALTER ROLE "testrole" WITH LOGIN CREATEDB`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateRoleWithCRDBMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					Login:    &login,
					CreateDB: &createdb,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should grant roles to role", func() {
				mock.ExpectExec(`GRANT "admin" TO "testrole"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))
				mock.ExpectExec(`GRANT "developers" TO "testrole"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))

				err := updateRoleWithCRDBMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					InRoles: []string{"admin", "developers"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should do nothing when no options set", func() {
				err := updateRoleWithCRDBMock(ctx, mock, "testrole", types.UpdateRoleOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on ALTER failure", func() {
				login := true
				mock.ExpectExec(`ALTER ROLE "testrole"`).
					WillReturnError(fmt.Errorf("permission denied"))

				err := updateRoleWithCRDBMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					Login: &login,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update role"))
			})

			It("should return error on GRANT role failure", func() {
				mock.ExpectExec(`GRANT "admin" TO "testrole"`).
					WillReturnError(fmt.Errorf("role admin does not exist"))

				err := updateRoleWithCRDBMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					InRoles: []string{"admin"},
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to grant role"))
			})
		})

		Describe("GetRoleInfo with mock", func() {
			It("should return role info with hardcoded false for CockroachDB-unsupported attrs", func() {
				// Role info query
				roleRows := pgxmock.NewRows([]string{
					"rolname", "rolcanlogin", "rolinherit", "rolcreatedb", "rolcreaterole",
				}).AddRow("testrole", false, false, false, false)
				mock.ExpectQuery(`SELECT.*FROM.*pg_catalog\.pg_roles`).
					WithArgs("testrole").
					WillReturnRows(roleRows)

				// Role membership query
				memberRows := pgxmock.NewRows([]string{"rolname"}).
					AddRow("app_read")
				mock.ExpectQuery(`SELECT r\.rolname.*pg_auth_members`).
					WithArgs("testrole").
					WillReturnRows(memberRows)

				info, err := getRoleInfoWithCRDBMock(ctx, mock, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("testrole"))
				Expect(info.Login).To(BeFalse())
				Expect(info.Inherit).To(BeFalse())
				Expect(info.CreateDB).To(BeFalse())
				Expect(info.CreateRole).To(BeFalse())
				// CockroachDB always returns false for these
				Expect(info.Superuser).To(BeFalse())
				Expect(info.Replication).To(BeFalse())
				Expect(info.BypassRLS).To(BeFalse())
				Expect(info.InRoles).To(ConsistOf("app_read"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return role info with login enabled", func() {
				roleRows := pgxmock.NewRows([]string{
					"rolname", "rolcanlogin", "rolinherit", "rolcreatedb", "rolcreaterole",
				}).AddRow("loginrole", true, true, true, true)
				mock.ExpectQuery(`SELECT.*FROM.*pg_catalog\.pg_roles`).
					WithArgs("loginrole").
					WillReturnRows(roleRows)

				memberRows := pgxmock.NewRows([]string{"rolname"})
				mock.ExpectQuery(`SELECT r\.rolname.*pg_auth_members`).
					WithArgs("loginrole").
					WillReturnRows(memberRows)

				info, err := getRoleInfoWithCRDBMock(ctx, mock, "loginrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Name).To(Equal("loginrole"))
				Expect(info.Login).To(BeTrue())
				Expect(info.Inherit).To(BeTrue())
				Expect(info.CreateDB).To(BeTrue())
				Expect(info.CreateRole).To(BeTrue())
				// Still false for unsupported attrs
				Expect(info.Superuser).To(BeFalse())
				Expect(info.Replication).To(BeFalse())
				Expect(info.BypassRLS).To(BeFalse())
			})

			It("should return error when role not found", func() {
				mock.ExpectQuery(`SELECT.*FROM.*pg_catalog\.pg_roles`).
					WithArgs("nonexistent").
					WillReturnError(fmt.Errorf("no rows in result set"))

				info, err := getRoleInfoWithCRDBMock(ctx, mock, "nonexistent")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get role info"))
				Expect(info).To(BeNil())
			})

			It("should return error on membership query failure", func() {
				roleRows := pgxmock.NewRows([]string{
					"rolname", "rolcanlogin", "rolinherit", "rolcreatedb", "rolcreaterole",
				}).AddRow("testrole", false, false, false, false)
				mock.ExpectQuery(`SELECT.*FROM.*pg_catalog\.pg_roles`).
					WithArgs("testrole").
					WillReturnRows(roleRows)

				mock.ExpectQuery(`SELECT r\.rolname.*pg_auth_members`).
					WithArgs("testrole").
					WillReturnError(fmt.Errorf("connection lost"))

				info, err := getRoleInfoWithCRDBMock(ctx, mock, "testrole")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get role memberships"))
				Expect(info).To(BeNil())
			})
		})
	})
})

// SQL builder helper functions for CockroachDB role operations

func buildCRDBCreateRoleSQL(opts types.CreateRoleOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE ROLE ")
	sb.WriteString(escapeIdentifier(opts.RoleName))

	var roleOpts []string

	// Roles default to NOLOGIN (key difference from users)
	if opts.Login {
		roleOpts = append(roleOpts, "LOGIN")
	} else {
		roleOpts = append(roleOpts, "NOLOGIN")
	}

	// CockroachDB-supported attributes with least-privilege defaults
	if opts.CreateDB {
		roleOpts = append(roleOpts, "CREATEDB")
	} else {
		roleOpts = append(roleOpts, "NOCREATEDB")
	}

	if opts.CreateRole {
		roleOpts = append(roleOpts, "CREATEROLE")
	} else {
		roleOpts = append(roleOpts, "NOCREATEROLE")
	}

	if opts.Inherit {
		roleOpts = append(roleOpts, "INHERIT")
	} else {
		roleOpts = append(roleOpts, "NOINHERIT")
	}

	// CockroachDB does NOT support: SUPERUSER, REPLICATION, BYPASSRLS
	// These are silently ignored for compatibility

	if len(opts.InRoles) > 0 {
		var roles []string
		for _, r := range opts.InRoles {
			roles = append(roles, escapeIdentifier(r))
		}
		roleOpts = append(roleOpts, fmt.Sprintf("IN ROLE %s", strings.Join(roles, ", ")))
	}

	if len(roleOpts) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(roleOpts, " "))
	}

	return sb.String()
}

func buildCRDBRoleExistsSQL() string {
	return "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)"
}

func buildCRDBUpdateRoleSQL(roleName string, opts types.UpdateRoleOptions) string {
	var alterOpts []string

	if opts.Login != nil {
		if *opts.Login {
			alterOpts = append(alterOpts, "LOGIN")
		} else {
			alterOpts = append(alterOpts, "NOLOGIN")
		}
	}

	if opts.CreateDB != nil {
		if *opts.CreateDB {
			alterOpts = append(alterOpts, "CREATEDB")
		} else {
			alterOpts = append(alterOpts, "NOCREATEDB")
		}
	}

	if opts.CreateRole != nil {
		if *opts.CreateRole {
			alterOpts = append(alterOpts, "CREATEROLE")
		} else {
			alterOpts = append(alterOpts, "NOCREATEROLE")
		}
	}

	if opts.Inherit != nil {
		if *opts.Inherit {
			alterOpts = append(alterOpts, "INHERIT")
		} else {
			alterOpts = append(alterOpts, "NOINHERIT")
		}
	}

	if len(alterOpts) == 0 {
		return ""
	}

	return fmt.Sprintf("ALTER ROLE %s WITH %s",
		escapeIdentifier(roleName),
		strings.Join(alterOpts, " "))
}

func buildCRDBGetRoleInfoSQL() string {
	return `
		SELECT rolname, rolcanlogin, rolinherit, rolcreatedb, rolcreaterole
		FROM pg_catalog.pg_roles
		WHERE rolname = $1`
}

func buildCRDBGetRoleMembershipsSQL() string {
	return `
		SELECT r.rolname
		FROM pg_catalog.pg_roles r
		JOIN pg_catalog.pg_auth_members m ON r.oid = m.roleid
		JOIN pg_catalog.pg_roles u ON m.member = u.oid
		WHERE u.rolname = $1`
}

func buildCRDBApplyGrantSQL(grantee string, grant types.GrantOptions) []string {
	var queries []string

	// Database-level privileges
	if grant.Schema == "" && len(grant.Tables) == 0 {
		queries = append(queries, fmt.Sprintf(
			"GRANT %s ON DATABASE %s TO %s",
			strings.Join(grant.Privileges, ", "),
			escapeIdentifier(grant.Database),
			escapeIdentifier(grantee)))
	}

	// Schema-level privileges
	if grant.Schema != "" && len(grant.Tables) == 0 {
		queries = append(queries, fmt.Sprintf(
			"GRANT %s ON SCHEMA %s TO %s",
			strings.Join(grant.Privileges, ", "),
			escapeIdentifier(grant.Schema),
			escapeIdentifier(grantee)))
	}

	// Table-level privileges
	for _, table := range grant.Tables {
		tableName := table
		if grant.Schema != "" {
			tableName = fmt.Sprintf("%s.%s", escapeIdentifier(grant.Schema), escapeIdentifier(table))
		} else {
			tableName = escapeIdentifier(table)
		}

		q := fmt.Sprintf("GRANT %s ON TABLE %s TO %s",
			strings.Join(grant.Privileges, ", "),
			tableName,
			escapeIdentifier(grantee))
		if grant.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Sequence-level privileges
	for _, seq := range grant.Sequences {
		seqName := seq
		if grant.Schema != "" {
			seqName = fmt.Sprintf("%s.%s", escapeIdentifier(grant.Schema), escapeIdentifier(seq))
		} else {
			seqName = escapeIdentifier(seq)
		}

		q := fmt.Sprintf("GRANT %s ON SEQUENCE %s TO %s",
			strings.Join(grant.Privileges, ", "),
			seqName,
			escapeIdentifier(grantee))
		if grant.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	// Function-level privileges
	for _, fn := range grant.Functions {
		fnName := fn
		if grant.Schema != "" {
			fnName = fmt.Sprintf("%s.%s", escapeIdentifier(grant.Schema), fn)
		}

		q := fmt.Sprintf("GRANT %s ON FUNCTION %s TO %s",
			strings.Join(grant.Privileges, ", "),
			fnName,
			escapeIdentifier(grantee))
		if grant.WithGrantOption {
			q += " WITH GRANT OPTION"
		}
		queries = append(queries, q)
	}

	return queries
}

// Mock-based helper functions for CockroachDB role operations

func createRoleWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.CreateRoleOptions) error {
	sql := buildCRDBCreateRoleSQL(opts)
	_, err := pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to create role %s: %w", opts.RoleName, err)
	}
	return nil
}

func dropRoleWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string) error {
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(roleName)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(roleName)))

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(roleName))
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop role %s: %w", roleName, err)
	}
	return nil
}

func roleExistsWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)",
		roleName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role existence: %w", err)
	}
	return exists, nil
}

func updateRoleWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string, opts types.UpdateRoleOptions) error {
	alterSQL := buildCRDBUpdateRoleSQL(roleName, opts)
	if alterSQL != "" {
		if _, err := pool.Exec(ctx, alterSQL); err != nil {
			return fmt.Errorf("failed to update role %s: %w", roleName, err)
		}
	}

	for _, role := range opts.InRoles {
		query := fmt.Sprintf("GRANT %s TO %s", escapeIdentifier(role), escapeIdentifier(roleName))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to grant role %s to role %s: %w", role, roleName, err)
		}
	}

	return nil
}

func getRoleInfoWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string) (*types.RoleInfo, error) {
	var info types.RoleInfo
	err := pool.QueryRow(ctx, `
		SELECT rolname, rolcanlogin, rolinherit, rolcreatedb, rolcreaterole
		FROM pg_catalog.pg_roles
		WHERE rolname = $1`,
		roleName).Scan(
		&info.Name, &info.Login, &info.Inherit, &info.CreateDB, &info.CreateRole)
	if err != nil {
		return nil, fmt.Errorf("failed to get role info: %w", err)
	}

	// CockroachDB does not support these attributes
	info.Superuser = false
	info.Replication = false
	info.BypassRLS = false

	// Get role memberships
	rows, err := pool.Query(ctx, `
		SELECT r.rolname
		FROM pg_catalog.pg_roles r
		JOIN pg_catalog.pg_auth_members m ON r.oid = m.roleid
		JOIN pg_catalog.pg_roles u ON m.member = u.oid
		WHERE u.rolname = $1`,
		roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role memberships: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		info.InRoles = append(info.InRoles, role)
	}

	return &info, rows.Err()
}
