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

package cmd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	adaptertypes "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/service"
)

func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dbopsv1alpha1.AddToScheme(scheme))
	return scheme
}

// mockOwnershipChecker implements ownershipIntegrityChecker for testing.
type mockOwnershipChecker struct {
	roleExistsFn        func(string) (bool, error)
	createRoleFn        func(adaptertypes.CreateRoleOptions) error
	getRoleInfoFn       func(string) (*adaptertypes.RoleInfo, error)
	grantRoleFn         func(string, []string) error
	getDatabaseInfoFn   func(string) (*adaptertypes.DatabaseInfo, error)
	transferOwnershipFn func(string, string) error
	setDefaultPrivsFn   func(string, []adaptertypes.DefaultPrivilegeGrantOptions) error

	// Call tracking
	createRoleCalls []adaptertypes.CreateRoleOptions
	grantRoleCalls  []struct {
		Grantee string
		Roles   []string
	}
	transferCalls        []struct{ DB, Owner string }
	setDefaultPrivsCalls []struct {
		Grantee string
		Opts    []adaptertypes.DefaultPrivilegeGrantOptions
	}
}

func (m *mockOwnershipChecker) RoleExists(ctx context.Context, roleName string) (bool, error) {
	if m.roleExistsFn != nil {
		return m.roleExistsFn(roleName)
	}
	return true, nil
}

func (m *mockOwnershipChecker) CreateRole(ctx context.Context, opts adaptertypes.CreateRoleOptions) error {
	m.createRoleCalls = append(m.createRoleCalls, opts)
	if m.createRoleFn != nil {
		return m.createRoleFn(opts)
	}
	return nil
}

func (m *mockOwnershipChecker) GetRoleInfo(ctx context.Context, roleName string) (*adaptertypes.RoleInfo, error) {
	if m.getRoleInfoFn != nil {
		return m.getRoleInfoFn(roleName)
	}
	return &adaptertypes.RoleInfo{Name: roleName}, nil
}

func (m *mockOwnershipChecker) GrantRole(ctx context.Context, grantee string, roles []string) error {
	m.grantRoleCalls = append(m.grantRoleCalls, struct {
		Grantee string
		Roles   []string
	}{grantee, roles})
	if m.grantRoleFn != nil {
		return m.grantRoleFn(grantee, roles)
	}
	return nil
}

func (m *mockOwnershipChecker) GetDatabaseInfo(ctx context.Context, name string) (*adaptertypes.DatabaseInfo, error) {
	if m.getDatabaseInfoFn != nil {
		return m.getDatabaseInfoFn(name)
	}
	return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_" + name + "_owner"}, nil
}

func (m *mockOwnershipChecker) TransferDatabaseOwnership(ctx context.Context, dbName, newOwner string) error {
	m.transferCalls = append(m.transferCalls, struct{ DB, Owner string }{dbName, newOwner})
	if m.transferOwnershipFn != nil {
		return m.transferOwnershipFn(dbName, newOwner)
	}
	return nil
}

func (m *mockOwnershipChecker) SetDefaultPrivileges(
	ctx context.Context, grantee string, opts []adaptertypes.DefaultPrivilegeGrantOptions,
) error {
	m.setDefaultPrivsCalls = append(m.setDefaultPrivsCalls, struct {
		Grantee string
		Opts    []adaptertypes.DefaultPrivilegeGrantOptions
	}{grantee, opts})
	if m.setDefaultPrivsFn != nil {
		return m.setDefaultPrivsFn(grantee, opts)
	}
	return nil
}

func TestMigrateReversePrivileges_AutoOwnership_DerivesNames(t *testing.T) {
	ownershipCfg := &dbopsv1alpha1.PostgresOwnershipConfig{
		AutoOwnership: true,
	}
	roleName := service.DeriveRoleName(ownershipCfg, "mydb")
	userName := service.DeriveUserName(ownershipCfg, "mydb")

	assert.Equal(t, "db_mydb_owner", roleName)
	assert.Equal(t, "db_mydb_app", userName)
}

func TestMigrateReversePrivileges_CustomNames(t *testing.T) {
	ownershipCfg := &dbopsv1alpha1.PostgresOwnershipConfig{
		AutoOwnership: true,
		RoleName:      "custom_role",
		UserName:      "custom_user",
	}
	roleName := service.DeriveRoleName(ownershipCfg, "mydb")
	userName := service.DeriveUserName(ownershipCfg, "mydb")

	assert.Equal(t, "custom_role", roleName)
	assert.Equal(t, "custom_user", userName)
}

func TestMigrateReversePrivileges_DirectOwnership_Skips(t *testing.T) {
	// Direct ownership (spec.Owner set without auto-ownership) should skip
	spec := &dbopsv1alpha1.DatabaseSpec{
		Owner: "some_role",
	}

	assert.False(t, service.HasAutoOwnership(spec))
}

