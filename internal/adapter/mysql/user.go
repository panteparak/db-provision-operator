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
	"fmt"
	"strings"

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateUser creates a new MySQL user
func (a *Adapter) CreateUser(ctx context.Context, opts types.CreateUserOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Get hosts to create user for
	hosts := opts.AllowedHosts
	if len(hosts) == 0 {
		hosts = []string{"%"} // Default to all hosts
	}

	for _, host := range hosts {
		var sb strings.Builder
		sb.WriteString("CREATE USER IF NOT EXISTS ")
		sb.WriteString(fmt.Sprintf("%s@%s", escapeLiteral(opts.Username), escapeLiteral(host)))

		// Add authentication
		if opts.Password != "" {
			sb.WriteString(fmt.Sprintf(" IDENTIFIED BY %s", escapeLiteral(opts.Password)))
		}

		if opts.AuthPlugin != "" {
			sb.WriteString(fmt.Sprintf(" WITH %s", opts.AuthPlugin))
		}

		_, err = db.ExecContext(ctx, sb.String())
		if err != nil {
			return fmt.Errorf("failed to create user %s@%s: %w", opts.Username, host, err)
		}

		// Apply resource limits and other options
		if err := a.applyUserOptions(ctx, opts.Username, host, opts); err != nil {
			return err
		}
	}

	return nil
}

// applyUserOptions applies options to an existing user
func (a *Adapter) applyUserOptions(ctx context.Context, username, host string, opts types.CreateUserOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	var alterParts []string

	// Resource limits
	if opts.MaxQueriesPerHour > 0 {
		alterParts = append(alterParts, fmt.Sprintf("MAX_QUERIES_PER_HOUR %d", opts.MaxQueriesPerHour))
	}
	if opts.MaxUpdatesPerHour > 0 {
		alterParts = append(alterParts, fmt.Sprintf("MAX_UPDATES_PER_HOUR %d", opts.MaxUpdatesPerHour))
	}
	if opts.MaxConnectionsPerHour > 0 {
		alterParts = append(alterParts, fmt.Sprintf("MAX_CONNECTIONS_PER_HOUR %d", opts.MaxConnectionsPerHour))
	}
	if opts.MaxUserConnections > 0 {
		alterParts = append(alterParts, fmt.Sprintf("MAX_USER_CONNECTIONS %d", opts.MaxUserConnections))
	}

	if len(alterParts) > 0 {
		query := fmt.Sprintf("ALTER USER %s@%s WITH %s",
			escapeLiteral(username),
			escapeLiteral(host),
			strings.Join(alterParts, " "))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to set user options: %w", err)
		}
	}

	// SSL requirements
	if opts.RequireSSL || opts.RequireX509 {
		var require string
		if opts.RequireX509 {
			require = "REQUIRE X509"
		} else {
			require = "REQUIRE SSL"
		}
		query := fmt.Sprintf("ALTER USER %s@%s %s",
			escapeLiteral(username),
			escapeLiteral(host),
			require)
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to set SSL requirement: %w", err)
		}
	}

	// Account lock
	if opts.AccountLocked {
		query := fmt.Sprintf("ALTER USER %s@%s ACCOUNT LOCK",
			escapeLiteral(username),
			escapeLiteral(host))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to lock account: %w", err)
		}
	}

	return nil
}

// DropUser drops an existing MySQL user
func (a *Adapter) DropUser(ctx context.Context, username string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Get all hosts for this user
	rows, err := db.QueryContext(ctx,
		"SELECT Host FROM mysql.user WHERE User = ?",
		username)
	if err != nil {
		return fmt.Errorf("failed to get user hosts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hosts []string
	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return err
		}
		hosts = append(hosts, host)
	}

	// Drop user for each host
	for _, host := range hosts {
		query := fmt.Sprintf("DROP USER IF EXISTS %s@%s",
			escapeLiteral(username),
			escapeLiteral(host))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to drop user %s@%s: %w", username, host, err)
		}
	}

	return nil
}

// UserExists checks if a MySQL user exists
func (a *Adapter) UserExists(ctx context.Context, username string) (bool, error) {
	db, err := a.getDB()
	if err != nil {
		return false, err
	}

	var exists bool
	err = db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM mysql.user WHERE User = ?)",
		username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// UpdateUser updates an existing MySQL user
