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
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	adaptertypes "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

var (
	migrateNamespace  string
	migrateKubeconfig string
)

// ownershipIntegrityChecker defines the minimal adapter interface needed for integrity checks.
// The full DatabaseAdapter satisfies this interface implicitly.
type ownershipIntegrityChecker interface {
	RoleExists(ctx context.Context, roleName string) (bool, error)
	CreateRole(ctx context.Context, opts adaptertypes.CreateRoleOptions) error
	GetRoleInfo(ctx context.Context, roleName string) (*adaptertypes.RoleInfo, error)
	GrantRole(ctx context.Context, grantee string, roles []string) error
	GetDatabaseInfo(ctx context.Context, name string) (*adaptertypes.DatabaseInfo, error)
	TransferDatabaseOwnership(ctx context.Context, dbName, newOwner string) error
	SetDefaultPrivileges(ctx context.Context, grantee string, opts []adaptertypes.DefaultPrivilegeGrantOptions) error
}

// integrityCheckResult holds the results of ownership integrity checks.
type integrityCheckResult struct {
	diffs   []dbopsv1alpha1.DriftDiff // Reuse API DriftDiff type directly
	fixed   int                       // Count of issues auto-fixed
	checked int                       // Total checks performed
}

// reversePrivilegesCmd applies missing reverse default privileges for pre-v0.13.0 databases.
var reversePrivilegesCmd = &cobra.Command{
	Use:   "reverse-privileges <database-cr-name>",
	Short: "Apply missing reverse default privileges for pre-v0.13.0 databases",
	Long: `Applies the missing reverse ALTER DEFAULT PRIVILEGES grants (appUser → ownerRole)
that were introduced in v0.13.0.

Before v0.13.0, SetDefaultPrivileges only applied forward grants (ownerRole → appUser).
This migration adds the reverse direction so that objects created by the app user
are accessible to the owner role and all rotated users that inherit from it.

Additionally, this command performs full ownership integrity checks:
  - Owner role existence
  - App user existence
  - Role membership (app user inherits from owner role)
  - Database owner correctness
  - Forward default privileges (idempotent reapplication)

Any issues found are auto-fixed (unless --dry-run), and K8s events are emitted
on the Database CR for audit visibility.

This command reads the Database CR from the Kubernetes API to determine the ownership
configuration, then connects to the database and applies the missing grants.

This operation is idempotent — safe to re-run on databases that already have
reverse privileges.

Only PostgreSQL and CockroachDB engines are supported (the only engines that
support ALTER DEFAULT PRIVILEGES).`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrateReversePrivileges,
}

