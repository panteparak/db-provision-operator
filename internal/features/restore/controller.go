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

package restore

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/util"
)

const (
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second
)

// Controller handles K8s reconciliation for DatabaseRestore resources.
type Controller struct {
	client.Client
	Scheme  *runtime.Scheme
	handler *Handler
	logger  logr.Logger
}

// ControllerConfig holds dependencies for the controller.
type ControllerConfig struct {
	Client  client.Client
	Scheme  *runtime.Scheme
	Handler *Handler
	Logger  logr.Logger
}

// NewController creates a new restore controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:  cfg.Client,
		Scheme:  cfg.Scheme,
		handler: cfg.Handler,
		logger:  cfg.Logger,
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaserestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaserestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaserestores/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for DatabaseRestore resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", req.NamespacedName)

	// Fetch the DatabaseRestore resource
	restore := &dbopsv1alpha1.DatabaseRestore{}
	if err := c.Get(ctx, req.NamespacedName, restore); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseRestore")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(restore) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(restore) {
		return c.handleDeletion(ctx, restore)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(restore, util.FinalizerDatabaseRestore) {
		controllerutil.AddFinalizer(restore, util.FinalizerDatabaseRestore)
		if err := c.Update(ctx, restore); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if restore is in terminal state
	if c.handler.IsTerminal(restore) {
		return ctrl.Result{}, nil
	}

	// Check active deadline
	if exceeded, message := c.handler.CheckDeadline(restore); exceeded {
		return c.handleDeadlineExceeded(ctx, restore, message)
	}

	return c.reconcile(ctx, restore)
}

func (c *Controller) reconcile(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	// Set initial phase if needed
	if restore.Status.Phase == "" {
		return c.handlePending(ctx, restore, "Initializing restore")
	}

	// Validate spec
	if err := c.handler.ValidateSpec(ctx, restore); err != nil {
		if _, ok := err.(*ValidationError); ok {
			return c.handleValidationError(ctx, restore, err)
		}
		return c.handleError(ctx, restore, err, "validate spec")
	}

	// Check if backup is ready (if using BackupRef)
	if restore.Spec.BackupRef != nil {
		backup, err := c.handler.repo.GetBackup(ctx, restore.Namespace, restore.Spec.BackupRef)
		if err != nil {
			if errors.IsNotFound(err) {
				return c.handlePending(ctx, restore, fmt.Sprintf("Waiting for DatabaseBackup %s", restore.Spec.BackupRef.Name))
			}
			return c.handleError(ctx, restore, err, "get backup")
		}

		if backup.Status.Phase != dbopsv1alpha1.PhaseCompleted {
			return c.handlePending(ctx, restore, fmt.Sprintf("Waiting for DatabaseBackup %s to complete", backup.Name))
		}
	}

	// Resolve target instance for readiness check
	targetInstance, _, err := c.handler.resolveTarget(ctx, restore)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handlePending(ctx, restore, err.Error())
		}
		return c.handleError(ctx, restore, err, "resolve target")
	}

	// Check if instance is ready
	if targetInstance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return c.handlePending(ctx, restore, fmt.Sprintf("Waiting for DatabaseInstance %s to be ready", targetInstance.Name))
	}

	// Update status to Running
	now := metav1.Now()
	restore.Status.Phase = dbopsv1alpha1.PhaseRunning
	restore.Status.Message = "Restore in progress"
	if restore.Status.StartedAt == nil {
		restore.Status.StartedAt = &now
	}
	restore.Status.Progress = &dbopsv1alpha1.RestoreProgress{
		Percentage:   0,
		CurrentPhase: "Starting",
	}
	if err := c.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status to Running")
	}

	// Execute restore
	log.Info("Executing restore")
	result, err := c.handler.Execute(ctx, restore)
	if err != nil {
		return c.handleError(ctx, restore, err, "execute restore")
	}

	// Update status to Completed
	return c.handleCompleted(ctx, restore, result)
}

func (c *Controller) handleDeletion(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	if !controllerutil.ContainsFinalizer(restore, util.FinalizerDatabaseRestore) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseRestore")

	// Clean up info metric
	c.handler.CleanupInfoMetric(restore)

	// Remove finalizer
	controllerutil.RemoveFinalizer(restore, util.FinalizerDatabaseRestore)
	if err := c.Update(ctx, restore); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseRestore")
	return ctrl.Result{}, nil
}

func (c *Controller) handleDeadlineExceeded(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, message string) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	log.Info("Restore deadline exceeded", "message", message)

	restore.Status.Phase = dbopsv1alpha1.PhaseFailed
	restore.Status.Message = message
	util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "DeadlineExceeded", message)

	if err := c.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(restore)

	return ctrl.Result{}, nil
}

func (c *Controller) handleValidationError(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, err error) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	log.Info("Restore validation failed", "error", err.Error())

	restore.Status.Phase = dbopsv1alpha1.PhaseFailed
	restore.Status.Message = err.Error()
	util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, "ValidationFailed", err.Error())

	if statusErr := c.Status().Update(ctx, restore); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(restore)

	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, err error, operation string) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)

	restore.Status.Phase = dbopsv1alpha1.PhaseFailed
	restore.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, restore); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(restore)

	return ctrl.Result{RequeueAfter: RequeueAfterError}, err
}

func (c *Controller) handlePending(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, message string) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	log.V(1).Info("Restore pending", "reason", message)

	restore.Status.Phase = dbopsv1alpha1.PhasePending
	restore.Status.Message = message
	util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionFalse, util.ReasonReconciling, message)

	if err := c.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(restore)

	return ctrl.Result{RequeueAfter: RequeueAfterPending}, nil
}

func (c *Controller) handleCompleted(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore, result *Result) (ctrl.Result, error) {
	log := c.logger.WithValues("restore", restore.Name, "namespace", restore.Namespace)

	completedAt := metav1.Now()
	restore.Status.Phase = dbopsv1alpha1.PhaseCompleted
	restore.Status.Message = "Restore completed successfully"
	restore.Status.CompletedAt = &completedAt
	restore.Status.Restore = &dbopsv1alpha1.RestoreInfo{
		TargetInstance: result.TargetInstance,
		TargetDatabase: result.TargetDatabase,
		SourceBackup:   result.SourceBackup,
	}
	restore.Status.Progress = &dbopsv1alpha1.RestoreProgress{
		Percentage:     100,
		CurrentPhase:   "Completed",
		TablesRestored: result.TablesRestored,
		TablesTotal:    result.TablesRestored,
	}
	restore.Status.Warnings = result.Warnings

	util.SetSyncedCondition(&restore.Status.Conditions, metav1.ConditionTrue, util.ReasonRestoreCompleted, "Restore completed successfully")
	util.SetReadyCondition(&restore.Status.Conditions, metav1.ConditionTrue, "Ready", "DatabaseRestore is ready")

	if err := c.Status().Update(ctx, restore); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(restore)

	log.Info("Successfully completed DatabaseRestore",
		"targetDatabase", result.TargetDatabase,
		"tablesRestored", result.TablesRestored,
		"duration", result.Duration)

	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseRestore{}).
		Named("databaserestore-feature").
		Complete(c)
}