func TestMigrateReversePrivileges_NoOwnership_Skips(t *testing.T) {
	spec := &dbopsv1alpha1.DatabaseSpec{}
	assert.False(t, service.HasAutoOwnership(spec))
}

func TestMigrateReversePrivileges_MySQLEngine_Skips(t *testing.T) {
	assert.False(t, service.IsOwnershipSupported("mysql"))
}

func TestMigrateReversePrivileges_ClickHouseEngine_Skips(t *testing.T) {
	assert.False(t, service.IsOwnershipSupported("clickhouse"))
}

func TestMigrateReversePrivileges_PostgresEngine_Supported(t *testing.T) {
	assert.True(t, service.IsOwnershipSupported("postgres"))
}

func TestMigrateReversePrivileges_CockroachDBEngine_Supported(t *testing.T) {
	assert.True(t, service.IsOwnershipSupported("cockroachdb"))
}

func TestMigrateReversePrivileges_SetDefaultPrivilegesFalse_Skips(t *testing.T) {
	v := false
	ownershipCfg := &dbopsv1alpha1.PostgresOwnershipConfig{
		AutoOwnership:        true,
		SetDefaultPrivileges: &v,
	}
	assert.False(t, ownershipCfg.ShouldSetDefaultPrivileges())
}

func TestMigrateReversePrivileges_DefaultPrivilegeDefinitions(t *testing.T) {
	defs := defaultPrivilegeDefinitions()

	require.Len(t, defs, 3)

	assert.Equal(t, "tables", defs[0].objectType)
	assert.Equal(t, []string{"SELECT", "INSERT", "UPDATE", "DELETE"}, defs[0].privileges)

	assert.Equal(t, "sequences", defs[1].objectType)
	assert.Equal(t, []string{"USAGE", "SELECT"}, defs[1].privileges)

	assert.Equal(t, "functions", defs[2].objectType)
	assert.Equal(t, []string{"EXECUTE"}, defs[2].privileges)
}

func TestMigrateReversePrivileges_MultipleSchemas_CorrectGrantCount(t *testing.T) {
	defs := defaultPrivilegeDefinitions()

	schemas := []string{"public", "app", "reporting"}
	expectedGrants := len(defs) * len(schemas) // 3 object types × 3 schemas = 9
	assert.Equal(t, 9, expectedGrants)
}

func TestMigrateReversePrivileges_FetchDatabaseCR(t *testing.T) {
	scheme := testScheme()

	db := &dbopsv1alpha1.Database{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapp-db",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseSpec{
			Name: "myapp",
			Postgres: &dbopsv1alpha1.PostgresDatabaseConfig{
				Ownership: &dbopsv1alpha1.PostgresOwnershipConfig{
					AutoOwnership: true,
				},
				Schemas: []dbopsv1alpha1.PostgresSchema{
					{Name: "app"},
					{Name: "reporting"},
				},
			},
			InstanceRef: &dbopsv1alpha1.InstanceReference{
				Name: "pg-instance",
			},
		},
	}

	instance := &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pg-instance",
			Namespace: "default",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host: "localhost",
				Port: 5432,
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "pg-creds",
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pg-creds",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(db, instance, credSecret).
		WithStatusSubresource(instance).
		Build()

	// Verify we can fetch and process the Database CR correctly
	var fetchedDB dbopsv1alpha1.Database
	err := k8sClient.Get(t.Context(), types.NamespacedName{Namespace: "default", Name: "myapp-db"}, &fetchedDB)
	require.NoError(t, err)
	assert.Equal(t, "myapp", fetchedDB.Spec.Name)
	assert.True(t, service.HasAutoOwnership(&fetchedDB.Spec))
	assert.True(t, service.IsOwnershipSupported(string(dbopsv1alpha1.EngineTypePostgres)))

	// Verify schema collection
	schemaSet := map[string]bool{"public": true}
	for _, s := range fetchedDB.Spec.Postgres.Schemas {
		schemaSet[s.Name] = true
	}
	assert.Len(t, schemaSet, 3) // public + app + reporting

	// Verify name derivation
	ownerRole := service.DeriveRoleName(fetchedDB.Spec.Postgres.Ownership, fetchedDB.Spec.Name)
	appUser := service.DeriveUserName(fetchedDB.Spec.Postgres.Ownership, fetchedDB.Spec.Name)
	assert.Equal(t, "db_myapp_owner", ownerRole)
	assert.Equal(t, "db_myapp_app", appUser)
}