func init() {
	reversePrivilegesCmd.Flags().StringVarP(&migrateNamespace, "namespace", "n", "",
		"Kubernetes namespace of the Database CR (default: current kubeconfig context namespace)")
	reversePrivilegesCmd.Flags().StringVar(&migrateKubeconfig, "kubeconfig", "",
		"Path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
}

// privilegeDefinition defines the privileges for a single object type.
type privilegeDefinition struct {
	objectType string
	privileges []string
}

// defaultPrivilegeDefinitions returns the standard privilege definitions
// that mirror ownership.go:175-179.
func defaultPrivilegeDefinitions() []privilegeDefinition {
	return []privilegeDefinition{
		{"tables", []string{"SELECT", "INSERT", "UPDATE", "DELETE"}},
		{"sequences", []string{"USAGE", "SELECT"}},
		{"functions", []string{"EXECUTE"}},
	}
}

func runMigrateReversePrivileges(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	dbCRName := args[0]

	// Build K8s client
	k8sClient, restConfig, ns, err := buildK8sClient()
	if err != nil {
		return fmt.Errorf("build kubernetes client: %w", err)
	}

	// Use flag namespace if set, otherwise use kubeconfig context namespace
	if migrateNamespace != "" {
		ns = migrateNamespace
	}
	if ns == "" {
		ns = "default"
	}

	printVerbose("Using namespace: %s", ns)

	// Fetch the Database CR
	var db dbopsv1alpha1.Database
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: ns,
		Name:      dbCRName,
	}, &db); err != nil {
		return fmt.Errorf("get Database CR %s/%s: %w", ns, dbCRName, err)
	}

	printVerbose("Found Database CR: %s (database name: %s)", db.Name, db.Spec.Name)

	// Determine ownership model
	if !service.HasAutoOwnership(&db.Spec) {
		if db.Spec.Owner != "" {
			fmt.Println("Database uses direct ownership (spec.owner) — reverse privileges are not applicable.")
			return nil
		}
		fmt.Println("No ownership model configured — nothing to migrate.")
		return nil
	}

	ownershipCfg := db.Spec.Postgres.Ownership

	// Check if default privileges are enabled
	if !ownershipCfg.ShouldSetDefaultPrivileges() {
		fmt.Println("Default privileges are explicitly disabled (setDefaultPrivileges=false) — skipping.")
		return nil
	}

	// Derive role and user names
	ownerRole := service.DeriveRoleName(ownershipCfg, db.Spec.Name)
	appUser := service.DeriveUserName(ownershipCfg, db.Spec.Name)

	printVerbose("Owner role: %s, App user: %s", ownerRole, appUser)

	// Resolve the instance
	resolver := instanceresolver.New(k8sClient)
	resolved, err := resolver.Resolve(ctx, db.Spec.InstanceRef, db.Spec.ClusterInstanceRef, ns)
	if err != nil {
		return fmt.Errorf("resolve instance: %w", err)
	}

	// Validate engine support
	engine := resolved.Engine()
	if !service.IsOwnershipSupported(engine) {
		fmt.Printf("Engine %q does not support ALTER DEFAULT PRIVILEGES — skipping.\n", engine)
		return nil
	}

	printVerbose("Engine: %s, Instance: %s", engine, resolved.Name)

	// Collect schemas: public + any configured schemas
	schemaSet := map[string]bool{"public": true}
	if db.Spec.Postgres != nil {
		for _, s := range db.Spec.Postgres.Schemas {
			schemaSet[s.Name] = true
		}
	}

	schemas := make([]string, 0, len(schemaSet))
	for s := range schemaSet {
		schemas = append(schemas, s)
	}

	printVerbose("Schemas: %s", strings.Join(schemas, ", "))

	privDefs := defaultPrivilegeDefinitions()

	// Dry-run mode: run integrity checks in dry-run, print SQL, and exit
	if dryRun {
		// Get credentials and connect even in dry-run for integrity checks
		dbAdapter, closeAdapter, connErr := connectToDatabase(ctx, k8sClient, resolved, db.Spec.Name)
		if connErr != nil {
			return connErr
		}
		defer closeAdapter()

		// Run integrity checks in dry-run mode
		result, err := runIntegrityChecks(ctx, dbAdapter, db.Spec.Name, ownerRole, appUser, schemas, true)
		if err != nil {
			return fmt.Errorf("integrity checks: %w", err)
		}

		// Emit K8s event for detected drift
		if len(result.diffs) > 0 {
			scheme := buildScheme()
			recorder, shutdownRecorder := buildEventRecorder(restConfig, scheme)
			defer shutdownRecorder()

			fieldNames := driftFieldNames(result.diffs)
			recorder.Eventf(&db, corev1.EventTypeWarning, "OwnershipDriftDetected",
				"Migration detected ownership drift in fields: %s (dry-run, not corrected)", strings.Join(fieldNames, ", "))

			// Update status.drift to reflect detected drift
			db.Status.Drift = &dbopsv1alpha1.DriftStatus{
				Detected:    true,
				LastChecked: &metav1.Time{Time: time.Now()},
				Diffs:       result.diffs,
			}
			if statusErr := k8sClient.Status().Update(ctx, &db); statusErr != nil {
				printVerbose("Warning: failed to update status.drift: %v", statusErr)
			}
		}

		fmt.Println()
		fmt.Println("Forward default privileges (dry-run):")
		for _, schema := range schemas {
			for _, dp := range privDefs {
				fmt.Printf("  ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA %s GRANT %s ON %s TO %s;\n",
					ownerRole, schema, strings.Join(dp.privileges, ", "),
					strings.ToUpper(dp.objectType), appUser)
			}
		}

		fmt.Println()
		fmt.Println("Reverse default privileges (dry-run):")
		for _, schema := range schemas {
			for _, dp := range privDefs {
				fmt.Printf("  ALTER DEFAULT PRIVILEGES FOR ROLE %s IN SCHEMA %s GRANT %s ON %s TO %s;\n",
					appUser, schema, strings.Join(dp.privileges, ", "),
					strings.ToUpper(dp.objectType), ownerRole)
			}
		}
		fmt.Printf("\nTotal: %d forward + %d reverse default privilege grants across %d schema(s) for database %q\n",
			len(privDefs)*len(schemas), len(privDefs)*len(schemas), len(schemas), db.Spec.Name)
		return nil
	}

	// Get credentials and connect
	dbAdapter, closeAdapter, connErr := connectToDatabase(ctx, k8sClient, resolved, db.Spec.Name)
	if connErr != nil {
		return connErr
	}
	defer closeAdapter()

	// Build event recorder
	scheme := buildScheme()
	recorder, shutdownRecorder := buildEventRecorder(restConfig, scheme)
	defer shutdownRecorder()

	// Run integrity checks with auto-fix
	result, err := runIntegrityChecks(ctx, dbAdapter, db.Spec.Name, ownerRole, appUser, schemas, false)
	if err != nil {
		return fmt.Errorf("integrity checks: %w", err)
	}

	// Emit K8s event and update status.drift
	if len(result.diffs) > 0 {
		fieldNames := driftFieldNames(result.diffs)
		recorder.Eventf(&db, corev1.EventTypeNormal, "OwnershipDriftCorrected",
			"Migration corrected ownership drift in fields: %s", strings.Join(fieldNames, ", "))

		// After fix: set detected=false since we just corrected everything
		db.Status.Drift = &dbopsv1alpha1.DriftStatus{
			Detected:    false,
			LastChecked: &metav1.Time{Time: time.Now()},
		}
		if statusErr := k8sClient.Status().Update(ctx, &db); statusErr != nil {
			printVerbose("Warning: failed to update status.drift: %v", statusErr)
		}
	}

	// Apply forward default privileges (idempotent)
	forwardCount := 0
	for _, schema := range schemas {
		for _, dp := range privDefs {
			opts := []adaptertypes.DefaultPrivilegeGrantOptions{
				{
					Database:   db.Spec.Name,
					Schema:     schema,
					GrantedBy:  ownerRole,
					ObjectType: dp.objectType,
					Privileges: dp.privileges,
				},
			}
			if err := dbAdapter.SetDefaultPrivileges(ctx, appUser, opts); err != nil {
				return fmt.Errorf("set forward default privileges for %s in schema %s: %w",
					dp.objectType, schema, err)
			}
			forwardCount++
			printVerbose("Applied forward default privileges for %s in schema %s", dp.objectType, schema)
		}
	}

	// Apply reverse default privileges
	reverseCount := 0
	for _, schema := range schemas {
		for _, dp := range privDefs {
			opts := []adaptertypes.DefaultPrivilegeGrantOptions{
				{
					Database:   db.Spec.Name,
					Schema:     schema,
					GrantedBy:  appUser,
					ObjectType: dp.objectType,
					Privileges: dp.privileges,
				},
			}
			if err := dbAdapter.SetDefaultPrivileges(ctx, ownerRole, opts); err != nil {
				return fmt.Errorf("set reverse default privileges for %s in schema %s: %w",
					dp.objectType, schema, err)
			}
			reverseCount++
			printVerbose("Applied reverse default privileges for %s in schema %s", dp.objectType, schema)
		}
	}

	fmt.Printf("\nForward default privileges:\n  Applied %d forward grants across %d schema(s)\n",
		forwardCount, len(schemas))
	fmt.Printf("\nReverse default privileges:\n  Applied %d reverse grants across %d schema(s)\n",
		reverseCount, len(schemas))
	return nil
}

