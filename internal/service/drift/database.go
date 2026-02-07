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

// DetectDatabaseDrift compares CR spec to actual database state.
// Returns a Result containing any differences found.
//
// The detection covers:
// - Owner: Who owns the database (destructive to change)
// - Encoding: Character encoding (immutable, cannot be changed after creation)
// - Extensions: PostgreSQL extensions installed (can be added/removed)
// - Schemas: Database schemas (can be added/removed)
// - Charset/Collation: MySQL character set and collation
func (s *Service) DetectDatabaseDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec) (*Result, error) {
	log := s.log.WithValues("database", spec.Name)
	log.V(1).Info("detecting database drift")

	// Get actual database state
	info, err := s.adapter.GetDatabaseInfo(ctx, spec.Name)
	if err != nil {
		log.Error(err, "failed to get database info")
		return nil, fmt.Errorf("get database info: %w", err)
	}

	result := NewResult("database", spec.Name)

	// Compare based on database engine
	if spec.Postgres != nil {
		s.detectPostgresDatabaseDrift(spec, info, result)
	}
	if spec.MySQL != nil {
		s.detectMySQLDatabaseDrift(spec, info, result)
	}

	if result.HasDrift() {
		log.Info("drift detected", "diffs", len(result.Diffs))
	} else {
		log.V(1).Info("no drift detected")
	}

	return result, nil
}

// detectPostgresDatabaseDrift detects drift for PostgreSQL-specific settings.
func (s *Service) detectPostgresDatabaseDrift(spec *dbopsv1alpha1.DatabaseSpec, info *types.DatabaseInfo, result *Result) {
	pgSpec := spec.Postgres

	// Owner comparison: Owner is typically from database info, not directly in spec
	// The owner is determined at creation time and is tracked in status
	// We compare with the actual owner from database info
	if info.Owner != "" {
		// Owner drift would be detected if spec had an owner field
		// For now, we track owner in status but don't compare for drift
	}

	// Encoding comparison (immutable)
	if pgSpec.Encoding != "" && !strings.EqualFold(pgSpec.Encoding, info.Encoding) {
		result.AddDiff(Diff{
			Field:     "encoding",
			Expected:  pgSpec.Encoding,
			Actual:    info.Encoding,
			Immutable: true, // Cannot be changed after creation
		})
	}

	// Extensions comparison
	s.compareExtensions(pgSpec.Extensions, info.Extensions, result)

	// Schemas comparison
	s.compareSchemas(pgSpec.Schemas, info.Schemas, result)
}

// detectMySQLDatabaseDrift detects drift for MySQL-specific settings.
func (s *Service) detectMySQLDatabaseDrift(spec *dbopsv1alpha1.DatabaseSpec, info *types.DatabaseInfo, result *Result) {
	mysqlSpec := spec.MySQL

	// Charset comparison
	if mysqlSpec.Charset != "" && !strings.EqualFold(mysqlSpec.Charset, info.Charset) {
		result.AddDiff(Diff{
			Field:    "charset",
			Expected: mysqlSpec.Charset,
			Actual:   info.Charset,
		})
	}

	// Collation comparison
	if mysqlSpec.Collation != "" && !strings.EqualFold(mysqlSpec.Collation, info.Collation) {
		result.AddDiff(Diff{
			Field:    "collation",
			Expected: mysqlSpec.Collation,
			Actual:   info.Collation,
		})
	}
}