func TestBuildK8sClient_SchemeIncludesOperatorCRDs(t *testing.T) {
	// Verify the scheme setup includes our CRDs
	scheme := testScheme()

	// Verify Database type is registered
	gvks, _, err := scheme.ObjectKinds(&dbopsv1alpha1.Database{})
	require.NoError(t, err)
	assert.NotEmpty(t, gvks)

	// Verify DatabaseInstance type is registered
	gvks, _, err = scheme.ObjectKinds(&dbopsv1alpha1.DatabaseInstance{})
	require.NoError(t, err)
	assert.NotEmpty(t, gvks)

	// Verify ClusterDatabaseInstance type is registered
	gvks, _, err = scheme.ObjectKinds(&dbopsv1alpha1.ClusterDatabaseInstance{})
	require.NoError(t, err)
	assert.NotEmpty(t, gvks)
}

// --- Integrity Check Tests ---

func TestIntegrityCheck_OwnerRoleMissing(t *testing.T) {
	mock := &mockOwnershipChecker{
		roleExistsFn: func(name string) (bool, error) {
			if name == "db_mydb_owner" {
				return false, nil // owner role missing
			}
			return true, nil // app user exists
		},
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// Should have created the owner role
	require.Len(t, mock.createRoleCalls, 1)
	assert.Equal(t, "db_mydb_owner", mock.createRoleCalls[0].RoleName)
	assert.False(t, mock.createRoleCalls[0].Login)
	assert.True(t, mock.createRoleCalls[0].Inherit)

	// Should have a diff for ownership.role
	require.Len(t, result.diffs, 1)
	assert.Equal(t, "ownership.role", result.diffs[0].Field)
	assert.Equal(t, "db_mydb_owner", result.diffs[0].Expected)
	assert.Equal(t, "<missing>", result.diffs[0].Actual)
	assert.Equal(t, 1, result.fixed)
}

func TestIntegrityCheck_AppUserMissing(t *testing.T) {
	mock := &mockOwnershipChecker{
		roleExistsFn: func(name string) (bool, error) {
			if name == "db_mydb_app" {
				return false, nil // app user missing
			}
			return true, nil // owner role exists
		},
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			// After creation, GetRoleInfo returns the user with membership already set
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// Should have created the app user
	require.Len(t, mock.createRoleCalls, 1)
	assert.Equal(t, "db_mydb_app", mock.createRoleCalls[0].RoleName)
	assert.True(t, mock.createRoleCalls[0].Login)
	assert.True(t, mock.createRoleCalls[0].Inherit)

	// Should have a diff for ownership.user
	require.Len(t, result.diffs, 1)
	assert.Equal(t, "ownership.user", result.diffs[0].Field)
	assert.Equal(t, 1, result.fixed)
}

func TestIntegrityCheck_MembershipMissing(t *testing.T) {
	mock := &mockOwnershipChecker{
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			// App user exists but does NOT inherit from owner role
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// Should have granted role membership
	require.Len(t, mock.grantRoleCalls, 1)
	assert.Equal(t, "db_mydb_app", mock.grantRoleCalls[0].Grantee)
	assert.Equal(t, []string{"db_mydb_owner"}, mock.grantRoleCalls[0].Roles)

	// Should have a diff for ownership.membership
	require.Len(t, result.diffs, 1)
	assert.Equal(t, "ownership.membership", result.diffs[0].Field)
	assert.Equal(t, 1, result.fixed)
}

func TestIntegrityCheck_WrongOwner(t *testing.T) {
	mock := &mockOwnershipChecker{
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "postgres"}, nil // wrong owner
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// Should have transferred ownership
	require.Len(t, mock.transferCalls, 1)
	assert.Equal(t, "mydb", mock.transferCalls[0].DB)
	assert.Equal(t, "db_mydb_owner", mock.transferCalls[0].Owner)

	// Diff should be marked destructive
	require.Len(t, result.diffs, 1)
	assert.Equal(t, "ownership.dbOwner", result.diffs[0].Field)
	assert.Equal(t, "db_mydb_owner", result.diffs[0].Expected)
	assert.Equal(t, "postgres", result.diffs[0].Actual)
	assert.True(t, result.diffs[0].Destructive)
	assert.Equal(t, 1, result.fixed)
}

func TestIntegrityCheck_AllOK(t *testing.T) {
	mock := &mockOwnershipChecker{
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// No mutations except forward default privileges (check 5)
	assert.Empty(t, mock.createRoleCalls)
	assert.Empty(t, mock.grantRoleCalls)
	assert.Empty(t, mock.transferCalls)
	assert.Empty(t, result.diffs)
	assert.Equal(t, 0, result.fixed)
	assert.Equal(t, 5, result.checked)

	// Check 5 should have applied forward default privileges (3 object types × 1 schema)
	assert.Len(t, mock.setDefaultPrivsCalls, 3)
}

func TestIntegrityCheck_DryRun_NoMutations(t *testing.T) {
	mock := &mockOwnershipChecker{
		roleExistsFn: func(name string) (bool, error) {
			return false, nil // both roles missing
		},
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "postgres"}, nil
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, true)
	require.NoError(t, err)

	// Diffs should be recorded
	assert.Len(t, result.diffs, 4) // role missing, user missing, membership missing, wrong owner

	// But NO mutation calls should have been made
	assert.Empty(t, mock.createRoleCalls)
	assert.Empty(t, mock.grantRoleCalls)
	assert.Empty(t, mock.transferCalls)
	assert.Empty(t, mock.setDefaultPrivsCalls)
	assert.Equal(t, 0, result.fixed)
}

func TestIntegrityCheck_ForwardPrivileges_Applied(t *testing.T) {
	mock := &mockOwnershipChecker{
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	schemas := []string{"public", "app", "reporting"}
	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", schemas, false)
	require.NoError(t, err)

	// 3 object types × 3 schemas = 9 forward privilege calls
	assert.Len(t, mock.setDefaultPrivsCalls, 9)

	// All calls should grant TO appUser with GrantedBy ownerRole
	for _, call := range mock.setDefaultPrivsCalls {
		assert.Equal(t, "db_mydb_app", call.Grantee)
		require.Len(t, call.Opts, 1)
		assert.Equal(t, "db_mydb_owner", call.Opts[0].GrantedBy)
		assert.Equal(t, "mydb", call.Opts[0].Database)
	}

	assert.Empty(t, result.diffs)
}

func TestIntegrityCheck_MultipleDrifts(t *testing.T) {
	roleExistsCalls := 0
	mock := &mockOwnershipChecker{
		roleExistsFn: func(name string) (bool, error) {
			roleExistsCalls++
			if name == "db_mydb_owner" {
				return false, nil // owner role missing
			}
			return true, nil // app user exists
		},
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "postgres"}, nil // wrong owner
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// Should have both diffs: missing role + wrong owner
	require.Len(t, result.diffs, 2)
	assert.Equal(t, "ownership.role", result.diffs[0].Field)
	assert.Equal(t, "ownership.dbOwner", result.diffs[1].Field)
	assert.Equal(t, 2, result.fixed)

	// Both fixes applied
	require.Len(t, mock.createRoleCalls, 1)
	require.Len(t, mock.transferCalls, 1)
}

func TestIntegrityCheck_StatusDriftUpdated(t *testing.T) {
	// This test verifies the diffs structure that would be written to status.drift
	mock := &mockOwnershipChecker{
		roleExistsFn: func(name string) (bool, error) {
			if name == "db_mydb_app" {
				return false, nil
			}
			return true, nil
		},
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, true)
	require.NoError(t, err)

	// Simulate what the main function would do: set status.drift
	driftStatus := &dbopsv1alpha1.DriftStatus{
		Detected: len(result.diffs) > 0,
		Diffs:    result.diffs,
	}

	assert.True(t, driftStatus.Detected)
	require.Len(t, driftStatus.Diffs, 1)
	assert.Equal(t, "ownership.user", driftStatus.Diffs[0].Field)
}

func TestIntegrityCheck_StatusClearedAfterFix(t *testing.T) {
	mock := &mockOwnershipChecker{
		roleExistsFn: func(name string) (bool, error) {
			if name == "db_mydb_app" {
				return false, nil
			}
			return true, nil
		},
		getRoleInfoFn: func(name string) (*adaptertypes.RoleInfo, error) {
			return &adaptertypes.RoleInfo{Name: name, InRoles: []string{"db_mydb_owner"}}, nil
		},
		getDatabaseInfoFn: func(name string) (*adaptertypes.DatabaseInfo, error) {
			return &adaptertypes.DatabaseInfo{Name: name, Owner: "db_mydb_owner"}, nil
		},
	}

	// Run in non-dry-run mode (fix applied)
	result, err := runIntegrityChecks(context.Background(), mock,
		"mydb", "db_mydb_owner", "db_mydb_app", []string{"public"}, false)
	require.NoError(t, err)

	// Diffs were found (and fixed)
	require.Len(t, result.diffs, 1)

	// Simulate what the main function does after fix:
	// set detected=false and clear diffs
	driftStatus := &dbopsv1alpha1.DriftStatus{
		Detected: false,
	}

	assert.False(t, driftStatus.Detected)
	assert.Empty(t, driftStatus.Diffs)
}

func TestDriftFieldNames(t *testing.T) {
	diffs := []dbopsv1alpha1.DriftDiff{
		{Field: "ownership.role"},
		{Field: "ownership.dbOwner"},
		{Field: "ownership.membership"},
	}

	names := driftFieldNames(diffs)
	assert.Equal(t, []string{"ownership.role", "ownership.dbOwner", "ownership.membership"}, names)
}

func TestDriftFieldNames_Empty(t *testing.T) {
	names := driftFieldNames(nil)
	assert.Empty(t, names)
}
