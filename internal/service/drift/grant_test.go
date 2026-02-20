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

func newTestGrantSpec() *dbopsv1alpha1.DatabaseGrantSpec {
	return &dbopsv1alpha1.DatabaseGrantSpec{
		Postgres: &dbopsv1alpha1.PostgresGrantConfig{},
	}
}

// --- Detection tests ---

func TestDetectGrantDrift_NoDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{
			{ObjectType: "ROLE", ObjectName: "admin"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Roles = []string{"admin"}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

func TestDetectGrantDrift_MissingRoleMembership(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil // No roles granted
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Roles = []string{"admin", "editor"}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var missingDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "roles.missing" {
			missingDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, missingDiff)
	assert.Equal(t, "admin, editor", missingDiff.Expected)
}

func TestDetectGrantDrift_ExtraRoleMembership_Destructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{
			{ObjectType: "ROLE", ObjectName: "admin"},
			{ObjectType: "ROLE", ObjectName: "legacy_role"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Roles = []string{"admin"}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var extraDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "roles.extra" {
			extraDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, extraDiff)
	assert.True(t, extraDiff.Destructive)
	assert.Equal(t, "legacy_role", extraDiff.Actual)
}

func TestDetectGrantDrift_MissingTableGrant(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil // No grants
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Tables:     []string{"users"},
			Privileges: []string{"SELECT", "INSERT"},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var tableDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "grants.table.users" {
			tableDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, tableDiff)
	assert.Equal(t, "SELECT, INSERT", tableDiff.Expected)
}

func TestDetectGrantDrift_ExtraPrivileges_Destructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{
			{
				Database:   "mydb",
				Schema:     "public",
				ObjectType: "TABLE",
				ObjectName: "users",
				Privileges: []string{"SELECT", "INSERT", "DELETE"},
			},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Tables:     []string{"users"},
			Privileges: []string{"SELECT", "INSERT"},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var extraDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "grants.table.users.extra" {
			extraDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, extraDiff)
	assert.True(t, extraDiff.Destructive)
	assert.Equal(t, "DELETE", extraDiff.Actual)
}

func TestDetectGrantDrift_GetGrantsError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return nil, fmt.Errorf("connection refused")
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get grants")
}

// --- Correction tests ---

func TestCorrectGrantDrift_MissingRoles(t *testing.T) {
	var grantedRoles []string
	adapter := testutil.NewMockAdapter()
	adapter.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		grantedRoles = roles
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Roles = []string{"admin"}

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "roles.missing",
		Expected: "admin",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, []string{"admin"}, grantedRoles)
}

func TestCorrectGrantDrift_MissingPrivileges(t *testing.T) {
	var capturedOpts []types.GrantOptions
	adapter := testutil.NewMockAdapter()
	adapter.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Tables:     []string{"users"},
			Privileges: []string{"SELECT"},
		},
	}

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "grants.table.users.missing",
		Expected: "SELECT",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, capturedOpts, 1)
	assert.Equal(t, "mydb", capturedOpts[0].Database)
	assert.Equal(t, "public", capturedOpts[0].Schema)
}

func TestCorrectGrantDrift_SkipsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false) // AllowDestructive = false
	spec := newTestGrantSpec()

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:       "roles.extra",
		Expected:    "",
		Actual:      "legacy_role",
		Destructive: true,
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Skipped, 1)
	assert.Contains(t, corrResult.Skipped[0].Reason, "destructive")
}

func TestCorrectGrantDrift_AllowsDestructive(t *testing.T) {
	var revokedRoles []string
	adapter := testutil.NewMockAdapter()
	adapter.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		revokedRoles = roles
		return nil
	}

	svc := newTestService(adapter, true) // AllowDestructive = true
	spec := newTestGrantSpec()

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:       "roles.extra",
		Expected:    "",
		Actual:      "legacy_role",
		Destructive: true,
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, []string{"legacy_role"}, revokedRoles)
}

func TestCorrectGrantDrift_AdapterError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		return fmt.Errorf("permission denied")
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "roles.missing",
		Expected: "admin",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Failed, 1)
}

func TestCorrectGrantDrift_RevokeExtraPrivileges(t *testing.T) {
	var revokedOpts []types.GrantOptions
	adapter := testutil.NewMockAdapter()
	adapter.RevokeFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
		revokedOpts = opts
		return nil
	}

	svc := newTestService(adapter, true) // AllowDestructive = true
	spec := newTestGrantSpec()

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:       "grants.table.users.extra",
		Expected:    "",
		Actual:      "DELETE, TRUNCATE",
		Destructive: true,
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, revokedOpts, 1)
	assert.Equal(t, []string{"DELETE", "TRUNCATE"}, revokedOpts[0].Privileges)
}

