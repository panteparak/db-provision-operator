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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	adapterpkg "github.com/db-provision-operator/internal/adapter/types"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseGrantReconciler reconciles a DatabaseGrant object
type DatabaseGrantReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseGrant resource
	var grant dbopsv1alpha1.DatabaseGrant
	if err := r.Get(ctx, req.NamespacedName, &grant); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseGrant")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(&grant) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(&grant) {
		if controllerutil.ContainsFinalizer(&grant, util.FinalizerDatabaseGrant) {
			if err := r.handleDeletion(ctx, &grant); err != nil {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, err
			}
			controllerutil.RemoveFinalizer(&grant, util.FinalizerDatabaseGrant)
			if err := r.Update(ctx, &grant); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&grant, util.FinalizerDatabaseGrant) {
		controllerutil.AddFinalizer(&grant, util.FinalizerDatabaseGrant)
		if err := r.Update(ctx, &grant); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set initial phase
	if grant.Status.Phase == "" {
		grant.Status.Phase = dbopsv1alpha1.PhasePending
		grant.Status.Message = "Initializing grant"
		if err := r.Status().Update(ctx, &grant); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch the referenced DatabaseUser
	userNamespace := grant.Namespace
	if grant.Spec.UserRef.Namespace != "" {
		userNamespace = grant.Spec.UserRef.Namespace
	}

	var dbUser dbopsv1alpha1.DatabaseUser
	if err := r.Get(ctx, types.NamespacedName{
		Name:      grant.Spec.UserRef.Name,
		Namespace: userNamespace,
	}, &dbUser); err != nil {
		if errors.IsNotFound(err) {
			grant.Status.Phase = dbopsv1alpha1.PhaseFailed
			grant.Status.Message = fmt.Sprintf("Referenced DatabaseUser %s/%s not found", userNamespace, grant.Spec.UserRef.Name)
			util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "UserNotFound", grant.Status.Message)
			if statusErr := r.Status().Update(ctx, &grant); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		log.Error(err, "Failed to fetch DatabaseUser")
		return ctrl.Result{}, err
	}

	// Check if user is ready
	if dbUser.Status.Phase != dbopsv1alpha1.PhaseReady {
		grant.Status.Phase = dbopsv1alpha1.PhasePending
		grant.Status.Message = fmt.Sprintf("Waiting for DatabaseUser %s to be ready", dbUser.Name)
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "UserNotReady", grant.Status.Message)
		if err := r.Status().Update(ctx, &grant); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get the username from the DatabaseUser
	username := dbUser.Spec.Username

	// Fetch the DatabaseInstance from the user's instanceRef
	instanceNamespace := dbUser.Namespace
	if dbUser.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = dbUser.Spec.InstanceRef.Namespace
	}

	var instance dbopsv1alpha1.DatabaseInstance
	if err := r.Get(ctx, types.NamespacedName{
		Name:      dbUser.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}, &instance); err != nil {
		if errors.IsNotFound(err) {
			grant.Status.Phase = dbopsv1alpha1.PhaseFailed
			grant.Status.Message = fmt.Sprintf("DatabaseInstance %s/%s not found", instanceNamespace, dbUser.Spec.InstanceRef.Name)
			util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "InstanceNotFound", grant.Status.Message)
			if statusErr := r.Status().Update(ctx, &grant); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		log.Error(err, "Failed to fetch DatabaseInstance")
		return ctrl.Result{}, err
	}

	// Check if instance is ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		grant.Status.Phase = dbopsv1alpha1.PhasePending
		grant.Status.Message = fmt.Sprintf("Waiting for DatabaseInstance %s to be ready", instance.Name)
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "InstanceNotReady", grant.Status.Message)
		if err := r.Status().Update(ctx, &grant); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get credentials from the instance's secret with retry
	var creds *secret.Credentials
	retryConfig := util.ConnectionRetryConfig()
	result := util.RetryWithBackoff(ctx, retryConfig, func() error {
		var err error
		creds, err = r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
		return err
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Failed to get credentials after retries",
			"attempts", result.Attempts,
			"duration", result.TotalTime)
		grant.Status.Phase = dbopsv1alpha1.PhaseFailed
		grant.Status.Message = fmt.Sprintf("Failed to get credentials: %v", result.LastError)
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "CredentialsFailed", grant.Status.Message)
		if err := r.Status().Update(ctx, &grant); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
	}

	// Get TLS credentials if TLS is enabled
	var tlsCreds *secret.TLSCredentials
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		result := util.RetryWithBackoff(ctx, retryConfig, func() error {
			var err error
			tlsCreds, err = r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
			return err
		})
		if result.LastError != nil {
			log.Error(result.LastError, "Failed to get TLS credentials")
			grant.Status.Phase = dbopsv1alpha1.PhaseFailed
			grant.Status.Message = fmt.Sprintf("Failed to get TLS credentials: %v", result.LastError)
			util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "TLSCredentialsFailed", grant.Status.Message)
			if err := r.Status().Update(ctx, &grant); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
		}
	}

	// Build connection config
	var tlsCA, tlsCert, tlsKey []byte
	if tlsCreds != nil {
		tlsCA = tlsCreds.CA
		tlsCert = tlsCreds.Cert
		tlsKey = tlsCreds.Key
	}
	connConfig := adapter.BuildConnectionConfig(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create database adapter and connect with retry
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, connConfig)
	if err != nil {
		grant.Status.Phase = dbopsv1alpha1.PhaseFailed
		grant.Status.Message = fmt.Sprintf("Unsupported database engine: %s", instance.Spec.Engine)
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "UnsupportedEngine", grant.Status.Message)
		if statusErr := r.Status().Update(ctx, &grant); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}
	defer func() { _ = dbAdapter.Close() }()

	result = util.RetryWithBackoff(ctx, retryConfig, func() error {
		return dbAdapter.Connect(ctx)
	})
	if result.LastError != nil {
		log.Error(result.LastError, "Failed to connect to database after retries",
			"attempts", result.Attempts,
			"duration", result.TotalTime)
		grant.Status.Phase = dbopsv1alpha1.PhaseFailed
		grant.Status.Message = fmt.Sprintf("Failed to connect to database: %v", result.LastError)
		util.SetConnectedCondition(&grant.Status.Conditions, metav1.ConditionFalse, "ConnectionFailed", grant.Status.Message)
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "ConnectionFailed", grant.Status.Message)
		if err := r.Status().Update(ctx, &grant); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(retryConfig, result)}, nil
	}

	util.SetConnectedCondition(&grant.Status.Conditions, metav1.ConditionTrue, "Connected", "Successfully connected to database")

	// Update status to Creating
	grant.Status.Phase = dbopsv1alpha1.PhaseCreating
	grant.Status.Message = "Applying grants"
	if err := r.Status().Update(ctx, &grant); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Initialize applied grants info
	appliedGrants := &dbopsv1alpha1.AppliedGrantsInfo{
		Roles:             []string{},
		DirectGrants:      0,
		DefaultPrivileges: 0,
	}

	// Apply grants based on engine type
	defaultRetryConfig := util.DefaultRetryConfig()
	engine := string(instance.Spec.Engine)

	switch instance.Spec.Engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if grant.Spec.Postgres != nil {
			// Apply role assignments
			if len(grant.Spec.Postgres.Roles) > 0 {
				result := util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
					return dbAdapter.GrantRole(ctx, username, grant.Spec.Postgres.Roles)
				})
				if result.LastError != nil {
					log.Error(result.LastError, "Failed to grant roles",
						"attempts", result.Attempts,
						"roles", grant.Spec.Postgres.Roles)
					metrics.RecordGrantOperation(metrics.OperationCreate, engine, grant.Namespace, metrics.StatusFailure)
					grant.Status.Phase = dbopsv1alpha1.PhaseFailed
					grant.Status.Message = fmt.Sprintf("Failed to grant roles: %v", result.LastError)
					util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionFalse, "GrantRolesFailed", grant.Status.Message)
					util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "GrantRolesFailed", grant.Status.Message)
					if err := r.Status().Update(ctx, &grant); err != nil {
						log.Error(err, "Failed to update status")
					}
					return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, result)}, nil
				}
				appliedGrants.Roles = append(appliedGrants.Roles, grant.Spec.Postgres.Roles...)
				log.Info("Granted roles", "username", username, "roles", grant.Spec.Postgres.Roles)
			}

			// Apply direct grants
			if len(grant.Spec.Postgres.Grants) > 0 {
				grantOpts := r.buildPostgresGrantOptions(grant.Spec.Postgres.Grants)
				result := util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
					return dbAdapter.Grant(ctx, username, grantOpts)
				})
				if result.LastError != nil {
					log.Error(result.LastError, "Failed to apply direct grants",
						"attempts", result.Attempts)
					metrics.RecordGrantOperation(metrics.OperationCreate, engine, grant.Namespace, metrics.StatusFailure)
					grant.Status.Phase = dbopsv1alpha1.PhaseFailed
					grant.Status.Message = fmt.Sprintf("Failed to apply direct grants: %v", result.LastError)
					util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionFalse, "DirectGrantsFailed", grant.Status.Message)
					util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "DirectGrantsFailed", grant.Status.Message)
					if err := r.Status().Update(ctx, &grant); err != nil {
						log.Error(err, "Failed to update status")
					}
					return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, result)}, nil
				}
				appliedGrants.DirectGrants = int32(len(grant.Spec.Postgres.Grants))
				log.Info("Applied direct grants", "username", username, "count", len(grant.Spec.Postgres.Grants))
			}

			// Apply default privileges
			if len(grant.Spec.Postgres.DefaultPrivileges) > 0 {
				defPrivOpts := r.buildDefaultPrivilegeOptions(grant.Spec.Postgres.DefaultPrivileges)
				result := util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
					return dbAdapter.SetDefaultPrivileges(ctx, username, defPrivOpts)
				})
				if result.LastError != nil {
					log.Error(result.LastError, "Failed to set default privileges",
						"attempts", result.Attempts)
					metrics.RecordGrantOperation(metrics.OperationCreate, engine, grant.Namespace, metrics.StatusFailure)
					grant.Status.Phase = dbopsv1alpha1.PhaseFailed
					grant.Status.Message = fmt.Sprintf("Failed to set default privileges: %v", result.LastError)
					util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionFalse, "DefaultPrivilegesFailed", grant.Status.Message)
					util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "DefaultPrivilegesFailed", grant.Status.Message)
					if err := r.Status().Update(ctx, &grant); err != nil {
						log.Error(err, "Failed to update status")
					}
					return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, result)}, nil
				}
				appliedGrants.DefaultPrivileges = int32(len(grant.Spec.Postgres.DefaultPrivileges))
				log.Info("Set default privileges", "username", username, "count", len(grant.Spec.Postgres.DefaultPrivileges))
			}
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if grant.Spec.MySQL != nil {
			// Apply role assignments (MySQL 8.0+)
			if len(grant.Spec.MySQL.Roles) > 0 {
				result := util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
					return dbAdapter.GrantRole(ctx, username, grant.Spec.MySQL.Roles)
				})
				if result.LastError != nil {
					log.Error(result.LastError, "Failed to grant roles",
						"attempts", result.Attempts,
						"roles", grant.Spec.MySQL.Roles)
					metrics.RecordGrantOperation(metrics.OperationCreate, engine, grant.Namespace, metrics.StatusFailure)
					grant.Status.Phase = dbopsv1alpha1.PhaseFailed
					grant.Status.Message = fmt.Sprintf("Failed to grant roles: %v", result.LastError)
					util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionFalse, "GrantRolesFailed", grant.Status.Message)
					util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "GrantRolesFailed", grant.Status.Message)
					if err := r.Status().Update(ctx, &grant); err != nil {
						log.Error(err, "Failed to update status")
					}
					return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, result)}, nil
				}
				appliedGrants.Roles = append(appliedGrants.Roles, grant.Spec.MySQL.Roles...)
				log.Info("Granted roles", "username", username, "roles", grant.Spec.MySQL.Roles)
			}

			// Apply direct grants
			if len(grant.Spec.MySQL.Grants) > 0 {
				grantOpts := r.buildMySQLGrantOptions(grant.Spec.MySQL.Grants)
				result := util.RetryWithBackoff(ctx, defaultRetryConfig, func() error {
					return dbAdapter.Grant(ctx, username, grantOpts)
				})
				if result.LastError != nil {
					log.Error(result.LastError, "Failed to apply direct grants",
						"attempts", result.Attempts)
					metrics.RecordGrantOperation(metrics.OperationCreate, engine, grant.Namespace, metrics.StatusFailure)
					grant.Status.Phase = dbopsv1alpha1.PhaseFailed
					grant.Status.Message = fmt.Sprintf("Failed to apply direct grants: %v", result.LastError)
					util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionFalse, "DirectGrantsFailed", grant.Status.Message)
					util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse, "DirectGrantsFailed", grant.Status.Message)
					if err := r.Status().Update(ctx, &grant); err != nil {
						log.Error(err, "Failed to update status")
					}
					return ctrl.Result{RequeueAfter: util.CalculateRequeueAfter(defaultRetryConfig, result)}, nil
				}
				appliedGrants.DirectGrants = int32(len(grant.Spec.MySQL.Grants))
				log.Info("Applied direct grants", "username", username, "count", len(grant.Spec.MySQL.Grants))
			}
		}
	}

	// Record successful grant operation
	metrics.RecordGrantOperation(metrics.OperationCreate, engine, grant.Namespace, metrics.StatusSuccess)

	// Update status to Ready
	grant.Status.Phase = dbopsv1alpha1.PhaseReady
	grant.Status.Message = "Grants applied successfully"
	grant.Status.ObservedGeneration = grant.Generation
	grant.Status.AppliedGrants = appliedGrants
	util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionTrue, "Synced", "All grants applied successfully")
	util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionTrue, "Ready", "DatabaseGrant is ready")

	if err := r.Status().Update(ctx, &grant); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("DatabaseGrant reconciled successfully",
		"grant", grant.Name,
		"username", username,
		"roles", len(appliedGrants.Roles),
		"directGrants", appliedGrants.DirectGrants,
		"defaultPrivileges", appliedGrants.DefaultPrivileges)

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// handleDeletion handles the cleanup when a DatabaseGrant is being deleted
func (r *DatabaseGrantReconciler) handleDeletion(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant) error {
	log := logf.FromContext(ctx)

	// Update status to Deleting
	grant.Status.Phase = dbopsv1alpha1.PhaseDeleting
	grant.Status.Message = "Revoking grants"
	if err := r.Status().Update(ctx, grant); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Fetch the referenced DatabaseUser
	userNamespace := grant.Namespace
	if grant.Spec.UserRef.Namespace != "" {
		userNamespace = grant.Spec.UserRef.Namespace
	}

	var dbUser dbopsv1alpha1.DatabaseUser
	if err := r.Get(ctx, types.NamespacedName{
		Name:      grant.Spec.UserRef.Name,
		Namespace: userNamespace,
	}, &dbUser); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Referenced DatabaseUser not found, skipping revocation", "user", grant.Spec.UserRef.Name)
			return nil
		}
		return err
	}

	username := dbUser.Spec.Username

	// Fetch the DatabaseInstance from the user's instanceRef
	instanceNamespace := dbUser.Namespace
	if dbUser.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = dbUser.Spec.InstanceRef.Namespace
	}

	var instance dbopsv1alpha1.DatabaseInstance
	if err := r.Get(ctx, types.NamespacedName{
		Name:      dbUser.Spec.InstanceRef.Name,
		Namespace: instanceNamespace,
	}, &instance); err != nil {
		if errors.IsNotFound(err) {
			log.Info("DatabaseInstance not found, skipping revocation", "instance", dbUser.Spec.InstanceRef.Name)
			return nil
		}
		return err
	}

	// Skip revocation if instance is not ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		log.Info("Instance not ready, skipping revocation", "instance", instance.Name)
		return nil
	}

	// Get credentials
	creds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		log.Error(err, "Failed to get credentials for revocation")
		return nil // Don't block deletion
	}

	// Get TLS credentials if needed
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
	connConfig := adapter.BuildConnectionConfig(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create adapter and connect
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, connConfig)
	if err != nil {
		log.Error(err, "Failed to create adapter for revocation")
		return nil
	}
	defer func() { _ = dbAdapter.Close() }()

	if err := dbAdapter.Connect(ctx); err != nil {
		log.Error(err, "Failed to connect for revocation")
		return nil
	}

	// Revoke grants based on engine type
	engine := string(instance.Spec.Engine)
	revocationFailed := false

	switch instance.Spec.Engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if grant.Spec.Postgres != nil {
			// Revoke role assignments
			if len(grant.Spec.Postgres.Roles) > 0 {
				if err := dbAdapter.RevokeRole(ctx, username, grant.Spec.Postgres.Roles); err != nil {
					log.Error(err, "Failed to revoke roles", "roles", grant.Spec.Postgres.Roles)
					revocationFailed = true
				} else {
					log.Info("Revoked roles", "username", username, "roles", grant.Spec.Postgres.Roles)
				}
			}

			// Revoke direct grants
			if len(grant.Spec.Postgres.Grants) > 0 {
				grantOpts := r.buildPostgresGrantOptions(grant.Spec.Postgres.Grants)
				if err := dbAdapter.Revoke(ctx, username, grantOpts); err != nil {
					log.Error(err, "Failed to revoke direct grants")
					revocationFailed = true
				} else {
					log.Info("Revoked direct grants", "username", username, "count", len(grant.Spec.Postgres.Grants))
				}
			}
		}

	case dbopsv1alpha1.EngineTypeMySQL:
		if grant.Spec.MySQL != nil {
			// Revoke role assignments
			if len(grant.Spec.MySQL.Roles) > 0 {
				if err := dbAdapter.RevokeRole(ctx, username, grant.Spec.MySQL.Roles); err != nil {
					log.Error(err, "Failed to revoke roles", "roles", grant.Spec.MySQL.Roles)
					revocationFailed = true
				} else {
					log.Info("Revoked roles", "username", username, "roles", grant.Spec.MySQL.Roles)
				}
			}

			// Revoke direct grants
			if len(grant.Spec.MySQL.Grants) > 0 {
				grantOpts := r.buildMySQLGrantOptions(grant.Spec.MySQL.Grants)
				if err := dbAdapter.Revoke(ctx, username, grantOpts); err != nil {
					log.Error(err, "Failed to revoke direct grants")
					revocationFailed = true
				} else {
					log.Info("Revoked direct grants", "username", username, "count", len(grant.Spec.MySQL.Grants))
				}
			}
		}
	}

	// Record grant deletion metrics
	if revocationFailed {
		metrics.RecordGrantOperation(metrics.OperationDelete, engine, grant.Namespace, metrics.StatusFailure)
	} else {
		metrics.RecordGrantOperation(metrics.OperationDelete, engine, grant.Namespace, metrics.StatusSuccess)
	}

	log.Info("DatabaseGrant cleanup completed", "grant", grant.Name)
	return nil
}

