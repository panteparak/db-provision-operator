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
	"strings"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter/types"
)

// DetectGrantDrift compares CR spec to actual grant state.
// Returns a Result containing any differences found.
//
// The detection covers:
// - Role memberships (Roles assigned to the user)
// - Direct privilege grants (database/schema/table/sequence/function level)
// - Default privileges (not currently supported in detection)
func (s *Service) DetectGrantDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, grantee string) (*Result, error) {
	log := s.log.WithValues("grantee", grantee)
	log.V(1).Info("detecting grant drift")

	// Get actual grants from database
	actualGrants, err := s.adapter.GetGrants(ctx, grantee)
	if err != nil {
		log.Error(err, "failed to get grants")
		return nil, fmt.Errorf("get grants: %w", err)
	}

	result := NewResult("grant", grantee)

	// Compare based on database engine
	if spec.Postgres != nil {
		s.detectPostgresGrantDrift(spec.Postgres, actualGrants, result)
	}
	if spec.MySQL != nil {
		s.detectMySQLGrantDrift(spec.MySQL, actualGrants, result)
	}

	if result.HasDrift() {
		log.Info("drift detected", "diffs", len(result.Diffs))
	} else {
		log.V(1).Info("no drift detected")
	}

	return result, nil
}

// detectPostgresGrantDrift detects drift for PostgreSQL-specific grant settings.
func (s *Service) detectPostgresGrantDrift(spec *dbopsv1alpha1.PostgresGrantConfig, actualGrants []types.GrantInfo, result *Result) {
	// Detect role membership drift
	s.detectRoleMembershipDrift(spec.Roles, actualGrants, result)

	// Detect direct grant drift for tables, sequences, and functions
	for _, grant := range spec.Grants {
		s.detectPostgresDirectGrantDrift(grant, actualGrants, result)
	}
}

// detectMySQLGrantDrift detects drift for MySQL-specific grant settings.
func (s *Service) detectMySQLGrantDrift(spec *dbopsv1alpha1.MySQLGrantConfig, actualGrants []types.GrantInfo, result *Result) {
	// Detect direct grant drift
	for _, grant := range spec.Grants {
		s.detectMySQLDirectGrantDrift(grant, actualGrants, result)
	}
}

