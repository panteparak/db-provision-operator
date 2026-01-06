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
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseUserReconciler reconciles a DatabaseUser object
type DatabaseUserReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles a DatabaseUser object
func (r *DatabaseUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseUser
	user := &dbopsv1alpha1.DatabaseUser{}
	if err := r.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(user) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(user) {
		return r.handleDeletion(ctx, user)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(user, util.FinalizerDatabaseUser) {
		controllerutil.AddFinalizer(user, util.FinalizerDatabaseUser)
		if err := r.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the user
	result, err := r.reconcileUser(ctx, user)
	if err != nil {
		log.Error(err, "Failed to reconcile user")
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = err.Error()
		util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
			util.ReasonReconcileFailed, err.Error())
		if statusErr := r.Status().Update(ctx, user); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return result, err
	}

	return result, nil
}

// reconcileUser handles the main reconciliation logic
func (r *DatabaseUserReconciler) reconcileUser(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the DatabaseInstance
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceRef := user.Spec.InstanceRef
	instanceNamespace := user.Namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      instanceRef.Name,
	}, instance); err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("DatabaseInstance not found: %v", err)
		util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
			util.ReasonInstanceNotFound, err.Error())
		if statusErr := r.Status().Update(ctx, user); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		// Don't return error for not found - status is updated, requeue to check if instance is created later
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if instance is ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		user.Status.Phase = dbopsv1alpha1.PhasePending
		user.Status.Message = "Waiting for DatabaseInstance to be ready"
		util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
			util.ReasonInstanceNotReady, "DatabaseInstance is not ready")
		if statusErr := r.Status().Update(ctx, user); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get admin credentials
	adminCreds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to get admin credentials: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
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
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to create adapter: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	defer func() { _ = dbAdapter.Close() }()

	// Connect to database
	if err := dbAdapter.Connect(ctx); err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to connect: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Get or generate user password
	userPassword, err := r.getOrGeneratePassword(ctx, user)
	if err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to get/generate password: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Check if user exists
	username := user.Spec.Username
	exists, err := dbAdapter.UserExists(ctx, username)
	if err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to check user existence: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	engine := string(instance.Spec.Engine)
	if !exists {
		// Create the user
		log.Info("Creating user", "username", username)
		user.Status.Phase = dbopsv1alpha1.PhaseCreating
		user.Status.Message = "Creating user"
		if statusErr := r.Status().Update(ctx, user); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}

		opts := r.buildCreateUserOptions(user, userPassword, instance.Spec.Engine)
		if err := dbAdapter.CreateUser(ctx, opts); err != nil {
			metrics.RecordUserOperation(metrics.OperationCreate, engine, user.Namespace, metrics.StatusFailure)
			user.Status.Phase = dbopsv1alpha1.PhaseFailed
			user.Status.Message = fmt.Sprintf("Failed to create user: %v", err)
			util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
				util.ReasonCreateFailed, err.Error())
			if statusErr := r.Status().Update(ctx, user); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
		metrics.RecordUserOperation(metrics.OperationCreate, engine, user.Namespace, metrics.StatusSuccess)

		log.Info("Successfully created user", "username", username)
	} else {
		// Update user if needed
		updateOpts := r.buildUpdateUserOptions(user, instance.Spec.Engine)
		if err := dbAdapter.UpdateUser(ctx, username, updateOpts); err != nil {
			metrics.RecordUserOperation(metrics.OperationUpdate, engine, user.Namespace, metrics.StatusFailure)
			log.Error(err, "Failed to update user", "username", username)
		} else {
			metrics.RecordUserOperation(metrics.OperationUpdate, engine, user.Namespace, metrics.StatusSuccess)
		}
	}

	// Create or update credentials secret
	if user.Spec.PasswordSecret != nil {
		secretName := user.Spec.PasswordSecret.SecretName
		if secretName == "" {
			secretName = fmt.Sprintf("%s-credentials", user.Name)
		}

		templateData := secret.TemplateData{
			Username:  username,
			Password:  userPassword,
			Host:      instance.Spec.Connection.Host,
			Port:      instance.Spec.Connection.Port,
			Database:  instance.Spec.Connection.Database,
			Namespace: user.Namespace,
			Name:      user.Name,
		}

		var secretData map[string][]byte
		if user.Spec.PasswordSecret.SecretTemplate != nil {
			var err error
			secretData, err = secret.RenderSecretTemplate(user.Spec.PasswordSecret.SecretTemplate, templateData)
			if err != nil {
				log.Error(err, "Failed to render secret template")
				secretData = nil
			}
		}
		if secretData == nil {
			// Default secret data if no template
			secretData = map[string][]byte{
				"username": []byte(username),
				"password": []byte(userPassword),
			}
		}

		if err := r.SecretManager.EnsureSecretWithOwner(ctx, user.Namespace, secretName, secretData, user, r.Scheme); err != nil {
			log.Error(err, "Failed to ensure credentials secret")
		} else {
			user.Status.Secret = &dbopsv1alpha1.SecretInfo{
				Name:      secretName,
				Namespace: user.Namespace,
			}
		}
	}

	// Update status
	user.Status.Phase = dbopsv1alpha1.PhaseReady
	user.Status.Message = "User is ready"
	user.Status.User = &dbopsv1alpha1.UserInfo{
		Username: username,
	}

	// Set conditions
	util.SetSyncedCondition(&user.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "User is synced")
	util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "User is ready")

	if err := r.Status().Update(ctx, user); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled DatabaseUser", "name", user.Name, "username", username)

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// getOrGeneratePassword gets an existing password or generates a new one
func (r *DatabaseUserReconciler) getOrGeneratePassword(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (string, error) {
	// Check if password is provided via existing secret
	if user.Spec.ExistingPasswordSecret != nil {
		password, err := r.SecretManager.GetPassword(ctx, user.Namespace, user.Spec.ExistingPasswordSecret)
		if err != nil {
			return "", err
		}
		return password, nil
	}

	// Check if credentials secret already exists
	if user.Status.Secret != nil && user.Status.Secret.Name != "" {
		exists, err := r.SecretManager.SecretExists(ctx, user.Namespace, user.Status.Secret.Name)
		if err == nil && exists {
			// Try to get existing password from the secret
			password, err := r.SecretManager.GetPassword(ctx, user.Namespace, &dbopsv1alpha1.ExistingPasswordSecret{
				Name: user.Status.Secret.Name,
				Key:  "password",
			})
			if err == nil {
				return password, nil
			}
		}
	}

	// Generate new password
	return secret.GeneratePassword(user.Spec.PasswordSecret)
}

// buildCreateUserOptions builds options for creating a user
func (r *DatabaseUserReconciler) buildCreateUserOptions(user *dbopsv1alpha1.DatabaseUser, password string, engine dbopsv1alpha1.EngineType) adapter.CreateUserOptions {
	opts := adapter.CreateUserOptions{
		Username: user.Spec.Username,
		Password: password,
		Login:    true,
		Inherit:  true,
	}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if user.Spec.Postgres != nil {
			opts.ConnectionLimit = user.Spec.Postgres.ConnectionLimit
			opts.ValidUntil = user.Spec.Postgres.ValidUntil
			opts.Superuser = user.Spec.Postgres.Superuser
			opts.CreateDB = user.Spec.Postgres.CreateDB
			opts.CreateRole = user.Spec.Postgres.CreateRole
			opts.Inherit = user.Spec.Postgres.Inherit
			opts.Login = user.Spec.Postgres.Login
			opts.Replication = user.Spec.Postgres.Replication
			opts.BypassRLS = user.Spec.Postgres.BypassRLS
			opts.InRoles = user.Spec.Postgres.InRoles
			opts.ConfigParams = user.Spec.Postgres.ConfigParameters
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if user.Spec.MySQL != nil {
			opts.MaxQueriesPerHour = user.Spec.MySQL.MaxQueriesPerHour
			opts.MaxUpdatesPerHour = user.Spec.MySQL.MaxUpdatesPerHour
			opts.MaxConnectionsPerHour = user.Spec.MySQL.MaxConnectionsPerHour
			opts.MaxUserConnections = user.Spec.MySQL.MaxUserConnections
			opts.AuthPlugin = string(user.Spec.MySQL.AuthPlugin)
			opts.RequireSSL = user.Spec.MySQL.RequireSSL
			opts.RequireX509 = user.Spec.MySQL.RequireX509
			opts.AllowedHosts = user.Spec.MySQL.AllowedHosts
			opts.AccountLocked = user.Spec.MySQL.AccountLocked
		}
	}

	return opts
}

// buildUpdateUserOptions builds options for updating a user
func (r *DatabaseUserReconciler) buildUpdateUserOptions(user *dbopsv1alpha1.DatabaseUser, engine dbopsv1alpha1.EngineType) adapter.UpdateUserOptions {
	opts := adapter.UpdateUserOptions{}

	switch engine {
	case dbopsv1alpha1.EngineTypePostgres:
		if user.Spec.Postgres != nil {
			if user.Spec.Postgres.ConnectionLimit != 0 {
				opts.ConnectionLimit = &user.Spec.Postgres.ConnectionLimit
			}
			if user.Spec.Postgres.ValidUntil != "" {
				opts.ValidUntil = &user.Spec.Postgres.ValidUntil
			}
			opts.InRoles = user.Spec.Postgres.InRoles
			opts.ConfigParams = user.Spec.Postgres.ConfigParameters
		}
	case dbopsv1alpha1.EngineTypeMySQL:
		if user.Spec.MySQL != nil {
			if user.Spec.MySQL.MaxQueriesPerHour != 0 {
				opts.MaxQueriesPerHour = &user.Spec.MySQL.MaxQueriesPerHour
			}
			if user.Spec.MySQL.MaxUpdatesPerHour != 0 {
				opts.MaxUpdatesPerHour = &user.Spec.MySQL.MaxUpdatesPerHour
			}
			if user.Spec.MySQL.MaxConnectionsPerHour != 0 {
				opts.MaxConnectionsPerHour = &user.Spec.MySQL.MaxConnectionsPerHour
			}
			if user.Spec.MySQL.MaxUserConnections != 0 {
				opts.MaxUserConnections = &user.Spec.MySQL.MaxUserConnections
			}
		}
	}

	return opts
}

// handleDeletion handles the deletion of a DatabaseUser
func (r *DatabaseUserReconciler) handleDeletion(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(user, util.FinalizerDatabaseUser) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseUser", "name", user.Name)

	// Drop the user from the database
	{
		// Get the DatabaseInstance
		instance := &dbopsv1alpha1.DatabaseInstance{}
		instanceRef := user.Spec.InstanceRef
		instanceNamespace := user.Namespace
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
					defer func() { _ = dbAdapter.Close() }()
					if err := dbAdapter.Connect(ctx); err == nil {
						log.Info("Dropping user", "username", user.Spec.Username)
						engine := string(instance.Spec.Engine)
						if err := dbAdapter.DropUser(ctx, user.Spec.Username); err != nil {
							metrics.RecordUserOperation(metrics.OperationDelete, engine, user.Namespace, metrics.StatusFailure)
							log.Error(err, "Failed to drop user", "username", user.Spec.Username)
						} else {
							metrics.RecordUserOperation(metrics.OperationDelete, engine, user.Namespace, metrics.StatusSuccess)
							log.Info("Successfully dropped user", "username", user.Spec.Username)
						}
					}
				}
			}
		}
	}

	// Delete credentials secret if it exists
	if user.Status.Secret != nil && user.Status.Secret.Name != "" {
		if err := r.SecretManager.DeleteSecret(ctx, user.Namespace, user.Status.Secret.Name); err != nil {
			log.Error(err, "Failed to delete credentials secret")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(user, util.FinalizerDatabaseUser)
	if err := r.Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted DatabaseUser", "name", user.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.SecretManager == nil {
		r.SecretManager = secret.NewManager(r.Client)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseUser{}).
		Named("databaseuser").
		Complete(r)
}