// buildPostgresGrantOptions converts PostgresGrant specs to adapter GrantOptions
func (r *DatabaseGrantReconciler) buildPostgresGrantOptions(grants []dbopsv1alpha1.PostgresGrant) []adapterpkg.GrantOptions {
	opts := make([]adapterpkg.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, adapterpkg.GrantOptions{
			Database:        g.Database,
			Schema:          g.Schema,
			Tables:          g.Tables,
			Sequences:       g.Sequences,
			Functions:       g.Functions,
			Privileges:      g.Privileges,
			WithGrantOption: g.WithGrantOption,
		})
	}
	return opts
}

// buildMySQLGrantOptions converts MySQLGrant specs to adapter GrantOptions
func (r *DatabaseGrantReconciler) buildMySQLGrantOptions(grants []dbopsv1alpha1.MySQLGrant) []adapterpkg.GrantOptions {
	opts := make([]adapterpkg.GrantOptions, 0, len(grants))
	for _, g := range grants {
		opts = append(opts, adapterpkg.GrantOptions{
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
	return opts
}

// buildDefaultPrivilegeOptions converts PostgresDefaultPrivilegeGrant specs to adapter DefaultPrivilegeGrantOptions
func (r *DatabaseGrantReconciler) buildDefaultPrivilegeOptions(defPrivs []dbopsv1alpha1.PostgresDefaultPrivilegeGrant) []adapterpkg.DefaultPrivilegeGrantOptions {
	opts := make([]adapterpkg.DefaultPrivilegeGrantOptions, 0, len(defPrivs))
	for _, dp := range defPrivs {
		opts = append(opts, adapterpkg.DefaultPrivilegeGrantOptions{
			Database:   dp.Database,
			Schema:     dp.Schema,
			GrantedBy:  dp.GrantedBy,
			ObjectType: dp.ObjectType,
			Privileges: dp.Privileges,
		})
	}
	return opts
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseGrant{}).
		Named("databasegrant").
		Complete(r)
}
