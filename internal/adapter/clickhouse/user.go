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

package clickhouse

import (
	"context"
	"fmt"

	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateUser creates a new ClickHouse user
func (a *Adapter) CreateUser(ctx context.Context, opts types.CreateUserOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("CREATE USER IF NOT EXISTS %s IDENTIFIED BY %s",
		escapeIdentifier(opts.Username),
		escapeLiteral(opts.Password))

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create user %s: %w", opts.Username, err)
	}

	return nil
}

// DropUser drops an existing ClickHouse user
func (a *Adapter) DropUser(ctx context.Context, username string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("DROP USER IF EXISTS %s", escapeIdentifier(username))
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop user %s: %w", username, err)
	}

	return nil
}

// UserExists checks if a ClickHouse user exists
func (a *Adapter) UserExists(ctx context.Context, username string) (bool, error) {
	db, err := a.getDB()
	if err != nil {
		return false, err
	}

	var count uint64
	err = db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT count() FROM system.users WHERE name = %s", escapeLiteral(username)),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return count > 0, nil
}

// UpdateUser is a no-op for ClickHouse as most user settings are managed via
// server configuration files rather than SQL statements.
func (a *Adapter) UpdateUser(_ context.Context, _ string, _ types.UpdateUserOptions) error {
	return nil
}

// UpdatePassword updates a ClickHouse user's password
func (a *Adapter) UpdatePassword(ctx context.Context, username, password string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := fmt.Sprintf("ALTER USER %s IDENTIFIED BY %s",
		escapeIdentifier(username),
		escapeLiteral(password))

	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update password for user %s: %w", username, err)
	}

	return nil
}

// GetUserInfo retrieves information about a ClickHouse user
func (a *Adapter) GetUserInfo(ctx context.Context, username string) (*types.UserInfo, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	var info types.UserInfo
	var name string
	err = db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT name FROM system.users WHERE name = %s", escapeLiteral(username)),
	).Scan(&name)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info for %s: %w", username, err)
	}

	info.Username = name
	return &info, nil
}

// DisableUser is a no-op for ClickHouse as it does not support NOLOGIN semantics.
func (a *Adapter) DisableUser(_ context.Context, _ string) error {
	return nil
}

// GetOwnedObjects returns an empty slice for ClickHouse as ClickHouse does not
// have object ownership semantics like PostgreSQL or MySQL.
func (a *Adapter) GetOwnedObjects(_ context.Context, _ string) ([]types.OwnedObject, error) {
	return []types.OwnedObject{}, nil
}
