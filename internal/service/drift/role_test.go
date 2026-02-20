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

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service/testutil"
)

func newTestService(adapter *testutil.MockAdapter, allowDestructive bool) *Service {
	return NewService(adapter, &Config{
		AllowDestructive: allowDestructive,
		Logger:           logr.Discard(),
	})
}

func newTestRoleSpec(roleName string) *dbopsv1alpha1.DatabaseRoleSpec {
	return &dbopsv1alpha1.DatabaseRoleSpec{
		RoleName: roleName,
		Postgres: &dbopsv1alpha1.PostgresRoleConfig{},
	}
}

// --- Detection tests ---

func TestDetectRoleDrift_NoDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:    roleName,
			Login:   true,
			Inherit: true,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true
	spec.Postgres.Inherit = true

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
	assert.Empty(t, result.Diffs)
}

func TestDetectRoleDrift_LoginDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{Name: roleName, Login: false}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())
	require.Len(t, result.Diffs, 1)
	assert.Equal(t, "login", result.Diffs[0].Field)
	assert.Equal(t, "true", result.Diffs[0].Expected)
	assert.Equal(t, "false", result.Diffs[0].Actual)
	assert.False(t, result.Diffs[0].Destructive)
}

func TestDetectRoleDrift_CreateDBDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{Name: roleName, CreateDB: false}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.CreateDB = true

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var createdbDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "createdb" {
			createdbDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, createdbDiff)
	assert.Equal(t, "true", createdbDiff.Expected)
	assert.Equal(t, "false", createdbDiff.Actual)
}

func TestDetectRoleDrift_SuperuserDrift_Destructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{Name: roleName, Superuser: false}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Superuser = true

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

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

func TestDetectRoleDrift_MultipleDiffs(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:    roleName,
			Login:   false,
			Inherit: false,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true
	spec.Postgres.Inherit = true

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.True(t, result.HasDrift())
	assert.Len(t, result.Diffs, 2)
}

func TestDetectRoleDrift_InRoles_Missing(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:    roleName,
			InRoles: []string{"admin"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.InRoles = []string{"admin", "editor", "viewer"}

	result, err := svc.DetectRoleDrift(context.Background(), spec)
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
	// Missing roles should be sorted
	assert.Equal(t, "editor, viewer", missingDiff.Expected)
	assert.Equal(t, "", missingDiff.Actual)
	assert.False(t, missingDiff.Destructive)
}

func TestDetectRoleDrift_InRoles_Extra(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:    roleName,
			InRoles: []string{"admin", "legacy_role"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.InRoles = []string{"admin"}

	result, err := svc.DetectRoleDrift(context.Background(), spec)
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
	assert.Equal(t, "legacy_role", extraDiff.Actual)
	assert.True(t, extraDiff.Destructive)
}

func TestDetectRoleDrift_InRoles_Match(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:    roleName,
			InRoles: []string{"admin", "editor"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.InRoles = []string{"admin", "editor"}

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

func TestDetectRoleDrift_AllAttributes(t *testing.T) {
	// Exercise all 7 attribute comparisons to maximize coverage
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:        roleName,
			Login:       false,
			Inherit:     false,
			CreateDB:    false,
			CreateRole:  false,
			Superuser:   false,
			Replication: false,
			BypassRLS:   false,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true
	spec.Postgres.Inherit = true
	spec.Postgres.CreateDB = true
	spec.Postgres.CreateRole = true
	spec.Postgres.Superuser = true
	spec.Postgres.Replication = true
	spec.Postgres.BypassRLS = true

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.True(t, result.HasDrift())
	assert.Len(t, result.Diffs, 7)

	fields := make(map[string]bool)
	for _, d := range result.Diffs {
		fields[d.Field] = true
	}
	assert.True(t, fields["login"])
	assert.True(t, fields["inherit"])
	assert.True(t, fields["createdb"])
	assert.True(t, fields["createrole"])
	assert.True(t, fields["superuser"])
	assert.True(t, fields["replication"])
	assert.True(t, fields["bypassrls"])
}

func TestDetectRoleDrift_MySQLSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{Name: roleName}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseRoleSpec{
		RoleName: "testrole",
		MySQL:    &dbopsv1alpha1.MySQLRoleConfig{},
	}

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	// MySQL drift detection is stubbed out, so no diffs
	assert.False(t, result.HasDrift())
}

func TestDetectRoleDrift_GetRoleInfoError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return nil, fmt.Errorf("connection refused")
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get role info")
}

func TestDetectRoleDrift_NilPostgresSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{
			Name:  roleName,
			Login: true,
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseRoleSpec{
		RoleName: "testrole",
		// Postgres is nil
	}

	result, err := svc.DetectRoleDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

// --- Correction tests ---

func TestCorrectRoleDrift_SingleAttribute_OnlyTargeted(t *testing.T) {
	// This is the CRITICAL test validating the bug fix:
	// When a single attribute drifts, only that attribute's pointer should be set
	// in UpdateRoleOptions, all others should be nil.
	attributes := []struct {
		field    string
		setBool  func(*dbopsv1alpha1.PostgresRoleConfig)
		checkSet func(types.UpdateRoleOptions) (*bool, string)
		checkNil []func(types.UpdateRoleOptions) (*bool, string)
	}{
		{
			field:    "login",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.Login = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			},
		},
		{
			field:    "inherit",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.Inherit = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			},
		},
		{
			field:    "createdb",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.CreateDB = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			},
		},
		{
			field:    "createrole",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.CreateRole = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			},
		},
		{
			field:    "superuser",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.Superuser = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			},
		},
		{
			field:    "replication",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.Replication = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			},
		},
		{
			field:    "bypassrls",
			setBool:  func(pg *dbopsv1alpha1.PostgresRoleConfig) { pg.BypassRLS = true },
			checkSet: func(o types.UpdateRoleOptions) (*bool, string) { return o.BypassRLS, "BypassRLS" },
			checkNil: []func(types.UpdateRoleOptions) (*bool, string){
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Login, "Login" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Inherit, "Inherit" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateDB, "CreateDB" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.CreateRole, "CreateRole" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Superuser, "Superuser" },
				func(o types.UpdateRoleOptions) (*bool, string) { return o.Replication, "Replication" },
			},
		},
	}

	for _, attr := range attributes {
		t.Run(attr.field, func(t *testing.T) {
			var capturedOpts types.UpdateRoleOptions
			adapter := testutil.NewMockAdapter()
			adapter.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
				capturedOpts = opts
				return nil
			}

			svc := newTestService(adapter, false)
			spec := newTestRoleSpec("testrole")
			attr.setBool(spec.Postgres)

			driftResult := NewResult("role", "testrole")
			driftResult.AddDiff(Diff{
				Field:    attr.field,
				Expected: "true",
				Actual:   "false",
			})

			corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
			require.NoError(t, err)
			require.Len(t, corrResult.Corrected, 1)

			// The targeted attribute must be set
			ptr, name := attr.checkSet(capturedOpts)
			require.NotNilf(t, ptr, "%s should be set", name)
			assert.True(t, *ptr)

			// All other attributes must be nil
			for _, checkFn := range attr.checkNil {
				ptr, name := checkFn(capturedOpts)
				assert.Nilf(t, ptr, "%s should be nil when correcting %s", name, attr.field)
			}
		})
	}
}