// runIntegrityChecks performs 5 ownership integrity check groups and auto-fixes issues
// unless isDryRun is true. Returns diffs for any issues found.
func runIntegrityChecks(ctx context.Context, checker ownershipIntegrityChecker,
	dbName, ownerRole, appUser string, schemas []string, isDryRun bool,
) (*integrityCheckResult, error) {
	result := &integrityCheckResult{}

	fmt.Println("Integrity checks:")

	// Check 1: Owner role exists
	result.checked++
	ownerExists, err := checker.RoleExists(ctx, ownerRole)
	if err != nil {
		return nil, fmt.Errorf("check owner role existence: %w", err)
	}
	if !ownerExists {
		result.diffs = append(result.diffs, dbopsv1alpha1.DriftDiff{
			Field:    "ownership.role",
			Expected: ownerRole,
			Actual:   "<missing>",
		})
		if isDryRun {
			fmt.Printf("  [CHECK] Owner role %q .............. MISSING (would create with NOLOGIN INHERIT)\n", ownerRole)
		} else {
			if err := checker.CreateRole(ctx, adaptertypes.CreateRoleOptions{
				RoleName: ownerRole,
				Login:    false,
				Inherit:  true,
			}); err != nil {
				return nil, fmt.Errorf("create owner role %s: %w", ownerRole, err)
			}
			result.fixed++
			fmt.Printf("  [CHECK] Owner role %q .............. MISSING → created (NOLOGIN INHERIT)\n", ownerRole)
		}
	} else {
		fmt.Printf("  [CHECK] Owner role %q .............. OK\n", ownerRole)
	}

	// Check 2: App user exists
	result.checked++
	appExists, err := checker.RoleExists(ctx, appUser)
	if err != nil {
		return nil, fmt.Errorf("check app user existence: %w", err)
	}
	if !appExists {
		result.diffs = append(result.diffs, dbopsv1alpha1.DriftDiff{
			Field:    "ownership.user",
			Expected: appUser,
			Actual:   "<missing>",
		})
		if isDryRun {
			fmt.Printf("  [CHECK] App user %q .............. MISSING (would create with LOGIN INHERIT)\n", appUser)
		} else {
			if err := checker.CreateRole(ctx, adaptertypes.CreateRoleOptions{
				RoleName: appUser,
				Login:    true,
				Inherit:  true,
			}); err != nil {
				return nil, fmt.Errorf("create app user %s: %w", appUser, err)
			}
			result.fixed++
			fmt.Printf("  [CHECK] App user %q .............. MISSING → created (LOGIN INHERIT)\n", appUser)
		}
	} else {
		fmt.Printf("  [CHECK] App user %q .............. OK\n", appUser)
	}

	// Check 3: Role membership (app user inherits from owner role)
	result.checked++
	roleInfo, err := checker.GetRoleInfo(ctx, appUser)
	if err != nil {
		// If the app user was just created, GetRoleInfo should work;
		// if it fails, we can't verify membership
		return nil, fmt.Errorf("get role info for %s: %w", appUser, err)
	}
	hasMembership := false
	for _, r := range roleInfo.InRoles {
		if r == ownerRole {
			hasMembership = true
			break
		}
	}
	if !hasMembership {
		result.diffs = append(result.diffs, dbopsv1alpha1.DriftDiff{
			Field:    "ownership.membership",
			Expected: fmt.Sprintf("%s IN ROLE %s", appUser, ownerRole),
			Actual:   fmt.Sprintf("%s not in %s", appUser, ownerRole),
		})
		if isDryRun {
			fmt.Printf("  [CHECK] Role membership app→owner ................ MISSING (would grant)\n")
		} else {
			if err := checker.GrantRole(ctx, appUser, []string{ownerRole}); err != nil {
				return nil, fmt.Errorf("grant role %s to %s: %w", ownerRole, appUser, err)
			}
			result.fixed++
			fmt.Printf("  [CHECK] Role membership app→owner ................ MISSING → granted\n")
		}
	} else {
		fmt.Println("  [CHECK] Role membership app→owner ................ OK")
	}

	// Check 4: Database owner
	result.checked++
	dbInfo, err := checker.GetDatabaseInfo(ctx, dbName)
	if err != nil {
		return nil, fmt.Errorf("get database info for %s: %w", dbName, err)
	}
	if dbInfo.Owner != ownerRole {
		result.diffs = append(result.diffs, dbopsv1alpha1.DriftDiff{
			Field:       "ownership.dbOwner",
			Expected:    ownerRole,
			Actual:      dbInfo.Owner,
			Destructive: true,
		})
		if isDryRun {
			fmt.Printf("  [CHECK] Database %q owner ................... DRIFT (current: %q, expected: %q) — would transfer\n",
				dbName, dbInfo.Owner, ownerRole)
		} else {
			if err := checker.TransferDatabaseOwnership(ctx, dbName, ownerRole); err != nil {
				return nil, fmt.Errorf("transfer database ownership: %w", err)
			}
			result.fixed++
			fmt.Printf("  [CHECK] Database %q owner ................... DRIFT (current: %q, expected: %q) → transferred\n",
				dbName, dbInfo.Owner, ownerRole)
		}
	} else {
		fmt.Printf("  [CHECK] Database %q owner ................... OK\n", dbName)
	}

	// Check 5: Forward default privileges (idempotent, no diff emitted)
	result.checked++
	if !isDryRun {
		privDefs := defaultPrivilegeDefinitions()
		forwardCount := 0
		for _, schema := range schemas {
			for _, dp := range privDefs {
				opts := []adaptertypes.DefaultPrivilegeGrantOptions{
					{
						Database:   dbName,
						Schema:     schema,
						GrantedBy:  ownerRole,
						ObjectType: dp.objectType,
						Privileges: dp.privileges,
					},
				}
				if err := checker.SetDefaultPrivileges(ctx, appUser, opts); err != nil {
					return nil, fmt.Errorf("set forward default privileges for %s in schema %s: %w",
						dp.objectType, schema, err)
				}
				forwardCount++
			}
		}
		fmt.Printf("\n  Forward default privileges: applied %d grants across %d schema(s)\n", forwardCount, len(schemas))
	} else {
		privDefs := defaultPrivilegeDefinitions()
		forwardCount := len(privDefs) * len(schemas)
		fmt.Printf("\n  Forward default privileges: would apply %d grants across %d schema(s)\n", forwardCount, len(schemas))
	}

	if len(result.diffs) > 0 {
		if isDryRun {
			fmt.Printf("\nOwnership drift: %d issue(s) detected (dry-run, not corrected)\n", len(result.diffs))
		} else {
			fmt.Printf("\nOwnership drift: %d issue(s) fixed\n", result.fixed)
		}
	} else {
		fmt.Println("\nOwnership drift: no issues found")
	}

	return result, nil
}

