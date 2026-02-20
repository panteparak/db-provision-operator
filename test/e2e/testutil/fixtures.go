//go:build e2e

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

package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const (
	// APIVersion for all dbops resources
	APIVersion = "dbops.dbprovision.io/v1alpha1"
)

// BuildDatabaseInstance creates an unstructured DatabaseInstance resource.
func BuildDatabaseInstance(name, namespace, engine, host string, port int64, secretRef SecretRef) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseInstance",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"engine": engine,
				"connection": map[string]interface{}{
					"host":     host,
					"port":     port,
					"database": getDefaultDatabase(engine),
					"secretRef": map[string]interface{}{
						"name":      secretRef.Name,
						"namespace": secretRef.Namespace,
					},
				},
			},
		},
	}
	return obj
}

// BuildDatabase creates an unstructured Database resource.
func BuildDatabase(name, namespace, instanceRef, dbName string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "Database",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"instanceRef": map[string]interface{}{
					"name": instanceRef,
				},
				"name":               dbName,
				"deletionPolicy":     "Delete",
				"deletionProtection": false,
			},
		},
	}
	return obj
}

// BuildDatabaseUser creates an unstructured DatabaseUser resource.
func BuildDatabaseUser(name, namespace, instanceRef, username string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseUser",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"instanceRef": map[string]interface{}{
					"name": instanceRef,
				},
				"username": username,
				"passwordSecret": map[string]interface{}{
					"generate":   true,
					"secretName": name + "-credentials",
				},
			},
		},
	}
	return obj
}

// BuildDatabaseRole creates an unstructured DatabaseRole resource.
func BuildDatabaseRole(name, namespace, instanceRef, roleName string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseRole",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"instanceRef": map[string]interface{}{
					"name": instanceRef,
				},
				"roleName": roleName,
			},
		},
	}
	return obj
}

// BuildDatabaseGrant creates an unstructured DatabaseGrant resource.
func BuildDatabaseGrant(name, namespace, instanceRef, grantee, privilege, objectType, objectName string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseGrant",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"userRef": map[string]interface{}{
					"name": grantee,
				},
			},
		},
	}

	// Add database reference if object type is database or related
	if objectType == "database" || objectType == "schema" || objectType == "table" {
		spec := obj.Object["spec"].(map[string]interface{})
		spec["databaseRef"] = map[string]interface{}{
			"name": objectName,
		}
	}

	return obj
}

// BuildDatabaseBackup creates an unstructured DatabaseBackup resource.
func BuildDatabaseBackup(name, namespace, instanceRef, databaseRef string, storage StorageConfig) *unstructured.Unstructured {
	storageSpec := map[string]interface{}{}

	if storage.PVC != nil {
		storageSpec["type"] = "pvc"
		storageSpec["pvc"] = map[string]interface{}{
			"claimName": storage.PVC.ClaimName,
		}
		if storage.PVC.SubPath != "" {
			storageSpec["pvc"].(map[string]interface{})["subPath"] = storage.PVC.SubPath
		}
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseBackup",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"databaseRef": map[string]interface{}{
					"name": databaseRef,
				},
				"storage": storageSpec,
			},
		},
	}
	return obj
}

// BuildDatabaseRestore creates an unstructured DatabaseRestore resource.
func BuildDatabaseRestore(name, namespace, backupRef, targetDatabaseRef string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseRestore",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"backupRef": map[string]interface{}{
					"name": backupRef,
				},
				"target": map[string]interface{}{
					"databaseRef": map[string]interface{}{
						"name": targetDatabaseRef,
					},
					"inPlace": true,
				},
				"confirmation": map[string]interface{}{
					"acknowledgeDataLoss": "I-UNDERSTAND-DATA-LOSS",
				},
			},
		},
	}
	return obj
}

// BuildDatabaseBackupSchedule creates an unstructured DatabaseBackupSchedule resource.
func BuildDatabaseBackupSchedule(name, namespace, instanceRef, schedule string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseBackupSchedule",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"schedule": schedule,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"databaseRef": map[string]interface{}{
							"name": instanceRef,
						},
						"storage": map[string]interface{}{
							"type": "pvc",
							"pvc": map[string]interface{}{
								"claimName": "backup-pvc",
							},
						},
					},
				},
			},
		},
	}
	return obj
}

