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
	"strings"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
	"github.com/db-provision-operator/internal/adapter/types"
)

// CreateUser creates a new ClickHouse user
func (a *Adapter) CreateUser(ctx context.Context, opts types.CreateUserOptions) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	b := sqlbuilder.ClickHouseCreateUser(opts.Username)
	if opts.Password != "" {
		b.IdentifiedBy(opts.Password)
	}

	query := b.Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create user %s: %w", opts.Username, err)
	}

	// Apply HOST restrictions if specified
	if len(opts.HostRestrictions) > 0 {
		hostClause := strings.Join(opts.HostRestrictions, ", ")
		alterQuery := fmt.Sprintf("ALTER USER %s HOST %s",
			escapeIdentifier(opts.Username), hostClause)
		_, err = db.ExecContext(ctx, alterQuery)
		if err != nil {
			return fmt.Errorf("failed to set host restrictions for %s: %w", opts.Username, err)
		}
	}

	// Set default database if specified
	if opts.DefaultDatabase != "" {
		alterQuery := fmt.Sprintf("ALTER USER %s DEFAULT DATABASE %s",
			escapeIdentifier(opts.Username), escapeIdentifier(opts.DefaultDatabase))
		_, err = db.ExecContext(ctx, alterQuery)
		if err != nil {
			return fmt.Errorf("failed to set default database for %s: %w", opts.Username, err)
		}
	}

	// Set default role if specified
	if opts.DefaultRole != "" {
		alterQuery := fmt.Sprintf("ALTER USER %s DEFAULT ROLE %s",
			escapeIdentifier(opts.Username), escapeIdentifier(opts.DefaultRole))
		_, err = db.ExecContext(ctx, alterQuery)
		if err != nil {
			return fmt.Errorf("failed to set default role for %s: %w", opts.Username, err)
		}
	}

	return nil
}

// DropUser drops an existing ClickHouse user
func (a *Adapter) DropUser(ctx context.Context, username string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := sqlbuilder.ClickHouseDropUser(username).IfExists().Build()
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
		"SELECT count() FROM system.users WHERE name = ?",
		username).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return count > 0, nil
}

// UpdateUser updates an existing ClickHouse user
func (a *Adapter) UpdateUser(ctx context.Context, username string, opts types.UpdateUserOptions) error {
	// ClickHouse users have minimal updatable attributes.
	// Host restrictions and default database/role changes are handled via ALTER USER.
	return nil
}

// UpdatePassword updates a user's password
func (a *Adapter) UpdatePassword(ctx context.Context, username, password string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	query := sqlbuilder.ClickHouseAlterUser(username).IdentifiedBy(password).Build()
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update password for %s: %w", username, err)
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
	info.Username = username

	// Check if user exists
	var count uint64
	err = db.QueryRowContext(ctx,
		"SELECT count() FROM system.users WHERE name = ?",
		username).Scan(&count)
	if err != nil || count == 0 {
		return nil, fmt.Errorf("user %s not found", username)
	}

	return &info, nil
}

// GetOwnedObjects returns objects owned by a user.
// ClickHouse does not have a traditional ownership model, so this returns nil.
func (a *Adapter) GetOwnedObjects(_ context.Context, _ string) ([]types.OwnedObject, error) {
	return nil, nil
}
