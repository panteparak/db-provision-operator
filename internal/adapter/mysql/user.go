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

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
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
		b := sqlbuilder.MySQLCreateUser(opts.Username, host)
		if opts.Password != "" {
			b.IdentifiedBy(opts.Password)
		}
		if opts.AuthPlugin != "" {
			b.AuthPlugin(opts.AuthPlugin)
		}

		query := b.Build()
		_, err = db.ExecContext(ctx, query)
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

	// Resource limits
	b := sqlbuilder.MySQLAlterUser(username, host)
	hasResourceOpts := false
	if opts.MaxQueriesPerHour > 0 {
		b.ResourceOption(fmt.Sprintf("MAX_QUERIES_PER_HOUR %d", opts.MaxQueriesPerHour))
		hasResourceOpts = true
	}
	if opts.MaxUpdatesPerHour > 0 {
		b.ResourceOption(fmt.Sprintf("MAX_UPDATES_PER_HOUR %d", opts.MaxUpdatesPerHour))
		hasResourceOpts = true
	}
	if opts.MaxConnectionsPerHour > 0 {
		b.ResourceOption(fmt.Sprintf("MAX_CONNECTIONS_PER_HOUR %d", opts.MaxConnectionsPerHour))
		hasResourceOpts = true
	}
	if opts.MaxUserConnections > 0 {
		b.ResourceOption(fmt.Sprintf("MAX_USER_CONNECTIONS %d", opts.MaxUserConnections))
		hasResourceOpts = true
	}

	if hasResourceOpts {
		query := b.Build()
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to set user options: %w", err)
		}
	}

	// SSL requirements
	if opts.RequireSSL || opts.RequireX509 {
		sslBuilder := sqlbuilder.MySQLAlterUser(username, host)
		if opts.RequireX509 {
			sslBuilder.RequireX509()
		} else {
			sslBuilder.RequireSSL()
		}
		query := sslBuilder.Build()
		_, err = db.ExecContext(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to set SSL requirement: %w", err)
		}
	}

	// Account lock
	if opts.AccountLocked {
		query := sqlbuilder.MySQLAlterUser(username, host).AccountLock().Build()
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
		query := sqlbuilder.MySQLDropUser(username, host).IfExists().Build()
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
		b := sqlbuilder.MySQLAlterUser(username, host)
		hasResourceOpts := false

		if opts.MaxQueriesPerHour != nil {
			b.ResourceOption(fmt.Sprintf("MAX_QUERIES_PER_HOUR %d", *opts.MaxQueriesPerHour))
			hasResourceOpts = true
		}
		if opts.MaxUpdatesPerHour != nil {
			b.ResourceOption(fmt.Sprintf("MAX_UPDATES_PER_HOUR %d", *opts.MaxUpdatesPerHour))
			hasResourceOpts = true
		}
		if opts.MaxConnectionsPerHour != nil {
			b.ResourceOption(fmt.Sprintf("MAX_CONNECTIONS_PER_HOUR %d", *opts.MaxConnectionsPerHour))
			hasResourceOpts = true
		}
		if opts.MaxUserConnections != nil {
			b.ResourceOption(fmt.Sprintf("MAX_USER_CONNECTIONS %d", *opts.MaxUserConnections))
			hasResourceOpts = true
		}

		if hasResourceOpts {
			query := b.Build()
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("failed to update user %s@%s: %w", username, host, err)
			}
		}

		// SSL requirements
		if opts.RequireSSL != nil || opts.RequireX509 != nil {
			sslBuilder := sqlbuilder.MySQLAlterUser(username, host)
			if opts.RequireX509 != nil && *opts.RequireX509 {
				sslBuilder.RequireX509()
			} else if opts.RequireSSL != nil && *opts.RequireSSL {
				sslBuilder.RequireSSL()
			} else {
				sslBuilder.RequireNone()
			}
			query := sslBuilder.Build()
			_, err = db.ExecContext(ctx, query)
			if err != nil {
				return fmt.Errorf("failed to set SSL requirement: %w", err)
			}
		}

		// Account lock
		if opts.AccountLocked != nil {
			lockBuilder := sqlbuilder.MySQLAlterUser(username, host)
			if *opts.AccountLocked {
				lockBuilder.AccountLock()
			} else {
				lockBuilder.AccountUnlock()
			}
			query := lockBuilder.Build()
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
		query := sqlbuilder.MySQLAlterUser(username, host).IdentifiedBy(password).Build()
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

// GetOwnedObjects returns database objects where the user is the DEFINER.
// In MySQL, objects don't have traditional "owners" like PostgreSQL, but
// views, routines, triggers, and events have a DEFINER attribute.
func (a *Adapter) GetOwnedObjects(ctx context.Context, username string) ([]types.OwnedObject, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var objects []types.OwnedObject

	// MySQL stores DEFINER as 'username@host', so we need to match the username part
	// For simplicity, we match any host variant of the username

	// Query views where user is DEFINER
	viewQuery := `
		SELECT TABLE_SCHEMA, TABLE_NAME, 'view'
		FROM information_schema.VIEWS
		WHERE DEFINER LIKE CONCAT(?, '@%')
		  AND TABLE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')`

	rows, err := db.QueryContext(ctx, viewQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to query views: %w", err)
	}

	for rows.Next() {
		var obj types.OwnedObject
		if err := rows.Scan(&obj.Schema, &obj.Name, &obj.Type); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed to scan view: %w", err)
		}
		objects = append(objects, obj)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating views: %w", err)
	}

	// Query routines (procedures and functions) where user is DEFINER
	routineQuery := `
		SELECT ROUTINE_SCHEMA, ROUTINE_NAME,
		       CASE ROUTINE_TYPE
		         WHEN 'PROCEDURE' THEN 'procedure'
		         WHEN 'FUNCTION' THEN 'function'
		       END
		FROM information_schema.ROUTINES
		WHERE DEFINER LIKE CONCAT(?, '@%')
		  AND ROUTINE_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')`

	rows, err = db.QueryContext(ctx, routineQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to query routines: %w", err)
	}

	for rows.Next() {
		var obj types.OwnedObject
		if err := rows.Scan(&obj.Schema, &obj.Name, &obj.Type); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed to scan routine: %w", err)
		}
		objects = append(objects, obj)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating routines: %w", err)
	}

	// Query triggers where user is DEFINER
	triggerQuery := `
		SELECT TRIGGER_SCHEMA, TRIGGER_NAME, 'trigger'
		FROM information_schema.TRIGGERS
		WHERE DEFINER LIKE CONCAT(?, '@%')
		  AND TRIGGER_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')`

	rows, err = db.QueryContext(ctx, triggerQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to query triggers: %w", err)
	}

	for rows.Next() {
		var obj types.OwnedObject
		if err := rows.Scan(&obj.Schema, &obj.Name, &obj.Type); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed to scan trigger: %w", err)
		}
		objects = append(objects, obj)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating triggers: %w", err)
	}

	// Query events where user is DEFINER
	eventQuery := `
		SELECT EVENT_SCHEMA, EVENT_NAME, 'event'
		FROM information_schema.EVENTS
		WHERE DEFINER LIKE CONCAT(?, '@%')
		  AND EVENT_SCHEMA NOT IN ('mysql', 'information_schema', 'performance_schema', 'sys')`

	rows, err = db.QueryContext(ctx, eventQuery, username)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var obj types.OwnedObject
		if err := rows.Scan(&obj.Schema, &obj.Name, &obj.Type); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		objects = append(objects, obj)
	}

	return objects, rows.Err()
}