func TestCorrectRoleDrift_SkipsImmutable(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{
		Field:     "encoding",
		Expected:  "UTF8",
		Actual:    "LATIN1",
		Immutable: true,
	})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Skipped, 1)
	assert.Contains(t, corrResult.Skipped[0].Reason, "immutable")
}

func TestCorrectRoleDrift_SkipsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false) // AllowDestructive = false
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Superuser = true

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Skipped, 1)
	assert.Contains(t, corrResult.Skipped[0].Reason, "destructive")
}

func TestCorrectRoleDrift_AllowsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, true) // AllowDestructive = true
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Superuser = true

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, "superuser", corrResult.Corrected[0].Diff.Field)
}

func TestCorrectRoleDrift_InRoles(t *testing.T) {
	var grantedRoles []string
	adapter := testutil.NewMockAdapter()
	adapter.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error {
		grantedRoles = roles
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.InRoles = []string{"admin", "editor"}

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{
		Field:    "inRoles.missing",
		Expected: "admin, editor",
		Actual:   "",
	})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, []string{"admin", "editor"}, grantedRoles)
}

func TestCorrectRoleDrift_AdapterError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
		return fmt.Errorf("permission denied")
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err) // CorrectRoleDrift itself doesn't return error
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "permission denied")
}

func TestCorrectRoleDrift_UnknownField(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{Field: "unknownField", Expected: "a", Actual: "b"})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "unsupported")
}

func TestCorrectRoleDrift_NilPostgresSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseRoleSpec{
		RoleName: "testrole",
		// Postgres is nil
	}

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	// With nil Postgres spec, updateRoleAttribute returns nil (no-op)
	require.Len(t, corrResult.Corrected, 1)
}

func TestCorrectRoleDrift_MultipleDiffs(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	callCount := 0
	adapter.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
		callCount++
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true
	spec.Postgres.Inherit = true
	spec.Postgres.CreateDB = true

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})
	driftResult.AddDiff(Diff{Field: "inherit", Expected: "true", Actual: "false"})
	driftResult.AddDiff(Diff{Field: "createdb", Expected: "true", Actual: "false"})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Len(t, corrResult.Corrected, 3)
	assert.Equal(t, 3, callCount)
}

func TestCorrectRoleDrift_InRoles_EmptySpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	// InRoles is empty

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{
		Field:    "inRoles.missing",
		Expected: "admin",
		Actual:   "",
	})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	// With empty InRoles, GrantRole is not called, correction succeeds as no-op
	require.Len(t, corrResult.Corrected, 1)
}

func TestCorrectRoleDrift_MixedResults(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
		// Fail only for createdb
		if opts.CreateDB != nil {
			return fmt.Errorf("createdb failed")
		}
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestRoleSpec("testrole")
	spec.Postgres.Login = true
	spec.Postgres.CreateDB = true
	spec.Postgres.Superuser = true

	driftResult := NewResult("role", "testrole")
	driftResult.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})
	driftResult.AddDiff(Diff{Field: "createdb", Expected: "true", Actual: "false"})
	driftResult.AddDiff(Diff{Field: "superuser", Expected: "true", Actual: "false", Destructive: true})

	corrResult, err := svc.CorrectRoleDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Len(t, corrResult.Corrected, 1) // login
	assert.Len(t, corrResult.Failed, 1)    // createdb
	assert.Len(t, corrResult.Skipped, 1)   // superuser (destructive, not allowed)
}
