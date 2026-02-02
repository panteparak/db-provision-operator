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

package cockroachdb

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

// TestCockroachDBAdapter_Integration tests the CockroachDB adapter against a real database
func TestCockroachDBAdapter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start CockroachDB container in insecure mode (no password for root)
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "cockroachdb",
		User:     "root",
		Database: "defaultdb",
	})
	require.NoError(t, err, "Failed to start CockroachDB container")
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
		require.NoError(t, err, "Failed to connect to CockroachDB")
	})

	t.Run("Ping", func(t *testing.T) {
		err := adapter.Ping(ctx)
		require.NoError(t, err, "Ping failed")
	})

	t.Run("GetVersion", func(t *testing.T) {
		version, err := adapter.GetVersion(ctx)
		require.NoError(t, err, "GetVersion failed")
		assert.NotEmpty(t, version, "Version should not be empty")
		assert.Contains(t, version, "CockroachDB", "Version should identify as CockroachDB")
		t.Logf("CockroachDB version: %s", version)
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
			assert.NotEmpty(t, info.Encoding)
			t.Logf("Database encoding: %s, owner: %s", info.Encoding, info.Owner)
		})

		// Verify database access
		t.Run("VerifyDatabaseAccess", func(t *testing.T) {
			err := adapter.VerifyDatabaseAccess(ctx, testDBName)
			require.NoError(t, err, "VerifyDatabaseAccess failed")
		})

		// Update database (create schema)
		t.Run("UpdateDatabase_CreateSchema", func(t *testing.T) {
			err := adapter.UpdateDatabase(ctx, testDBName, types.UpdateDatabaseOptions{
				Schemas: []types.SchemaOptions{
					{Name: "app"},
				},
			})
			require.NoError(t, err, "UpdateDatabase with schema failed")

			// Verify schema exists by checking database info
			info, err := adapter.GetDatabaseInfo(ctx, testDBName)
			require.NoError(t, err)
			assert.Contains(t, info.Schemas, "app", "Schema 'app' should be present")
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

	t.Run("DatabaseWithOwner", func(t *testing.T) {
		testDBName := "owned_test_db"
		ownerName := "db_owner"

		// Create the owner user first
		err := adapter.CreateUser(ctx, types.CreateUserOptions{
			Username: ownerName,
			Password: "ownerpass",
			Login:    true,
			CreateDB: true,
		})
		require.NoError(t, err, "CreateUser for owner failed")

		// Create database with owner
		err = adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name:  testDBName,
			Owner: ownerName,
		})
		require.NoError(t, err, "CreateDatabase with owner failed")

		// Verify owner is set
		info, err := adapter.GetDatabaseInfo(ctx, testDBName)
		require.NoError(t, err, "GetDatabaseInfo failed")
		assert.Equal(t, ownerName, info.Owner, "Database owner should be %s", ownerName)

		// Cleanup
		_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
		_ = adapter.DropUser(ctx, ownerName)
	})

	t.Run("UserOperations", func(t *testing.T) {
		testUsername := "integration_test_user"
		testPassword := "testpass123"

		// Create user with least-privilege defaults
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

		// Get user info and verify least-privilege defaults
		t.Run("GetUserInfo_LeastPrivilege", func(t *testing.T) {
			info, err := adapter.GetUserInfo(ctx, testUsername)
			require.NoError(t, err, "GetUserInfo failed")
			assert.Equal(t, testUsername, info.Username)
			assert.True(t, info.Login, "User should have LOGIN")
			assert.False(t, info.CreateDB, "User should NOT have CREATEDB (least-privilege)")
			assert.False(t, info.CreateRole, "User should NOT have CREATEROLE (least-privilege)")
			assert.False(t, info.Superuser, "CockroachDB has no SUPERUSER concept")
			assert.False(t, info.Replication, "CockroachDB has no REPLICATION concept")
			assert.False(t, info.BypassRLS, "CockroachDB has no BYPASSRLS concept")
		})

		// Update password
		t.Run("UpdatePassword", func(t *testing.T) {
			newPassword := "newpass456"
			err := adapter.UpdatePassword(ctx, testUsername, newPassword)
			require.NoError(t, err, "UpdatePassword failed")

			// Verify user still exists (password change doesn't affect admin connection)
			exists, err := adapter.UserExists(ctx, testUsername)
			require.NoError(t, err)
			assert.True(t, exists)
		})

		// Update user attributes
		t.Run("UpdateUser", func(t *testing.T) {
			createDB := true
			err := adapter.UpdateUser(ctx, testUsername, types.UpdateUserOptions{
				CreateDB: &createDB,
			})
			require.NoError(t, err, "UpdateUser failed")

			info, err := adapter.GetUserInfo(ctx, testUsername)
			require.NoError(t, err)
			assert.True(t, info.CreateDB, "User should now have CREATEDB")
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

	t.Run("UserWithRoleMembership", func(t *testing.T) {
		roleName := "app_group"
		userName := "member_user"

		// Create role first
		err := adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName: roleName,
		})
		require.NoError(t, err)

		// Create user with IN ROLE
		err = adapter.CreateUser(ctx, types.CreateUserOptions{
			Username: userName,
			Password: "testpass",
			Login:    true,
			InRoles:  []string{roleName},
		})
		require.NoError(t, err)

		// Verify membership
		info, err := adapter.GetUserInfo(ctx, userName)
		require.NoError(t, err)
		assert.Contains(t, info.InRoles, roleName, "User should be member of %s", roleName)

		// Cleanup
		_ = adapter.DropUser(ctx, userName)
		_ = adapter.DropRole(ctx, roleName)
	})

	t.Run("RoleOperations", func(t *testing.T) {
		testRoleName := "integration_test_role"

		// Create role with NOLOGIN default
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

		// Get role info and verify least-privilege defaults
		t.Run("GetRoleInfo_LeastPrivilege", func(t *testing.T) {
			info, err := adapter.GetRoleInfo(ctx, testRoleName)
			require.NoError(t, err, "GetRoleInfo failed")
			assert.Equal(t, testRoleName, info.Name)
			assert.False(t, info.Login, "Role should NOT have LOGIN")
			assert.True(t, info.Inherit, "Role should have INHERIT (explicitly set)")
			assert.False(t, info.CreateDB, "Role should NOT have CREATEDB (least-privilege)")
			assert.False(t, info.CreateRole, "Role should NOT have CREATEROLE (least-privilege)")
			assert.False(t, info.Superuser, "CockroachDB has no SUPERUSER concept")
			assert.False(t, info.Replication, "CockroachDB has no REPLICATION concept")
			assert.False(t, info.BypassRLS, "CockroachDB has no BYPASSRLS concept")
		})

		// Update role
		t.Run("UpdateRole", func(t *testing.T) {
			loginTrue := true
			err := adapter.UpdateRole(ctx, testRoleName, types.UpdateRoleOptions{
				Login: &loginTrue,
			})
			require.NoError(t, err, "UpdateRole failed")

			info, err := adapter.GetRoleInfo(ctx, testRoleName)
			require.NoError(t, err)
			assert.True(t, info.Login, "Role should now have LOGIN")
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

	t.Run("RoleWithMembers", func(t *testing.T) {
		parentRole := "parent_group"
		childRole := "child_group"

		// Create parent role
		err := adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName: parentRole,
		})
		require.NoError(t, err)

		// Create child role with IN ROLE parent
		err = adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName: childRole,
			InRoles:  []string{parentRole},
		})
		require.NoError(t, err)

		// Verify child is member of parent
		info, err := adapter.GetRoleInfo(ctx, childRole)
		require.NoError(t, err)
		assert.Contains(t, info.InRoles, parentRole, "Child should be member of parent")

		// Cleanup (child first to avoid dependency issues)
		_ = adapter.DropRole(ctx, childRole)
		_ = adapter.DropRole(ctx, parentRole)
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
			Login:    true,
		})
		require.NoError(t, err)

		// Grant database-level privileges
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

		// Get grants and verify
		t.Run("GetGrants", func(t *testing.T) {
			grants, err := adapter.GetGrants(ctx, testUsername)
			require.NoError(t, err, "GetGrants failed")

			// Should have at least the database grant we just added
			foundGrant := false
			for _, g := range grants {
				if g.Database == testDBName {
					foundGrant = true
					t.Logf("Grant for %s on %s: %v", testUsername, testDBName, g.Privileges)
					break
				}
			}
			assert.True(t, foundGrant, "Should have a grant on %s", testDBName)
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

		// Grant role membership
		t.Run("GrantRole", func(t *testing.T) {
			roleName := "grant_test_role"
			err := adapter.CreateRole(ctx, types.CreateRoleOptions{
				RoleName: roleName,
			})
			require.NoError(t, err)

			err = adapter.GrantRole(ctx, testUsername, []string{roleName})
			require.NoError(t, err, "GrantRole failed")

			// Verify membership
			info, err := adapter.GetUserInfo(ctx, testUsername)
			require.NoError(t, err)
			assert.Contains(t, info.InRoles, roleName, "User should be member of role")

			// Revoke role membership
			err = adapter.RevokeRole(ctx, testUsername, []string{roleName})
			require.NoError(t, err, "RevokeRole failed")

			info, err = adapter.GetUserInfo(ctx, testUsername)
			require.NoError(t, err)
			assert.NotContains(t, info.InRoles, roleName, "User should no longer be member of role")

			// Cleanup
			_ = adapter.DropRole(ctx, roleName)
		})

		// Cleanup
		_ = adapter.DropUser(ctx, testUsername)
		_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
	})

	t.Run("SafeDropPattern", func(t *testing.T) {
		// Test that DropUser handles REASSIGN OWNED BY + DROP OWNED BY correctly
		// when the user owns objects in a database
		testDBName := "owned_objects_db"
		testUsername := "object_owner"

		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name: testDBName,
		})
		require.NoError(t, err)

		err = adapter.CreateUser(ctx, types.CreateUserOptions{
			Username: testUsername,
			Password: "testpass",
			Login:    true,
		})
		require.NoError(t, err)

		// Grant privileges so user could own objects
		err = adapter.Grant(ctx, testUsername, []types.GrantOptions{
			{
				Database:   testDBName,
				Privileges: []string{"ALL"},
			},
		})
		require.NoError(t, err)

		// Drop user should succeed even with grants (safe pattern handles cleanup)
		err = adapter.DropUser(ctx, testUsername)
		require.NoError(t, err, "DropUser with owned objects should succeed")

		exists, err := adapter.UserExists(ctx, testUsername)
		require.NoError(t, err)
		assert.False(t, exists, "User should not exist after safe drop")

		// Cleanup
		_ = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
	})

	t.Run("Close", func(t *testing.T) {
		err := adapter.Close()
		require.NoError(t, err, "Close failed")
	})
}