// compareExtensions compares expected extensions to actual extensions.
func (s *Service) compareExtensions(expected []dbopsv1alpha1.PostgresExtension, actual []types.ExtensionInfo, result *Result) {
	// Build maps for comparison
	actualMap := make(map[string]string)
	for _, ext := range actual {
		actualMap[ext.Name] = ext.Version
	}

	expectedMap := make(map[string]string)
	for _, ext := range expected {
		expectedMap[ext.Name] = ext.Version
	}

	// Find missing extensions (in expected but not in actual)
	var missing []string
	for name := range expectedMap {
		if _, ok := actualMap[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		result.AddDiff(Diff{
			Field:    "extensions.missing",
			Expected: strings.Join(missing, ", "),
			Actual:   "",
		})
	}

	// Find extra extensions (in actual but not in expected)
	// Note: This is considered informational, not drift, unless we have a strict mode
	// For now, we only report missing extensions as drift

	// Check version mismatches (only if version was specified)
	for name, expectedVersion := range expectedMap {
		if expectedVersion == "" {
			continue // No version specified, any version is acceptable
		}
		if actualVersion, ok := actualMap[name]; ok && actualVersion != expectedVersion {
			result.AddDiff(Diff{
				Field:    fmt.Sprintf("extensions.%s.version", name),
				Expected: expectedVersion,
				Actual:   actualVersion,
			})
		}
	}
}

// compareSchemas compares expected schemas to actual schemas.
func (s *Service) compareSchemas(expected []dbopsv1alpha1.PostgresSchema, actual []string, result *Result) {
	// Build set of actual schemas
	actualSet := make(map[string]bool)
	for _, schema := range actual {
		actualSet[schema] = true
	}

	// Find missing schemas
	var missing []string
	for _, schema := range expected {
		if !actualSet[schema.Name] {
			missing = append(missing, schema.Name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		result.AddDiff(Diff{
			Field:    "schemas.missing",
			Expected: strings.Join(missing, ", "),
			Actual:   "",
		})
	}
}

// CorrectDatabaseDrift applies corrections for detected drift.
// Only applies corrections that are allowed by config.
//
// Returns a CorrectionResult showing what was corrected, skipped, or failed.
func (s *Service) CorrectDatabaseDrift(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, driftResult *Result) (*CorrectionResult, error) {
	log := s.log.WithValues("database", spec.Name)
	log.Info("correcting database drift", "diffs", len(driftResult.Diffs))

	result := NewCorrectionResult(spec.Name)

	for _, diff := range driftResult.Diffs {
		// Skip immutable fields - they require resource recreation
		if diff.Immutable {
			result.AddSkipped(diff, "immutable field, requires resource recreation")
			log.V(1).Info("skipped immutable field", "field", diff.Field)
			continue
		}

		// Skip destructive corrections unless explicitly allowed
		if diff.Destructive && !s.config.AllowDestructive {
			result.AddSkipped(diff, "destructive correction not allowed (set annotation to enable)")
			log.V(1).Info("skipped destructive correction", "field", diff.Field)
			continue
		}

		// Apply correction based on field
		if err := s.applyDatabaseCorrection(ctx, spec, diff); err != nil {
			result.AddFailed(diff, err)
			log.Error(err, "failed to correct drift", "field", diff.Field)
			continue
		}

		result.AddCorrected(diff)
		log.Info("corrected drift", "field", diff.Field,
			"from", diff.Actual, "to", diff.Expected)
	}

	return result, nil
}

// applyDatabaseCorrection applies a single correction to a database.
func (s *Service) applyDatabaseCorrection(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec, diff Diff) error {
	switch {
	case diff.Field == "owner":
		// Owner change is handled by ALTER DATABASE ... OWNER TO
		// This is destructive and requires special handling
		return s.correctDatabaseOwner(ctx, spec.Name, diff.Expected)

	case strings.HasPrefix(diff.Field, "extensions.missing"):
		// Install missing extensions
		return s.installMissingExtensions(ctx, spec)

	case strings.HasPrefix(diff.Field, "extensions.") && strings.HasSuffix(diff.Field, ".version"):
		// Extension version mismatch - update extension
		extName := strings.TrimPrefix(strings.TrimSuffix(diff.Field, ".version"), "extensions.")
		return s.updateExtensionVersion(ctx, spec.Name, extName, diff.Expected)

	case strings.HasPrefix(diff.Field, "schemas.missing"):
		// Create missing schemas
		return s.createMissingSchemas(ctx, spec)

	case diff.Field == "charset" || diff.Field == "collation":
		// MySQL charset/collation change
		return s.correctMySQLCharset(ctx, spec)

	default:
		return fmt.Errorf("unsupported drift correction for field: %s", diff.Field)
	}
}

// correctDatabaseOwner changes the database owner.
// Note: Owner changes are destructive and require direct ALTER DATABASE command.
func (s *Service) correctDatabaseOwner(ctx context.Context, dbName, newOwner string) error {
	// The adapter's UpdateDatabase doesn't directly support owner changes
	// This would need a direct SQL execution: ALTER DATABASE dbname OWNER TO newowner
	// For now, we return an error indicating this needs special handling
	return fmt.Errorf("owner change requires direct ALTER DATABASE command (not yet implemented)")
}

// installMissingExtensions installs missing PostgreSQL extensions.
func (s *Service) installMissingExtensions(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec) error {
	if spec.Postgres == nil {
		return nil
	}

	var extensions []types.ExtensionOptions
	for _, ext := range spec.Postgres.Extensions {
		extensions = append(extensions, types.ExtensionOptions{
			Name:    ext.Name,
			Schema:  ext.Schema,
			Version: ext.Version,
		})
	}

	return s.adapter.UpdateDatabase(ctx, spec.Name, types.UpdateDatabaseOptions{
		Extensions: extensions,
	})
}

// updateExtensionVersion updates a PostgreSQL extension to a specific version.
func (s *Service) updateExtensionVersion(ctx context.Context, dbName, extName, version string) error {
	return s.adapter.UpdateDatabase(ctx, dbName, types.UpdateDatabaseOptions{
		Extensions: []types.ExtensionOptions{
			{Name: extName, Version: version},
		},
	})
}

// createMissingSchemas creates missing database schemas.
func (s *Service) createMissingSchemas(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec) error {
	if spec.Postgres == nil {
		return nil
	}

	var schemas []types.SchemaOptions
	for _, schema := range spec.Postgres.Schemas {
		schemas = append(schemas, types.SchemaOptions{
			Name:  schema.Name,
			Owner: schema.Owner,
		})
	}

	return s.adapter.UpdateDatabase(ctx, spec.Name, types.UpdateDatabaseOptions{
		Schemas: schemas,
	})
}

// correctMySQLCharset corrects MySQL charset and collation.
func (s *Service) correctMySQLCharset(ctx context.Context, spec *dbopsv1alpha1.DatabaseSpec) error {
	if spec.MySQL == nil {
		return nil
	}

	return s.adapter.UpdateDatabase(ctx, spec.Name, types.UpdateDatabaseOptions{
		Charset:   spec.MySQL.Charset,
		Collation: spec.MySQL.Collation,
	})
}
