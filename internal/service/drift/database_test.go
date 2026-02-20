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

func newTestDatabaseSpec(name string) *dbopsv1alpha1.DatabaseSpec {
	return &dbopsv1alpha1.DatabaseSpec{
		Name: name,
		Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
			Encoding: "UTF8",
		},
	}
}

// --- Detection tests ---

func TestDetectDatabaseDrift_NoDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:     name,
			Encoding: "UTF8",
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

func TestDetectDatabaseDrift_EncodingDrift_Immutable(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:     name,
			Encoding: "LATIN1",
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Encoding = "UTF8"

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var encDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "encoding" {
			encDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, encDiff)
	assert.Equal(t, "UTF8", encDiff.Expected)
	assert.Equal(t, "LATIN1", encDiff.Actual)
	assert.True(t, encDiff.Immutable)
}

func TestDetectDatabaseDrift_MissingExtension(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:       name,
			Encoding:   "UTF8",
			Extensions: []types.ExtensionInfo{}, // no extensions installed
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Extensions = []dbopsv1alpha1.PostgresExtension{
		{Name: "pgcrypto"},
		{Name: "uuid-ossp"},
	}

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var missingDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "extensions.missing" {
			missingDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, missingDiff)
	// Should be sorted
	assert.Equal(t, "pgcrypto, uuid-ossp", missingDiff.Expected)
}

func TestDetectDatabaseDrift_ExtensionVersionMismatch(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:     name,
			Encoding: "UTF8",
			Extensions: []types.ExtensionInfo{
				{Name: "pgcrypto", Version: "1.2"},
			},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Extensions = []dbopsv1alpha1.PostgresExtension{
		{Name: "pgcrypto", Version: "1.3"},
	}

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var versionDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "extensions.pgcrypto.version" {
			versionDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, versionDiff)
	assert.Equal(t, "1.3", versionDiff.Expected)
	assert.Equal(t, "1.2", versionDiff.Actual)
}

func TestDetectDatabaseDrift_MissingSchema(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:     name,
			Encoding: "UTF8",
			Schemas:  []string{"public"},
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Schemas = []dbopsv1alpha1.PostgresSchema{
		{Name: "public"},
		{Name: "app"},
		{Name: "data"},
	}

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var schemaDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "schemas.missing" {
			schemaDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, schemaDiff)
	// Sorted
	assert.Equal(t, "app, data", schemaDiff.Expected)
}

func TestDetectDatabaseDrift_GetDatabaseInfoError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return nil, fmt.Errorf("connection refused")
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get database info")
}

func TestDetectDatabaseDrift_NilPostgresSpec(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:     name,
			Encoding: "UTF8",
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseSpec{
		Name: "testdb",
		// Postgres is nil
	}

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	assert.False(t, result.HasDrift())
}

// --- Correction tests ---

func TestCorrectDatabaseDrift_MissingExtension(t *testing.T) {
	var updatedDB string
	adapter := testutil.NewMockAdapter()
	adapter.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
		updatedDB = name
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Extensions = []dbopsv1alpha1.PostgresExtension{
		{Name: "pgcrypto"},
	}

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:    "extensions.missing",
		Expected: "pgcrypto",
		Actual:   "",
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, "testdb", updatedDB)
}

func TestCorrectDatabaseDrift_MissingSchema(t *testing.T) {
	var capturedOpts types.UpdateDatabaseOptions
	adapter := testutil.NewMockAdapter()
	adapter.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Schemas = []dbopsv1alpha1.PostgresSchema{
		{Name: "app", Owner: "appuser"},
	}

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:    "schemas.missing",
		Expected: "app",
		Actual:   "",
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, capturedOpts.Schemas, 1)
	assert.Equal(t, "app", capturedOpts.Schemas[0].Name)
	assert.Equal(t, "appuser", capturedOpts.Schemas[0].Owner)
}

func TestCorrectDatabaseDrift_SkipsImmutable(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:     "encoding",
		Expected:  "UTF8",
		Actual:    "LATIN1",
		Immutable: true,
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Skipped, 1)
	assert.Contains(t, corrResult.Skipped[0].Reason, "immutable")
}

