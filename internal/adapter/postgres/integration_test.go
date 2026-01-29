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

package postgres

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

// TestPostgresAdapter_Integration tests the PostgreSQL adapter against a real database
func TestPostgresAdapter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start PostgreSQL container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "postgresql",
		User:     "postgres",
		Password: "testpass",
		Database: "postgres",
	})
	require.NoError(t, err, "Failed to start PostgreSQL container")
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
		require.NoError(t, err, "Failed to connect to PostgreSQL")
	})

	t.Run("Ping", func(t *testing.T) {
		err := adapter.Ping(ctx)
		require.NoError(t, err, "Ping failed")
	})

	t.Run("GetVersion", func(t *testing.T) {
		version, err := adapter.GetVersion(ctx)
		require.NoError(t, err, "GetVersion failed")
		assert.NotEmpty(t, version, "Version should not be empty")
		t.Logf("PostgreSQL version: %s", version)
	})

	t.Run("DatabaseOperations", func(t *testing.T) {
		testDBName := "integration_test_db"

		// Create database
		t.Run("CreateDatabase", func(t *testing.T) {
			opts := types.CreateDatabaseOptions{
				Name:             testDBName,
				AllowConnections: true,
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
			assert.NotEmpty(t, info.Encoding)
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
		testUsername := "integration_test_user"
		testPassword := "testpass123"

		// Create user
		t.Run("CreateUser", func(t *testing.T) {
			opts := types.CreateUserOptions{
				Username: testUsername,
				Password: testPassword,
				Login:    true,
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
			assert.True(t, info.Login, "User should have login capability")
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
		testRoleName := "integration_test_role"

		// Create role
		t.Run("CreateRole", func(t *testing.T) {
			opts := types.CreateRoleOptions{
				RoleName: testRoleName,
				Login:    false,
				Inherit:  true,
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
			assert.False(t, info.Login, "Role should not have login capability")
			assert.True(t, info.Inherit, "Role should have inherit capability")
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
			Name:             testDBName,
			AllowConnections: true,
		})
		require.NoError(t, err)

		err = adapter.CreateUser(ctx, types.CreateUserOptions{
			Username: testUsername,
			Password: "testpass",
			Login:    true,
		})
		require.NoError(t, err)

		// Grant privileges
		t.Run("Grant", func(t *testing.T) {
			opts := []types.GrantOptions{
				{
					Database:   testDBName,
					Privileges: []string{"CONNECT", "CREATE"},
				},
			}
			err := adapter.Grant(ctx, testUsername, opts)
			require.NoError(t, err, "Grant failed")
		})

		// Get grants
		t.Run("GetGrants", func(t *testing.T) {
			grants, err := adapter.GetGrants(ctx, testUsername)
			require.NoError(t, err, "GetGrants failed")
			// Should have at least the database grant we just added
			t.Logf("Grants for %s: %+v", testUsername, grants)
		})

		// Revoke privileges
		t.Run("Revoke", func(t *testing.T) {
			opts := []types.GrantOptions{
				{
					Database:   testDBName,
					Privileges: []string{"CREATE"},
				},
			}
			err := adapter.Revoke(ctx, testUsername, opts)
			require.NoError(t, err, "Revoke failed")
		})

		// Cleanup
		_ = adapter.DropUser(ctx, testUsername)
		_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		require.NoError(t, err, "Close failed")
	})
}

// TestPostgresAdapter_DatabaseWithExtensions tests creating databases with extensions
func TestPostgresAdapter_DatabaseWithExtensions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start PostgreSQL container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "postgresql",
		User:     "postgres",
		Password: "testpass",
		Database: "postgres",
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

	testDBName := "extension_test_db"

	// Create database
	err = adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
		Name:             testDBName,
		AllowConnections: true,
	})
	require.NoError(t, err)

	// Update database to add extensions (uuid-ossp is a common extension)
	t.Run("AddExtension", func(t *testing.T) {
		err := adapter.UpdateDatabase(ctx, testDBName, types.UpdateDatabaseOptions{
			Extensions: []types.ExtensionOptions{
				{Name: "uuid-ossp"},
			},
		})
		require.NoError(t, err, "Adding extension failed")

		// Verify extension is installed
		info, err := adapter.GetDatabaseInfo(ctx, testDBName)
		require.NoError(t, err)

		hasExtension := false
		for _, ext := range info.Extensions {
			if ext.Name == "uuid-ossp" {
				hasExtension = true
				break
			}
		}
		assert.True(t, hasExtension, "uuid-ossp extension should be installed")
	})

	// Cleanup
	_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
}

// TestPostgresAdapter_ConcurrentOperations tests concurrent database operations
func TestPostgresAdapter_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start PostgreSQL container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "postgresql",
		User:     "postgres",
		Password: "testpass",
		Database: "postgres",
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

	// Create multiple users concurrently
	t.Run("ConcurrentUserCreation", func(t *testing.T) {
		numUsers := 5
		errCh := make(chan error, numUsers)

		for i := 0; i < numUsers; i++ {
			go func(idx int) {
				username := fmt.Sprintf("concurrent_user_%d", idx)
				err := adapter.CreateUser(ctx, types.CreateUserOptions{
					Username: username,
					Password: "testpass",
					Login:    true,
				})
				errCh <- err
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < numUsers; i++ {
			err := <-errCh
			assert.NoError(t, err, "Concurrent user creation failed")
		}

		// Verify all users exist
		for i := 0; i < numUsers; i++ {
			username := fmt.Sprintf("concurrent_user_%d", i)
			exists, err := adapter.UserExists(ctx, username)
			assert.NoError(t, err)
			assert.True(t, exists, "User %s should exist", username)

			// Cleanup
			_ = adapter.DropUser(ctx, username)
		}
	})
}
