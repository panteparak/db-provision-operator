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

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
	Recorder      record.EventRecorder
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile reconciles a Database object
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Database
	database := &dbopsv1alpha1.Database{}
	if err := r.Get(ctx, req.NamespacedName, database); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(database) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(database) {
		return r.handleDeletion(ctx, database)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(database, util.FinalizerDatabase) {
		controllerutil.AddFinalizer(database, util.FinalizerDatabase)
		if err := r.Update(ctx, database); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the database
	result, err := r.reconcileDatabase(ctx, database)
	if err != nil {
		log.Error(err, "Failed to reconcile database")
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = err.Error()
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonReconcileFailed, err.Error())
		if statusErr := r.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return result, err
	}

	return result, nil
}

// reconcileDatabase handles the main reconciliation logic
func (r *DatabaseReconciler) reconcileDatabase(ctx context.Context, database *dbopsv1alpha1.Database) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the DatabaseInstance
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceRef := database.Spec.InstanceRef
	instanceNamespace := database.Namespace
	if instanceRef.Namespace != "" {
		instanceNamespace = instanceRef.Namespace
	}

	if err := r.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      instanceRef.Name,
	}, instance); err != nil {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = fmt.Sprintf("DatabaseInstance not found: %v", err)
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonInstanceNotFound, err.Error())
		if statusErr := r.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		// Don't return error for not found - status is updated, requeue to check if instance is created later
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if instance is ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		database.Status.Phase = dbopsv1alpha1.PhasePending
		database.Status.Message = "Waiting for DatabaseInstance to be ready"
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonInstanceNotReady, "DatabaseInstance is not ready")
		if statusErr := r.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Get credentials
	creds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = fmt.Sprintf("Failed to get credentials: %v", err)
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonSecretNotFound, err.Error())
		if statusErr := r.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err := r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if err != nil {
			log.Error(err, "Failed to get TLS credentials")
		} else {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config from instance and credentials
	cfg := service.ConfigFromInstance(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = log

	// Create database service
	svc, err := service.NewDatabaseService(cfg)
	if err != nil {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = fmt.Sprintf("Failed to create service: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	defer func() { _ = svc.Close() }()

	// Connect to database
	if err := svc.Connect(ctx); err != nil {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = fmt.Sprintf("Failed to connect: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	dbName := database.Spec.Name
	engine := cfg.Engine

	// Check if database exists and create if needed
	exists, err := svc.Exists(ctx, dbName)
	if err != nil {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = fmt.Sprintf("Failed to check database existence: %v", err)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	if !exists {
		// Update status to Creating
		log.Info("Creating database", "name", dbName)
		database.Status.Phase = dbopsv1alpha1.PhaseCreating
		database.Status.Message = "Creating database"
		if statusErr := r.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}

		// Create the database using service layer
		createStart := time.Now()
		result, err := svc.CreateOnly(ctx, &database.Spec)
		if err != nil {
			metrics.RecordDatabaseOperation(metrics.OperationCreate, engine, database.Namespace, metrics.StatusFailure)
			database.Status.Phase = dbopsv1alpha1.PhaseFailed
			database.Status.Message = fmt.Sprintf("Failed to create database: %v", err)
			util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
				util.ReasonCreateFailed, err.Error())
			if statusErr := r.Status().Update(ctx, database); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}

		if result.Created {
			createDuration := time.Since(createStart).Seconds()
			metrics.RecordDatabaseOperation(metrics.OperationCreate, engine, database.Namespace, metrics.StatusSuccess)
			metrics.RecordDatabaseOperationDuration(metrics.OperationCreate, engine, database.Namespace, createDuration)
			log.Info("Successfully created database", "name", dbName)
		}
	}

	// Verify the database is accepting connections before marking it as Ready.
	// This is particularly important for PostgreSQL where a newly created database
	// may temporarily not accept connections while being initialized from a template.
	if err := svc.VerifyAccess(ctx, dbName); err != nil {
		log.Info("Database not yet accepting connections, will retry", "name", dbName, "error", err)
		database.Status.Phase = dbopsv1alpha1.PhaseCreating
		database.Status.Message = "Waiting for database to accept connections"
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonCreating, "Database is initializing")
		if statusErr := r.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		// Requeue quickly to check again
		return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
	}

	// Update database settings if needed (extensions, schemas, etc.)
	if _, err := svc.Update(ctx, dbName, &database.Spec); err != nil {
		log.Error(err, "Failed to update database settings", "name", dbName)
		// Don't fail, just log the error
	}

	// Get database info
	dbInfo, err := svc.Get(ctx, dbName)
	if err != nil {
		log.Error(err, "Failed to get database info")
	}

	// Perform drift detection if enabled
	driftResult := r.performDriftDetection(ctx, database, instance, cfg, svc.Adapter())

	// Update status
	database.Status.Phase = dbopsv1alpha1.PhaseReady
	database.Status.Message = "Database is ready"
	database.Status.Drift = driftResult
	if dbInfo != nil {
		database.Status.Database = &dbopsv1alpha1.DatabaseInfo{
			Name:      dbName,
			Owner:     dbInfo.Owner,
			SizeBytes: dbInfo.SizeBytes,
		}
		// Record database size metric
		metrics.SetDatabaseSize(dbName, instance.Name, engine, database.Namespace, float64(dbInfo.SizeBytes))
	}

	// Set conditions
	util.SetSyncedCondition(&database.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Database is synced")
	util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Database is ready")

	if err := r.Status().Update(ctx, database); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled Database", "name", database.Name, "database", dbName)

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// handleDeletion handles the deletion of a Database
func (r *DatabaseReconciler) handleDeletion(ctx context.Context, database *dbopsv1alpha1.Database) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(database, util.FinalizerDatabase) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of Database", "name", database.Name)

	// Check deletion policy
	deletionPolicy := database.Spec.DeletionPolicy
	if deletionPolicy == "" {
		deletionPolicy = dbopsv1alpha1.DeletionPolicyRetain
	}

	// Check for deletion protection
	if database.Spec.DeletionProtection && !util.HasForceDeleteAnnotation(database) {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = "Deletion blocked by deletion protection. Set force-delete annotation to proceed."
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := r.Status().Update(ctx, database); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Handle based on deletion policy
	if deletionPolicy == dbopsv1alpha1.DeletionPolicyDelete {
		// Get the DatabaseInstance
		instance := &dbopsv1alpha1.DatabaseInstance{}
		instanceRef := database.Spec.InstanceRef
		instanceNamespace := database.Namespace
		if instanceRef.Namespace != "" {
			instanceNamespace = instanceRef.Namespace
		}

		if err := r.Get(ctx, types.NamespacedName{
			Namespace: instanceNamespace,
			Name:      instanceRef.Name,
		}, instance); err == nil {
			// Get credentials
			creds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
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
				cfg := service.ConfigFromInstance(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
				cfg.Logger = log
				svc, err := service.NewDatabaseService(cfg)
				if err == nil {
					defer func() { _ = svc.Close() }()
					if err := svc.Connect(ctx); err == nil {
						log.Info("Dropping database", "name", database.Spec.Name)
						engine := cfg.Engine
						deleteStart := time.Now()
						force := util.HasForceDeleteAnnotation(database)

						if _, err := svc.Delete(ctx, database.Spec.Name, force); err != nil {
							log.Error(err, "Failed to drop database", "name", database.Spec.Name)
							metrics.RecordDatabaseOperation(metrics.OperationDelete, engine, database.Namespace, metrics.StatusFailure)
						} else {
							log.Info("Successfully dropped database", "name", database.Spec.Name)
							deleteDuration := time.Since(deleteStart).Seconds()
							metrics.RecordDatabaseOperation(metrics.OperationDelete, engine, database.Namespace, metrics.StatusSuccess)
							metrics.RecordDatabaseOperationDuration(metrics.OperationDelete, engine, database.Namespace, deleteDuration)
							// Clean up database size metric
							metrics.DeleteDatabaseMetrics(database.Spec.Name, instance.Name, engine, database.Namespace)
						}
					}
				}
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(database, util.FinalizerDatabase)
	if err := r.Update(ctx, database); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted Database", "name", database.Name)
	return ctrl.Result{}, nil
}

// performDriftDetection performs drift detection for a database.
// Returns nil if drift detection is disabled or an error occurs.
func (r *DatabaseReconciler) performDriftDetection(
	ctx context.Context,
	database *dbopsv1alpha1.Database,
	instance *dbopsv1alpha1.DatabaseInstance,
	cfg *service.Config,
	adp adapter.DatabaseAdapter,
) *dbopsv1alpha1.DriftStatus {
	log := logf.FromContext(ctx)

	// Get effective drift policy (CR override or instance default)
	policy := r.getEffectiveDriftPolicy(database, instance)

	// Skip if drift detection is disabled
	if policy.Mode == dbopsv1alpha1.DriftModeIgnore {
		log.V(1).Info("Drift detection disabled")
		return nil
	}

	// Create drift service
	driftCfg := &drift.Config{
		AllowDestructive: r.hasDestructiveDriftAnnotation(database),
		Logger:           log,
	}
	driftSvc := drift.NewService(adp, driftCfg)

	// Detect drift
	driftResult, err := driftSvc.DetectDatabaseDrift(ctx, &database.Spec)
	if err != nil {
		log.Error(err, "Failed to detect drift")
		metrics.RecordDriftDetection("database", database.Namespace, metrics.StatusFailure)
		// Don't fail reconcile for drift check failures
		return nil
	}

	// Record drift detection metric
	metrics.RecordDriftDetection("database", database.Namespace, metrics.StatusSuccess)
	metrics.SetDriftDetected("database", database.Spec.Name, database.Namespace, driftResult.HasDrift())

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
			r.Recorder.Eventf(database, corev1.EventTypeWarning, "DriftDetected",
				"Database %s has drifted: %d differences found", database.Spec.Name, len(driftResult.Diffs))
		}

		// Correct drift if mode is "correct"
		if policy.Mode == dbopsv1alpha1.DriftModeCorrect {
			correctionResult, err := driftSvc.CorrectDatabaseDrift(ctx, &database.Spec, driftResult)
			if err != nil {
				log.Error(err, "Failed to correct drift")
				metrics.RecordDriftCorrection("database", database.Namespace, metrics.StatusFailure)
			} else {
				// Record correction metrics
				if len(correctionResult.Corrected) > 0 {
					metrics.RecordDriftCorrection("database", database.Namespace, metrics.StatusSuccess)
				}
				// Emit events for corrections
				if r.Recorder != nil {
					for _, corrected := range correctionResult.Corrected {
						r.Recorder.Eventf(database, corev1.EventTypeNormal, "DriftCorrected",
							"Corrected drift for %s: %s -> %s",
							corrected.Diff.Field, corrected.Diff.Actual, corrected.Diff.Expected)
					}
					for _, skipped := range correctionResult.Skipped {
						r.Recorder.Eventf(database, corev1.EventTypeWarning, "DriftSkipped",
							"Skipped drift correction for %s: %s",
							skipped.Diff.Field, skipped.Reason)
					}
				}
			}
		}
	}

	return status
}

// getEffectiveDriftPolicy returns the drift policy to use for the database.
// CR-level policy overrides instance-level default.
func (r *DatabaseReconciler) getEffectiveDriftPolicy(
	database *dbopsv1alpha1.Database,
	instance *dbopsv1alpha1.DatabaseInstance,
) *dbopsv1alpha1.DriftPolicy {
	// CR-level override
	if database.Spec.DriftPolicy != nil {
		return database.Spec.DriftPolicy
	}

	// Instance-level default
	if instance.Spec.DriftPolicy != nil {
		return instance.Spec.DriftPolicy
	}

	// Default to detect mode
	return &dbopsv1alpha1.DriftPolicy{
		Mode:     dbopsv1alpha1.DriftModeDetect,
		Interval: "5m",
	}
}

// hasDestructiveDriftAnnotation checks if destructive drift corrections are allowed.
func (r *DatabaseReconciler) hasDestructiveDriftAnnotation(database *dbopsv1alpha1.Database) bool {
	if database.Annotations == nil {
		return false
	}
	return database.Annotations[dbopsv1alpha1.AnnotationAllowDestructiveDrift] == "true"
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.SecretManager == nil {
		r.SecretManager = secret.NewManager(r.Client)
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("database-controller")
	}

	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.Database{}).
		Named("database").
		Complete(r)
}
