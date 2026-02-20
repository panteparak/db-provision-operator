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

package drift

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service/testutil"
)

func newTestUserSpec(username string) *dbopsv1alpha1.DatabaseUserSpec {
	return &dbopsv1alpha1.DatabaseUserSpec{
		Username: username,
		Postgres: &dbopsv1alpha1.PostgresUserConfig{},
	}
}

// --- Detection tests ---

func TestDetectUserDrift_NoDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{
			Username:        username,
			ConnectionLimit: 10,
			Inherit:         true,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.ConnectionLimit = 10
	spec.Postgres.Inherit = true

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

func TestDetectUserDrift_ConnectionLimit(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{
			Username:        username,
			ConnectionLimit: 5,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.ConnectionLimit = 20

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var connDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "connectionLimit" {
			connDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, connDiff)
	assert.Equal(t, "20", connDiff.Expected)
	assert.Equal(t, "5", connDiff.Actual)
}

func TestDetectUserDrift_SuperuserDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{Username: username, Superuser: false}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.Superuser = true

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())
	assert.True(t, result.HasDestructiveDrift())

	var suDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "superuser" {
			suDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, suDiff)
	assert.True(t, suDiff.Destructive)
}

func TestDetectUserDrift_MultipleAttributes(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{
			Username:        username,
			ConnectionLimit: 5,
			CreateDB:        false,
			Replication:     false,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.ConnectionLimit = 100
	spec.Postgres.CreateDB = true
	spec.Postgres.Replication = true

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.True(t, result.HasDrift())
	assert.Len(t, result.Diffs, 3)
}

func TestDetectUserDrift_InRoles_Missing(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{
			Username: username,
			InRoles:  []string{"admin"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.InRoles = []string{"admin", "editor"}

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var missingDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "inRoles.missing" {
			missingDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, missingDiff)
	assert.Equal(t, "editor", missingDiff.Expected)
}

func TestDetectUserDrift_InRoles_Extra(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{
			Username: username,
			InRoles:  []string{"admin", "legacy"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.InRoles = []string{"admin"}

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var extraDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "inRoles.extra" {
			extraDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, extraDiff)
	assert.True(t, extraDiff.Destructive)
}

func TestDetectUserDrift_AllAttributes(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{
			Username:        username,
			ConnectionLimit: 0,
			Superuser:       false,
			CreateDB:        false,
			CreateRole:      false,
			Inherit:         false,
			Replication:     false,
			BypassRLS:       false,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.ConnectionLimit = 100
	spec.Postgres.Superuser = true
	spec.Postgres.CreateDB = true
	spec.Postgres.CreateRole = true
	spec.Postgres.Inherit = true
	spec.Postgres.Replication = true
	spec.Postgres.BypassRLS = true

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.True(t, result.HasDrift())
	assert.Len(t, result.Diffs, 7)
}

func TestDetectUserDrift_MySQLSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{Username: username}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseUserSpec{
		Username: "testuser",
		MySQL:    &dbopsv1alpha1.MySQLUserConfig{},
	}

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

func TestDetectUserDrift_GetUserInfoError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return nil, fmt.Errorf("connection refused")
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get user info")
}

func TestDetectUserDrift_NilPostgresSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{Username: username, Login: true}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseUserSpec{
		Username: "testuser",
		// Postgres is nil
	}

	result, err := svc.DetectUserDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

// --- Correction tests ---

func TestCorrectUserDrift_SkipsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false) // AllowDestructive = false
	spec := newTestUserSpec("testuser")
	spec.Postgres.Superuser = true

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Skipped, 1)
	assert.Contains(t, corrResult.Skipped[0].Reason, "destructive")
}

func TestCorrectUserDrift_AllowsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, true) // AllowDestructive = true
	spec := newTestUserSpec("testuser")
	spec.Postgres.Superuser = true

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
}

func TestCorrectUserDrift_InRoles(t *testing.T) {
	var grantedRoles []string
	adapter := testutil.NewMockAdapter()
	adapter.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		grantedRoles = roles
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.InRoles = []string{"admin", "editor"}

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "inRoles.missing",
		Expected: "admin, editor",
		Actual:   "",
	})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, []string{"admin", "editor"}, grantedRoles)
}

func TestCorrectUserDrift_UnknownField(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{Field: "unknownField", Expected: "a", Actual: "b"})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "unsupported")
}

func TestCorrectUserDrift_InRoles_EmptySpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	// InRoles is empty

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "inRoles.missing",
		Expected: "admin",
		Actual:   "",
	})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
}

func TestCorrectUserDrift_NilPostgresSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseUserSpec{
		Username: "testuser",
		// Postgres is nil
	}

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{Field: "connectionLimit", Expected: "100", Actual: "5"})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	// With nil Postgres spec, updateUserAttributes returns nil (no-op)
	require.Len(t, corrResult.Corrected, 1)
}

func TestCorrectUserDrift_AdapterError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error {
		return fmt.Errorf("connection lost")
	}

	svc := newTestService(adapter, false)
	spec := newTestUserSpec("testuser")
	spec.Postgres.ConnectionLimit = 100

	driftResult := NewResult("user", "testuser")
	driftResult.AddDiff(Diff{Field: "connectionLimit", Expected: "100", Actual: "5"})

	corrResult, err := svc.CorrectUserDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "connection lost")
}
