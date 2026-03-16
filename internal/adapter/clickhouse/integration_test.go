//go:build integration

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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/db-provision-operator/internal/adapter/types"
)

// TestClickHouseAdapter_Integration tests the ClickHouse adapter against a real database
func TestClickHouseAdapter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start ClickHouse container using generic testcontainers
	req := testcontainers.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:24-alpine",
		ExposedPorts: []string{"9000/tcp"},
		WaitingFor:   wait.ForLog("Ready for connections").WithStartupTimeout(60 * time.Second),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start ClickHouse container")
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Terminate(stopCtx)
	}()

	host, err := container.Host(ctx)
	require.NoError(t, err, "Failed to get container host")

	mappedPort, err := container.MappedPort(ctx, "9000")
	require.NoError(t, err, "Failed to get mapped port")
	port := mappedPort.Int()

	// Create adapter with real connection
	adapter := NewAdapter(types.ConnectionConfig{
		Host:     host,
		Port:     int32(port),
		Database: "default",
		Username: "default",
		Password: "",
	})

	t.Run("Connect", func(t *testing.T) {
		err := adapter.Connect(ctx)
		require.NoError(t, err, "Failed to connect to ClickHouse")
	})

	t.Run("Ping", func(t *testing.T) {
		err := adapter.Ping(ctx)
		require.NoError(t, err, "Ping failed")
	})

	t.Run("GetVersion", func(t *testing.T) {
		version, err := adapter.GetVersion(ctx)
		require.NoError(t, err, "GetVersion failed")
		assert.NotEmpty(t, version, "Version should not be empty")
		t.Logf("ClickHouse version: %s", version)
	})

	t.Run("DatabaseOperations", func(t *testing.T) {
		testDBName := "integration_test_db"

		// Create database
		t.Run("CreateDatabase", func(t *testing.T) {
			opts := types.CreateDatabaseOptions{
				Name: testDBName,
			}
			err := adapter.CreateDatabase(ctx, opts)
			require.NoError(t, err, "CreateDatabase failed")
		})

		// Verify database exists
		t.Run("DatabaseExists", func(t *testing.T) {
			exists, err := adapter.DatabaseExists(ctx, testDBName)
			require.NoError(t, err, "DatabaseExists failed")
			assert.True(t, exists, "Database should exist")
		})

		// Get database info
		t.Run("GetDatabaseInfo", func(t *testing.T) {
			info, err := adapter.GetDatabaseInfo(ctx, testDBName)
			require.NoError(t, err, "GetDatabaseInfo failed")
			assert.Equal(t, testDBName, info.Name)
		})

		// Verify database access
		t.Run("VerifyDatabaseAccess", func(t *testing.T) {
			err := adapter.VerifyDatabaseAccess(ctx, testDBName)
			require.NoError(t, err, "VerifyDatabaseAccess failed")
		})

		// Drop database
		t.Run("DropDatabase", func(t *testing.T) {
			err := adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
			require.NoError(t, err, "DropDatabase failed")

			exists, err := adapter.DatabaseExists(ctx, testDBName)
			require.NoError(t, err)
			assert.False(t, exists, "Database should not exist after drop")
		})
	})

	t.Run("UserOperations", func(t *testing.T) {
		testUsername := "test_user"
		testPassword := "testpass123"

		// Create user
		t.Run("CreateUser", func(t *testing.T) {
			opts := types.CreateUserOptions{
				Username: testUsername,
				Password: testPassword,
			}
			err := adapter.CreateUser(ctx, opts)
			require.NoError(t, err, "CreateUser failed")
		})

		// Verify user exists
		t.Run("UserExists", func(t *testing.T) {
			exists, err := adapter.UserExists(ctx, testUsername)
			require.NoError(t, err, "UserExists failed")
			assert.True(t, exists, "User should exist")
		})

		// Get user info
		t.Run("GetUserInfo", func(t *testing.T) {
			info, err := adapter.GetUserInfo(ctx, testUsername)
			require.NoError(t, err, "GetUserInfo failed")
			assert.Equal(t, testUsername, info.Username)
		})

		// Update password
		t.Run("UpdatePassword", func(t *testing.T) {
			newPassword := "newpass456"
			err := adapter.UpdatePassword(ctx, testUsername, newPassword)
			require.NoError(t, err, "UpdatePassword failed")

			// Verify user can still be queried (password change doesn't affect admin connection)
			exists, err := adapter.UserExists(ctx, testUsername)
			require.NoError(t, err)
			assert.True(t, exists)
		})

		// Drop user
		t.Run("DropUser", func(t *testing.T) {
			err := adapter.DropUser(ctx, testUsername)
			require.NoError(t, err, "DropUser failed")

			exists, err := adapter.UserExists(ctx, testUsername)
			require.NoError(t, err)
			assert.False(t, exists, "User should not exist after drop")
		})
	})

	t.Run("RoleOperations", func(t *testing.T) {
		testRoleName := "test_role"

		// Create role
		t.Run("CreateRole", func(t *testing.T) {
			opts := types.CreateRoleOptions{
				RoleName: testRoleName,
			}
			err := adapter.CreateRole(ctx, opts)
			require.NoError(t, err, "CreateRole failed")
		})

		// Verify role exists
		t.Run("RoleExists", func(t *testing.T) {
			exists, err := adapter.RoleExists(ctx, testRoleName)
			require.NoError(t, err, "RoleExists failed")
			assert.True(t, exists, "Role should exist")
		})

		// Get role info
		t.Run("GetRoleInfo", func(t *testing.T) {
			info, err := adapter.GetRoleInfo(ctx, testRoleName)
			require.NoError(t, err, "GetRoleInfo failed")
			assert.Equal(t, testRoleName, info.Name)
		})

		// Drop role
		t.Run("DropRole", func(t *testing.T) {
			err := adapter.DropRole(ctx, testRoleName)
			require.NoError(t, err, "DropRole failed")

			exists, err := adapter.RoleExists(ctx, testRoleName)
			require.NoError(t, err)
			assert.False(t, exists, "Role should not exist after drop")
		})
	})

	t.Run("GrantOperations", func(t *testing.T) {
		// Create test database and user for grant testing
		testDBName := "grant_test_db"
		testUsername := "grant_test_user"

		// Setup
		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name: testDBName,
		})
		require.NoError(t, err)

		err = adapter.CreateUser(ctx, types.CreateUserOptions{
			Username: testUsername,
			Password: "testpass",
		})
		require.NoError(t, err)

		// Grant privileges
		t.Run("Grant", func(t *testing.T) {
			opts := []types.GrantOptions{
				{
					Database:   testDBName,
					Privileges: []string{"SELECT", "INSERT"},
				},
			}
			err := adapter.Grant(ctx, testUsername, opts)
			require.NoError(t, err, "Grant failed")
		})

		// Get grants and verify privileges are present
		t.Run("GetGrants", func(t *testing.T) {
			grants, err := adapter.GetGrants(ctx, testUsername)
			require.NoError(t, err, "GetGrants failed")
			t.Logf("Grants for %s: %+v", testUsername, grants)

			// Verify at least one grant exists for the test database
			found := false
			for _, g := range grants {
				if g.Database == testDBName {
					found = true
					assert.NotEmpty(t, g.Privileges, "Privileges should not be empty")
					break
				}
			}
			assert.True(t, found, "Should have grants on %s", testDBName)
		})

		// Revoke privileges
		t.Run("Revoke", func(t *testing.T) {
			opts := []types.GrantOptions{
				{
					Database:   testDBName,
					Privileges: []string{"SELECT", "INSERT"},
				},
			}
			err := adapter.Revoke(ctx, testUsername, opts)
			require.NoError(t, err, "Revoke failed")
		})

		// Verify privileges are removed
		t.Run("GetGrantsAfterRevoke", func(t *testing.T) {
			grants, err := adapter.GetGrants(ctx, testUsername)
			require.NoError(t, err, "GetGrants after revoke failed")

			for _, g := range grants {
				if g.Database == testDBName {
					// If there are still grants on this DB, they should not include revoked privileges
					for _, priv := range g.Privileges {
						assert.NotEqual(t, "SELECT", priv, "SELECT should have been revoked")
						assert.NotEqual(t, "INSERT", priv, "INSERT should have been revoked")
					}
				}
			}
		})

		// Cleanup
		_ = adapter.DropUser(ctx, testUsername)
		_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
	})

	t.Run("BackupNotSupported", func(t *testing.T) {
		_, err := adapter.Backup(ctx, types.BackupOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not yet supported")
	})

	t.Run("RestoreNotSupported", func(t *testing.T) {
		_, err := adapter.Restore(ctx, types.RestoreOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not yet supported")
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		require.NoError(t, err, "Close failed")
	})
}
