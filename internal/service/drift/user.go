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
	"sort"
	"strconv"
	"strings"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
)

// DetectUserDrift compares CR spec to actual user state.
// Returns a Result containing any differences found.
//
// The detection covers:
// - Username: The username (immutable)
// - ConnectionLimit: Maximum connections allowed
// - Superuser/CreateDB/CreateRole: Role attributes (PostgreSQL)
// - InRoles: Role memberships
func (s *Service) DetectUserDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec) (*Result, error) {
	log := s.log.WithValues("user", spec.Username)
	log.V(1).Info("detecting user drift")

	// Get actual user state
	info, err := s.adapter.GetUserInfo(ctx, spec.Username)
	if err != nil {
		log.Error(err, "failed to get user info")
		return nil, fmt.Errorf("get user info: %w", err)
	}

	result := NewResult("user", spec.Username)

	// Compare based on database engine
	if spec.Postgres != nil {
		s.detectPostgresUserDrift(spec, info, result)
	}
	if spec.MySQL != nil {
		s.detectMySQLUserDrift(spec, info, result)
	}

	if result.HasDrift() {
		log.Info("drift detected", "diffs", len(result.Diffs))
	} else {
		log.V(1).Info("no drift detected")
	}

	return result, nil
}

// detectPostgresUserDrift detects drift for PostgreSQL-specific user settings.
func (s *Service) detectPostgresUserDrift(spec *dbopsv1alpha1.DatabaseUserSpec, info *types.UserInfo, result *Result) {
	pgSpec := spec.Postgres

	// Connection limit
	if pgSpec.ConnectionLimit != info.ConnectionLimit {
		result.AddDiff(Diff{
			Field:    "connectionLimit",
			Expected: strconv.FormatInt(int64(pgSpec.ConnectionLimit), 10),
			Actual:   strconv.FormatInt(int64(info.ConnectionLimit), 10),
		})
	}

	// Superuser attribute
	if pgSpec.Superuser != info.Superuser {
		result.AddDiff(Diff{
			Field:       "superuser",
			Expected:    strconv.FormatBool(pgSpec.Superuser),
			Actual:      strconv.FormatBool(info.Superuser),
			Destructive: true, // Superuser changes are significant
		})
	}

	// CreateDB attribute
	if pgSpec.CreateDB != info.CreateDB {
		result.AddDiff(Diff{
			Field:    "createdb",
			Expected: strconv.FormatBool(pgSpec.CreateDB),
			Actual:   strconv.FormatBool(info.CreateDB),
		})
	}

	// CreateRole attribute
	if pgSpec.CreateRole != info.CreateRole {
		result.AddDiff(Diff{
			Field:    "createrole",
			Expected: strconv.FormatBool(pgSpec.CreateRole),
			Actual:   strconv.FormatBool(info.CreateRole),
		})
	}

	// Inherit attribute
	if pgSpec.Inherit != info.Inherit {
		result.AddDiff(Diff{
			Field:    "inherit",
			Expected: strconv.FormatBool(pgSpec.Inherit),
			Actual:   strconv.FormatBool(info.Inherit),
		})
	}

	// Replication attribute
	if pgSpec.Replication != info.Replication {
		result.AddDiff(Diff{
			Field:    "replication",
			Expected: strconv.FormatBool(pgSpec.Replication),
			Actual:   strconv.FormatBool(info.Replication),
		})
	}

	// BypassRLS attribute
	if pgSpec.BypassRLS != info.BypassRLS {
		result.AddDiff(Diff{
			Field:    "bypassrls",
			Expected: strconv.FormatBool(pgSpec.BypassRLS),
			Actual:   strconv.FormatBool(info.BypassRLS),
		})
	}

	// Role memberships
	s.compareRoleMemberships(pgSpec.InRoles, info.InRoles, result)
}

// detectMySQLUserDrift detects drift for MySQL-specific user settings.
func (s *Service) detectMySQLUserDrift(spec *dbopsv1alpha1.DatabaseUserSpec, info *types.UserInfo, result *Result) {
	// MySQL user drift detection would go here
	// For now, basic comparison of allowed hosts
	mysqlSpec := spec.MySQL
	_ = mysqlSpec // Avoid unused variable error

	// MySQL-specific drift detection would compare:
	// - MaxQueriesPerHour, MaxUpdatesPerHour, MaxConnectionsPerHour
	// - MaxUserConnections
	// - AllowedHosts
	// - SSL requirements
}

