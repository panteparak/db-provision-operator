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

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateUser creates a new CockroachDB user (role with LOGIN).
//
// Least-privilege enforcement:
//   - Always defaults to LOGIN (users must be able to connect)
//   - Defaults to NOCREATEDB, NOCREATEROLE (no admin capabilities)
//   - CockroachDB does NOT support SUPERUSER, REPLICATION, or BYPASSRLS
//   - These PostgreSQL-specific attributes are silently ignored
//
// Insecure mode handling:
//   - CockroachDB in --insecure mode does NOT support password authentication
//   - If password setting fails with "insecure mode" error, the user is created without password
//
// The LOGIN attribute distinguishes users from roles in CockroachDB.
func (a *Adapter) CreateUser(ctx context.Context, opts types.CreateUserOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// Build CREATE ROLE statement
	query, hasPassword := a.buildCreateUserQuery(opts)

	_, err = pool.Exec(ctx, query)
	if err != nil {
		// Check if this is an insecure mode error - CockroachDB doesn't support passwords in insecure mode
		if hasPassword && isInsecureModeError(err) {
			// Retry without password for insecure mode compatibility
			queryNoPassword, _ := a.buildCreateUserQuery(types.CreateUserOptions{
				Username:        opts.Username,
				Password:        "", // No password
				CreateDB:        opts.CreateDB,
				CreateRole:      opts.CreateRole,
				Inherit:         opts.Inherit,
				ConnectionLimit: opts.ConnectionLimit,
				ValidUntil:      opts.ValidUntil,
				InRoles:         opts.InRoles,
				ConfigParams:    opts.ConfigParams,
			})
			if _, retryErr := pool.Exec(ctx, queryNoPassword); retryErr != nil {
				return fmt.Errorf("failed to create user %s (insecure mode, no password): %w", opts.Username, retryErr)
			}
			// User created successfully without password in insecure mode
		} else {
			return fmt.Errorf("failed to create user %s: %w", opts.Username, err)
		}
	}

	// Set configuration parameters if any
	for param, value := range opts.ConfigParams {
		query := fmt.Sprintf("ALTER ROLE %s SET %s = %s",
			escapeIdentifier(opts.Username),
			escapeIdentifier(param),
			escapeLiteral(value))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to set config param %s for user %s: %w", param, opts.Username, err)
		}
	}

	return nil
}

// buildCreateUserQuery constructs the CREATE ROLE query for a user.
// Returns the query string and whether a password was included.
func (a *Adapter) buildCreateUserQuery(opts types.CreateUserOptions) (string, bool) {
	var sb strings.Builder
	sb.WriteString("CREATE ROLE ")
	sb.WriteString(escapeIdentifier(opts.Username))

	var roleOpts []string
	hasPassword := false

	// Users always get LOGIN (this is what distinguishes them from roles)
	roleOpts = append(roleOpts, "LOGIN")

	if opts.Password != "" {
		roleOpts = append(roleOpts, fmt.Sprintf("PASSWORD %s", escapeLiteral(opts.Password)))
		hasPassword = true
	}

	// CockroachDB-supported role attributes with least-privilege defaults
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

	// Note: CockroachDB does NOT support INHERIT/NOINHERIT
	// The Inherit field is silently ignored for CockroachDB compatibility

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

	return sb.String(), hasPassword
}

// isInsecureModeError checks if the error indicates CockroachDB is running in insecure mode
// and cannot set passwords.
func isInsecureModeError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "insecure mode") ||
		strings.Contains(errStr, "password is not supported in insecure")
}

// DropUser drops an existing CockroachDB user.
// Follows the safe cleanup pattern: REASSIGN OWNED BY + DROP OWNED BY before DROP ROLE.
// This ensures all objects owned by the user are transferred to CURRENT_USER
// and all privileges are revoked before the role is dropped.
func (a *Adapter) DropUser(ctx context.Context, username string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	// Reassign owned objects to current user, then drop remaining owned objects
	_, _ = pool.Exec(ctx, fmt.Sprintf("REASSIGN OWNED BY %s TO CURRENT_USER", escapeIdentifier(username)))
	_, _ = pool.Exec(ctx, fmt.Sprintf("DROP OWNED BY %s", escapeIdentifier(username)))

	query := fmt.Sprintf("DROP ROLE IF EXISTS %s", escapeIdentifier(username))
	_, err = pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user %s: %w", username, err)
	}

	return nil
}

// UserExists checks if a CockroachDB user exists.
// CockroachDB exposes users/roles through pg_catalog.pg_roles for PostgreSQL compatibility.
func (a *Adapter) UserExists(ctx context.Context, username string) (bool, error) {
	pool, err := a.getPool()
	if err != nil {
		return false, err
	}

	var exists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// UpdateUser updates an existing CockroachDB user.
// Only CockroachDB-supported attributes are applied; Superuser, Replication,
// and BypassRLS fields are silently ignored as they are PostgreSQL-specific.
func (a *Adapter) UpdateUser(ctx context.Context, username string, opts types.UpdateUserOptions) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

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

	// Note: CockroachDB does NOT support INHERIT/NOINHERIT
	// The Inherit field is silently ignored for CockroachDB compatibility

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

	if len(alterOpts) > 0 {
		query := fmt.Sprintf("ALTER ROLE %s WITH %s",
			escapeIdentifier(username),
			strings.Join(alterOpts, " "))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to update user %s: %w", username, err)
		}
	}

	// Handle role membership changes
	for _, role := range opts.InRoles {
		query := fmt.Sprintf("GRANT %s TO %s", escapeIdentifier(role), escapeIdentifier(username))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to grant role %s to user %s: %w", role, username, err)
		}
	}

	// Set configuration parameters
	for param, value := range opts.ConfigParams {
		query := fmt.Sprintf("ALTER ROLE %s SET %s = %s",
			escapeIdentifier(username),
			escapeIdentifier(param),
			escapeLiteral(value))
		if _, err := pool.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to set config param %s for user %s: %w", param, username, err)
		}
	}

	return nil
}

// UpdatePassword updates a user's password in CockroachDB.
// In insecure mode, password updates are silently ignored since passwords don't work.
func (a *Adapter) UpdatePassword(ctx context.Context, username, password string) error {
	pool, err := a.getPool()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("ALTER ROLE %s WITH PASSWORD %s",
		escapeIdentifier(username),
		escapeLiteral(password))
	_, err = pool.Exec(ctx, query)
	if err != nil {
		// In insecure mode, password updates are not supported - silently ignore
		if isInsecureModeError(err) {
			return nil
		}
		return fmt.Errorf("failed to update password for user %s: %w", username, err)
	}

	return nil
}

// GetUserInfo retrieves information about a CockroachDB user.
// CockroachDB's pg_roles has fewer columns than PostgreSQL:
// no rolsuper, rolreplication, rolbypassrls. These are always false.
func (a *Adapter) GetUserInfo(ctx context.Context, username string) (*types.UserInfo, error) {
	pool, err := a.getPool()
	if err != nil {
		return nil, err
	}

	var info types.UserInfo
	var validUntil *string
	err = pool.QueryRow(ctx, `
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
