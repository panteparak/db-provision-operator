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

package database

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/reconcileutil"
	"github.com/db-provision-operator/internal/util"
)

const (
	// RequeueAfterReady is the requeue duration after a successful reconcile.
	RequeueAfterReady = 5 * time.Minute

	// RequeueAfterError is the requeue duration after an error.
	RequeueAfterError = 30 * time.Second

	// RequeueAfterPending is the requeue duration when waiting for dependencies.
	RequeueAfterPending = 10 * time.Second

	// RequeueAfterCreating is the requeue duration when verifying creation.
	RequeueAfterCreating = 2 * time.Second
)

// Controller handles K8s reconciliation for Database resources.
// It is a thin wrapper that delegates business logic to the Handler.
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

// NewController creates a new database controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:  cfg.Client,
		Scheme:  cfg.Scheme,
		handler: cfg.Handler,
		logger:  cfg.Logger,
	}
}

// Reconcile implements the reconciliation loop for Database resources.
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("database", req.NamespacedName)

	// 1. Fetch the Database resource
	database := &dbopsv1alpha1.Database{}
	if err := c.Get(ctx, req.NamespacedName, database); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(database) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// 3. Handle deletion
	if util.IsMarkedForDeletion(database) {
		return c.handleDeletion(ctx, database)
	}

	// 4. Add finalizer if not present
	if !controllerutil.ContainsFinalizer(database, util.FinalizerDatabase) {
		controllerutil.AddFinalizer(database, util.FinalizerDatabase)
		if err := c.Update(ctx, database); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 5. Reconcile the database
	return c.reconcile(ctx, database)
}

// reconcile handles the main reconciliation logic.
func (c *Controller) reconcile(ctx context.Context, database *dbopsv1alpha1.Database) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("database", database.Name, "namespace", database.Namespace)

	// 1. Check if database exists
	exists, err := c.handler.Exists(ctx, database.Spec.Name, &database.Spec, database.Namespace)
	if err != nil {
		return c.handleError(ctx, database, err, "check existence")
	}

	// 2. Create if not exists
	if !exists {
		log.Info("Creating database", "name", database.Spec.Name)
		c.updatePhase(ctx, database, dbopsv1alpha1.PhaseCreating, "Creating database")

		_, err := c.handler.Create(ctx, &database.Spec, database.Namespace)
		if err != nil {
			return c.handleError(ctx, database, err, "create database")
		}
	}

	// 3. Verify access
	if err := c.handler.VerifyAccess(ctx, database.Spec.Name, &database.Spec, database.Namespace); err != nil {
		log.Info("Database not yet accepting connections, will retry", "name", database.Spec.Name)
		c.updatePhase(ctx, database, dbopsv1alpha1.PhaseCreating, "Waiting for database to accept connections")
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonCreating, "Database is initializing")
		if statusErr := c.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{RequeueAfter: RequeueAfterCreating}, nil
	}

	// 4. Update settings if needed
	if _, err := c.handler.Update(ctx, database.Spec.Name, &database.Spec, database.Namespace); err != nil {
		log.Error(err, "Failed to update database settings")
		// Don't fail reconciliation for update errors
	}

	// 5. Update metrics
	if err := c.handler.UpdateDatabaseMetrics(ctx, database.Spec.Name, &database.Spec, database.Namespace); err != nil {
		log.Error(err, "Failed to update database metrics")
	}

	// 6. Get database info for status
	info, err := c.handler.GetInfo(ctx, database.Spec.Name, &database.Spec, database.Namespace)
	if err != nil {
		log.Error(err, "Failed to get database info")
	}

	// 7. Update status to Ready
	database.Status.Phase = dbopsv1alpha1.PhaseReady
	database.Status.Message = "Database is ready"

	if info != nil {
		database.Status.Database = &dbopsv1alpha1.DatabaseInfo{
			Name:      info.Name,
			Owner:     info.Owner,
			SizeBytes: info.SizeBytes,
		}
	}

	util.SetSyncedCondition(&database.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Database is synced")
	util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Database is ready")

	if err := c.Status().Update(ctx, database); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(database)

	log.Info("Successfully reconciled Database", "name", database.Name, "database", database.Spec.Name)
	return ctrl.Result{RequeueAfter: RequeueAfterReady}, nil
}

// handleDeletion handles the deletion of a Database resource.
func (c *Controller) handleDeletion(ctx context.Context, database *dbopsv1alpha1.Database) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("database", database.Name, "namespace", database.Namespace)

	if !controllerutil.ContainsFinalizer(database, util.FinalizerDatabase) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of Database")

	// Check deletion policy
	deletionPolicy := database.Spec.DeletionPolicy
	if deletionPolicy == "" {
		deletionPolicy = dbopsv1alpha1.DeletionPolicyRetain
	}

	// Check deletion protection
	if database.Spec.DeletionProtection && !util.HasForceDeleteAnnotation(database) {
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = "Deletion blocked by deletion protection. Set force-delete annotation to proceed."
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, database); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Handle based on deletion policy
	if deletionPolicy == dbopsv1alpha1.DeletionPolicyDelete {
		force := util.HasForceDeleteAnnotation(database)
		if err := c.handler.Delete(ctx, database.Spec.Name, &database.Spec, database.Namespace, force); err != nil {
			log.Error(err, "Failed to delete database")
			if !force {
				// Don't remove finalizer if external deletion fails — prevents data leaks.
				// The resource will be retried until the external deletion succeeds.
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
			// Force delete: continue with finalizer removal despite failure
		} else {
			log.Info("Successfully deleted database from instance")
		}
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(database)

	// Remove finalizer
	controllerutil.RemoveFinalizer(database, util.FinalizerDatabase)
	if err := c.Update(ctx, database); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of Database")
	return ctrl.Result{}, nil
}

// handleError handles errors during reconciliation.
func (c *Controller) handleError(ctx context.Context, database *dbopsv1alpha1.Database, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("database", database.Name, "namespace", database.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)

	database.Status.Phase = dbopsv1alpha1.PhaseFailed
	database.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, database); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(database)

	return reconcileutil.ClassifyRequeue(err)
}

// updatePhase updates the database phase and message.
func (c *Controller) updatePhase(ctx context.Context, database *dbopsv1alpha1.Database, phase dbopsv1alpha1.Phase, message string) {
	database.Status.Phase = phase
	database.Status.Message = message
	if err := c.Status().Update(ctx, database); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.Database{}).
		Named("database").
		Complete(c)
}
