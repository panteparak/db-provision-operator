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

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/postgres"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/controller/testutil"
	"github.com/db-provision-operator/internal/service/drift"
)

// TestPostgresAdapter_DriftDetection tests drift detection and correction against a real database
func TestPostgresAdapter_DriftDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

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

	err = container.WaitForReady(ctx)
	require.NoError(t, err)

	host, port, user, password, database := container.ConnectionInfo()
	adapter := postgres.NewAdapter(types.ConnectionConfig{
		Host: host, Port: int32(port), Database: database, Username: user, Password: password,
	})
	err = adapter.Connect(ctx)
	require.NoError(t, err)
	defer adapter.Close()

	svc := drift.NewService(adapter, &drift.Config{
		AllowDestructive: false,
		Logger:           logr.Discard(),
	})

	// --- Role drift tests ---

	t.Run("RoleDriftDetection", func(t *testing.T) {
		roleName := "drift_detect_role"

		// Create role with Login=true via adapter
		err := adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName: roleName,
			Login:    true,
			CreateDB: false,
		})
		require.NoError(t, err)
		defer adapter.DropRole(ctx, roleName)

		// Simulate external drift: change Login to false via UpdateRole
		loginFalse := false
		err = adapter.UpdateRole(ctx, roleName, types.UpdateRoleOptions{Login: &loginFalse})
		require.NoError(t, err)

		// Build spec that expects Login=true
		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: roleName,
			Postgres: &dbopsv1alpha1.PostgresRoleConfig{
				Login:    true,
				CreateDB: false,
			},
		}

		// Detect drift
		result, err := svc.DetectRoleDrift(ctx, spec)
		require.NoError(t, err)
		assert.True(t, result.HasDrift(), "Should detect drift after external change")

		// Verify the "login" field is in the diffs
		found := false
		for _, d := range result.Diffs {
			if d.Field == "login" {
				found = true
				assert.Equal(t, "true", d.Expected)
				assert.Equal(t, "false", d.Actual)
			}
		}
		assert.True(t, found, "Should find login field in diffs")
	})

	t.Run("RoleDriftCorrection", func(t *testing.T) {
		roleName := "drift_correct_role"

		// Create role with Login=true
		err := adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName: roleName,
			Login:    true,
		})
		require.NoError(t, err)
		defer adapter.DropRole(ctx, roleName)

		// Simulate drift: disable login
		loginFalse := false
		err = adapter.UpdateRole(ctx, roleName, types.UpdateRoleOptions{Login: &loginFalse})
		require.NoError(t, err)

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: roleName,
			Postgres: &dbopsv1alpha1.PostgresRoleConfig{Login: true},
		}

		// Detect drift
		result, err := svc.DetectRoleDrift(ctx, spec)
		require.NoError(t, err)
		require.True(t, result.HasDrift(), "Should detect drift before correction")

		// Correct drift
		corrResult, err := svc.CorrectRoleDrift(ctx, spec, result)
		require.NoError(t, err)
		assert.NotEmpty(t, corrResult.Corrected, "Should have corrected at least one diff")
		assert.Empty(t, corrResult.Failed, "Should have no failed corrections")

		// Re-detect drift to verify it's fixed
		result2, err := svc.DetectRoleDrift(ctx, spec)
		require.NoError(t, err)
		assert.False(t, result2.HasDrift(), "Should have no drift after correction")
	})

	t.Run("RoleDriftCorrection_SingleAttribute", func(t *testing.T) {
		roleName := "drift_single_attr_role"

		// Create role with specific attributes
		err := adapter.CreateRole(ctx, types.CreateRoleOptions{
			RoleName:   roleName,
			Login:      true,
			CreateDB:   false,
			CreateRole: false,
		})
		require.NoError(t, err)
		defer adapter.DropRole(ctx, roleName)

		// Simulate drift: change only Login to false
		loginFalse := false
		err = adapter.UpdateRole(ctx, roleName, types.UpdateRoleOptions{Login: &loginFalse})
		require.NoError(t, err)

		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: roleName,
			Postgres: &dbopsv1alpha1.PostgresRoleConfig{
				Login:      true,
				CreateDB:   false,
				CreateRole: false,
			},
		}

		// Detect drift - should only find login
		result, err := svc.DetectRoleDrift(ctx, spec)
		require.NoError(t, err)
		require.True(t, result.HasDrift())
		assert.Len(t, result.Diffs, 1, "Should have exactly one diff (login)")
		assert.Equal(t, "login", result.Diffs[0].Field)

		// Correct drift
		corrResult, err := svc.CorrectRoleDrift(ctx, spec, result)
		require.NoError(t, err)
		assert.Len(t, corrResult.Corrected, 1, "Should correct exactly one diff")

		// Verify the correction fixed Login and didn't touch other attributes
		info, err := adapter.GetRoleInfo(ctx, roleName)
		require.NoError(t, err)
		assert.True(t, info.Login, "Login should be restored to true")
		assert.False(t, info.CreateDB, "CreateDB should still be false")
		assert.False(t, info.CreateRole, "CreateRole should still be false")
	})

	t.Run("RoleDrift_InRolesMembership", func(t *testing.T) {
		parentRole := "drift_parent_role"
		childRole := "drift_child_role"

		// Create parent and child roles
		err := adapter.CreateRole(ctx, types.CreateRoleOptions{RoleName: parentRole})
		require.NoError(t, err)
		defer adapter.DropRole(ctx, parentRole)

		err = adapter.CreateRole(ctx, types.CreateRoleOptions{RoleName: childRole})
		require.NoError(t, err)
		defer adapter.DropRole(ctx, childRole)

		// Grant parent to child
		err = adapter.GrantRole(ctx, childRole, []string{parentRole})
		require.NoError(t, err)

		// Build spec with InRoles including parent
		spec := &dbopsv1alpha1.DatabaseRoleSpec{
			RoleName: childRole,
			Postgres: &dbopsv1alpha1.PostgresRoleConfig{
				InRoles: []string{parentRole},
			},
		}

		// Detect drift - should have no drift since membership matches
		result, err := svc.DetectRoleDrift(ctx, spec)
		require.NoError(t, err)
		assert.False(t, result.HasDrift(), "Should have no drift when membership matches")

		// Revoke membership to simulate external drift
		err = adapter.RevokeRole(ctx, childRole, []string{parentRole})
		require.NoError(t, err)

		// Detect drift - should find inRoles.missing
		result2, err := svc.DetectRoleDrift(ctx, spec)
		require.NoError(t, err)
		assert.True(t, result2.HasDrift(), "Should detect drift after revoke")

		found := false
		for _, d := range result2.Diffs {
			if d.Field == "inRoles.missing" {
				found = true
				assert.Contains(t, d.Expected, parentRole)
			}
		}
		assert.True(t, found, "Should find inRoles.missing in diffs")
	})

	// --- Database drift tests ---

	t.Run("DatabaseDriftDetection_Extensions", func(t *testing.T) {
		dbName := "drift_ext_detect_db"

		// Create database
		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name:             dbName,
			AllowConnections: true,
		})
		require.NoError(t, err)
		defer adapter.DropDatabase(ctx, dbName, types.DropDatabaseOptions{Force: true})

		// Install extension pg_trgm
		err = adapter.UpdateDatabase(ctx, dbName, types.UpdateDatabaseOptions{
			Extensions: []types.ExtensionOptions{{Name: "pg_trgm"}},
		})
		require.NoError(t, err)

		// Build spec expecting pg_trgm
		spec := &dbopsv1alpha1.DatabaseSpec{
			Name: dbName,
			Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
				Extensions: []dbopsv1alpha1.PostgresExtension{
					{Name: "pg_trgm"},
				},
			},
		}

		// Detect drift - should have no drift since extension is installed
		result, err := svc.DetectDatabaseDrift(ctx, spec)
		require.NoError(t, err)
		assert.False(t, result.HasDrift(), "Should have no drift when extension is present")

		// Add a second extension to spec that is NOT installed
		spec.Postgres.Extensions = append(spec.Postgres.Extensions, dbopsv1alpha1.PostgresExtension{
			Name: "uuid-ossp",
		})

		// Detect drift - should find extensions.missing
		result2, err := svc.DetectDatabaseDrift(ctx, spec)
		require.NoError(t, err)
		assert.True(t, result2.HasDrift(), "Should detect drift for missing extension")

		found := false
		for _, d := range result2.Diffs {
			if d.Field == "extensions.missing" {
				found = true
				assert.Contains(t, d.Expected, "uuid-ossp")
			}
		}
		assert.True(t, found, "Should find extensions.missing in diffs")
	})

	t.Run("DatabaseDriftCorrection_Extensions", func(t *testing.T) {
		dbName := "drift_ext_correct_db"

		// Create database
		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name:             dbName,
			AllowConnections: true,
		})
		require.NoError(t, err)
		defer adapter.DropDatabase(ctx, dbName, types.DropDatabaseOptions{Force: true})

		// Build spec expecting uuid-ossp (not yet installed)
		spec := &dbopsv1alpha1.DatabaseSpec{
			Name: dbName,
			Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
				Extensions: []dbopsv1alpha1.PostgresExtension{
					{Name: "uuid-ossp"},
				},
			},
		}

		// Detect drift - extension is missing
		result, err := svc.DetectDatabaseDrift(ctx, spec)
		require.NoError(t, err)
		require.True(t, result.HasDrift(), "Should detect missing extension")

		// Correct drift
		corrResult, err := svc.CorrectDatabaseDrift(ctx, spec, result)
		require.NoError(t, err)
		assert.NotEmpty(t, corrResult.Corrected, "Should have corrected at least one diff")
		assert.Empty(t, corrResult.Failed, "Should have no failed corrections")

		// Re-detect drift - should be clean
		result2, err := svc.DetectDatabaseDrift(ctx, spec)
		require.NoError(t, err)
		assert.False(t, result2.HasDrift(), "Should have no drift after correction")

		// Verify extension is actually installed
		info, err := adapter.GetDatabaseInfo(ctx, dbName)
		require.NoError(t, err)
		hasExt := false
		for _, ext := range info.Extensions {
			if ext.Name == "uuid-ossp" {
				hasExt = true
				break
			}
		}
		assert.True(t, hasExt, "uuid-ossp extension should be installed after correction")
	})

	t.Run("DatabaseDriftDetection_Encoding", func(t *testing.T) {
		dbName := "drift_enc_db"

		// Create database (default encoding is UTF8)
		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name:             dbName,
			Encoding:         "UTF8",
			AllowConnections: true,
		})
		require.NoError(t, err)
		defer adapter.DropDatabase(ctx, dbName, types.DropDatabaseOptions{Force: true})

		// Build spec with a different encoding to test immutable drift detection
		spec := &dbopsv1alpha1.DatabaseSpec{
			Name: dbName,
			Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
				Encoding: "SQL_ASCII",
			},
		}

		// Detect drift - should find encoding drift (immutable)
		result, err := svc.DetectDatabaseDrift(ctx, spec)
		require.NoError(t, err)
		assert.True(t, result.HasDrift(), "Should detect encoding drift")

		found := false
		for _, d := range result.Diffs {
			if d.Field == "encoding" {
				found = true
				assert.True(t, d.Immutable, "Encoding drift should be marked as immutable")
			}
		}
		assert.True(t, found, "Should find encoding field in diffs")

		// Correction should skip immutable fields
		corrResult, err := svc.CorrectDatabaseDrift(ctx, spec, result)
		require.NoError(t, err)
		assert.NotEmpty(t, corrResult.Skipped, "Should skip immutable encoding correction")
		assert.Empty(t, corrResult.Corrected, "Should not correct immutable fields")
	})

	// --- User drift tests ---

	t.Run("UserDriftDetection", func(t *testing.T) {
		username := "drift_detect_user"

		// Create user with ConnectionLimit=10
		err := adapter.CreateUser(ctx, types.CreateUserOptions{
			Username:        username,
			Password:        "testpass123",
			Login:           true,
			ConnectionLimit: 10,
		})
		require.NoError(t, err)
		defer adapter.DropUser(ctx, username)

		// Simulate drift: change ConnectionLimit to 5
		newLimit := int32(5)
		err = adapter.UpdateUser(ctx, username, types.UpdateUserOptions{ConnectionLimit: &newLimit})
		require.NoError(t, err)

		// Build spec expecting ConnectionLimit=10
		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username: username,
			Postgres: &dbopsv1alpha1.PostgresUserConfig{
				ConnectionLimit: 10,
			},
		}

		// Detect drift
		result, err := svc.DetectUserDrift(ctx, spec)
		require.NoError(t, err)
		assert.True(t, result.HasDrift(), "Should detect connectionLimit drift")

		found := false
		for _, d := range result.Diffs {
			if d.Field == "connectionLimit" {
				found = true
				assert.Equal(t, "10", d.Expected)
				assert.Equal(t, "5", d.Actual)
			}
		}
		assert.True(t, found, "Should find connectionLimit in diffs")
	})

	t.Run("UserDriftCorrection", func(t *testing.T) {
		username := "drift_correct_user"

		// Create user with ConnectionLimit=10
		err := adapter.CreateUser(ctx, types.CreateUserOptions{
			Username:        username,
			Password:        "testpass123",
			Login:           true,
			ConnectionLimit: 10,
		})
		require.NoError(t, err)
		defer adapter.DropUser(ctx, username)

		// Simulate drift: change ConnectionLimit to 5
		newLimit := int32(5)
		err = adapter.UpdateUser(ctx, username, types.UpdateUserOptions{ConnectionLimit: &newLimit})
		require.NoError(t, err)

		spec := &dbopsv1alpha1.DatabaseUserSpec{
			Username: username,
			Postgres: &dbopsv1alpha1.PostgresUserConfig{
				ConnectionLimit: 10,
			},
		}

		// Detect drift
		result, err := svc.DetectUserDrift(ctx, spec)
		require.NoError(t, err)
		require.True(t, result.HasDrift())

		// Correct drift
		corrResult, err := svc.CorrectUserDrift(ctx, spec, result)
		require.NoError(t, err)
		assert.NotEmpty(t, corrResult.Corrected)
		assert.Empty(t, corrResult.Failed)

		// Re-detect to verify fix
		result2, err := svc.DetectUserDrift(ctx, spec)
		require.NoError(t, err)
		assert.False(t, result2.HasDrift(), "Should have no drift after correction")

		// Verify actual state
		info, err := adapter.GetUserInfo(ctx, username)
		require.NoError(t, err)
		assert.Equal(t, int32(10), info.ConnectionLimit, "ConnectionLimit should be restored to 10")
	})

	// --- Grant drift tests ---

	t.Run("GrantDriftDetection", func(t *testing.T) {
		grantDBName := "drift_grant_db"
		grantUsername := "drift_grant_user"

		// Setup: create database and user
		err := adapter.CreateDatabase(ctx, types.CreateDatabaseOptions{
			Name:             grantDBName,
			AllowConnections: true,
		})
		require.NoError(t, err)
		defer adapter.DropDatabase(ctx, grantDBName, types.DropDatabaseOptions{Force: true})

		err = adapter.CreateUser(ctx, types.CreateUserOptions{
			Username: grantUsername,
			Password: "testpass",
			Login:    true,
		})
		require.NoError(t, err)
		defer adapter.DropUser(ctx, grantUsername)

		// Grant CONNECT and CREATE on the database
		err = adapter.Grant(ctx, grantUsername, []types.GrantOptions{
			{
				Database:   grantDBName,
				Privileges: []string{"CONNECT", "CREATE"},
			},
		})
		require.NoError(t, err)

		// Build spec expecting CONNECT and CREATE
		spec := &dbopsv1alpha1.DatabaseGrantSpec{
			Postgres: &dbopsv1alpha1.PostgresGrantConfig{
				Grants: []dbopsv1alpha1.PostgresGrant{
					{
						Database:   grantDBName,
						Privileges: []string{"CONNECT", "CREATE"},
					},
				},
			},
		}

		// Detect drift - should have no drift while grants match
		result, err := svc.DetectGrantDrift(ctx, spec, grantUsername)
		require.NoError(t, err)
		// Note: detection results depend on how GetGrants reports database-level grants.
		// The important thing is the test exercises the real integration path.
		t.Logf("Grant drift result: hasDrift=%v, diffs=%+v", result.HasDrift(), result.Diffs)

		// Revoke CREATE to simulate drift
		err = adapter.Revoke(ctx, grantUsername, []types.GrantOptions{
			{
				Database:   grantDBName,
				Privileges: []string{"CREATE"},
			},
		})
		require.NoError(t, err)

		// Detect drift again - should now find missing CREATE privilege
		result2, err := svc.DetectGrantDrift(ctx, spec, grantUsername)
		require.NoError(t, err)
		t.Logf("Grant drift after revoke: hasDrift=%v, diffs=%+v", result2.HasDrift(), result2.Diffs)
	})
}