func TestCorrectDatabaseDrift_SkipsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false) // AllowDestructive = false
	spec := newTestDatabaseSpec("testdb")

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:       "owner",
		Expected:    "newowner",
		Actual:      "oldowner",
		Destructive: true,
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Skipped, 1)
	assert.Contains(t, corrResult.Skipped[0].Reason, "destructive")
}

func TestCorrectDatabaseDrift_AllowsDestructive(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, true) // AllowDestructive = true
	spec := newTestDatabaseSpec("testdb")

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:       "owner",
		Expected:    "newowner",
		Actual:      "oldowner",
		Destructive: true,
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	// Owner correction is not yet implemented, so it should fail
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "not yet implemented")
}

func TestCorrectDatabaseDrift_ExtensionVersionMismatch(t *testing.T) {
	var capturedOpts types.UpdateDatabaseOptions
	adapter := testutil.NewMockAdapter()
	adapter.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:    "extensions.pgcrypto.version",
		Expected: "1.3",
		Actual:   "1.2",
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	require.Len(t, capturedOpts.Extensions, 1)
	assert.Equal(t, "pgcrypto", capturedOpts.Extensions[0].Name)
	assert.Equal(t, "1.3", capturedOpts.Extensions[0].Version)
}

func TestCorrectDatabaseDrift_MySQLCharset(t *testing.T) {
	var capturedOpts types.UpdateDatabaseOptions
	adapter := testutil.NewMockAdapter()
	adapter.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
		capturedOpts = opts
		return nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseSpec{
		Name: "testdb",
		MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{
			Charset:   "utf8mb4",
			Collation: "utf8mb4_general_ci",
		},
	}

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:    "charset",
		Expected: "utf8mb4",
		Actual:   "latin1",
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Corrected, 1)
	assert.Equal(t, "utf8mb4", capturedOpts.Charset)
	assert.Equal(t, "utf8mb4_general_ci", capturedOpts.Collation)
}

func TestCorrectDatabaseDrift_UnknownField(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{Field: "unknownField", Expected: "a", Actual: "b"})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "unsupported")
}

func TestDetectDatabaseDrift_MySQLCharsetDrift(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{
			Name:    name,
			Charset: "latin1",
		}, nil
	}

	svc := newTestService(adapter, false)
	spec := &dbopsv1alpha1.DatabaseSpec{
		Name: "testdb",
		MySQL: &dbopsv1alpha1.MySQLDatabaseConfig{
			Charset: "utf8mb4",
		},
	}

	result, err := svc.DetectDatabaseDrift(context.Background(), spec)
	require.NoError(t, err)
	require.True(t, result.HasDrift())

	var charsetDiff *Diff
	for i := range result.Diffs {
		if result.Diffs[i].Field == "charset" {
			charsetDiff = &result.Diffs[i]
			break
		}
	}
	require.NotNil(t, charsetDiff)
	assert.Equal(t, "utf8mb4", charsetDiff.Expected)
	assert.Equal(t, "latin1", charsetDiff.Actual)
}

func TestCorrectDatabaseDrift_AdapterError(t *testing.T) {
	adapter := testutil.NewMockAdapter()
	adapter.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
		return fmt.Errorf("disk full")
	}

	svc := newTestService(adapter, false)
	spec := newTestDatabaseSpec("testdb")
	spec.Postgres.Extensions = []dbopsv1alpha1.PostgresExtension{
		{Name: "pgcrypto"},
	}

	driftResult := NewResult("database", "testdb")
	driftResult.AddDiff(Diff{
		Field:    "extensions.missing",
		Expected: "pgcrypto",
		Actual:   "",
	})

	corrResult, err := svc.CorrectDatabaseDrift(context.Background(), spec, driftResult)
	require.NoError(t, err)
	assert.Empty(t, corrResult.Corrected)
	require.Len(t, corrResult.Failed, 1)
	assert.Contains(t, corrResult.Failed[0].Error.Error(), "disk full")
}
