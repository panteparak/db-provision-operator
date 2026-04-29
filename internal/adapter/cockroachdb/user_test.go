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

var _ = Describe("User Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(testutil.DefaultConnectionConfig())
	})

	Describe("CreateUser", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username: "testuser",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation - Least Privilege Enforcement", func() {
			It("should always include LOGIN for users", func() {
				opts := types.CreateUserOptions{Username: "testuser"}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("LOGIN"))
			})

			It("should default to NOCREATEDB", func() {
				opts := types.CreateUserOptions{Username: "testuser"}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("NOCREATEDB"))
			})

			It("should default to NOCREATEROLE", func() {
				opts := types.CreateUserOptions{Username: "testuser"}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("NOCREATEROLE"))
			})

			It("should default to NOINHERIT", func() {
				opts := types.CreateUserOptions{Username: "testuser"}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("NOINHERIT"))
			})

			It("should NOT include SUPERUSER (unsupported in CockroachDB)", func() {
				opts := types.CreateUserOptions{
					Username:  "testuser",
					Superuser: true,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).NotTo(ContainSubstring("SUPERUSER"))
			})

			It("should NOT include REPLICATION (unsupported in CockroachDB)", func() {
				opts := types.CreateUserOptions{
					Username:    "testuser",
					Replication: true,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).NotTo(ContainSubstring("REPLICATION"))
			})

			It("should NOT include BYPASSRLS (unsupported in CockroachDB)", func() {
				opts := types.CreateUserOptions{
					Username:  "testuser",
					BypassRLS: true,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).NotTo(ContainSubstring("BYPASSRLS"))
			})
		})

		Context("SQL Generation - Options", func() {
			It("should include password when specified", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Password: "secretpass",
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring(`PASSWORD 'secretpass'`))
			})

			It("should include CREATEDB when true", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					CreateDB: true,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("CREATEDB"))
				Expect(sql).NotTo(ContainSubstring("NOCREATEDB"))
			})

			It("should include CREATEROLE when true", func() {
				opts := types.CreateUserOptions{
					Username:   "testuser",
					CreateRole: true,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("CREATEROLE"))
				Expect(sql).NotTo(ContainSubstring("NOCREATEROLE"))
			})

			It("should include INHERIT when true", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Inherit:  true,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("INHERIT"))
				Expect(sql).NotTo(ContainSubstring("NOINHERIT"))
			})

			It("should include CONNECTION LIMIT", func() {
				opts := types.CreateUserOptions{
					Username:        "testuser",
					ConnectionLimit: 10,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring("CONNECTION LIMIT 10"))
			})

			It("should include VALID UNTIL", func() {
				opts := types.CreateUserOptions{
					Username:   "testuser",
					ValidUntil: "2026-12-31",
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring(`VALID UNTIL '2026-12-31'`))
			})

			It("should include IN ROLE for role membership", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					InRoles:  []string{"admin", "developers"},
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring(`IN ROLE "admin", "developers"`))
			})

			It("should escape special characters in username", func() {
				opts := types.CreateUserOptions{
					Username: `test"user`,
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring(`"test""user"`))
			})

			It("should escape single quotes in password", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Password: "pass'word",
				}
				sql := buildCRDBCreateUserSQL(opts)
				Expect(sql).To(ContainSubstring(`'pass''word'`))
			})
		})
	})

	Describe("DropUser", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.DropUser(ctx, "testuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should use safe drop pattern: REASSIGN + DROP OWNED + DROP ROLE", func() {
			username := "appuser"
			expectedReassign := fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(username))
			expectedDropOwned := fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(username))
			expectedDropRole := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(username))

			Expect(expectedReassign).To(ContainSubstring(`"appuser"`))
			Expect(expectedDropOwned).To(ContainSubstring(`"appuser"`))
			Expect(expectedDropRole).To(ContainSubstring(`"appuser"`))
		})
	})

	Describe("UserExists", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				exists, err := adapter.UserExists(ctx, "testuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(exists).To(BeFalse())
			})
		})

		It("should use pg_catalog.pg_roles for CockroachDB", func() {
			sql := buildCRDBUserExistsSQL()
			Expect(sql).To(ContainSubstring("pg_catalog.pg_roles"))
			Expect(sql).To(ContainSubstring("rolname = $1"))
		})
	})

	Describe("UpdateUser", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				createdb := true
				err := adapter.UpdateUser(ctx, "testuser", types.UpdateUserOptions{
					CreateDB: &createdb,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		Context("SQL Generation", func() {
			It("should ALTER ROLE with CREATEDB", func() {
				createdb := true
				sql := buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{
					CreateDB: &createdb,
				})
				Expect(sql).To(ContainSubstring("ALTER ROLE"))
				Expect(sql).To(ContainSubstring("CREATEDB"))
			})

			It("should ALTER ROLE with NOCREATEDB", func() {
				createdb := false
				sql := buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{
					CreateDB: &createdb,
				})
				Expect(sql).To(ContainSubstring("NOCREATEDB"))
			})

			It("should ALTER ROLE with LOGIN/NOLOGIN", func() {
				login := true
				sql := buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{
					Login: &login,
				})
				Expect(sql).To(ContainSubstring("LOGIN"))

				nologin := false
				sql = buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{
					Login: &nologin,
				})
				Expect(sql).To(ContainSubstring("NOLOGIN"))
			})

			It("should handle CONNECTION LIMIT", func() {
				limit := int32(50)
				sql := buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{
					ConnectionLimit: &limit,
				})
				Expect(sql).To(ContainSubstring("CONNECTION LIMIT 50"))
			})

			It("should handle VALID UNTIL", func() {
				until := "2027-01-01"
				sql := buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{
					ValidUntil: &until,
				})
				Expect(sql).To(ContainSubstring(`VALID UNTIL '2027-01-01'`))
			})

			It("should return empty string when no options set", func() {
				sql := buildCRDBUpdateUserSQL("testuser", types.UpdateUserOptions{})
				Expect(sql).To(BeEmpty())
			})
		})
	})

	Describe("UpdatePassword", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.UpdatePassword(ctx, "testuser", "newpassword123")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should generate ALTER ROLE with new password", func() {
			username := "testuser"
			password := "newpassword123"
			expectedSQL := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD %s",
				escapeIdentifier(username),
				escapeLiteral(password))

			Expect(expectedSQL).To(ContainSubstring(`"testuser"`))
			Expect(expectedSQL).To(ContainSubstring(`'newpassword123'`))
		})
	})

	Describe("GetUserInfo", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				info, err := adapter.GetUserInfo(ctx, "testuser")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(info).To(BeNil())
			})
		})

		It("should query pg_catalog.pg_roles without superuser/replication/bypassrls columns", func() {
			sql := buildCRDBGetUserInfoSQL()
			Expect(sql).To(ContainSubstring("rolname"))
			Expect(sql).To(ContainSubstring("rolcanlogin"))
			Expect(sql).To(ContainSubstring("rolcreatedb"))
			Expect(sql).To(ContainSubstring("rolcreaterole"))
			Expect(sql).To(ContainSubstring("rolinherit"))
			// CockroachDB does NOT have these columns
			Expect(sql).NotTo(ContainSubstring("rolsuper"))
			Expect(sql).NotTo(ContainSubstring("rolreplication"))
			Expect(sql).NotTo(ContainSubstring("rolbypassrls"))
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

		Describe("CreateUser with mock", func() {
			It("should execute CREATE ROLE with LOGIN (least privilege)", func() {
				mock.ExpectExec(`CREATE ROLE "testuser" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createUserWithCRDBMock(ctx, mock, types.CreateUserOptions{
					Username: "testuser",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should include password when specified", func() {
				mock.ExpectExec(`CREATE ROLE "testuser" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createUserWithCRDBMock(ctx, mock, types.CreateUserOptions{
					Username: "testuser",
					Password: "secretpass",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectExec(`CREATE ROLE "testuser"`).
					WillReturnError(fmt.Errorf("role already exists"))

				err := createUserWithCRDBMock(ctx, mock, types.CreateUserOptions{
					Username: "testuser",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create user"))
			})
		})

		Describe("DropUser with mock", func() {
			It("should execute REASSIGN + DROP OWNED + DROP ROLE", func() {
				mock.ExpectExec(`REASSIGN OWNED BY "testuser"`).
					WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
				mock.ExpectExec(`DROP OWNED BY "testuser"`).
					WillReturnResult(pgxmock.NewResult("DROP", 0))
				mock.ExpectExec(`DROP ROLE IF EXISTS "testuser"`).
					WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))

				err := dropUserWithCRDBMock(ctx, mock, "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("UserExists with mock", func() {
			It("should return true when user exists", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(true)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("testuser").
					WillReturnRows(rows)

				exists, err := userExistsWithCRDBMock(ctx, mock, "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return false when user does not exist", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := userExistsWithCRDBMock(ctx, mock, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("UpdateUser with mock", func() {
			It("should ALTER ROLE with CREATEDB", func() {
				createdb := true
				mock.ExpectExec(`ALTER ROLE "testuser" WITH CREATEDB`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateUserWithCRDBMock(ctx, mock, "testuser", types.UpdateUserOptions{
					CreateDB: &createdb,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should grant roles to user", func() {
				mock.ExpectExec(`GRANT "admin" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))
				mock.ExpectExec(`GRANT "developers" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))

				err := updateUserWithCRDBMock(ctx, mock, "testuser", types.UpdateUserOptions{
					InRoles: []string{"admin", "developers"},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("UpdatePassword with mock", func() {
			It("should ALTER ROLE with new password", func() {
				mock.ExpectExec(`ALTER ROLE "testuser" WITH PASSWORD`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updatePasswordWithCRDBMock(ctx, mock, "testuser", "newpassword123")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("GetUserInfo with mock", func() {
			It("should return user info with hardcoded false for CockroachDB-unsupported attrs", func() {
				// User info query
				userRows := pgxmock.NewRows([]string{
					"rolname", "rolconnlimit", "rolvaliduntil",
					"rolcreatedb", "rolcreaterole", "rolinherit", "rolcanlogin",
				}).AddRow("testuser", int32(-1), (*string)(nil), false, false, true, true)
				mock.ExpectQuery(`SELECT.*FROM.*pg_catalog\.pg_roles`).
					WithArgs("testuser").
					WillReturnRows(userRows)

				// Role membership query
				memberRows := pgxmock.NewRows([]string{"rolname"}).
					AddRow("app_read").
					AddRow("app_write")
				mock.ExpectQuery(`SELECT r\.rolname.*pg_auth_members`).
					WithArgs("testuser").
					WillReturnRows(memberRows)

				info, err := getUserInfoWithCRDBMock(ctx, mock, "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())
				Expect(info.Username).To(Equal("testuser"))
				Expect(info.Login).To(BeTrue())
				Expect(info.Inherit).To(BeTrue())
				Expect(info.CreateDB).To(BeFalse())
				Expect(info.CreateRole).To(BeFalse())
				// CockroachDB always returns false for these
				Expect(info.Superuser).To(BeFalse())
				Expect(info.Replication).To(BeFalse())
				Expect(info.BypassRLS).To(BeFalse())
				Expect(info.InRoles).To(ConsistOf("app_read", "app_write"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})
})

// SQL builder helper functions for CockroachDB user operations

func buildCRDBCreateUserSQL(opts types.CreateUserOptions) string {
	var sb strings.Builder
	sb.WriteString("CREATE ROLE ")
	sb.WriteString(escapeIdentifier(opts.Username))

	var roleOpts []string

	// Users always get LOGIN (distinguishes from roles)
	roleOpts = append(roleOpts, "LOGIN")

	if opts.Password != "" {
		roleOpts = append(roleOpts, fmt.Sprintf("PASSWORD %s", escapeLiteral(opts.Password)))
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

	if opts.ConnectionLimit != 0 {
		roleOpts = append(roleOpts, fmt.Sprintf("CONNECTION LIMIT %d", opts.ConnectionLimit))
	}

	if opts.ValidUntil != "" {
		roleOpts = append(roleOpts, fmt.Sprintf("VALID UNTIL %s", escapeLiteral(opts.ValidUntil)))
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

	return sb.String()
}

func buildCRDBUserExistsSQL() string {
	return "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)"
}

func buildCRDBUpdateUserSQL(username string, opts types.UpdateUserOptions) string {
	var alterOpts []string

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

	if opts.Login != nil {
		if *opts.Login {
			alterOpts = append(alterOpts, "LOGIN")
		} else {
			alterOpts = append(alterOpts, "NOLOGIN")
		}
	}

	if opts.ConnectionLimit != nil {
		alterOpts = append(alterOpts, fmt.Sprintf("CONNECTION LIMIT %d", *opts.ConnectionLimit))
	}

	if opts.ValidUntil != nil {
		alterOpts = append(alterOpts, fmt.Sprintf("VALID UNTIL %s", escapeLiteral(*opts.ValidUntil)))
	}

	if len(alterOpts) == 0 {
		return ""
	}

	return fmt.Sprintf("ALTER ROLE %s WITH %s",
		escapeIdentifier(username),
		strings.Join(alterOpts, " "))
}

func buildCRDBGetUserInfoSQL() string {
	return `
		SELECT rolname, rolconnlimit, rolvaliduntil,
		       rolcreatedb, rolcreaterole,
		       rolinherit, rolcanlogin
		FROM pg_catalog.pg_roles
		WHERE rolname = $1`
}

// Mock-based helper functions for CockroachDB user operations

func createUserWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.CreateUserOptions) error {
	sql := buildCRDBCreateUserSQL(opts)
	_, err := pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to create user %s: %w", opts.Username, err)
	}
	return nil
}

func dropUserWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string) error {
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(username)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(username)))

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(username))
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user %s: %w", username, err)
	}
	return nil
}

func userExistsWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}
	return exists, nil
}

func updateUserWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string, opts types.UpdateUserOptions) error {
	alterSQL := buildCRDBUpdateUserSQL(username, opts)
	if alterSQL != "" {
		if _, err := pool.Exec(ctx, alterSQL); err != nil {
			return fmt.Errorf("failed to update user %s: %w", username, err)
		}
	}

	for _, role := range opts.InRoles {
		query := fmt.Sprintf("GRANT %s TO %s", escapeIdentifier(role), escapeIdentifier(username))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to grant role %s to user %s: %w", role, username, err)
		}
	}

	return nil
}

func updatePasswordWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, username, password string) error {
	query := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD %s",
		escapeIdentifier(username),
		escapeLiteral(password))
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update password for user %s: %w", username, err)
	}
	return nil
}

func getUserInfoWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string) (*types.UserInfo, error) {
	var info types.UserInfo
	var validUntil *string
	err := pool.QueryRow(ctx, `
		SELECT rolname, rolconnlimit, rolvaliduntil,
		       rolcreatedb, rolcreaterole,
		       rolinherit, rolcanlogin
		FROM pg_catalog.pg_roles
		WHERE rolname = $1`,
		username).Scan(
		&info.Username, &info.ConnectionLimit, &validUntil,
		&info.CreateDB, &info.CreateRole,
		&info.Inherit, &info.Login)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	if validUntil != nil {
		info.ValidUntil = *validUntil
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
		username)
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