func TestDetectGrantDrift_SequenceGrant(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Sequences:  []string{"user_id_seq"},
			Privileges: []string{"USAGE"},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var seqDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "grants.sequence.user_id_seq" {
			seqDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, seqDiff)
	assert.Equal(t, "USAGE", seqDiff.Expected)
}

func TestDetectGrantDrift_FunctionGrant(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Functions:  []string{"my_func"},
			Privileges: []string{"EXECUTE"},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var fnDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "grants.function.my_func" {
			fnDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, fnDiff)
	assert.Equal(t, "EXECUTE", fnDiff.Expected)
}

func TestCorrectGrantDrift_MissingSequencePrivileges(t *testing.T) {
	var capturedOpts []types.GrantOptions
	adapter := testutil.NewMockAdapter()
	adapter.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Sequences:  []string{"user_id_seq"},
			Privileges: []string{"USAGE"},
		},
	}

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "grants.sequence.user_id_seq.missing",
		Expected: "USAGE",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, capturedOpts, 1)
	assert.Equal(t, []string{"user_id_seq"}, capturedOpts[0].Sequences)
}

func TestCorrectGrantDrift_MissingFunctionPrivileges(t *testing.T) {
	var capturedOpts []types.GrantOptions
	adapter := testutil.NewMockAdapter()
	adapter.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	spec.Postgres.Grants = []dbopsv1alpha1.PostgresGrant{
		{
			Database:   "mydb",
			Schema:     "public",
			Functions:  []string{"my_func"},
			Privileges: []string{"EXECUTE"},
		},
	}

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "grants.function.my_func.missing",
		Expected: "EXECUTE",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, capturedOpts, 1)
	assert.Equal(t, []string{"my_func"}, capturedOpts[0].Functions)
}

func TestDetectGrantDrift_MySQLMissingGrant(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseGrantSpec{
		MySQL: &dbopsv1alpha1.MySQLGrantConfig{
			Grants: []dbopsv1alpha1.MySQLGrant{
				{
					Database:   "mydb",
					Table:      "users",
					Privileges: []string{"SELECT", "INSERT"},
				},
			},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())
}

func TestDetectGrantDrift_MySQLMissingPrivileges(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{
			{
				Database:   "mydb",
				ObjectType: "TABLE",
				ObjectName: "users",
				Privileges: []string{"SELECT"},
			},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseGrantSpec{
		MySQL: &dbopsv1alpha1.MySQLGrantConfig{
			Grants: []dbopsv1alpha1.MySQLGrant{
				{
					Database:   "mydb",
					Table:      "users",
					Privileges: []string{"SELECT", "INSERT"},
				},
			},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())
}

func TestDetectGrantDrift_MySQLDefaultTable(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseGrantSpec{
		MySQL: &dbopsv1alpha1.MySQLGrantConfig{
			Grants: []dbopsv1alpha1.MySQLGrant{
				{
					Database:   "mydb",
					Table:      "", // empty → defaults to "*"
					Privileges: []string{"ALL"},
				},
			},
		},
	}

	result, err := svc.DetectGrantDrift(context.Background(), spec, "testuser")
	require.NoError(t, err)
	require.True(t, result.HasDrift())
}

func TestCorrectGrantDrift_MySQLMissingGrant(t *testing.T) {
	var capturedOpts []types.GrantOptions
	adapter := testutil.NewMockAdapter()
	adapter.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseGrantSpec{
		MySQL: &dbopsv1alpha1.MySQLGrantConfig{
			Grants: []dbopsv1alpha1.MySQLGrant{
				{
					Database:   "mydb",
					Table:      "users",
					Privileges: []string{"SELECT"},
				},
			},
		},
	}

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "grants.mydb.users.missing",
		Expected: "SELECT",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, capturedOpts, 1)
	assert.Equal(t, "mydb", capturedOpts[0].Database)
}

func TestCorrectGrantDrift_MissingGrantNoMatch(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()
	// No grants configured in spec

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "grants.table.orphan.missing",
		Expected: "SELECT",
		Actual:   "",
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "could not find matching grant")
}

func TestCorrectGrantDrift_UnknownField(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{Field: "unknownField", Expected: "a", Actual: "b"})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "unsupported")
}

func TestCorrectGrantDrift_MixedResults(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestGrantSpec()

	driftResult := NewResult("grant", "testuser")
	driftResult.AddDiff(Diff{
		Field:    "roles.missing",
		Expected: "admin",
		Actual:   "",
	})
	driftResult.AddDiff(Diff{
		Field:       "roles.extra",
		Expected:    "",
		Actual:      "legacy",
		Destructive: true,
	})

	corrResult, err := svc.CorrectGrantDrift(context.Background(), spec, "testuser", driftResult)
	require.NoError(t, err)
	assert.Len(t, corrResult.Corrected, 1) // roles.missing corrected
	assert.Len(t, corrResult.Skipped, 1)   // roles.extra skipped (destructive)
}
