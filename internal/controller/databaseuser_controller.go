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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service"
	"github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseUserReconciler reconciles a DatabaseUser object
type DatabaseUserReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
	Recorder      record.EventRecorder
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
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err := r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if err == nil {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config from instance and credentials
	cfg := service.ConfigFromInstance(&instance.Spec, adminCreds.Username, adminCreds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = log

	// Create user service
	svc, err := service.NewUserService(cfg)
	if err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to create service: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	defer func() { _ = svc.Close() }()

	// Connect to database
	if err := svc.Connect(ctx); err != nil {
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

	username := user.Spec.Username
	engine := cfg.Engine

	// Check if user exists
	exists, err := svc.Exists(ctx, username)
	if err != nil {
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = fmt.Sprintf("Failed to check user existence: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	if !exists {
		// Create the user
		log.Info("Creating user", "username", username)
		user.Status.Phase = dbopsv1alpha1.PhaseCreating
		user.Status.Message = "Creating user"
		if statusErr := r.Status().Update(ctx, user); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}

		result, err := svc.Create(ctx, service.CreateUserServiceOptions{
			Spec:     &user.Spec,
			Password: userPassword,
		})
		if err != nil {
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

		if result.Created {
			metrics.RecordUserOperation(metrics.OperationCreate, engine, user.Namespace, metrics.StatusSuccess)
			log.Info("Successfully created user", "username", username)
		}
	} else {
		// Update user if needed
		if _, err := svc.Update(ctx, username, &user.Spec); err != nil {
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

	// Perform drift detection if enabled
	driftResult := r.performDriftDetection(ctx, user, instance, cfg, svc.Adapter())

	// Update status
	user.Status.Phase = dbopsv1alpha1.PhaseReady
	user.Status.Message = "User is ready"
	user.Status.Drift = driftResult
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
			// Get credentials
			adminCreds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
			if err == nil {
				// Get TLS credentials if enabled
				var tlsCA, tlsCert, tlsKey []byte
				if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
					tlsCreds, err := r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
					if err == nil {
						tlsCA = tlsCreds.CA
						tlsCert = tlsCreds.Cert
						tlsKey = tlsCreds.Key
					}
				}

				// Build service config and create service
				cfg := service.ConfigFromInstance(&instance.Spec, adminCreds.Username, adminCreds.Password, tlsCA, tlsCert, tlsKey)
				cfg.Logger = log
				svc, err := service.NewUserService(cfg)
				if err == nil {
					defer func() { _ = svc.Close() }()
					if err := svc.Connect(ctx); err == nil {
						log.Info("Dropping user", "username", user.Spec.Username)
						engine := cfg.Engine
						if _, err := svc.Delete(ctx, user.Spec.Username); err != nil {
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

// performDriftDetection performs drift detection for a user.
// Returns nil if drift detection is disabled or an error occurs.
func (r *DatabaseUserReconciler) performDriftDetection(
	ctx context.Context,
	user *dbopsv1alpha1.DatabaseUser,
	instance *dbopsv1alpha1.DatabaseInstance,
	cfg *service.Config,
	adp adapter.DatabaseAdapter,
) *dbopsv1alpha1.DriftStatus {
	log := logf.FromContext(ctx)

	// Get effective drift policy (CR override or instance default)
	policy := r.getEffectiveDriftPolicy(user, instance)

	// Skip if drift detection is disabled
	if policy.Mode == dbopsv1alpha1.DriftModeIgnore {
		log.V(1).Info("Drift detection disabled")
		return nil
	}

	// Create drift service
	driftCfg := &drift.Config{
		AllowDestructive: r.hasDestructiveDriftAnnotation(user),
		Logger:           log,
	}
	driftSvc := drift.NewService(adp, driftCfg)

	// Detect drift
	driftResult, err := driftSvc.DetectUserDrift(ctx, &user.Spec)
	if err != nil {
		log.Error(err, "Failed to detect drift")
		metrics.RecordDriftDetection("user", user.Namespace, metrics.StatusFailure)
		return nil
	}

	// Record drift detection metric
	metrics.RecordDriftDetection("user", user.Namespace, metrics.StatusSuccess)
	metrics.SetDriftDetected("user", user.Spec.Username, user.Namespace, driftResult.HasDrift())

	// Update drift status
	now := metav1.Now()
	status := &dbopsv1alpha1.DriftStatus{
		Detected:    driftResult.HasDrift(),
		LastChecked: &now,
	}

	// Convert diffs to API type
	for _, d := range driftResult.Diffs {
		status.Diffs = append(status.Diffs, dbopsv1alpha1.DriftDiff{
			Field:       d.Field,
			Expected:    d.Expected,
			Actual:      d.Actual,
			Destructive: d.Destructive,
			Immutable:   d.Immutable,
		})
	}

	// Emit event if drift detected
	if driftResult.HasDrift() {
		if r.Recorder != nil {
			r.Recorder.Eventf(user, corev1.EventTypeWarning, "DriftDetected",
				"User %s has drifted: %d differences found", user.Spec.Username, len(driftResult.Diffs))
		}

		// Correct drift if mode is "correct"
		if policy.Mode == dbopsv1alpha1.DriftModeCorrect {
			correctionResult, err := driftSvc.CorrectUserDrift(ctx, &user.Spec, driftResult)
			if err != nil {
				log.Error(err, "Failed to correct drift")
				metrics.RecordDriftCorrection("user", user.Namespace, metrics.StatusFailure)
			} else {
				// Record correction metrics
				if len(correctionResult.Corrected) > 0 {
					metrics.RecordDriftCorrection("user", user.Namespace, metrics.StatusSuccess)
				}
				if r.Recorder != nil {
					for _, corrected := range correctionResult.Corrected {
						r.Recorder.Eventf(user, corev1.EventTypeNormal, "DriftCorrected",
							"Corrected drift for %s: %s -> %s",
							corrected.Diff.Field, corrected.Diff.Actual, corrected.Diff.Expected)
					}
					for _, skipped := range correctionResult.Skipped {
						r.Recorder.Eventf(user, corev1.EventTypeWarning, "DriftSkipped",
							"Skipped drift correction for %s: %s",
							skipped.Diff.Field, skipped.Reason)
					}
				}
			}
		}
	}

	return status
}

// getEffectiveDriftPolicy returns the drift policy to use for the user.
func (r *DatabaseUserReconciler) getEffectiveDriftPolicy(
	user *dbopsv1alpha1.DatabaseUser,
	instance *dbopsv1alpha1.DatabaseInstance,
) *dbopsv1alpha1.DriftPolicy {
	if user.Spec.DriftPolicy != nil {
		return user.Spec.DriftPolicy
	}
	if instance.Spec.DriftPolicy != nil {
		return instance.Spec.DriftPolicy
	}
	return &dbopsv1alpha1.DriftPolicy{
		Mode:     dbopsv1alpha1.DriftModeDetect,
		Interval: "5m",
	}
}

// hasDestructiveDriftAnnotation checks if destructive drift corrections are allowed.
func (r *DatabaseUserReconciler) hasDestructiveDriftAnnotation(user *dbopsv1alpha1.DatabaseUser) bool {
	if user.Annotations == nil {
		return false
	}
	return user.Annotations[dbopsv1alpha1.AnnotationAllowDestructiveDrift] == "true"
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.SecretManager == nil {
		r.SecretManager = secret.NewManager(r.Client)
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("databaseuser-controller")
	}

	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseUser{}).
		Named("databaseuser").
		Complete(r)
}
