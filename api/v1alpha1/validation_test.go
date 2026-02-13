//go:build envtest

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

package v1alpha1

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// setupTestEnv starts an envtest environment and returns a client and context.
// Cleanup is handled automatically via t.Cleanup.
func setupTestEnv(t *testing.T) (client.Client, context.Context) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(AddToScheme(scheme))

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	require.NotNil(t, k8sClient)

	t.Cleanup(func() {
		cancel()
		_ = testEnv.Stop()
	})

	return k8sClient, ctx
}

// TestDatabaseValidation tests Database CRD validation
func TestDatabaseValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		database  *Database
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid database - should succeed",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-db",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Name:        "valid_db",
				},
			},
			wantErr: false,
		},
		{
			name: "empty name - should fail",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-name",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Name:        "",
				},
			},
			wantErr:   true,
			errSubstr: "name",
		},
		{
			name: "name too long - should fail",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-long-name",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Name:        "this_name_is_way_too_long_and_exceeds_the_maximum_length_of_sixty_three_characters_allowed",
				},
			},
			wantErr:   true,
			errSubstr: "name",
		},
		{
			name: "invalid name pattern (starts with number) - should fail",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid-pattern",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Name:        "123invalid",
				},
			},
			wantErr:   true,
			errSubstr: "name",
		},
		{
			name: "invalid name pattern (contains hyphen) - should fail",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid-hyphen",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Name:        "invalid-name",
				},
			},
			wantErr:   true,
			errSubstr: "name",
		},
		{
			name: "valid deletion policy Retain - should succeed",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-retain-policy",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef:    &InstanceReference{Name: "test-instance"},
					Name:           "valid_db",
					DeletionPolicy: DeletionPolicyRetain,
				},
			},
			wantErr: false,
		},
		{
			name: "valid deletion policy Delete - should succeed",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-delete-policy",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef:    &InstanceReference{Name: "test-instance"},
					Name:           "valid_db",
					DeletionPolicy: DeletionPolicyDelete,
				},
			},
			wantErr: false,
		},
		{
			name:    "empty instance ref name - should fail",
			database: &Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-instance-ref",
					Namespace: "default",
				},
				Spec: DatabaseSpec{
					InstanceRef: &InstanceReference{Name: ""},
					Name:        "valid_db",
				},
			},
			wantErr:   true,
			errSubstr: "instanceRef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.database)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				// Cleanup created resource
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.database)
				}
			}
		})
	}
}

// TestDatabaseUserValidation tests DatabaseUser CRD validation
func TestDatabaseUserValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		user      *DatabaseUser
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid user - should succeed",
			user: &DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-user",
					Namespace: "default",
				},
				Spec: DatabaseUserSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Username:    "valid_user",
				},
			},
			wantErr: false,
		},
		{
			name: "empty username - should fail",
			user: &DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-username",
					Namespace: "default",
				},
				Spec: DatabaseUserSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Username:    "",
				},
			},
			wantErr:   true,
			errSubstr: "username",
		},
		{
			name: "username too long - should fail",
			user: &DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-long-username",
					Namespace: "default",
				},
				Spec: DatabaseUserSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Username:    "this_username_is_way_too_long_and_exceeds_maximum_length_allowed",
				},
			},
			wantErr:   true,
			errSubstr: "username",
		},
		{
			name: "invalid username pattern - should fail",
			user: &DatabaseUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid-username",
					Namespace: "default",
				},
				Spec: DatabaseUserSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					Username:    "123invalid",
				},
			},
			wantErr:   true,
			errSubstr: "username",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.user)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.user)
				}
			}
		})
	}
}

