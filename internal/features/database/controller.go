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
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	controllerdrift "github.com/db-provision-operator/internal/controller/drift"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/reconcileutil"
	reconcilecontext "github.com/db-provision-operator/internal/shared/reconcile"
	"github.com/db-provision-operator/internal/util"
)

const (
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
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	handler              *Handler
	logger               logr.Logger
	defaultDriftInterval time.Duration
	predicates           []predicate.Predicate
	driftOrchestrator    *controllerdrift.Orchestrator
}

// ControllerConfig holds dependencies for the controller.
type ControllerConfig struct {
	Client               client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	Handler              *Handler
	Logger               logr.Logger
	DefaultDriftInterval time.Duration
	Predicates           []predicate.Predicate
}

// NewController creates a new database controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:               cfg.Client,
		Scheme:               cfg.Scheme,
		Recorder:             cfg.Recorder,
		handler:              cfg.Handler,
		logger:               cfg.Logger,
		defaultDriftInterval: cfg.DefaultDriftInterval,
		predicates:           cfg.Predicates,
		driftOrchestrator: &controllerdrift.Orchestrator{
			Recorder:             cfg.Recorder,
			DefaultDriftInterval: cfg.DefaultDriftInterval,
		},
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants,verbs=list

// Reconcile implements the reconciliation loop for Database resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("database", req.NamespacedName)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

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
	return c.reconcile(ctx, database, reconcileID)
}

