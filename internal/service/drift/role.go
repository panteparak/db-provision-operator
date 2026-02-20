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

// DetectRoleDrift compares CR spec to actual role state.
// Returns a Result containing any differences found.
//
// The detection covers:
// - RoleName: The role name (immutable)
// - Role attributes: Login, Inherit, CreateDB, CreateRole, etc.
// - InRoles: Role memberships (roles this role is a member of)
func (s *Service) DetectRoleDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec) (*Result, error) {
	log := s.log.WithValues("role", spec.RoleName)
	log.V(1).Info("detecting role drift")

	// Get actual role state
	info, err := s.adapter.GetRoleInfo(ctx, spec.RoleName)
	if err != nil {
		log.Error(err, "failed to get role info")
		return nil, fmt.Errorf("get role info: %w", err)
	}

	result := NewResult("role", spec.RoleName)

	// Compare based on database engine
	if spec.Postgres != nil {
		s.detectPostgresRoleDrift(spec, info, result)
	}
	if spec.MySQL != nil {
		s.detectMySQLRoleDrift(spec, info, result)
	}

	if result.HasDrift() {
		log.Info("drift detected", "diffs", len(result.Diffs))
	} else {
		log.V(1).Info("no drift detected")
	}

	return result, nil
}

// detectPostgresRoleDrift detects drift for PostgreSQL-specific role settings.
func (s *Service) detectPostgresRoleDrift(spec *dbopsv1alpha1.DatabaseRoleSpec, info *types.RoleInfo, result *Result) {
	pgSpec := spec.Postgres

	// Login attribute
	if pgSpec.Login != info.Login {
		result.AddDiff(Diff{
			Field:    "login",
			Expected: strconv.FormatBool(pgSpec.Login),
			Actual:   strconv.FormatBool(info.Login),
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

	// Superuser attribute
	if pgSpec.Superuser != info.Superuser {
		result.AddDiff(Diff{
			Field:       "superuser",
			Expected:    strconv.FormatBool(pgSpec.Superuser),
			Actual:      strconv.FormatBool(info.Superuser),
			Destructive: true, // Superuser changes are significant
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

	// Role memberships (InRoles)
	s.compareRoleRoleMemberships(pgSpec.InRoles, info.InRoles, result)
}

// detectMySQLRoleDrift detects drift for MySQL-specific role settings.
func (s *Service) detectMySQLRoleDrift(spec *dbopsv1alpha1.DatabaseRoleSpec, info *types.RoleInfo, result *Result) {
	// MySQL roles are handled differently
	// In MySQL 8.0+, roles are essentially users without login capability
	// Role drift detection for MySQL would be similar to user drift
	mysqlSpec := spec.MySQL
	_ = mysqlSpec // Avoid unused variable error
}

// compareRoleRoleMemberships compares expected role memberships to actual for a role.
func (s *Service) compareRoleRoleMemberships(expected []string, actual []string, result *Result) {
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

// CorrectRoleDrift applies corrections for detected role drift.
func (s *Service) CorrectRoleDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, driftResult *Result) (*CorrectionResult, error) {
	log := s.log.WithValues("role", spec.RoleName)
	log.Info("correcting role drift", "diffs", len(driftResult.Diffs))

	result := NewCorrectionResult(spec.RoleName)

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
		if err := s.applyRoleCorrection(ctx, spec, diff); err != nil {
			result.AddFailed(diff, err)
			log.Error(err, "failed to correct drift", "field", diff.Field)
			continue
		}

		result.AddCorrected(diff)
		log.Info("corrected drift", "field", diff.Field)
	}

	return result, nil
}

// applyRoleCorrection applies a single correction to a role.
func (s *Service) applyRoleCorrection(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, diff Diff) error {
	switch {
	case diff.Field == "login" ||
		diff.Field == "inherit" ||
		diff.Field == "createdb" ||
		diff.Field == "createrole" ||
		diff.Field == "superuser" ||
		diff.Field == "replication" ||
		diff.Field == "bypassrls":
		// Update only the specific drifted attribute
		return s.updateRoleAttribute(ctx, spec, diff.Field)

	case strings.HasPrefix(diff.Field, "inRoles"):
		// Update role memberships
		return s.updateRoleRoles(ctx, spec)

	default:
		return fmt.Errorf("unsupported drift correction for field: %s", diff.Field)
	}
}

// updateRoleAttribute updates a single role attribute based on the drifted field.
// Only the specific drifted attribute is included in the ALTER ROLE statement
// to avoid requiring privileges for attributes that haven't changed (e.g. SUPERUSER).
func (s *Service) updateRoleAttribute(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec, field string) error {
	if spec.Postgres == nil {
		return nil
	}

	pgSpec := spec.Postgres
	opts := types.UpdateRoleOptions{}

	switch field {
	case "login":
		v := pgSpec.Login
		opts.Login = &v
	case "inherit":
		v := pgSpec.Inherit
		opts.Inherit = &v
	case "createdb":
		v := pgSpec.CreateDB
		opts.CreateDB = &v
	case "createrole":
		v := pgSpec.CreateRole
		opts.CreateRole = &v
	case "superuser":
		v := pgSpec.Superuser
		opts.Superuser = &v
	case "replication":
		v := pgSpec.Replication
		opts.Replication = &v
	case "bypassrls":
		v := pgSpec.BypassRLS
		opts.BypassRLS = &v
	default:
		return fmt.Errorf("unknown role attribute: %s", field)
	}

	return s.adapter.UpdateRole(ctx, spec.RoleName, opts)
}

// updateRoleRoles updates role memberships based on spec.
func (s *Service) updateRoleRoles(ctx context.Context, spec *dbopsv1alpha1.DatabaseRoleSpec) error {
	if spec.Postgres == nil {
		return nil
	}

	// Grant roles from spec
	if len(spec.Postgres.InRoles) > 0 {
		return s.adapter.GrantRole(ctx, spec.RoleName, spec.Postgres.InRoles)
	}

	return nil
}