// TestDatabaseInstanceValidation tests DatabaseInstance CRD validation
func TestDatabaseInstanceValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		instance  *DatabaseInstance
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid postgres instance - should succeed",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-instance",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     5432,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid mysql instance - should succeed",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mysql-instance",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypeMySQL,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     3306,
						Database: "mysql",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty host - should fail",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-host",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "",
						Port:     5432,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "host",
		},
		{
			name: "port too low - should fail",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-low-port",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     0,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "port",
		},
		{
			name: "port too high - should fail",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-high-port",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     70000,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.instance)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.instance)
				}
			}
		})
	}
}

// TestDatabaseRoleValidation tests DatabaseRole CRD validation
func TestDatabaseRoleValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		role      *DatabaseRole
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid role - should succeed",
			role: &DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-role",
					Namespace: "default",
				},
				Spec: DatabaseRoleSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					RoleName:    "valid_role",
				},
			},
			wantErr: false,
		},
		{
			name: "empty role name - should fail",
			role: &DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-role",
					Namespace: "default",
				},
				Spec: DatabaseRoleSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					RoleName:    "",
				},
			},
			wantErr:   true,
			errSubstr: "roleName",
		},
		{
			name: "invalid role name pattern - should fail",
			role: &DatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-invalid-role",
					Namespace: "default",
				},
				Spec: DatabaseRoleSpec{
					InstanceRef: &InstanceReference{Name: "test-instance"},
					RoleName:    "123-invalid",
				},
			},
			wantErr:   true,
			errSubstr: "roleName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.role)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.role)
				}
			}
		})
	}
}

// TestHealthCheckConfigValidation tests HealthCheckConfig validation
func TestHealthCheckConfigValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		instance  *DatabaseInstance
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid health check config - should succeed",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc-valid",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     5432,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
					HealthCheck: &HealthCheckConfig{
						Enabled:         true,
						IntervalSeconds: 30,
						TimeoutSeconds:  5,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "interval too low - should fail",
			instance: &DatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc-low-interval",
					Namespace: "default",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     5432,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name: "test-secret",
						},
					},
					HealthCheck: &HealthCheckConfig{
						Enabled:         true,
						IntervalSeconds: 2, // Minimum is 5
						TimeoutSeconds:  5,
					},
				},
			},
			wantErr:   true,
			errSubstr: "intervalSeconds",
		},
		// Note: TimeoutSeconds with value 0 cannot be tested because:
		// - Field has `omitempty` in JSON tag
		// - Go's zero value (0) is omitted from JSON serialization
		// - API server applies default value (5), which passes Minimum=1 validation
		// - This is expected behavior for optional numeric fields with defaults
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.instance)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.instance)
				}
			}
		})
	}
}

// TestBackupScheduleValidation tests DatabaseBackupSchedule validation
func TestBackupScheduleValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		schedule  *DatabaseBackupSchedule
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid schedule - should succeed",
			schedule: &DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-schedule",
					Namespace: "default",
				},
				Spec: DatabaseBackupScheduleSpec{
					Schedule: "0 0 * * *",
					Template: BackupTemplateSpec{
						Spec: DatabaseBackupSpec{
							DatabaseRef: DatabaseReference{
								Name: "test-db",
							},
							Storage: StorageConfig{
								Type: StorageTypePVC,
								PVC: &PVCStorageConfig{
									ClaimName: "backup-pvc",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty schedule - should fail",
			schedule: &DatabaseBackupSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-empty-schedule",
					Namespace: "default",
				},
				Spec: DatabaseBackupScheduleSpec{
					Schedule: "",
					Template: BackupTemplateSpec{
						Spec: DatabaseBackupSpec{
							DatabaseRef: DatabaseReference{
								Name: "test-db",
							},
							Storage: StorageConfig{
								Type: StorageTypePVC,
								PVC: &PVCStorageConfig{
									ClaimName: "backup-pvc",
								},
							},
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.schedule)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.schedule)
				}
			}
		})
	}
}