func (a *Adapter) UpdateUser(ctx context.Context, username string, opts types.UpdateUserOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Get all hosts for this user
	rows, err := db.QueryContext(ctx,
		"SELECT Host FROM mysql.user WHERE User = ?",
		username)
	if err != nil {
		return fmt.Errorf("failed to get user hosts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hosts []string
	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return err
		}
		hosts = append(hosts, host)
	}

	// Apply updates for each host
	for _, host := range hosts {
		var alterParts []string

		if opts.MaxQueriesPerHour != nil {
			alterParts = append(alterParts, fmt.Sprintf("MAX_QUERIES_PER_HOUR %d", *opts.MaxQueriesPerHour))
		}
		if opts.MaxUpdatesPerHour != nil {
			alterParts = append(alterParts, fmt.Sprintf("MAX_UPDATES_PER_HOUR %d", *opts.MaxUpdatesPerHour))
		}
		if opts.MaxConnectionsPerHour != nil {
			alterParts = append(alterParts, fmt.Sprintf("MAX_CONNECTIONS_PER_HOUR %d", *opts.MaxConnectionsPerHour))
		}
		if opts.MaxUserConnections != nil {
			alterParts = append(alterParts, fmt.Sprintf("MAX_USER_CONNECTIONS %d", *opts.MaxUserConnections))
		}

		if len(alterParts) > 0 {
			query := fmt.Sprintf("ALTER USER %s@%s WITH %s",
				escapeLiteral(username),
				escapeLiteral(host),
				strings.Join(alterParts, " "))
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("failed to update user %s@%s: %w", username, host, err)
			}
		}

		// SSL requirements
		if opts.RequireSSL != nil || opts.RequireX509 != nil {
			var require string
			if opts.RequireX509 != nil && *opts.RequireX509 {
				require = "REQUIRE X509"
			} else if opts.RequireSSL != nil && *opts.RequireSSL {
				require = "REQUIRE SSL"
			} else {
				require = "REQUIRE NONE"
			}
			query := fmt.Sprintf("ALTER USER %s@%s %s",
				escapeLiteral(username),
				escapeLiteral(host),
				require)
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("failed to set SSL requirement: %w", err)
			}
		}

		// Account lock
		if opts.AccountLocked != nil {
			var lock string
			if *opts.AccountLocked {
				lock = "ACCOUNT LOCK"
			} else {
				lock = "ACCOUNT UNLOCK"
			}
			query := fmt.Sprintf("ALTER USER %s@%s %s",
				escapeLiteral(username),
				escapeLiteral(host),
				lock)
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("failed to set account lock: %w", err)
			}
		}
	}

	return nil
}

// UpdatePassword updates a user's password
func (a *Adapter) UpdatePassword(ctx context.Context, username, password string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Get all hosts for this user
	rows, err := db.QueryContext(ctx,
		"SELECT Host FROM mysql.user WHERE User = ?",
		username)
	if err != nil {
		return fmt.Errorf("failed to get user hosts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var hosts []string
	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return err
		}
		hosts = append(hosts, host)
	}

	// Update password for each host
	for _, host := range hosts {
		query := fmt.Sprintf("ALTER USER %s@%s IDENTIFIED BY %s",
			escapeLiteral(username),
			escapeLiteral(host),
			escapeLiteral(password))
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to update password for %s@%s: %w", username, host, err)
		}
	}

	return nil
}

// GetUserInfo retrieves information about a MySQL user
func (a *Adapter) GetUserInfo(ctx context.Context, username string) (*types.UserInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var info types.UserInfo
	info.Username = username

	// Get all hosts for this user
	rows, err := db.QueryContext(ctx,
		"SELECT Host FROM mysql.user WHERE User = ?",
		username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var host string
		if err := rows.Scan(&host); err != nil {
			return nil, err
		}
		info.AllowedHosts = append(info.AllowedHosts, host)
	}

	if len(info.AllowedHosts) == 0 {
		return nil, fmt.Errorf("user %s not found", username)
	}

	return &info, nil
}