// detectRoleMembershipDrift compares expected role memberships to actual.
func (s *Service) detectRoleMembershipDrift(expectedRoles []string, actualGrants []types.GrantInfo, result *Result) {
	// Build set of actual role memberships
	actualRoles := make(map[string]bool)
	for _, grant := range actualGrants {
		if strings.EqualFold(grant.ObjectType, "ROLE") {
			actualRoles[grant.ObjectName] = true
		}
	}

	// Find missing role memberships
	var missing []string
	for _, role := range expectedRoles {
		if !actualRoles[role] {
			missing = append(missing, role)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		result.AddDiff(Diff{
			Field:    "roles.missing",
			Expected: strings.Join(missing, ", "),
			Actual:   "",
		})
	}

	// Find extra role memberships
	expectedSet := make(map[string]bool)
	for _, role := range expectedRoles {
		expectedSet[role] = true
	}

	var extra []string
	for role := range actualRoles {
		if !expectedSet[role] {
			extra = append(extra, role)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		result.AddDiff(Diff{
			Field:       "roles.extra",
			Expected:    "",
			Actual:      strings.Join(extra, ", "),
			Destructive: true, // Removing role memberships is destructive
		})
	}
}

// detectPostgresDirectGrantDrift compares expected grants to actual for PostgreSQL.
func (s *Service) detectPostgresDirectGrantDrift(grant dbopsv1alpha1.PostgresGrant, actualGrants []types.GrantInfo, result *Result) {
	// Build a map of actual grants for quick lookup
	// Key format: "database.schema.objectType.objectName"
	actualGrantMap := make(map[string][]string)
	for _, g := range actualGrants {
		if strings.EqualFold(g.ObjectType, "ROLE") {
			continue // Skip role memberships
		}
		key := buildGrantKey(g.Database, g.Schema, g.ObjectType, g.ObjectName)
		actualGrantMap[key] = g.Privileges
	}

	// Check table grants
	for _, table := range grant.Tables {
		s.checkObjectGrant(grant.Database, grant.Schema, "TABLE", table, grant.Privileges, actualGrantMap, result)
	}

	// Check sequence grants
	for _, seq := range grant.Sequences {
		s.checkObjectGrant(grant.Database, grant.Schema, "SEQUENCE", seq, grant.Privileges, actualGrantMap, result)
	}

	// Check function grants
	for _, fn := range grant.Functions {
		s.checkObjectGrant(grant.Database, grant.Schema, "FUNCTION", fn, grant.Privileges, actualGrantMap, result)
	}
}

// checkObjectGrant checks if an object has the expected privileges.
func (s *Service) checkObjectGrant(database, schema, objectType, objectName string, expectedPrivs []string, actualGrantMap map[string][]string, result *Result) {
	key := buildGrantKey(database, schema, objectType, objectName)
	actualPrivileges, exists := actualGrantMap[key]

	if !exists {
		// Grant doesn't exist at all
		result.AddDiff(Diff{
			Field:    fmt.Sprintf("grants.%s.%s", strings.ToLower(objectType), objectName),
			Expected: strings.Join(expectedPrivs, ", "),
			Actual:   "",
		})
		return
	}

	// Check for missing privileges
	actualPrivSet := make(map[string]bool)
	for _, priv := range actualPrivileges {
		actualPrivSet[strings.ToUpper(priv)] = true
	}

	var missingPrivs []string
	for _, priv := range expectedPrivs {
		if !actualPrivSet[strings.ToUpper(priv)] {
			missingPrivs = append(missingPrivs, priv)
		}
	}
	if len(missingPrivs) > 0 {
		result.AddDiff(Diff{
			Field:    fmt.Sprintf("grants.%s.%s.missing", strings.ToLower(objectType), objectName),
			Expected: strings.Join(missingPrivs, ", "),
			Actual:   "",
		})
	}

	// Check for extra privileges
	expectedPrivSet := make(map[string]bool)
	for _, priv := range expectedPrivs {
		expectedPrivSet[strings.ToUpper(priv)] = true
	}

	var extraPrivs []string
	for _, priv := range actualPrivileges {
		if !expectedPrivSet[strings.ToUpper(priv)] {
			extraPrivs = append(extraPrivs, priv)
		}
	}
	if len(extraPrivs) > 0 {
		result.AddDiff(Diff{
			Field:       fmt.Sprintf("grants.%s.%s.extra", strings.ToLower(objectType), objectName),
			Expected:    "",
			Actual:      strings.Join(extraPrivs, ", "),
			Destructive: true, // Revoking privileges is destructive
		})
	}
}

// detectMySQLDirectGrantDrift detects drift for a MySQL grant.
func (s *Service) detectMySQLDirectGrantDrift(grant dbopsv1alpha1.MySQLGrant, actualGrants []types.GrantInfo, result *Result) {
	// Build a map of actual grants for quick lookup
	actualGrantMap := make(map[string][]string)
	for _, g := range actualGrants {
		key := buildGrantKey(g.Database, "", g.ObjectType, g.ObjectName)
		actualGrantMap[key] = g.Privileges
	}

	tableName := "*"
	if grant.Table != "" {
		tableName = grant.Table
	}
	key := buildGrantKey(grant.Database, "", "TABLE", tableName)
	actualPrivileges, exists := actualGrantMap[key]

	if !exists {
		// Grant doesn't exist at all
		result.AddDiff(Diff{
			Field:    fmt.Sprintf("grants.%s.%s", grant.Database, tableName),
			Expected: strings.Join(grant.Privileges, ", "),
			Actual:   "",
		})
		return
	}

	// Check for missing privileges
	actualPrivSet := make(map[string]bool)
	for _, priv := range actualPrivileges {
		actualPrivSet[strings.ToUpper(priv)] = true
	}

	var missingPrivs []string
	for _, priv := range grant.Privileges {
		if !actualPrivSet[strings.ToUpper(priv)] {
			missingPrivs = append(missingPrivs, priv)
		}
	}
	if len(missingPrivs) > 0 {
		result.AddDiff(Diff{
			Field:    fmt.Sprintf("grants.%s.%s.missing", grant.Database, tableName),
			Expected: strings.Join(missingPrivs, ", "),
			Actual:   "",
		})
	}
}

// buildGrantKey builds a unique key for a grant.
func buildGrantKey(database, schema, objectType, objectName string) string {
	parts := []string{database}
	if schema != "" {
		parts = append(parts, schema)
	}
	parts = append(parts, objectType, objectName)
	return strings.ToLower(strings.Join(parts, "."))
}

// CorrectGrantDrift applies corrections for detected grant drift.
func (s *Service) CorrectGrantDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, grantee string, driftResult *Result) (*CorrectionResult, error) {
	log := s.log.WithValues("grantee", grantee)
	log.Info("correcting grant drift", "diffs", len(driftResult.Diffs))

	result := NewCorrectionResult(grantee)

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
		if err := s.applyGrantCorrection(ctx, spec, grantee, diff); err != nil {
			result.AddFailed(diff, err)
			log.Error(err, "failed to correct drift", "field", diff.Field)
			continue
		}

		result.AddCorrected(diff)
		log.Info("corrected drift", "field", diff.Field)
	}

	return result, nil
}