// reconcile handles the main reconciliation logic.
func (c *Controller) reconcile(ctx context.Context, database *dbopsv1alpha1.Database, reconcileID string) (ctrl.Result, error) {
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

		createResult, err := c.handler.Create(ctx, &database.Spec, database.Namespace)
		if err != nil {
			return c.handleError(ctx, database, err, "create database")
		}

		// Store ownership status if provisioned
		if createResult != nil && createResult.Ownership != nil {
			if database.Status.Postgres == nil {
				database.Status.Postgres = &dbopsv1alpha1.PostgresDatabaseStatus{}
			}
			database.Status.Postgres.Ownership = &dbopsv1alpha1.PostgresOwnershipStatus{
				RoleName: createResult.Ownership.RoleName,
				UserName: createResult.Ownership.UserName,
				Phase:    "Ready",
			}
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

	// 4.5 Apply init SQL if configured
	var initSQLFailed bool
	if database.Spec.InitSQL != nil {
		initResult, initErr := c.handler.ApplyInitSQL(ctx, database)
		if initResult != nil {
			if initResult.Skipped {
				// Ensure status consistency even if it was externally cleared
				if database.Status.InitSQL == nil {
					database.Status.InitSQL = &dbopsv1alpha1.InitSQLStatus{
						Applied: true,
						Hash:    initResult.Hash,
					}
				}
			} else {
				now := metav1.Now()
				database.Status.InitSQL = &dbopsv1alpha1.InitSQLStatus{
					Applied:            initResult.Applied,
					Hash:               initResult.Hash,
					StatementsExecuted: initResult.StatementsExecuted,
				}
				if initResult.Applied {
					database.Status.InitSQL.AppliedAt = &now
				}
				if initErr != nil {
					database.Status.InitSQL.Error = initErr.Error()
				}
			}
		}
		if initErr != nil {
			initSQLFailed = true
			util.SetSyncedCondition(&database.Status.Conditions, metav1.ConditionFalse,
				util.ReasonInitSQLFailed, fmt.Sprintf("Init SQL failed: %v", initErr))
			if database.Spec.InitSQL.FailurePolicy == dbopsv1alpha1.InitSQLFailurePolicyBlock {
				return c.handleError(ctx, database, initErr, "apply init SQL")
			}
			log.Error(initErr, "Init SQL failed, continuing per failure policy")
		}
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

	// 7. Resolve instance for drift detection
	resolved, err := c.handler.ResolveInstance(ctx, &database.Spec, database.Namespace)
	if err != nil {
		log.Error(err, "Failed to resolve instance for drift detection")
		// Continue anyway, drift detection will use defaults
	}

	// 8. Perform drift detection
	c.driftOrchestrator.PerformDriftDetection(ctx,
		&databaseDriftableResource{database},
		&controllerdrift.ResolvedInstancePolicy{Resolved: resolved},
		&databaseDriftDetector{handler: c.handler, spec: &database.Spec, namespace: database.Namespace},
	)

	// 9. Update status to Ready
	database.Status.Phase = dbopsv1alpha1.PhaseReady
	database.Status.Message = "Database is ready"

	if info != nil {
		database.Status.Database = &dbopsv1alpha1.DatabaseInfo{
			Name:      info.Name,
			Owner:     info.Owner,
			SizeBytes: info.SizeBytes,
		}
	}

	if !initSQLFailed {
		util.SetSyncedCondition(&database.Status.Conditions, metav1.ConditionTrue,
			util.ReasonReconcileSuccess, "Database is synced")
	}
	util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Database is ready")

	// Set reconcileID for end-to-end tracing
	now := metav1.Now()
	database.Status.ReconcileID = reconcileID
	database.Status.LastReconcileTime = &now

	if err := c.Status().Update(ctx, database); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(database)

	log.Info("Successfully reconciled Database", "name", database.Name, "database", database.Spec.Name)
	return ctrl.Result{RequeueAfter: c.driftOrchestrator.GetRequeueInterval(
		&databaseDriftableResource{database},
		&controllerdrift.ResolvedInstancePolicy{Resolved: resolved},
	)}, nil
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

	// Check for child dependencies
	hasChildren, msg, children, err := c.hasChildDependencies(ctx, database)
	if err != nil {
		log.Error(err, "Failed to check child dependencies")
		// Fail-open: proceed without blocking on check errors
	}

	if hasChildren && !util.HasForceDeleteAnnotation(database) {
		// Normal deletion: block on children
		log.Info("Deletion blocked by child dependencies", "children", children, "message", msg)
		util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDependenciesExist, msg)
		database.Status.Phase = dbopsv1alpha1.PhaseFailed
		database.Status.Message = msg
		if statusErr := c.Status().Update(ctx, database); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		c.Recorder.Eventf(database, corev1.EventTypeWarning, "DeletionBlocked",
			"Deletion blocked: %s", msg)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if hasChildren && util.HasForceDeleteAnnotation(database) {
		// Force-delete with children: require confirmation
		if !util.IsForceDeleteConfirmed(database, children) {
			hash := util.ComputeDeletionHash(children)
			database.Status.Phase = dbopsv1alpha1.PhasePendingDeletion
			database.Status.Message = fmt.Sprintf(
				"Force-delete requires confirmation: %d child(ren) will be cascade-deleted. "+
					"Set annotation %s=%s to confirm.",
				len(children), util.AnnotationConfirmForceDelete, hash)
			database.Status.DeletionConfirmation = &dbopsv1alpha1.DeletionConfirmation{
				Required:       true,
				Hash:           hash,
				Children:       children,
				RemainingCount: len(children),
				Message:        database.Status.Message,
			}
			util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
				util.ReasonPendingDeletionConfirmation, database.Status.Message)
			if statusErr := c.Status().Update(ctx, database); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			c.Recorder.Eventf(database, corev1.EventTypeWarning, "ForceDeletePending",
				"Confirmation required: %d children will be cascade-deleted: %s",
				len(children), strings.Join(children, ", "))
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		// Confirmed — cascade delete children
		remaining, remainingNames, err := c.cascadeDeleteChildren(ctx, database)
		if err != nil {
			return c.handleError(ctx, database, err, "cascade delete children")
		}
		if remaining > 0 {
			log.Info("Waiting for children to be deleted", "remaining", remaining)
			database.Status.Phase = dbopsv1alpha1.PhaseDeleting
			database.Status.Message = fmt.Sprintf("Cascade-deleting: %d child(ren) remaining", remaining)
			database.Status.DeletionConfirmation = &dbopsv1alpha1.DeletionConfirmation{
				Required:       false,
				Hash:           util.ComputeDeletionHash(children),
				Children:       remainingNames,
				RemainingCount: remaining,
				Message:        database.Status.Message,
			}
			util.SetReadyCondition(&database.Status.Conditions, metav1.ConditionFalse,
				util.ReasonCascadeDeleting, database.Status.Message)
			if statusErr := c.Status().Update(ctx, database); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		c.Recorder.Eventf(database, corev1.EventTypeNormal, "CascadeDeleteComplete",
			"All %d children cascade-deleted", len(children))
	}

	// Clear deletion confirmation before proceeding
	database.Status.DeletionConfirmation = nil

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

	c.Recorder.Event(database, corev1.EventTypeWarning, "ReconcileFailed",
		reconcilecontext.EventMessage(ctx, fmt.Sprintf("Failed to %s: %v", operation, err)))

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
		WithPredicates(c.predicates...).
		Complete(c)
}

// hasChildDependencies checks if any DatabaseGrant resources reference this database.
// Returns true with a descriptive message if children exist.
func (c *Controller) hasChildDependencies(ctx context.Context, database *dbopsv1alpha1.Database) (bool, string, []string, error) {
	var childNames []string

	var grants dbopsv1alpha1.DatabaseGrantList
	if err := c.List(ctx, &grants, client.InNamespace(database.Namespace)); err != nil {
		return false, "", nil, fmt.Errorf("list grants: %w", err)
	}
	for _, g := range grants.Items {
		if g.Spec.DatabaseRef != nil && g.Spec.DatabaseRef.Name == database.Name {
			ns := g.Spec.DatabaseRef.Namespace
			if ns == "" || ns == database.Namespace {
				childNames = append(childNames, "DatabaseGrant/"+g.Name)
			}
		}
	}

	if len(childNames) == 0 {
		return false, "", nil, nil
	}

	msg := fmt.Sprintf("cannot delete: %d DatabaseGrant(s) still reference this database: %s. "+
		"Delete grants first or use force-delete annotation", len(childNames), strings.Join(childNames, ", "))
	return true, msg, childNames, nil
}

// cascadeDeleteChildren deletes all DatabaseGrant CRs referencing this database.
// Returns the count of children still existing (pending finalizer removal by their controllers).
func (c *Controller) cascadeDeleteChildren(ctx context.Context, database *dbopsv1alpha1.Database) (int, []string, error) {
	var remaining []string

	var grants dbopsv1alpha1.DatabaseGrantList
	if err := c.List(ctx, &grants, client.InNamespace(database.Namespace)); err != nil {
		return 0, nil, fmt.Errorf("list grants: %w", err)
	}
	for i := range grants.Items {
		g := &grants.Items[i]
		if g.Spec.DatabaseRef != nil && g.Spec.DatabaseRef.Name == database.Name {
			ns := g.Spec.DatabaseRef.Namespace
			if ns == "" || ns == database.Namespace {
				if err := c.Delete(ctx, g); err != nil {
					if !apierrors.IsNotFound(err) {
						return 0, nil, fmt.Errorf("delete grant %s: %w", g.Name, err)
					}
				}
				if err := c.Get(ctx, client.ObjectKeyFromObject(g), g); err == nil {
					remaining = append(remaining, "DatabaseGrant/"+g.Name)
				}
			}
		}
	}

	return len(remaining), remaining, nil
}
