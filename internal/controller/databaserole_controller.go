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

package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	adaptypes "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseRoleReconciler reconciles a DatabaseRole object
type DatabaseRoleReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile reconciles a DatabaseRole object
func (r *DatabaseRoleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseRole
	role := &dbopsv1alpha1.DatabaseRole{}
	if err := r.Get(ctx, req.NamespacedName, role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(role) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(role) {
		return r.handleDeletion(ctx, role)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(role, util.FinalizerDatabaseRole) {
		controllerutil.AddFinalizer(role, util.FinalizerDatabaseRole)
		if err := r.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the role
	result, err := r.reconcileRole(ctx, role)
	if err != nil {
		log.Error(err, "Failed to reconcile role")
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = err.Error()
		util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
			util.ReasonReconcileFailed, err.Error())
		if statusErr := r.Status().Update(ctx, role); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return result, err
	}

	return result, nil
}

// reconcileRole handles the main reconciliation logic
func (r *DatabaseRoleReconciler) reconcileRole(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the DatabaseInstance
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceRef := role.Spec.InstanceRef
	instanceNamespace := role.Namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      instanceRef.Name,
	}, instance); err != nil {
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = fmt.Sprintf("DatabaseInstance not found: %v", err)
		util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
			util.ReasonInstanceNotFound, err.Error())
		if statusErr := r.Status().Update(ctx, role); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		// Don't return error for not found - status is updated, requeue to check if instance is created later
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if instance is ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		role.Status.Phase = dbopsv1alpha1.PhasePending
		role.Status.Message = "Waiting for DatabaseInstance to be ready"
		util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
			util.ReasonInstanceNotReady, "DatabaseInstance is not ready")
		if statusErr := r.Status().Update(ctx, role); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get admin credentials with retry
	var adminCreds *secret.Credentials
	retryConfig := util.ConnectionRetryConfig()
	retryResult := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		adminCreds, err = r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
		return err
	})
	if retryResult.LastError != nil {
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = fmt.Sprintf("Failed to get admin credentials: %v", retryResult.LastError)
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, retryResult)}, retryResult.LastError
	}

	// Get TLS credentials if enabled
	var tlsCreds *secret.TLSCredentials
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, _ = r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
	}

	// Build connection config
	var tlsCA, tlsCert, tlsKey []byte
	if tlsCreds != nil {
		tlsCA = tlsCreds.CA
		tlsCert = tlsCreds.Cert
		tlsKey = tlsCreds.Key
	}
	config := adapter.BuildConnectionConfig(&instance.Spec, adminCreds.Username, adminCreds.Password, tlsCA, tlsCert, tlsKey)

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, config)
	if err != nil {
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = fmt.Sprintf("Failed to create adapter: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	defer dbAdapter.Close()

	// Connect to database with retry
	retryResult = util.RetryWithBackoff(ctx, retryConfig, func() error {
		return dbAdapter.Connect(ctx)
	})
	if retryResult.LastError != nil {
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = fmt.Sprintf("Failed to connect: %v", retryResult.LastError)
		log.Error(retryResult.LastError, "Failed to connect after retries",
			"attempts", retryResult.Attempts,
			"duration", retryResult.TotalTime)
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, retryResult)}, retryResult.LastError
	}

	roleName := role.Spec.RoleName
	engine := string(instance.Spec.Engine)

	// Check if role exists with retry
	var exists bool
	defaultRetryConfig := util.DefaultRetryConfig()
	retryResult = util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
		var err error
		exists, err = dbAdapter.RoleExists(ctx, roleName)
		return err
	})
	if retryResult.LastError != nil {
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = fmt.Sprintf("Failed to check role existence: %v", retryResult.LastError)
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, retryResult)}, retryResult.LastError
	}

	if !exists {
		// Create the role
		log.Info("Creating role", "roleName", roleName)
		role.Status.Phase = dbopsv1alpha1.PhaseCreating
		role.Status.Message = "Creating role"
		if statusErr := r.Status().Update(ctx, role); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}

		opts := r.buildCreateRoleOptions(role, instance.Spec.Engine)
		retryResult = util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
			return dbAdapter.CreateRole(ctx, opts)
		})
		if retryResult.LastError != nil {
			metrics.RecordRoleOperation(metrics.OperationCreate, engine, role.Namespace, metrics.StatusFailure)
			role.Status.Phase = dbopsv1alpha1.PhaseFailed
			role.Status.Message = fmt.Sprintf("Failed to create role: %v", retryResult.LastError)
			util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
				util.ReasonCreateFailed, retryResult.LastError.Error())
			if statusErr := r.Status().Update(ctx, role); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, retryResult)}, retryResult.LastError
		}
		metrics.RecordRoleOperation(metrics.OperationCreate, engine, role.Namespace, metrics.StatusSuccess)

		log.Info("Successfully created role", "roleName", roleName)
	} else {
		// Update role if needed
		updateOpts := r.buildUpdateRoleOptions(role, instance.Spec.Engine)
		retryResult = util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
			return dbAdapter.UpdateRole(ctx, roleName, updateOpts)
		})
		if retryResult.LastError != nil {
			metrics.RecordRoleOperation(metrics.OperationUpdate, engine, role.Namespace, metrics.StatusFailure)
			log.Error(retryResult.LastError, "Failed to update role", "roleName", roleName)
		} else {
			metrics.RecordRoleOperation(metrics.OperationUpdate, engine, role.Namespace, metrics.StatusSuccess)
		}
	}

	// Update status
	role.Status.Phase = dbopsv1alpha1.PhaseReady
	role.Status.Message = "Role is ready"
	role.Status.ObservedGeneration = role.Generation
	role.Status.Role = &dbopsv1alpha1.RoleInfo{
		Name:      roleName,
		CreatedAt: &metav1.Time{Time: time.Now()},
	}

	// Set conditions
	util.SetSyncedCondition(&role.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Role is synced")
	util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Role is ready")

	if err := r.Status().Update(ctx, role); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled DatabaseRole", "name", role.Name, "roleName", roleName)

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// buildCreateRoleOptions builds options for creating a role
func (r *DatabaseRoleReconciler) buildCreateRoleOptions(role *dbopsv1alpha1.DatabaseRole, engine dbopsv1alpha1.EngineType) adaptypes.CreateRoleOptions {
	opts := adaptypes.CreateRoleOptions{
		RoleName: role.Spec.RoleName,
		Inherit:  true,
	}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if role.Spec.Postgres != nil {
			opts.Login = role.Spec.Postgres.Login
			opts.Inherit = role.Spec.Postgres.Inherit
			opts.CreateDB = role.Spec.Postgres.CreateDB
			opts.CreateRole = role.Spec.Postgres.CreateRole
			opts.Superuser = role.Spec.Postgres.Superuser
			opts.Replication = role.Spec.Postgres.Replication
			opts.BypassRLS = role.Spec.Postgres.BypassRLS
			opts.InRoles = role.Spec.Postgres.InRoles

			// Convert grants
			if len(role.Spec.Postgres.Grants) > 0 {
				opts.Grants = make([]adaptypes.GrantOptions, 0, len(role.Spec.Postgres.Grants))
				for _, g := range role.Spec.Postgres.Grants {
					opts.Grants = append(opts.Grants, adaptypes.GrantOptions{
						Database:        g.Database,
						Schema:          g.Schema,
						Tables:          g.Tables,
						Sequences:       g.Sequences,
						Functions:       g.Functions,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if role.Spec.MySQL != nil {
			opts.UseNativeRoles = role.Spec.MySQL.UseNativeRoles

			// Convert grants
			if len(role.Spec.MySQL.Grants) > 0 {
				opts.Grants = make([]adaptypes.GrantOptions, 0, len(role.Spec.MySQL.Grants))
				for _, g := range role.Spec.MySQL.Grants {
					opts.Grants = append(opts.Grants, adaptypes.GrantOptions{
						Level:           string(g.Level),
						Database:        g.Database,
						Table:           g.Table,
						Columns:         g.Columns,
						Procedure:       g.Procedure,
						Function:        g.Function,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	}

	return opts
}

// buildUpdateRoleOptions builds options for updating a role
func (r *DatabaseRoleReconciler) buildUpdateRoleOptions(role *dbopsv1alpha1.DatabaseRole, engine dbopsv1alpha1.EngineType) adaptypes.UpdateRoleOptions {
	opts := adaptypes.UpdateRoleOptions{}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if role.Spec.Postgres != nil {
			login := role.Spec.Postgres.Login
			opts.Login = &login
			inherit := role.Spec.Postgres.Inherit
			opts.Inherit = &inherit
			createDB := role.Spec.Postgres.CreateDB
			opts.CreateDB = &createDB
			createRole := role.Spec.Postgres.CreateRole
			opts.CreateRole = &createRole
			superuser := role.Spec.Postgres.Superuser
			opts.Superuser = &superuser
			replication := role.Spec.Postgres.Replication
			opts.Replication = &replication
			bypassRLS := role.Spec.Postgres.BypassRLS
			opts.BypassRLS = &bypassRLS
			opts.InRoles = role.Spec.Postgres.InRoles

			// Convert grants
			if len(role.Spec.Postgres.Grants) > 0 {
				opts.Grants = make([]adaptypes.GrantOptions, 0, len(role.Spec.Postgres.Grants))
				for _, g := range role.Spec.Postgres.Grants {
					opts.Grants = append(opts.Grants, adaptypes.GrantOptions{
						Database:        g.Database,
						Schema:          g.Schema,
						Tables:          g.Tables,
						Sequences:       g.Sequences,
						Functions:       g.Functions,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if role.Spec.MySQL != nil {
			// Convert grants for add
			if len(role.Spec.MySQL.Grants) > 0 {
				opts.AddGrants = make([]adaptypes.GrantOptions, 0, len(role.Spec.MySQL.Grants))
				for _, g := range role.Spec.MySQL.Grants {
					opts.AddGrants = append(opts.AddGrants, adaptypes.GrantOptions{
						Level:           string(g.Level),
						Database:        g.Database,
						Table:           g.Table,
						Columns:         g.Columns,
						Procedure:       g.Procedure,
						Function:        g.Function,
						Privileges:      g.Privileges,
						WithGrantOption: g.WithGrantOption,
					})
				}
			}
		}
	}

	return opts
}

// handleDeletion handles the deletion of a DatabaseRole
func (r *DatabaseRoleReconciler) handleDeletion(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(role, util.FinalizerDatabaseRole) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseRole", "name", role.Name)

	// Drop the role from the database
	{
		// Get the DatabaseInstance
		instance := &dbopsv1alpha1.DatabaseInstance{}
		instanceRef := role.Spec.InstanceRef
		instanceNamespace := role.Namespace
		if instanceRef.Namespace != "" {
			instanceNamespace = instanceRef.Namespace
		}

		if err := r.Get(ctx, types.NamespacedName{
			Namespace: instanceNamespace,
			Name:      instanceRef.Name,
		}, instance); err == nil {
			// Get credentials and connect
			adminCreds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
			if err == nil {
				var tlsCreds *secret.TLSCredentials
				if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
					tlsCreds, _ = r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
				}

				var tlsCA, tlsCert, tlsKey []byte
				if tlsCreds != nil {
					tlsCA = tlsCreds.CA
					tlsCert = tlsCreds.Cert
					tlsKey = tlsCreds.Key
				}
				config := adapter.BuildConnectionConfig(&instance.Spec, adminCreds.Username, adminCreds.Password, tlsCA, tlsCert, tlsKey)

				dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, config)
				if err == nil {
					defer dbAdapter.Close()
					if err := dbAdapter.Connect(ctx); err == nil {
						log.Info("Dropping role", "roleName", role.Spec.RoleName)
						engine := string(instance.Spec.Engine)
						if err := dbAdapter.DropRole(ctx, role.Spec.RoleName); err != nil {
							log.Error(err, "Failed to drop role", "roleName", role.Spec.RoleName)
							metrics.RecordRoleOperation(metrics.OperationDelete, engine, role.Namespace, metrics.StatusFailure)
						} else {
							log.Info("Successfully dropped role", "roleName", role.Spec.RoleName)
							metrics.RecordRoleOperation(metrics.OperationDelete, engine, role.Namespace, metrics.StatusSuccess)
						}
					}
				}
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(role, util.FinalizerDatabaseRole)
	if err := r.Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted DatabaseRole", "name", role.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseRoleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.SecretManager == nil {
		r.SecretManager = secret.NewManager(r.Client)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseRole{}).
		Named("databaserole").
		Complete(r)
}
