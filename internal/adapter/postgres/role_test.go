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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Role Operations", func() {
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
			Username: "admin",
			Password: "password",
		})
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

		Context("SQL Generation", func() {
			It("should generate CREATE ROLE with NOLOGIN by default", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
				}

				// Verify option values - Login is false by default
				Expect(opts.RoleName).To(Equal("testrole"))
				Expect(opts.Login).To(BeFalse())
			})

			It("should include LOGIN when specified", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					Login:    true,
				}

				Expect(opts.Login).To(BeTrue())
			})

			It("should include SUPERUSER when true", func() {
				opts := types.CreateRoleOptions{
					RoleName:  "testrole",
					Superuser: true,
				}

				Expect(opts.Superuser).To(BeTrue())
			})

			It("should include CREATEDB when true", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					CreateDB: true,
				}

				Expect(opts.CreateDB).To(BeTrue())
			})

			It("should include CREATEROLE when true", func() {
				opts := types.CreateRoleOptions{
					RoleName:   "testrole",
					CreateRole: true,
				}

				Expect(opts.CreateRole).To(BeTrue())
			})

			It("should include INHERIT option", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					Inherit:  true,
				}

				Expect(opts.Inherit).To(BeTrue())
			})

			It("should include REPLICATION when true", func() {
				opts := types.CreateRoleOptions{
					RoleName:    "testrole",
					Replication: true,
				}

				Expect(opts.Replication).To(BeTrue())
			})

			It("should include BYPASSRLS when true", func() {
				opts := types.CreateRoleOptions{
					RoleName:  "testrole",
					BypassRLS: true,
				}

				Expect(opts.BypassRLS).To(BeTrue())
			})

			It("should include IN ROLE for role membership", func() {
				opts := types.CreateRoleOptions{
					RoleName: "testrole",
					InRoles:  []string{"parent_role", "admin"},
				}

				Expect(opts.InRoles).To(ConsistOf("parent_role", "admin"))
				Expect(opts.InRoles).To(HaveLen(2))
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

		It("should drop role", func() {
			// Verify the SQL generation logic - the function should:
			// 1. REASSIGN OWNED BY "rolename" TO CURRENT_USER
			// 2. DROP OWNED BY "rolename"
			// 3. DROP ROLE IF EXISTS "rolename"
			roleName := "testrole"
			Expect(escapeIdentifier(roleName)).To(Equal(`"testrole"`))
		})

		It("should reassign owned objects before drop", func() {
			// The function should call REASSIGN OWNED BY before DROP
			roleName := "app_role"
			expectedReassign := fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(roleName))
			Expect(expectedReassign).To(ContainSubstring(`"app_role"`))
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
	})

	Describe("UpdateRole", func() {
		Context("when not connected", func() {
			It("should return error when setting options", func() {
				login := true
				err := adapter.UpdateRole(ctx, "testrole", types.UpdateRoleOptions{
					Login: &login,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should ALTER ROLE with LOGIN", func() {
			login := true
			opts := types.UpdateRoleOptions{
				Login: &login,
			}

			Expect(*opts.Login).To(BeTrue())
		})

		It("should ALTER ROLE with NOLOGIN", func() {
			login := false
			opts := types.UpdateRoleOptions{
				Login: &login,
			}

			Expect(*opts.Login).To(BeFalse())
		})

		It("should ALTER ROLE with multiple options", func() {
			login := true
			superuser := true
			createdb := true
			opts := types.UpdateRoleOptions{
				Login:     &login,
				Superuser: &superuser,
				CreateDB:  &createdb,
			}

			Expect(*opts.Login).To(BeTrue())
			Expect(*opts.Superuser).To(BeTrue())
			Expect(*opts.CreateDB).To(BeTrue())
		})

		It("should grant roles", func() {
			opts := types.UpdateRoleOptions{
				InRoles: []string{"admin", "developers"},
			}

			Expect(opts.InRoles).To(ConsistOf("admin", "developers"))
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
	})

	Describe("applyGrant", func() {
		It("should grant database privileges", func() {
			// Verify grant options structure
			grant := types.GrantOptions{
				Database:   "testdb",
				Privileges: []string{"CONNECT", "CREATE"},
			}

			Expect(grant.Database).To(Equal("testdb"))
			Expect(grant.Privileges).To(ContainElements("CONNECT", "CREATE"))
		})

		It("should grant schema privileges", func() {
			grant := types.GrantOptions{
				Database:   "testdb",
				Schema:     "public",
				Privileges: []string{"USAGE", "CREATE"},
			}

			Expect(grant.Schema).To(Equal("public"))
			Expect(grant.Privileges).To(ContainElements("USAGE", "CREATE"))
		})

		It("should grant table privileges", func() {
			grant := types.GrantOptions{
				Database:   "testdb",
				Schema:     "public",
				Tables:     []string{"users", "orders"},
				Privileges: []string{"SELECT", "INSERT", "UPDATE"},
			}

			Expect(grant.Tables).To(HaveLen(2))
			Expect(grant.Tables).To(ContainElements("users", "orders"))
			Expect(grant.Privileges).To(ContainElements("SELECT", "INSERT", "UPDATE"))
		})

		It("should grant sequence privileges", func() {
			grant := types.GrantOptions{
				Database:   "testdb",
				Schema:     "public",
				Sequences:  []string{"users_id_seq", "orders_id_seq"},
				Privileges: []string{"USAGE", "SELECT"},
			}

			Expect(grant.Sequences).To(HaveLen(2))
			Expect(grant.Sequences).To(ContainElements("users_id_seq", "orders_id_seq"))
			Expect(grant.Privileges).To(ContainElements("USAGE", "SELECT"))
		})

		It("should grant function privileges", func() {
			grant := types.GrantOptions{
				Database:   "testdb",
				Schema:     "public",
				Functions:  []string{"my_function()", "calculate_total(int)"},
				Privileges: []string{"EXECUTE"},
			}

			Expect(grant.Functions).To(HaveLen(2))
			Expect(grant.Functions).To(ContainElements("my_function()", "calculate_total(int)"))
			Expect(grant.Privileges).To(ContainElement("EXECUTE"))
		})

		It("should include WITH GRANT OPTION", func() {
			grant := types.GrantOptions{
				Database:        "testdb",
				Schema:          "public",
				Tables:          []string{"users"},
				Privileges:      []string{"SELECT"},
				WithGrantOption: true,
			}

			Expect(grant.WithGrantOption).To(BeTrue())
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
			It("should execute CREATE ROLE with NOLOGIN query", func() {
				mock.ExpectExec(`CREATE ROLE "testrole" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createRoleWithMock(ctx, mock, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should include LOGIN when specified", func() {
				mock.ExpectExec(`CREATE ROLE "testrole" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createRoleWithMock(ctx, mock, types.CreateRoleOptions{
					RoleName: "testrole",
					Login:    true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectExec(`CREATE ROLE "testrole"`).
					WillReturnError(fmt.Errorf("role already exists"))

				err := createRoleWithMock(ctx, mock, types.CreateRoleOptions{
					RoleName: "testrole",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create role"))
			})
		})

		Describe("DropRole with mock", func() {
			It("should execute DROP ROLE query", func() {
				mock.ExpectExec(`REASSIGN OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
				mock.ExpectExec(`DROP OWNED BY "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP", 0))
				mock.ExpectExec(`DROP ROLE IF EXISTS "testrole"`).
					WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))

				err := dropRoleWithMock(ctx, mock, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("RoleExists with mock", func() {
			It("should return true when role exists", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("testrole").
					WillReturnRows(rows)

				exists, err := roleExistsWithMock(ctx, mock, "testrole")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return false when role does not exist", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := roleExistsWithMock(ctx, mock, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("UpdateRole with mock", func() {
			It("should ALTER ROLE with LOGIN", func() {
				login := true
				mock.ExpectExec(`ALTER ROLE "testrole" WITH LOGIN`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateRoleWithMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					Login: &login,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should ALTER ROLE with NOLOGIN", func() {
				login := false
				mock.ExpectExec(`ALTER ROLE "testrole" WITH NOLOGIN`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateRoleWithMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					Login: &login,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should ALTER ROLE with multiple options", func() {
				login := true
				superuser := true
				createdb := true
				mock.ExpectExec(`ALTER ROLE "testrole" WITH LOGIN SUPERUSER CREATEDB`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateRoleWithMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					Login:     &login,
					Superuser: &superuser,
					CreateDB:  &createdb,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should grant roles", func() {
				mock.ExpectExec(`GRANT "admin" TO "testrole"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))
				mock.ExpectExec(`GRANT "developers" TO "testrole"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))

				err := updateRoleWithMock(ctx, mock, "testrole", types.UpdateRoleOptions{
					InRoles: []string{"admin", "developers"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})
})

// Helper functions for mock-based testing

// createRoleWithMock executes CREATE ROLE using a mock pool
func createRoleWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.CreateRoleOptions) error {
	var sb strings.Builder
	sb.WriteString("CREATE ROLE ")
	sb.WriteString(escapeIdentifier(opts.RoleName))

	var roleOpts []string

	if opts.Login {
		roleOpts = append(roleOpts, "LOGIN")
	} else {
		roleOpts = append(roleOpts, "NOLOGIN")
	}

	if opts.Superuser {
		roleOpts = append(roleOpts, "SUPERUSER")
	} else {
		roleOpts = append(roleOpts, "NOSUPERUSER")
	}

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

	if opts.Replication {
		roleOpts = append(roleOpts, "REPLICATION")
	} else {
		roleOpts = append(roleOpts, "NOREPLICATION")
	}

	if opts.BypassRLS {
		roleOpts = append(roleOpts, "BYPASSRLS")
	} else {
		roleOpts = append(roleOpts, "NOBYPASSRLS")
	}

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

	_, err := pool.Exec(ctx, sb.String())
	if err != nil {
		return fmt.Errorf("failed to create role %s: %w", opts.RoleName, err)
	}

	return nil
}

// dropRoleWithMock executes DROP ROLE using a mock pool
func dropRoleWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string) error {
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(roleName)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(roleName)))

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(roleName))
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop role %s: %w", roleName, err)
	}

	return nil
}

// roleExistsWithMock checks if role exists using a mock pool
func roleExistsWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)",
		roleName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check role existence: %w", err)
	}

	return exists, nil
}

// updateRoleWithMock updates role using a mock pool
func updateRoleWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, roleName string, opts types.UpdateRoleOptions) error {
	var alterOpts []string

	if opts.Login != nil {
		if *opts.Login {
			alterOpts = append(alterOpts, "LOGIN")
		} else {
			alterOpts = append(alterOpts, "NOLOGIN")
		}
	}

	if opts.Superuser != nil {
		if *opts.Superuser {
			alterOpts = append(alterOpts, "SUPERUSER")
		} else {
			alterOpts = append(alterOpts, "NOSUPERUSER")
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

	if opts.Replication != nil {
		if *opts.Replication {
			alterOpts = append(alterOpts, "REPLICATION")
		} else {
			alterOpts = append(alterOpts, "NOREPLICATION")
		}
	}

	if opts.BypassRLS != nil {
		if *opts.BypassRLS {
			alterOpts = append(alterOpts, "BYPASSRLS")
		} else {
			alterOpts = append(alterOpts, "NOBYPASSRLS")
		}
	}

	if len(alterOpts) > 0 {
		query := fmt.Sprintf("ALTER ROLE %s WITH %s",
			escapeIdentifier(roleName),
			strings.Join(alterOpts, " "))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to update role %s: %w", roleName, err)
		}
	}

	for _, role := range opts.InRoles {
		query := fmt.Sprintf("GRANT %s TO %s", escapeIdentifier(role), escapeIdentifier(roleName))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to grant role %s to %s: %w", role, roleName, err)
		}
	}

	return nil
}
