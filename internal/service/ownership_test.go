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

package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service/testutil"
)

func newTestOwnershipService(adapter *testutil.MockAdapter) *OwnershipService {
	cfg := &Config{
		Engine: "postgres",
		Host:   "localhost",
		Port:   5432,
	}
	rs := NewResourceServiceWithAdapter(adapter, cfg, "OwnershipServiceTest")
	return NewOwnershipService(rs)
}

// --- DeriveRoleName ---

func TestDeriveRoleName_Default(t *testing.T) {
	name := DeriveRoleName(nil, "myappdb")
	assert.Equal(t, "db_myappdb_owner", name)
}

func TestDeriveRoleName_Custom(t *testing.T) {
	cfg := &dbopsv1alpha1.PostgresOwnershipConfig{RoleName: "custom_owner"}
	name := DeriveRoleName(cfg, "myappdb")
	assert.Equal(t, "custom_owner", name)
}

func TestDeriveRoleName_Empty(t *testing.T) {
	cfg := &dbopsv1alpha1.PostgresOwnershipConfig{}
	name := DeriveRoleName(cfg, "myappdb")
	assert.Equal(t, "db_myappdb_owner", name)
}

func TestDeriveRoleName_Truncation(t *testing.T) {
	// Database name at 63 characters generates db_<63>_owner which is > 63
	longName := strings.Repeat("a", 63)
	name := DeriveRoleName(nil, longName)
	assert.LessOrEqual(t, len(name), 63)
}

// --- DeriveUserName ---

func TestDeriveUserName_Default(t *testing.T) {
	name := DeriveUserName(nil, "myappdb")
	assert.Equal(t, "db_myappdb_app", name)
}

func TestDeriveUserName_Custom(t *testing.T) {
	cfg := &dbopsv1alpha1.PostgresOwnershipConfig{UserName: "custom_app"}
	name := DeriveUserName(cfg, "myappdb")
	assert.Equal(t, "custom_app", name)
}

func TestDeriveUserName_Truncation(t *testing.T) {
	longName := strings.Repeat("b", 63)
	name := DeriveUserName(nil, longName)
	assert.LessOrEqual(t, len(name), 63)
}

// --- EnsureOwnerRole ---

func TestEnsureOwnerRole_CreatesNewRole(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
		return false, nil
	}
	var createdRole string
	adapter.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
		createdRole = opts.RoleName
		assert.False(t, opts.Login, "owner role should not have login")
		assert.True(t, opts.Inherit, "owner role should inherit")
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.EnsureOwnerRole(context.Background(), "db_myapp_owner")
	require.NoError(t, err)
	assert.Equal(t, "db_myapp_owner", createdRole)
}

func TestEnsureOwnerRole_AlreadyExists(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
		return true, nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.EnsureOwnerRole(context.Background(), "db_myapp_owner")
	require.NoError(t, err)
	assert.Equal(t, 0, adapter.GetCallCount("CreateRole"))
}

func TestEnsureOwnerRole_CheckError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
		return false, fmt.Errorf("connection refused")
	}

	svc := newTestOwnershipService(adapter)
	err := svc.EnsureOwnerRole(context.Background(), "db_myapp_owner")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check role existence")
}

// --- EnsureOwnerUser ---

func TestEnsureOwnerUser_CreatesAndGrants(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
		return false, nil
	}
	var createdRole string
	adapter.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error {
		createdRole = opts.RoleName
		assert.True(t, opts.Login, "app user should have login")
		assert.True(t, opts.Inherit, "app user should inherit")
		return nil
	}
	var grantedTo string
	var grantedRoles []string
	adapter.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		grantedTo = grantee
		grantedRoles = roles
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.EnsureOwnerUser(context.Background(), "db_myapp_app", "db_myapp_owner")
	require.NoError(t, err)
	assert.Equal(t, "db_myapp_app", createdRole)
	assert.Equal(t, "db_myapp_app", grantedTo)
	assert.Equal(t, []string{"db_myapp_owner"}, grantedRoles)
}

func TestEnsureOwnerUser_AlreadyExistsStillGrants(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) {
		return true, nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.EnsureOwnerUser(context.Background(), "db_myapp_app", "db_myapp_owner")
	require.NoError(t, err)
	assert.Equal(t, 0, adapter.GetCallCount("CreateRole"))
	assert.Equal(t, 1, adapter.GetCallCount("GrantRole"))
}

// --- TransferOwnership ---