// applyGrantCorrection applies a single correction to grants.
func (s *Service) applyGrantCorrection(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, grantee string, diff Diff) error {
	switch {
	case diff.Field == "roles.missing":
		// Grant missing roles
		roles := strings.Split(diff.Expected, ", ")
		return s.adapter.GrantRole(ctx, grantee, roles)

	case diff.Field == "roles.extra":
		// Revoke extra roles (destructive)
		roles := strings.Split(diff.Actual, ", ")
		return s.adapter.RevokeRole(ctx, grantee, roles)

	case strings.Contains(diff.Field, ".missing"):
		// Grant missing privileges
		return s.applyMissingGrants(ctx, spec, grantee, diff)

	case strings.Contains(diff.Field, ".extra"):
		// Revoke extra privileges (destructive)
		return s.revokeExtraGrants(ctx, grantee, diff)

	default:
		return fmt.Errorf("unsupported drift correction for field: %s", diff.Field)
	}
}

// applyMissingGrants applies missing grant privileges.
func (s *Service) applyMissingGrants(ctx context.Context, spec *dbopsv1alpha1.DatabaseGrantSpec, grantee string, diff Diff) error {
	privileges := strings.Split(diff.Expected, ", ")

	// For PostgreSQL grants
	if spec.Postgres != nil {
		for _, grant := range spec.Postgres.Grants {
			// Check if this grant matches the diff
			for _, table := range grant.Tables {
				if strings.Contains(diff.Field, table) {
					opts := []types.GrantOptions{{
						Database:   grant.Database,
						Schema:     grant.Schema,
						Tables:     []string{table},
						Privileges: privileges,
					}}
					return s.adapter.Grant(ctx, grantee, opts)
				}
			}
			for _, seq := range grant.Sequences {
				if strings.Contains(diff.Field, seq) {
					opts := []types.GrantOptions{{
						Database:   grant.Database,
						Schema:     grant.Schema,
						Sequences:  []string{seq},
						Privileges: privileges,
					}}
					return s.adapter.Grant(ctx, grantee, opts)
				}
			}
			for _, fn := range grant.Functions {
				if strings.Contains(diff.Field, fn) {
					opts := []types.GrantOptions{{
						Database:   grant.Database,
						Schema:     grant.Schema,
						Functions:  []string{fn},
						Privileges: privileges,
					}}
					return s.adapter.Grant(ctx, grantee, opts)
				}
			}
		}
	}

	// For MySQL grants
	if spec.MySQL != nil {
		for _, grant := range spec.MySQL.Grants {
			tableName := grant.Table
			if tableName == "" {
				tableName = "*"
			}
			if strings.Contains(diff.Field, tableName) || strings.Contains(diff.Field, grant.Database) {
				opts := []types.GrantOptions{{
					Database:   grant.Database,
					Table:      tableName,
					Privileges: privileges,
				}}
				return s.adapter.Grant(ctx, grantee, opts)
			}
		}
	}

	return fmt.Errorf("could not find matching grant for field: %s", diff.Field)
}

// revokeExtraGrants revokes extra grant privileges.
func (s *Service) revokeExtraGrants(ctx context.Context, grantee string, diff Diff) error {
	privileges := strings.Split(diff.Actual, ", ")

	// Parse the field to extract grant details
	// Field format: "grants.objectType.objectName.extra" or "grants.database.table.extra"
	parts := strings.Split(diff.Field, ".")
	if len(parts) < 3 {
		return fmt.Errorf("invalid field format: %s", diff.Field)
	}

	// Use the Revoke method through grant
	opts := []types.GrantOptions{{
		Tables:     []string{parts[2]},
		Privileges: privileges,
	}}

	return s.adapter.Revoke(ctx, grantee, opts)
}
