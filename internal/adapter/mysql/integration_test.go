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

package mysql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/controller/testutil"
)

// TestMySQLAdapter_Integration tests the MySQL adapter against a real database
func TestMySQLAdapter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start MySQL container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "mysql",
		User:     "root",
		Password: "testpass",
		Database: "mysql",
	})
	require.NoError(t, err, "Failed to start MySQL container")
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Stop(stopCtx)
	}()

	// Wait for database to be ready
	err = container.WaitForReady(ctx)
	require.NoError(t, err, "Database not ready")

	host, port, user, password, database := container.ConnectionInfo()

	// Create adapter with real connection
	adapter := NewAdapter(types.ConnectionConfig{
		Host:     host,
		Port:     int32(port),
		Database: database,
		Username: user,
		Password: password,
	})

	t.Run("Connect", func(t *testing.T) {
		err := adapter.Connect(ctx)
		require.NoError(t, err, "Failed to connect to MySQL")
	})

	t.Run("Ping", func(t *testing.T) {
		err := adapter.Ping(ctx)
		require.NoError(t, err, "Ping failed")
	})

	t.Run("GetVersion", func(t *testing.T) {
		version, err := adapter.GetVersion(ctx)
		require.NoError(t, err, "GetVersion failed")
		assert.NotEmpty(t, version, "Version should not be empty")
		t.Logf("MySQL version: %s", version)
	})

	t.Run("DatabaseOperations", func(t *testing.T) {
		testDBName := "integration_test_db"

		// Create database
		t.Run("CreateDatabase", func(t *testing.T) {
			opts := types.CreateDatabaseOptions{
				Name:    testDBName,
				Charset: "utf8mb4",
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
			assert.NotEmpty(t, info.Charset)
			t.Logf("Database charset: %s", info.Charset)
		})

		// Drop database
		t.Run("DropDatabase", func(t *testing.T) {
			err := adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{})
			require.NoError(t, err, "DropDatabase failed")

			exists, err := adapter.DatabaseExists(ctx, testDBName)
			require.NoError(t, err)
			assert.False(t, exists, "Database should not exist after drop")
		})
	})

	t.Run("UserOperations", func(t *testing.T) {
		testUsername := "integration_test_user"
		testPassword := "testpass123"

		// Create user
		t.Run("CreateUser", func(t *testing.T) {
			opts := types.CreateUserOptions{
				Username:     testUsername,
				Password:     testPassword,
				AllowedHosts: []string{"%"},
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

			// Verify user still exists
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
		testRoleName := "integration_test_role"

		// Create role (MySQL 8.0+ supports native roles)
		t.Run("CreateRole", func(t *testing.T) {
			opts := types.CreateRoleOptions{
				RoleName:       testRoleName,
				UseNativeRoles: true,
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
			Username:     testUsername,
			Password:     "testpass",
			AllowedHosts: []string{"%"},
		})
		require.NoError(t, err)

		// Grant privileges
		t.Run("Grant", func(t *testing.T) {
			opts := []types.GrantOptions{
				{
					Database:   testDBName,
					Privileges: []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
					Level:      "database",
				},
			}
			err := adapter.Grant(ctx, testUsername, opts)
			require.NoError(t, err, "Grant failed")
		})

		// Get grants
		t.Run("GetGrants", func(t *testing.T) {
			grants, err := adapter.GetGrants(ctx, testUsername)
			require.NoError(t, err, "GetGrants failed")
			t.Logf("Grants for %s: %+v", testUsername, grants)
		})

		// Revoke privileges
		t.Run("Revoke", func(t *testing.T) {
			opts := []types.GrantOptions{
				{
					Database:   testDBName,
					Privileges: []string{"DELETE"},
					Level:      "database",
				},
			}
			err := adapter.Revoke(ctx, testUsername, opts)
			require.NoError(t, err, "Revoke failed")
		})

		// Cleanup
		_ = adapter.DropUser(ctx, testUsername)
		_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{})
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		require.NoError(t, err, "Close failed")
	})
}

// TestMariaDBAdapter_Integration tests the MySQL adapter against MariaDB
func TestMariaDBAdapter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start MariaDB container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "mariadb",
		User:     "root",
		Password: "testpass",
		Database: "mysql",
	})
	require.NoError(t, err, "Failed to start MariaDB container")
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Stop(stopCtx)
	}()

	// Wait for database to be ready
	err = container.WaitForReady(ctx)
	require.NoError(t, err, "Database not ready")

	host, port, user, password, database := container.ConnectionInfo()

	// Create adapter with real connection (MySQL adapter works with MariaDB)
	adapter := NewAdapter(types.ConnectionConfig{
		Host:     host,
		Port:     int32(port),
		Database: database,
		Username: user,
		Password: password,
	})

	t.Run("Connect", func(t *testing.T) {
		err := adapter.Connect(ctx)
		require.NoError(t, err, "Failed to connect to MariaDB")
	})

	t.Run("GetVersion", func(t *testing.T) {
		version, err := adapter.GetVersion(ctx)
		require.NoError(t, err, "GetVersion failed")
		assert.Contains(t, version, "MariaDB", "Version should contain 'MariaDB'")
		t.Logf("MariaDB version: %s", version)
	})

	// Basic operations to verify MariaDB compatibility
	t.Run("DatabaseOperations", func(t *testing.T) {
		testDBName := "mariadb_test_db"

		// Create database
		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name:    testDBName,
			Charset: "utf8mb4",
		})
		require.NoError(t, err, "CreateDatabase on MariaDB failed")

		// Verify database exists
		exists, err := adapter.DatabaseExists(ctx, testDBName)
		require.NoError(t, err)
		assert.True(t, exists, "Database should exist on MariaDB")

		// Cleanup
		err = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{})
		require.NoError(t, err)
	})

	t.Run("UserOperations", func(t *testing.T) {
		testUsername := "mariadb_test_user"

		// Create user
		err := adapter.CreateUser(ctx, types.CreateUserOptions{
			Username:     testUsername,
			Password:     "testpass123",
			AllowedHosts: []string{"%"},
		})
		require.NoError(t, err, "CreateUser on MariaDB failed")

		// Verify user exists
		exists, err := adapter.UserExists(ctx, testUsername)
		require.NoError(t, err)
		assert.True(t, exists, "User should exist on MariaDB")

		// Cleanup
		err = adapter.DropUser(ctx, testUsername)
		require.NoError(t, err)
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		require.NoError(t, err)
	})
}