func TestTransferOwnership_Success(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	var transferredDB, transferredOwner string
	adapter.TransferDatabaseOwnershipFunc = func(ctx context.Context, dbName, newOwner string) error {
		transferredDB = dbName
		transferredOwner = newOwner
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.TransferOwnership(context.Background(), "myappdb", "db_myapp_owner")
	require.NoError(t, err)
	assert.Equal(t, "myappdb", transferredDB)
	assert.Equal(t, "db_myapp_owner", transferredOwner)
}

func TestTransferOwnership_Error(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.TransferDatabaseOwnershipFunc = func(ctx context.Context, dbName, newOwner string) error {
		return fmt.Errorf("permission denied")
	}

	svc := newTestOwnershipService(adapter)
	err := svc.TransferOwnership(context.Background(), "myappdb", "db_myapp_owner")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transfer ownership")
}

// --- SetDefaultPrivileges ---

func TestSetDefaultPrivileges_AllSchemas(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	callCount := 0
	adapter.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
		callCount++
		assert.Equal(t, "db_myapp_app", grantee)
		require.Len(t, opts, 1)
		assert.Equal(t, "db_myapp_owner", opts[0].GrantedBy)
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.SetDefaultPrivileges(context.Background(), "myappdb", "db_myapp_owner", "db_myapp_app",
		[]string{"app", "data"})
	require.NoError(t, err)
	// 3 schemas (public, app, data) × 3 object types (tables, sequences, functions) = 9
	assert.Equal(t, 9, callCount)
}

func TestSetDefaultPrivileges_OnlyPublic(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	callCount := 0
	adapter.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
		callCount++
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.SetDefaultPrivileges(context.Background(), "myappdb", "db_myapp_owner", "db_myapp_app", nil)
	require.NoError(t, err)
	// 1 schema (public) × 3 object types = 3
	assert.Equal(t, 3, callCount)
}

// --- DropOwnershipResources ---

func TestDropOwnershipResources_DropsUserThenRole(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	var dropOrder []string
	adapter.DropRoleFunc = func(ctx context.Context, roleName string) error {
		dropOrder = append(dropOrder, roleName)
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.DropOwnershipResources(context.Background(), "db_myapp_owner", "db_myapp_app")
	require.NoError(t, err)
	assert.Equal(t, []string{"db_myapp_app", "db_myapp_owner"}, dropOrder)
}

func TestDropOwnershipResources_UserDropFailContinues(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	var dropOrder []string
	adapter.DropRoleFunc = func(ctx context.Context, roleName string) error {
		dropOrder = append(dropOrder, roleName)
		if roleName == "db_myapp_app" {
			return fmt.Errorf("cannot drop user")
		}
		return nil
	}

	svc := newTestOwnershipService(adapter)
	err := svc.DropOwnershipResources(context.Background(), "db_myapp_owner", "db_myapp_app")
	require.NoError(t, err)
	// Both were attempted despite user failure
	assert.Equal(t, []string{"db_myapp_app", "db_myapp_owner"}, dropOrder)
}

// --- HasAutoOwnership ---

func TestHasAutoOwnership_True(t *testing.T) {
	spec := &dbopsv1alpha1.DatabaseSpec{
		Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
			Ownership: &dbopsv1alpha1.PostgresOwnershipConfig{
				AutoOwnership: true,
			},
		},
	}
	assert.True(t, HasAutoOwnership(spec))
}

func TestHasAutoOwnership_False(t *testing.T) {
	spec := &dbopsv1alpha1.DatabaseSpec{}
	assert.False(t, HasAutoOwnership(spec))

	spec = &dbopsv1alpha1.DatabaseSpec{
		Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{},
	}
	assert.False(t, HasAutoOwnership(spec))
}

// --- IsOwnershipSupported ---

func TestIsOwnershipSupported(t *testing.T) {
	assert.True(t, IsOwnershipSupported("postgres"))
	assert.True(t, IsOwnershipSupported("cockroachdb"))
	assert.False(t, IsOwnershipSupported("mysql"))
	assert.False(t, IsOwnershipSupported("mariadb"))
}

// --- ShouldSetDefaultPrivileges ---

func TestShouldSetDefaultPrivileges_DefaultsToTrue(t *testing.T) {
	cfg := &dbopsv1alpha1.PostgresOwnershipConfig{}
	assert.True(t, cfg.ShouldSetDefaultPrivileges())
}

func TestShouldSetDefaultPrivileges_ExplicitTrue(t *testing.T) {
	v := true
	cfg := &dbopsv1alpha1.PostgresOwnershipConfig{SetDefaultPrivileges: &v}
	assert.True(t, cfg.ShouldSetDefaultPrivileges())
}

func TestShouldSetDefaultPrivileges_ExplicitFalse(t *testing.T) {
	v := false
	cfg := &dbopsv1alpha1.PostgresOwnershipConfig{SetDefaultPrivileges: &v}
	assert.False(t, cfg.ShouldSetDefaultPrivileges())
}