// DatabaseBuildOptions configures optional fields for BuildDatabaseWithOptions.
type DatabaseBuildOptions struct {
	DeletionProtection bool
	DeletionPolicy     string                   // "Retain" or "Delete"
	DriftMode          string                   // "detect", "correct", "ignore"
	DriftInterval      string                   // e.g. "10s"
	Schemas            []map[string]interface{} // postgres.schemas entries
}

// BuildDatabaseWithOptions creates an unstructured Database resource with additional options.
// It wraps BuildDatabase and overlays optional spec fields.
func BuildDatabaseWithOptions(name, namespace, instanceRef, dbName string, opts DatabaseBuildOptions) *unstructured.Unstructured {
	obj := BuildDatabase(name, namespace, instanceRef, dbName)

	spec := obj.Object["spec"].(map[string]interface{})
	spec["deletionProtection"] = opts.DeletionProtection

	if opts.DeletionPolicy != "" {
		spec["deletionPolicy"] = opts.DeletionPolicy
	}

	if opts.DriftMode != "" {
		driftPolicy := map[string]interface{}{
			"mode": opts.DriftMode,
		}
		if opts.DriftInterval != "" {
			driftPolicy["interval"] = opts.DriftInterval
		}
		spec["driftPolicy"] = driftPolicy
	}

	if len(opts.Schemas) > 0 {
		schemas := make([]interface{}, len(opts.Schemas))
		for i, s := range opts.Schemas {
			schemas[i] = s
		}
		spec["postgres"] = map[string]interface{}{
			"schemas": schemas,
		}
	}

	return obj
}

// RoleBuildOptions configures optional fields for BuildDatabaseRoleWithOptions.
type RoleBuildOptions struct {
	DriftMode      string                 // "detect", "correct", "ignore"
	DriftInterval  string                 // e.g. "10s"
	PostgresConfig map[string]interface{} // postgres attributes (e.g. createDB, login)
}

// BuildDatabaseRoleWithOptions creates an unstructured DatabaseRole resource with additional options.
// It wraps BuildDatabaseRole and overlays driftPolicy and postgres config.
func BuildDatabaseRoleWithOptions(name, namespace, instanceRef, roleName string, opts RoleBuildOptions) *unstructured.Unstructured {
	obj := BuildDatabaseRole(name, namespace, instanceRef, roleName)

	spec := obj.Object["spec"].(map[string]interface{})

	if opts.DriftMode != "" {
		driftPolicy := map[string]interface{}{
			"mode": opts.DriftMode,
		}
		if opts.DriftInterval != "" {
			driftPolicy["interval"] = opts.DriftInterval
		}
		spec["driftPolicy"] = driftPolicy
	}

	if len(opts.PostgresConfig) > 0 {
		spec["postgres"] = opts.PostgresConfig
	}

	return obj
}

// UserBuildOptions configures optional fields for BuildDatabaseUserWithOptions.
type UserBuildOptions struct {
	DriftMode      string                 // "detect", "correct", "ignore"
	DriftInterval  string                 // e.g. "10s"
	PostgresConfig map[string]interface{} // postgres attributes (connectionLimit, createDB, etc.)
}

// BuildDatabaseUserWithOptions creates an unstructured DatabaseUser resource with additional options.
// It wraps BuildDatabaseUser and overlays driftPolicy and postgres config.
func BuildDatabaseUserWithOptions(name, namespace, instanceRef, username string, opts UserBuildOptions) *unstructured.Unstructured {
	obj := BuildDatabaseUser(name, namespace, instanceRef, username)

	spec := obj.Object["spec"].(map[string]interface{})

	if opts.DriftMode != "" {
		driftPolicy := map[string]interface{}{
			"mode": opts.DriftMode,
		}
		if opts.DriftInterval != "" {
			driftPolicy["interval"] = opts.DriftInterval
		}
		spec["driftPolicy"] = driftPolicy
	}

	if len(opts.PostgresConfig) > 0 {
		spec["postgres"] = opts.PostgresConfig
	}

	return obj
}

// GrantBuildOptions configures optional fields for BuildDatabaseGrantWithOptions.
type GrantBuildOptions struct {
	DriftMode     string                 // "detect", "correct", "ignore"
	DriftInterval string                 // e.g. "10s"
	Postgres      map[string]interface{} // postgres grant config (grants, roles)
}