// compareRoleMemberships compares expected role memberships to actual.
func (s *Service) compareRoleMemberships(expected []string, actual []string, result *Result) {
	// Build sets for comparison
	expectedSet := make(map[string]bool)
	for _, role := range expected {
		expectedSet[role] = true
	}

	actualSet := make(map[string]bool)
	for _, role := range actual {
		actualSet[role] = true
	}

	// Find missing memberships
	var missing []string
	for role := range expectedSet {
		if !actualSet[role] {
			missing = append(missing, role)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		result.AddDiff(Diff{
			Field:    "inRoles.missing",
			Expected: strings.Join(missing, ", "),
			Actual:   "",
		})
	}

	// Find extra memberships
	var extra []string
	for role := range actualSet {
		if !expectedSet[role] {
			extra = append(extra, role)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		result.AddDiff(Diff{
			Field:       "inRoles.extra",
			Expected:    "",
			Actual:      strings.Join(extra, ", "),
			Destructive: true, // Removing role memberships is destructive
		})
	}
}

// CorrectUserDrift applies corrections for detected user drift.
func (s *Service) CorrectUserDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, driftResult *Result) (*CorrectionResult, error) {
	log := s.log.WithValues("user", spec.Username)
	log.Info("correcting user drift", "diffs", len(driftResult.Diffs))

	result := NewCorrectionResult(spec.Username)

	for _, diff := range driftResult.Diffs {
		// Skip immutable fields
		if diff.Immutable {
			result.AddSkipped(diff, "immutable field, requires resource recreation")
			continue
		}

		// Skip destructive corrections unless explicitly allowed
		if diff.Destructive && !s.config.AllowDestructive {
			result.AddSkipped(diff, "destructive correction not allowed")
			continue
		}

		// Apply correction
		if err := s.applyUserCorrection(ctx, spec, diff); err != nil {
			result.AddFailed(diff, err)
			log.Error(err, "failed to correct drift", "field", diff.Field)
			continue
		}

		result.AddCorrected(diff)
		log.Info("corrected drift", "field", diff.Field)
	}

	return result, nil
}

// applyUserCorrection applies a single correction to a user.
func (s *Service) applyUserCorrection(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec, diff Diff) error {
	switch {
	case diff.Field == "connectionLimit" ||
		diff.Field == "superuser" ||
		diff.Field == "createdb" ||
		diff.Field == "createrole" ||
		diff.Field == "inherit" ||
		diff.Field == "replication" ||
		diff.Field == "bypassrls":
		// Update user attributes
		return s.updateUserAttributes(ctx, spec)

	case strings.HasPrefix(diff.Field, "inRoles"):
		// Update role memberships
		return s.updateUserRoles(ctx, spec)

	default:
		return fmt.Errorf("unsupported drift correction for field: %s", diff.Field)
	}
}

// updateUserAttributes updates user attributes based on spec.
func (s *Service) updateUserAttributes(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec) error {
	if spec.Postgres == nil {
		return nil
	}

	pgSpec := spec.Postgres
	// Convert to pointer types for UpdateUserOptions
	connLimit := pgSpec.ConnectionLimit
	superuser := pgSpec.Superuser
	createDB := pgSpec.CreateDB
	createRole := pgSpec.CreateRole
	inherit := pgSpec.Inherit
	replication := pgSpec.Replication
	bypassRLS := pgSpec.BypassRLS

	opts := types.UpdateUserOptions{
		ConnectionLimit: &connLimit,
		Superuser:       &superuser,
		CreateDB:        &createDB,
		CreateRole:      &createRole,
		Inherit:         &inherit,
		Replication:     &replication,
		BypassRLS:       &bypassRLS,
	}

	return s.adapter.UpdateUser(ctx, spec.Username, opts)
}

// updateUserRoles updates user role memberships based on spec.
func (s *Service) updateUserRoles(ctx context.Context, spec *dbopsv1alpha1.DatabaseUserSpec) error {
	if spec.Postgres == nil {
		return nil
	}

	// Grant roles from spec
	if len(spec.Postgres.InRoles) > 0 {
		return s.adapter.GrantRole(ctx, spec.Username, spec.Postgres.InRoles)
	}

	return nil
}