// TestDatabaseGrantValidation tests DatabaseGrant CRD validation
func TestDatabaseGrantValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		grant     *DatabaseGrant
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid grant with userRef - should succeed",
			grant: &DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-grant-user",
					Namespace: "default",
				},
				Spec: DatabaseGrantSpec{
					UserRef: &UserReference{Name: "test-user"},
					Postgres: &PostgresGrantConfig{
						Roles: []string{"app_read"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid grant with roleRef - should succeed",
			grant: &DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-grant-role",
					Namespace: "default",
				},
				Spec: DatabaseGrantSpec{
					RoleRef: &RoleReference{Name: "test-role"},
					Postgres: &PostgresGrantConfig{
						Roles: []string{"app_read"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing both userRef and roleRef - should fail",
			grant: &DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-ref-grant",
					Namespace: "default",
				},
				Spec: DatabaseGrantSpec{
					Postgres: &PostgresGrantConfig{
						Roles: []string{"app_read"},
					},
				},
			},
			wantErr:   true,
			errSubstr: "userRef",
		},
		{
			name: "both userRef and roleRef - should fail",
			grant: &DatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-both-ref-grant",
					Namespace: "default",
				},
				Spec: DatabaseGrantSpec{
					UserRef: &UserReference{Name: "test-user"},
					RoleRef: &RoleReference{Name: "test-role"},
					Postgres: &PostgresGrantConfig{
						Roles: []string{"app_read"},
					},
				},
			},
			wantErr:   true,
			errSubstr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.grant)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.grant)
				}
			}
		})
	}
}

// TestClusterDatabaseInstanceValidation tests ClusterDatabaseInstance CRD validation
func TestClusterDatabaseInstanceValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		instance  *ClusterDatabaseInstance
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid cluster instance - should succeed",
			instance: &ClusterDatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-valid-cluster-instance",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "db.example.com",
						Port:     5432,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty host - should fail",
			instance: &ClusterDatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-empty-host",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "",
						Port:     5432,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "host",
		},
		{
			name: "invalid port - should fail",
			instance: &ClusterDatabaseInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-invalid-port",
				},
				Spec: DatabaseInstanceSpec{
					Engine: EngineTypePostgres,
					Connection: ConnectionConfig{
						Host:     "localhost",
						Port:     70000,
						Database: "postgres",
						SecretRef: &CredentialSecretRef{
							Name:      "test-secret",
							Namespace: "default",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.instance)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.instance)
				}
			}
		})
	}
}

// TestClusterDatabaseRoleValidation tests ClusterDatabaseRole CRD validation
func TestClusterDatabaseRoleValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		role      *ClusterDatabaseRole
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid cluster role - should succeed",
			role: &ClusterDatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-valid-cluster-role",
				},
				Spec: ClusterDatabaseRoleSpec{
					ClusterInstanceRef: ClusterInstanceReference{
						Name: "test-cluster-instance",
					},
					RoleName: "valid_role",
				},
			},
			wantErr: false,
		},
		{
			name: "empty role name - should fail",
			role: &ClusterDatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-empty-role",
				},
				Spec: ClusterDatabaseRoleSpec{
					ClusterInstanceRef: ClusterInstanceReference{
						Name: "test-cluster-instance",
					},
					RoleName: "",
				},
			},
			wantErr:   true,
			errSubstr: "roleName",
		},
		{
			name: "invalid role name pattern (starts with number) - should fail",
			role: &ClusterDatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-invalid-role",
				},
				Spec: ClusterDatabaseRoleSpec{
					ClusterInstanceRef: ClusterInstanceReference{
						Name: "test-cluster-instance",
					},
					RoleName: "123invalid",
				},
			},
			wantErr:   true,
			errSubstr: "roleName",
		},
		{
			name: "role name with hyphen - should fail",
			role: &ClusterDatabaseRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster-hyphen-role",
				},
				Spec: ClusterDatabaseRoleSpec{
					ClusterInstanceRef: ClusterInstanceReference{
						Name: "test-cluster-instance",
					},
					RoleName: "invalid-role",
				},
			},
			wantErr:   true,
			errSubstr: "roleName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.role)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.role)
				}
			}
		})
	}
}