// TestCockroachDBAdapter_ConcurrentOperations tests concurrent database operations
func TestCockroachDBAdapter_ConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start CockroachDB container
	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "cockroachdb",
		User:     "root",
		Database: "defaultdb",
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

// TestCockroachDBAdapter_ReconnectBehavior tests adapter reconnection behavior
func TestCockroachDBAdapter_ReconnectBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	container, err := testutil.StartDatabaseContainer(ctx, testutil.DatabaseContainerConfig{
		Engine:   "cockroachdb",
		User:     "root",
		Database: "defaultdb",
	})
	require.NoError(t, err)
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = container.Stop(stopCtx)
	}()

	err = container.WaitForReady(ctx)
	require.NoError(t, err)

	host, port, user, password, database := container.ConnectionInfo()

	adapter := NewAdapter(types.ConnectionConfig{
		Host:     host,
		Port:     int32(port),
		Database: database,
		Username: user,
		Password: password,
	})

	// Connect
	err = adapter.Connect(ctx)
	require.NoError(t, err)

	// Close
	err = adapter.Close()
	require.NoError(t, err)

	// Operations should fail after close
	err = adapter.Ping(ctx)
	assert.Error(t, err, "Ping should fail after close")
	assert.Contains(t, err.Error(), "not connected")

	// Reconnect should succeed
	err = adapter.Connect(ctx)
	require.NoError(t, err, "Reconnect should succeed")

	// Operations should work again
	err = adapter.Ping(ctx)
	require.NoError(t, err, "Ping should succeed after reconnect")

	// Connect again while connected should be a no-op
	err = adapter.Connect(ctx)
	require.NoError(t, err, "Double connect should be a no-op")

	err = adapter.Close()
	require.NoError(t, err)
}
