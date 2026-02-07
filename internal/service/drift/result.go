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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// Result contains the result of a drift detection check.
// It captures the differences between desired (CR spec) and actual (database) state.
type Result struct {
	// ResourceType is the type of resource checked (database, user, role, grant)
	ResourceType string `json:"resourceType"`

	// ResourceName is the name of the resource
	ResourceName string `json:"resourceName"`

	// CheckedAt is when the drift check was performed
	CheckedAt time.Time `json:"checkedAt"`

	// Diffs contains the specific differences found
	Diffs []Diff `json:"diffs,omitempty"`
}

// NewResult creates a new drift result for the given resource.
func NewResult(resourceType, resourceName string) *Result {
	return &Result{
		ResourceType: resourceType,
		ResourceName: resourceName,
		CheckedAt:    time.Now(),
	}
}

// HasDrift returns true if any drift was detected.
func (r *Result) HasDrift() bool {
	return len(r.Diffs) > 0
}

// HasDestructiveDrift returns true if any destructive drift was detected.
// Destructive drift requires explicit opt-in to correct (via annotation).
func (r *Result) HasDestructiveDrift() bool {
	for _, d := range r.Diffs {
		if d.Destructive {
			return true
		}
	}
	return false
}

// HasImmutableDrift returns true if any immutable field has drifted.
// Immutable drift cannot be corrected and requires resource recreation.
func (r *Result) HasImmutableDrift() bool {
	for _, d := range r.Diffs {
		if d.Immutable {
			return true
		}
	}
	return false
}

// AddDiff adds a difference to the result.
func (r *Result) AddDiff(diff Diff) {
	r.Diffs = append(r.Diffs, diff)
}

// ToAPIStatus converts the drift result to the API DriftStatus type.
func (r *Result) ToAPIStatus() *dbopsv1alpha1.DriftStatus {
	lastChecked := metav1.NewTime(r.CheckedAt)
	status := &dbopsv1alpha1.DriftStatus{
		Detected:    r.HasDrift(),
		LastChecked: &lastChecked,
	}

	for _, d := range r.Diffs {
		status.Diffs = append(status.Diffs, dbopsv1alpha1.DriftDiff{
			Field:       d.Field,
			Expected:    d.Expected,
			Actual:      d.Actual,
			Destructive: d.Destructive,
			Immutable:   d.Immutable,
		})
	}

	return status
}

// Diff represents a single difference between desired and actual state.
type Diff struct {
	// Field is the name of the field that differs
	Field string `json:"field"`

	// Expected is the expected value from the CR spec (as string for display)
	Expected string `json:"expected"`

	// Actual is the actual value in the database (as string for display)
	Actual string `json:"actual"`

	// Destructive indicates if correcting this drift would be destructive.
	// Destructive changes include: owner changes, dropping extensions, etc.
	Destructive bool `json:"destructive,omitempty"`

	// Immutable indicates if this field cannot be changed after creation.
	// Immutable fields include: encoding, database name, username, etc.
	Immutable bool `json:"immutable,omitempty"`
}

// CorrectionResult contains the result of drift correction.
type CorrectionResult struct {
	// ResourceName is the name of the resource
	ResourceName string

	// Corrected contains diffs that were successfully corrected
	Corrected []CorrectedDiff

	// Skipped contains diffs that were skipped (not corrected)
	Skipped []SkippedDiff

	// Failed contains diffs that failed to correct
	Failed []FailedDiff
}

// NewCorrectionResult creates a new correction result.
func NewCorrectionResult(resourceName string) *CorrectionResult {
	return &CorrectionResult{
		ResourceName: resourceName,
	}
}

// HasCorrections returns true if any corrections were made.
func (r *CorrectionResult) HasCorrections() bool {
	return len(r.Corrected) > 0
}

// HasFailures returns true if any corrections failed.
func (r *CorrectionResult) HasFailures() bool {
	return len(r.Failed) > 0
}

// AddCorrected adds a successfully corrected diff.
func (r *CorrectionResult) AddCorrected(diff Diff) {
	r.Corrected = append(r.Corrected, CorrectedDiff{Diff: diff})
}

// AddSkipped adds a skipped diff with reason.
func (r *CorrectionResult) AddSkipped(diff Diff, reason string) {
	r.Skipped = append(r.Skipped, SkippedDiff{Diff: diff, Reason: reason})
}

// AddFailed adds a failed correction with error.
func (r *CorrectionResult) AddFailed(diff Diff, err error) {
	r.Failed = append(r.Failed, FailedDiff{Diff: diff, Error: err})
}

// CorrectedDiff represents a diff that was successfully corrected.
type CorrectedDiff struct {
	Diff Diff
}

// SkippedDiff represents a diff that was skipped (not corrected).
type SkippedDiff struct {
	Diff   Diff
	Reason string
}

// FailedDiff represents a diff that failed to correct.
type FailedDiff struct {
	Diff  Diff
	Error error
}