// TestClusterDatabaseGrantValidation tests ClusterDatabaseGrant CRD validation
func TestClusterDatabaseGrantValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		grant     *ClusterDatabaseGrant
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid cluster grant with userRef - should succeed",
			grant: &ClusterDatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-valid-cluster-grant",
				},
				Spec: ClusterDatabaseGrantSpec{
					ClusterInstanceRef: ClusterInstanceReference{
						Name: "test-cluster-instance",
					},
					UserRef: &NamespacedUserReference{
						Name:      "test-user",
						Namespace: "default",
					},
					Postgres: &PostgresGrantConfig{
						Roles: []string{"app_read"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid cluster grant with roleRef - should succeed",
			grant: &ClusterDatabaseGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-valid-cluster-grant-role",
				},
				Spec: ClusterDatabaseGrantSpec{
					ClusterInstanceRef: ClusterInstanceReference{
						Name: "test-cluster-instance",
					},
					RoleRef: &NamespacedRoleReference{
						Name: "test-role",
					},
					Postgres: &PostgresGrantConfig{
						Roles: []string{"app_read"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.grant)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.grant)
				}
			}
		})
	}
}

// TestDatabaseBackupValidation tests DatabaseBackup CRD validation
func TestDatabaseBackupValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		backup    *DatabaseBackup
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid backup - should succeed",
			backup: &DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-backup",
					Namespace: "default",
				},
				Spec: DatabaseBackupSpec{
					DatabaseRef: DatabaseReference{
						Name: "test-db",
					},
					Storage: StorageConfig{
						Type: StorageTypePVC,
						PVC: &PVCStorageConfig{
							ClaimName: "backup-pvc",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing databaseRef name - should fail",
			backup: &DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-dbref-backup",
					Namespace: "default",
				},
				Spec: DatabaseBackupSpec{
					DatabaseRef: DatabaseReference{
						Name: "",
					},
					Storage: StorageConfig{
						Type: StorageTypePVC,
						PVC: &PVCStorageConfig{
							ClaimName: "backup-pvc",
						},
					},
				},
			},
			wantErr:   true,
			errSubstr: "databaseRef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.backup)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.backup)
				}
			}
		})
	}
}

// TestDatabaseRestoreValidation tests DatabaseRestore CRD validation
func TestDatabaseRestoreValidation(t *testing.T) {
	k8sClient, ctx := setupTestEnv(t)

	tests := []struct {
		name      string
		restore   *DatabaseRestore
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid restore with backupRef - should succeed",
			restore: &DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-valid-restore",
					Namespace: "default",
				},
				Spec: DatabaseRestoreSpec{
					BackupRef: &BackupReference{
						Name: "test-backup",
					},
					Target: RestoreTarget{
						InstanceRef: &InstanceReference{
							Name: "test-instance",
						},
						DatabaseName: "restored_db",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "mutually exclusive target refs - should fail",
			restore: &DatabaseRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-exclusive-restore",
					Namespace: "default",
				},
				Spec: DatabaseRestoreSpec{
					BackupRef: &BackupReference{
						Name: "test-backup",
					},
					Target: RestoreTarget{
						InstanceRef: &InstanceReference{
							Name: "test-instance",
						},
						ClusterInstanceRef: &ClusterInstanceReference{
							Name: "test-cluster-instance",
						},
						DatabaseName: "restored_db",
					},
				},
			},
			wantErr:   true,
			errSubstr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k8sClient.Create(ctx, tt.restore)

			if tt.wantErr {
				assert.Error(t, err, "Expected validation error")
				if tt.errSubstr != "" && err != nil {
					assert.Contains(t, err.Error(), tt.errSubstr,
						"Error should mention '%s'", tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "Expected no validation error")
				if err == nil {
					_ = k8sClient.Delete(ctx, tt.restore)
				}
			}
		})
	}
}
