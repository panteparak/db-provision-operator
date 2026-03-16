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

// TestClickHouseAdapter_Integration tests the ClickHouse adapter against a real database.
// All operations run sequentially in a single test to avoid container overhead and
// ensure deterministic ordering (create before exists, exists before drop, etc.).
func TestClickHouseAdapter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start ClickHouse container using generic testcontainers.
	// Use ForListeningPort instead of ForLog because the ready log message
	// varies between ClickHouse versions and the Alpine image.
	req := testcontainers.ContainerRequest{
		Image:        "clickhouse/clickhouse-server:24-alpine",
		ExposedPorts: []string{"9000/tcp"},
		WaitingFor:   wait.ForListeningPort("9000/tcp").WithStartupTimeout(60 * time.Second),
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

	// --- Connection ---

	err = adapter.Connect(ctx)
	require.NoError(t, err, "Failed to connect to ClickHouse")

	err = adapter.Ping(ctx)
	require.NoError(t, err, "Ping failed")

	version, err := adapter.GetVersion(ctx)
	require.NoError(t, err, "GetVersion failed")
	assert.NotEmpty(t, version, "Version should not be empty")
	t.Logf("ClickHouse version: %s", version)

	// --- Database operations ---

	testDBName := "integration_test_db"

	err = adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: testDBName})
	require.NoError(t, err, "CreateDatabase failed")

	exists, err := adapter.DatabaseExists(ctx, testDBName)
	require.NoError(t, err, "DatabaseExists failed")
	assert.True(t, exists, "Database should exist")

	info, err := adapter.GetDatabaseInfo(ctx, testDBName)
	require.NoError(t, err, "GetDatabaseInfo failed")
	assert.Equal(t, testDBName, info.Name)

	err = adapter.VerifyDatabaseAccess(ctx, testDBName)
	require.NoError(t, err, "VerifyDatabaseAccess failed")

	err = adapter.DropDatabase(ctx, testDBName, types.DropDatabaseOptions{Force: true})
	require.NoError(t, err, "DropDatabase failed")

	exists, err = adapter.DatabaseExists(ctx, testDBName)
	require.NoError(t, err)
	assert.False(t, exists, "Database should not exist after drop")

	// --- User operations ---

	testUsername := "test_user"
	testPassword := "testpass123"

	err = adapter.CreateUser(ctx, types.CreateUserOptions{
		Username: testUsername,
		Password: testPassword,
	})
	require.NoError(t, err, "CreateUser failed")

	exists, err = adapter.UserExists(ctx, testUsername)
	require.NoError(t, err, "UserExists failed")
	assert.True(t, exists, "User should exist")

	userInfo, err := adapter.GetUserInfo(ctx, testUsername)
	require.NoError(t, err, "GetUserInfo failed")
	assert.Equal(t, testUsername, userInfo.Username)

	err = adapter.UpdatePassword(ctx, testUsername, "newpass456")
	require.NoError(t, err, "UpdatePassword failed")

	exists, err = adapter.UserExists(ctx, testUsername)
	require.NoError(t, err)
	assert.True(t, exists, "User should still exist after password update")

	err = adapter.DropUser(ctx, testUsername)
	require.NoError(t, err, "DropUser failed")

	exists, err = adapter.UserExists(ctx, testUsername)
	require.NoError(t, err)
	assert.False(t, exists, "User should not exist after drop")

	// --- Role operations ---

	testRoleName := "test_role"

	err = adapter.CreateRole(ctx, types.CreateRoleOptions{RoleName: testRoleName})
	require.NoError(t, err, "CreateRole failed")

	exists, err = adapter.RoleExists(ctx, testRoleName)
	require.NoError(t, err, "RoleExists failed")
	assert.True(t, exists, "Role should exist")

	roleInfo, err := adapter.GetRoleInfo(ctx, testRoleName)
	require.NoError(t, err, "GetRoleInfo failed")
	assert.Equal(t, testRoleName, roleInfo.Name)

	err = adapter.DropRole(ctx, testRoleName)
	require.NoError(t, err, "DropRole failed")

	exists, err = adapter.RoleExists(ctx, testRoleName)
	require.NoError(t, err)
	assert.False(t, exists, "Role should not exist after drop")

	// --- Grant operations ---

	grantDBName := "grant_test_db"
	grantUsername := "grant_test_user"

	err = adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: grantDBName})
	require.NoError(t, err, "Setup: CreateDatabase for grants failed")

	err = adapter.CreateUser(ctx, types.CreateUserOptions{
		Username: grantUsername,
		Password: "testpass",
	})
	require.NoError(t, err, "Setup: CreateUser for grants failed")

	// Grant privileges
	err = adapter.Grant(ctx, grantUsername, []types.GrantOptions{
		{
			Database:   grantDBName,
			Privileges: []string{"SELECT", "INSERT"},
		},
	})
	require.NoError(t, err, "Grant failed")

	// Verify grants
	grants, err := adapter.GetGrants(ctx, grantUsername)
	require.NoError(t, err, "GetGrants failed")
	t.Logf("Grants for %s: %+v", grantUsername, grants)

	found := false
	for _, g := range grants {
		if g.Database == grantDBName {
			found = true
			assert.NotEmpty(t, g.Privileges, "Privileges should not be empty")
			break
		}
	}
	assert.True(t, found, "Should have grants on %s", grantDBName)

	// Revoke privileges
	err = adapter.Revoke(ctx, grantUsername, []types.GrantOptions{
		{
			Database:   grantDBName,
			Privileges: []string{"SELECT", "INSERT"},
		},
	})
	require.NoError(t, err, "Revoke failed")

	// Verify privileges are removed
	grants, err = adapter.GetGrants(ctx, grantUsername)
	require.NoError(t, err, "GetGrants after revoke failed")

	for _, g := range grants {
		if g.Database == grantDBName {
			for _, priv := range g.Privileges {
				assert.NotEqual(t, "SELECT", priv, "SELECT should have been revoked")
				assert.NotEqual(t, "INSERT", priv, "INSERT should have been revoked")
			}
		}
	}

	// Cleanup grants test resources
	_ = adapter.DropUser(ctx, grantUsername)
	_ = adapter.DropDatabase(ctx, grantDBName, types.DropDatabaseOptions{Force: true})

	// --- Backup/Restore not supported ---

	_, err = adapter.Backup(ctx, types.BackupOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet supported")

	_, err = adapter.Restore(ctx, types.RestoreOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet supported")

	// --- Close ---

	err = adapter.Close()
	require.NoError(t, err, "Close failed")
}