// connectToDatabase resolves credentials and connects to the target database.
func connectToDatabase(ctx context.Context, k8sClient client.Client,
	resolved *instanceresolver.ResolvedInstance, dbName string,
) (adaptertypes.DatabaseAdapter, func(), error) {
	secretMgr := secret.NewManager(k8sClient)
	creds, err := secretMgr.GetCredentials(ctx, resolved.CredentialNamespace, resolved.Spec.Connection.SecretRef)
	if err != nil {
		return nil, nil, fmt.Errorf("get credentials: %w", err)
	}

	var tlsCA, tlsCert, tlsKey []byte
	if resolved.Spec.TLS != nil && resolved.Spec.TLS.Enabled {
		tlsCreds, tlsErr := secretMgr.GetTLSCredentials(ctx, resolved.CredentialNamespace, resolved.Spec.TLS)
		if tlsErr != nil {
			printVerbose("Warning: failed to get TLS credentials: %v", tlsErr)
		} else {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	connConfig := adapter.BuildConnectionConfig(resolved.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	connConfig.Database = dbName

	dbAdapter, err := adapter.NewAdapter(dbopsv1alpha1.EngineType(resolved.Engine()), connConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create adapter: %w", err)
	}

	if err := dbAdapter.Connect(ctx); err != nil {
		_ = dbAdapter.Close()
		return nil, nil, fmt.Errorf("connect to database %s: %w", dbName, err)
	}

	return dbAdapter, func() { _ = dbAdapter.Close() }, nil
}

// driftFieldNames extracts field names from a slice of DriftDiff.
func driftFieldNames(diffs []dbopsv1alpha1.DriftDiff) []string {
	names := make([]string, len(diffs))
	for i, d := range diffs {
		names[i] = d.Field
	}
	return names
}

// buildScheme creates a runtime scheme with operator CRDs registered.
func buildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dbopsv1alpha1.AddToScheme(scheme))
	return scheme
}

// buildEventRecorder creates a K8s event recorder for emitting events on CRs.
// Returns the recorder and a shutdown function that must be called when done.
func buildEventRecorder(restConfig *rest.Config, scheme *runtime.Scheme) (record.EventRecorder, func()) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		// If we can't create a clientset, return a no-op recorder
		printVerbose("Warning: failed to create clientset for event recording: %v", err)
		return record.NewFakeRecorder(100), func() {}
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: clientset.CoreV1().Events(""),
	})
	recorder := broadcaster.NewRecorder(scheme, corev1.EventSource{Component: "dbctl-migrate"})
	return recorder, func() { broadcaster.Shutdown() }
}

// buildK8sClient creates a Kubernetes client using kubeconfig and returns the client,
// the REST config (for building event recorders), and the default namespace.
func buildK8sClient() (client.Client, *rest.Config, string, error) {
	// Build kubeconfig loading rules
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if migrateKubeconfig != "" {
		loadingRules.ExplicitPath = migrateKubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	// Get the namespace from the current context
	namespace, _, err := kubeConfig.Namespace()
	if err != nil {
		namespace = "default"
	}

	// Get REST config
	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, nil, "", fmt.Errorf("get kubeconfig: %w", err)
	}

	// Build scheme with operator CRDs
	scheme := buildScheme()

	// Create client
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, "", fmt.Errorf("create kubernetes client: %w", err)
	}

	return k8sClient, restConfig, namespace, nil
}
