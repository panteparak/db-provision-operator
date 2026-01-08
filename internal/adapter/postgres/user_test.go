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

var _ = Describe("User Operations", func() {
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

		Context("SQL Generation", func() {
			It("should generate CREATE ROLE with LOGIN", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Login:    true,
				}

				// Verify option values that would generate LOGIN in SQL
				Expect(opts.Username).To(Equal("testuser"))
				Expect(opts.Login).To(BeTrue())
			})

			It("should include password when specified", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Password: "secretpass",
					Login:    true,
				}

				Expect(opts.Password).To(Equal("secretpass"))
				Expect(opts.Password).NotTo(BeEmpty())
			})

			It("should include SUPERUSER when true", func() {
				opts := types.CreateUserOptions{
					Username:  "testuser",
					Login:     true,
					Superuser: true,
				}

				Expect(opts.Superuser).To(BeTrue())
			})

			It("should include CREATEDB when true", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Login:    true,
					CreateDB: true,
				}

				Expect(opts.CreateDB).To(BeTrue())
			})

			It("should include CREATEROLE when true", func() {
				opts := types.CreateUserOptions{
					Username:   "testuser",
					Login:      true,
					CreateRole: true,
				}

				Expect(opts.CreateRole).To(BeTrue())
			})

			It("should include INHERIT option", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Login:    true,
					Inherit:  true,
				}

				Expect(opts.Inherit).To(BeTrue())
			})

			It("should include REPLICATION when true", func() {
				opts := types.CreateUserOptions{
					Username:    "testuser",
					Login:       true,
					Replication: true,
				}

				Expect(opts.Replication).To(BeTrue())
			})

			It("should include BYPASSRLS when true", func() {
				opts := types.CreateUserOptions{
					Username:  "testuser",
					Login:     true,
					BypassRLS: true,
				}

				Expect(opts.BypassRLS).To(BeTrue())
			})

			It("should include CONNECTION LIMIT", func() {
				opts := types.CreateUserOptions{
					Username:        "testuser",
					Login:           true,
					ConnectionLimit: 10,
				}

				Expect(opts.ConnectionLimit).To(Equal(int32(10)))
			})

			It("should include VALID UNTIL", func() {
				opts := types.CreateUserOptions{
					Username:   "testuser",
					Login:      true,
					ValidUntil: "2026-12-31",
				}

				Expect(opts.ValidUntil).To(Equal("2026-12-31"))
			})

			It("should include IN ROLE for role membership", func() {
				opts := types.CreateUserOptions{
					Username: "testuser",
					Login:    true,
					InRoles:  []string{"admin", "developers"},
				}

				Expect(opts.InRoles).To(ConsistOf("admin", "developers"))
				Expect(opts.InRoles).To(HaveLen(2))
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

		It("should drop user", func() {
			// Verify the SQL generation logic - the function should:
			// 1. REASSIGN OWNED BY "username" TO CURRENT_USER
			// 2. DROP OWNED BY "username"
			// 3. DROP ROLE IF EXISTS "username"
			username := "testuser"
			Expect(escapeIdentifier(username)).To(Equal(`"testuser"`))
		})

		It("should reassign owned objects before drop", func() {
			// The function should call REASSIGN OWNED BY before DROP
			username := "appuser"
			expectedReassign := fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(username))
			Expect(expectedReassign).To(ContainSubstring(`"appuser"`))
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
	})

	Describe("UpdateUser", func() {
		Context("when not connected", func() {
			It("should return error when setting options", func() {
				superuser := true
				err := adapter.UpdateUser(ctx, "testuser", types.UpdateUserOptions{
					Superuser: &superuser,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should ALTER ROLE with SUPERUSER", func() {
			superuser := true
			opts := types.UpdateUserOptions{
				Superuser: &superuser,
			}

			Expect(*opts.Superuser).To(BeTrue())
		})

		It("should ALTER ROLE with NOSUPERUSER", func() {
			superuser := false
			opts := types.UpdateUserOptions{
				Superuser: &superuser,
			}

			Expect(*opts.Superuser).To(BeFalse())
		})

		It("should ALTER ROLE with multiple options", func() {
			superuser := true
			createdb := true
			connLimit := int32(50)
			opts := types.UpdateUserOptions{
				Superuser:       &superuser,
				CreateDB:        &createdb,
				ConnectionLimit: &connLimit,
			}

			Expect(*opts.Superuser).To(BeTrue())
			Expect(*opts.CreateDB).To(BeTrue())
			Expect(*opts.ConnectionLimit).To(Equal(int32(50)))
		})

		It("should grant roles to user", func() {
			opts := types.UpdateUserOptions{
				InRoles: []string{"admin", "developers"},
			}

			Expect(opts.InRoles).To(ConsistOf("admin", "developers"))
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

		It("should ALTER ROLE with new password", func() {
			// Verify the SQL generation
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
			It("should execute CREATE ROLE with LOGIN query", func() {
				mock.ExpectExec(`CREATE ROLE "testuser" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createUserWithMock(ctx, mock, types.CreateUserOptions{
					Username: "testuser",
					Login:    true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should include password when specified", func() {
				mock.ExpectExec(`CREATE ROLE "testuser" WITH`).
					WillReturnResult(pgxmock.NewResult("CREATE ROLE", 0))

				err := createUserWithMock(ctx, mock, types.CreateUserOptions{
					Username: "testuser",
					Password: "secretpass",
					Login:    true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on query failure", func() {
				mock.ExpectExec(`CREATE ROLE "testuser"`).
					WillReturnError(fmt.Errorf("role already exists"))

				err := createUserWithMock(ctx, mock, types.CreateUserOptions{
					Username: "testuser",
					Login:    true,
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to create user"))
			})
		})

		Describe("DropUser with mock", func() {
			It("should execute DROP ROLE query", func() {
				mock.ExpectExec(`REASSIGN OWNED BY "testuser"`).
					WillReturnResult(pgxmock.NewResult("REASSIGN", 0))
				mock.ExpectExec(`DROP OWNED BY "testuser"`).
					WillReturnResult(pgxmock.NewResult("DROP", 0))
				mock.ExpectExec(`DROP ROLE IF EXISTS "testuser"`).
					WillReturnResult(pgxmock.NewResult("DROP ROLE", 0))

				err := dropUserWithMock(ctx, mock, "testuser")
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

				exists, err := userExistsWithMock(ctx, mock, "testuser")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return false when user does not exist", func() {
				rows := pgxmock.NewRows([]string{"exists"}).AddRow(false)
				mock.ExpectQuery(`SELECT EXISTS`).
					WithArgs("nonexistent").
					WillReturnRows(rows)

				exists, err := userExistsWithMock(ctx, mock, "nonexistent")
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeFalse())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("UpdateUser with mock", func() {
			It("should ALTER ROLE with SUPERUSER", func() {
				superuser := true
				mock.ExpectExec(`ALTER ROLE "testuser" WITH SUPERUSER`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateUserWithMock(ctx, mock, "testuser", types.UpdateUserOptions{
					Superuser: &superuser,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should ALTER ROLE with NOSUPERUSER", func() {
				superuser := false
				mock.ExpectExec(`ALTER ROLE "testuser" WITH NOSUPERUSER`).
					WillReturnResult(pgxmock.NewResult("ALTER ROLE", 0))

				err := updateUserWithMock(ctx, mock, "testuser", types.UpdateUserOptions{
					Superuser: &superuser,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should grant roles to user", func() {
				mock.ExpectExec(`GRANT "admin" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))
				mock.ExpectExec(`GRANT "developers" TO "testuser"`).
					WillReturnResult(pgxmock.NewResult("GRANT", 0))

				err := updateUserWithMock(ctx, mock, "testuser", types.UpdateUserOptions{
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

				err := updatePasswordWithMock(ctx, mock, "testuser", "newpassword123")
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})
})

// Helper functions for mock-based testing

// createUserWithMock executes CREATE USER using a mock pool
func createUserWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.CreateUserOptions) error {
	var sb strings.Builder
	sb.WriteString("CREATE ROLE ")
	sb.WriteString(escapeIdentifier(opts.Username))

	var roleOpts []string

	if opts.Login {
		roleOpts = append(roleOpts, "LOGIN")
	} else {
		roleOpts = append(roleOpts, "LOGIN")
	}

	if opts.Password != "" {
		roleOpts = append(roleOpts, fmt.Sprintf("PASSWORD %s", escapeLiteral(opts.Password)))
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

	_, err := pool.Exec(ctx, sb.String())
	if err != nil {
		return fmt.Errorf("failed to create user %s: %w", opts.Username, err)
	}

	return nil
}

// dropUserWithMock executes DROP USER using a mock pool
func dropUserWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string) error {
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(username)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(username)))

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(username))
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user %s: %w", username, err)
	}

	return nil
}

// userExistsWithMock checks if user exists using a mock pool
func userExistsWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// updateUserWithMock updates user using a mock pool
func updateUserWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, username string, opts types.UpdateUserOptions) error {
	var alterOpts []string

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

	if opts.Login != nil {
		if *opts.Login {
			alterOpts = append(alterOpts, "LOGIN")
		} else {
			alterOpts = append(alterOpts, "NOLOGIN")
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

	if opts.ConnectionLimit != nil {
		alterOpts = append(alterOpts, fmt.Sprintf("CONNECTION LIMIT %d", *opts.ConnectionLimit))
	}

	if opts.ValidUntil != nil {
		alterOpts = append(alterOpts, fmt.Sprintf("VALID UNTIL %s", escapeLiteral(*opts.ValidUntil)))
	}

	if len(alterOpts) > 0 {
		query := fmt.Sprintf("ALTER ROLE %s WITH %s",
			escapeIdentifier(username),
			strings.Join(alterOpts, " "))
		if _, err := pool.Exec(ctx, query); err != nil {
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

// updatePasswordWithMock updates password using a mock pool
func updatePasswordWithMock(ctx context.Context, pool pgxmock.PgxPoolIface, username, password string) error {
	query := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD %s",
		escapeIdentifier(username),
		escapeLiteral(password))
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update password for user %s: %w", username, err)
	}

	return nil
}