// BuildDatabaseGrantWithOptions creates an unstructured DatabaseGrant resource with drift support.
// It accepts roleRef or userRef via the granteeType ("role" or "user") parameter.
func BuildDatabaseGrantWithOptions(name, namespace, granteeType, granteeName string, opts GrantBuildOptions) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": APIVersion,
			"kind":       "DatabaseGrant",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{},
		},
	}

	spec := obj.Object["spec"].(map[string]interface{})

	if granteeType == "role" {
		spec["roleRef"] = map[string]interface{}{
			"name": granteeName,
		}
	} else {
		spec["userRef"] = map[string]interface{}{
			"name": granteeName,
		}
	}

	if opts.DriftMode != "" {
		driftPolicy := map[string]interface{}{
			"mode": opts.DriftMode,
		}
		if opts.DriftInterval != "" {
			driftPolicy["interval"] = opts.DriftInterval
		}
		spec["driftPolicy"] = driftPolicy
	}

	if len(opts.Postgres) > 0 {
		spec["postgres"] = opts.Postgres
	}

	return obj
}

// getDefaultDatabase returns the default database name for the given engine.
func getDefaultDatabase(engine string) string {
	switch engine {
	case "postgres", "cockroachdb":
		return "postgres"
	case "mysql":
		return "mysql"
	default:
		return "default"
	}
}

// ===== Fixture Loader Functions =====

// FixtureData holds template values for fixture rendering
type FixtureData struct {
	// Common fields
	Name            string
	Namespace       string
	InstanceName    string
	DatabaseName    string
	Username        string
	UserName        string // alias for Username
	RoleName        string
	Engine          string
	AdminDatabase   string
	Host            string
	Port            int
	SecretName      string
	SecretNamespace string
}

// getTestDataPath returns the path to the fixtures/testdata directory
func getTestDataPath() string {
	// Get the directory of this source file
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "fixtures", "testdata")
}

// LoadFixture loads a YAML fixture file, renders it with template data, and returns an unstructured object
func LoadFixture(fixtureType, fixtureName string, data FixtureData) (*unstructured.Unstructured, error) {
	// Construct the fixture path
	fixturePath := filepath.Join(getTestDataPath(), fixtureType, fixtureName+".yaml")

	// Read the fixture file
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture %s/%s: %w", fixtureType, fixtureName, err)
	}

	// Parse as template
	tmpl, err := template.New(fixtureName).Funcs(template.FuncMap{
		"default": func(defaultVal, val interface{}) interface{} {
			if val == nil || val == "" || val == 0 {
				return defaultVal
			}
			return val
		},
	}).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse fixture template %s/%s: %w", fixtureType, fixtureName, err)
	}

	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render fixture template %s/%s: %w", fixtureType, fixtureName, err)
	}

	// Parse YAML to unstructured
	obj := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(buf.Bytes(), &obj.Object); err != nil {
		return nil, fmt.Errorf("failed to unmarshal fixture %s/%s: %w", fixtureType, fixtureName, err)
	}

	return obj, nil
}

// LoadInstanceFixture is a convenience function for loading DatabaseInstance fixtures
func LoadInstanceFixture(fixtureName string, data FixtureData) (*unstructured.Unstructured, error) {
	return LoadFixture("instance", fixtureName, data)
}

// LoadDatabaseFixture is a convenience function for loading Database fixtures
func LoadDatabaseFixture(fixtureName string, data FixtureData) (*unstructured.Unstructured, error) {
	return LoadFixture("database", fixtureName, data)
}

// LoadUserFixture is a convenience function for loading DatabaseUser fixtures
func LoadUserFixture(fixtureName string, data FixtureData) (*unstructured.Unstructured, error) {
	return LoadFixture("user", fixtureName, data)
}

// LoadRoleFixture is a convenience function for loading DatabaseRole fixtures
func LoadRoleFixture(fixtureName string, data FixtureData) (*unstructured.Unstructured, error) {
	return LoadFixture("role", fixtureName, data)
}

// LoadGrantFixture is a convenience function for loading DatabaseGrant fixtures
func LoadGrantFixture(fixtureName string, data FixtureData) (*unstructured.Unstructured, error) {
	return LoadFixture("grant", fixtureName, data)
}
