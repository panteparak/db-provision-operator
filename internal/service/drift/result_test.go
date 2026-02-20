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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResult_HasDrift_Empty(t *testing.T) {
	result := NewResult("role", "testrole")
	assert.False(t, result.HasDrift())
}

func TestResult_HasDrift_WithDiffs(t *testing.T) {
	result := NewResult("role", "testrole")
	result.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})
	assert.True(t, result.HasDrift())
}

func TestResult_HasDestructiveDrift_None(t *testing.T) {
	result := NewResult("role", "testrole")
	result.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})
	assert.False(t, result.HasDestructiveDrift())
}

func TestResult_HasDestructiveDrift_Present(t *testing.T) {
	result := NewResult("role", "testrole")
	result.AddDiff(Diff{Field: "login", Expected: "true", Actual: "false"})
	result.AddDiff(Diff{Field: "superuser", Expected: "true", Actual: "false", Destructive: true})
	assert.True(t, result.HasDestructiveDrift())
}

func TestResult_HasImmutableDrift_None(t *testing.T) {
	result := NewResult("database", "testdb")
	result.AddDiff(Diff{Field: "owner", Expected: "newowner", Actual: "oldowner"})
	assert.False(t, result.HasImmutableDrift())
}

func TestResult_HasImmutableDrift_Present(t *testing.T) {
	result := NewResult("database", "testdb")
	result.AddDiff(Diff{Field: "encoding", Expected: "UTF8", Actual: "LATIN1", Immutable: true})
	assert.True(t, result.HasImmutableDrift())
}

func TestResult_AddDiff(t *testing.T) {
	result := NewResult("role", "testrole")
	assert.Empty(t, result.Diffs)

	diff1 := Diff{Field: "login", Expected: "true", Actual: "false"}
	diff2 := Diff{Field: "createdb", Expected: "true", Actual: "false"}
	result.AddDiff(diff1)
	result.AddDiff(diff2)

	assert.Len(t, result.Diffs, 2)
	assert.Equal(t, "login", result.Diffs[0].Field)
	assert.Equal(t, "createdb", result.Diffs[1].Field)
}

func TestResult_ToAPIStatus_NoDrift(t *testing.T) {
	result := NewResult("role", "testrole")
	status := result.ToAPIStatus()

	assert.False(t, status.Detected)
	require.NotNil(t, status.LastChecked)
	assert.Empty(t, status.Diffs)
}

func TestResult_ToAPIStatus_WithDrift(t *testing.T) {
	result := NewResult("role", "testrole")
	result.AddDiff(Diff{
		Field:       "superuser",
		Expected:    "true",
		Actual:      "false",
		Destructive: true,
	})
	result.AddDiff(Diff{
		Field:     "encoding",
		Expected:  "UTF8",
		Actual:    "LATIN1",
		Immutable: true,
	})

	status := result.ToAPIStatus()

	assert.True(t, status.Detected)
	require.NotNil(t, status.LastChecked)
	require.Len(t, status.Diffs, 2)

	assert.Equal(t, "superuser", status.Diffs[0].Field)
	assert.Equal(t, "true", status.Diffs[0].Expected)
	assert.Equal(t, "false", status.Diffs[0].Actual)
	assert.True(t, status.Diffs[0].Destructive)
	assert.False(t, status.Diffs[0].Immutable)

	assert.Equal(t, "encoding", status.Diffs[1].Field)
	assert.True(t, status.Diffs[1].Immutable)
}

func TestCorrectionResult_HasCorrections(t *testing.T) {
	cr := NewCorrectionResult("testrole")
	assert.False(t, cr.HasCorrections())

	cr.AddCorrected(Diff{Field: "login"})
	assert.True(t, cr.HasCorrections())
}

func TestCorrectionResult_HasFailures(t *testing.T) {
	cr := NewCorrectionResult("testrole")
	assert.False(t, cr.HasFailures())

	cr.AddFailed(Diff{Field: "login"}, fmt.Errorf("permission denied"))
	assert.True(t, cr.HasFailures())
}

func TestCorrectionResult_AddCorrected(t *testing.T) {
	cr := NewCorrectionResult("testrole")
	diff := Diff{Field: "login", Expected: "true", Actual: "false"}
	cr.AddCorrected(diff)

	require.Len(t, cr.Corrected, 1)
	assert.Equal(t, "login", cr.Corrected[0].Diff.Field)
}

func TestCorrectionResult_AddSkipped(t *testing.T) {
	cr := NewCorrectionResult("testrole")
	diff := Diff{Field: "superuser", Destructive: true}
	cr.AddSkipped(diff, "destructive correction not allowed")

	require.Len(t, cr.Skipped, 1)
	assert.Equal(t, "superuser", cr.Skipped[0].Diff.Field)
	assert.Equal(t, "destructive correction not allowed", cr.Skipped[0].Reason)
}

func TestCorrectionResult_AddFailed(t *testing.T) {
	cr := NewCorrectionResult("testrole")
	diff := Diff{Field: "login"}
	err := fmt.Errorf("connection lost")
	cr.AddFailed(diff, err)

	require.Len(t, cr.Failed, 1)
	assert.Equal(t, "login", cr.Failed[0].Diff.Field)
	assert.EqualError(t, cr.Failed[0].Error, "connection lost")
}

func TestNewResult_SetsCheckedAt(t *testing.T) {
	before := time.Now()
	result := NewResult("role", "testrole")
	after := time.Now()

	assert.Equal(t, "role", result.ResourceType)
	assert.Equal(t, "testrole", result.ResourceName)
	assert.False(t, result.CheckedAt.Before(before))
	assert.False(t, result.CheckedAt.After(after))
}
