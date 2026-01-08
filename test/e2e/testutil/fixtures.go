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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