// TestMySQLAdapter_ConcurrentOperations tests concurrent database operations
func TestMySQLAdapter_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start MySQL container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "mysql",
		User:     "root",
		Password: "testpass",
		Database: "mysql",
	})
	require.NoError(t, err)
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Stop(stopCtx)
	}()

	// Wait for database to be ready
	err = container.WaitForReady(ctx)
	require.NoError(t, err, "Database not ready")

	host, port, user, password, database := container.ConnectionInfo()

	adapter := NewAdapter(types.ConnectionConfig{
		Host:     host,
		Port:     int32(port),
		Database: database,
		Username: user,
		Password: password,
	})

	err = adapter.Connect(ctx)
	require.NoError(t, err)
	defer adapter.Close()

	// Create multiple databases concurrently
	t.Run("ConcurrentDatabaseCreation", func(t *testing.T) {
		numDatabases := 5
		errCh := make(chan error, numDatabases)

		for i := 0; i < numDatabases; i++ {
			go func(idx int) {
				dbName := fmt.Sprintf("concurrent_db_%d", idx)
				err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
					Name: dbName,
				})
				errCh <- err
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < numDatabases; i++ {
			err := <-errCh
			assert.NoError(t, err, "Concurrent database creation failed")
		}

		// Verify all databases exist
		for i := 0; i < numDatabases; i++ {
			dbName := fmt.Sprintf("concurrent_db_%d", i)
			exists, err := adapter.DatabaseExists(ctx, dbName)
			assert.NoError(t, err)
			assert.True(t, exists, "Database %s should exist", dbName)

			// Cleanup
			_ = adapter.DropDatabase(ctx, dbName, types.DropDatabaseOptions{})
		}
	})
}
