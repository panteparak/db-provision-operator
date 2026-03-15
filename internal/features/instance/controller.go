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

package instance

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
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/reconcileutil"
	reconcilecontext "github.com/db-provision-operator/internal/shared/reconcile"
	"github.com/db-provision-operator/internal/util"
)

const (
	// DefaultHealthCheckInterval is the default interval for health checks.
	DefaultHealthCheckInterval = 60 * time.Second

	// RequeueAfterError is the requeue duration after an error.
	RequeueAfterError = 30 * time.Second
)

// Controller handles K8s reconciliation for DatabaseInstance resources.
type Controller struct {
	client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	handler    *Handler
	logger     logr.Logger
	predicates []predicate.Predicate
}

// ControllerConfig holds dependencies for the controller.
type ControllerConfig struct {
	Client     client.Client
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	Handler    *Handler
	Logger     logr.Logger
	Predicates []predicate.Predicate
}

// NewController creates a new instance controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:     cfg.Client,
		Scheme:     cfg.Scheme,
		Recorder:   cfg.Recorder,
		handler:    cfg.Handler,
		logger:     cfg.Logger,
		predicates: cfg.Predicates,
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles,verbs=get;list;delete

// Reconcile implements the reconciliation loop for DatabaseInstance resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("instance", req.NamespacedName)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

	// 1. Fetch the DatabaseInstance resource
	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := c.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 2. Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(instance) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// 3. Handle deletion
	if util.IsMarkedForDeletion(instance) {
		return c.handleDeletion(ctx, instance)
	}

	// 4. Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, util.FinalizerDatabaseInstance) {
		controllerutil.AddFinalizer(instance, util.FinalizerDatabaseInstance)
		if err := c.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 5. Reconcile the instance
	return c.reconcile(ctx, instance, reconcileID)
}

// reconcile handles the main reconciliation logic.
func (c *Controller) reconcile(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, reconcileID string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("instance", instance.Name, "namespace", instance.Namespace)

	// Attempt connection
	result, err := c.handler.Connect(ctx, instance.Name, instance.Namespace)
	if err != nil {
		c.handler.HandleDisconnect(ctx, instance, err.Error())
		c.Recorder.Eventf(instance, corev1.EventTypeWarning, "ConnectionFailed", "Failed to connect to database: %v", err)
		return c.handleError(ctx, instance, err, "connect")
	}

	// Connection successful
	c.handler.HandleHealthCheckPassed(ctx, instance)

	// Update status
	now := metav1.Now()
	instance.Status.Phase = dbopsv1alpha1.PhaseReady
	instance.Status.Message = "Connected successfully"
	instance.Status.Version = result.Version
	instance.Status.LastCheckedAt = &now

	// Set conditions
	util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionTrue,
		util.ReasonConnectionSuccess, "Successfully connected to database")
	util.SetHealthyCondition(&instance.Status.Conditions, metav1.ConditionTrue,
		util.ReasonHealthCheckPassed, "Database is healthy")
	util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Instance is ready")

	// Set reconcileID for end-to-end tracing
	instance.Status.ReconcileID = reconcileID
	instance.Status.LastReconcileTime = &now

	if err := c.Status().Update(ctx, instance); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(instance)

	// Only record event on initial connection or version change
	if instance.Status.ObservedGeneration != instance.Generation {
		c.Recorder.Eventf(instance, corev1.EventTypeNormal, "Connected", "Successfully connected to %s %s", instance.Spec.Engine, result.Version)
		instance.Status.ObservedGeneration = instance.Generation
	}

	log.Info("Successfully reconciled DatabaseInstance",
		"engine", instance.Spec.Engine,
		"version", result.Version)

	// Determine health check interval
	healthCheckInterval := DefaultHealthCheckInterval
	if instance.Spec.HealthCheck != nil && instance.Spec.HealthCheck.IntervalSeconds > 0 {
		healthCheckInterval = time.Duration(instance.Spec.HealthCheck.IntervalSeconds) * time.Second
	}

	return ctrl.Result{RequeueAfter: healthCheckInterval}, nil
}

// handleDeletion handles the deletion of a DatabaseInstance resource.
func (c *Controller) handleDeletion(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("instance", instance.Name, "namespace", instance.Namespace)

	if !controllerutil.ContainsFinalizer(instance, util.FinalizerDatabaseInstance) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseInstance")

	// Check deletion protection
	if c.hasDeletionProtection(instance) && !util.HasForceDeleteAnnotation(instance) {
		c.Recorder.Eventf(instance, corev1.EventTypeWarning, "DeletionBlocked", "Deletion blocked by deletion protection")
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = "Deletion blocked by deletion protection"
		util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, instance); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Check for child dependencies
	hasChildren, msg, children, err := c.hasChildDependencies(ctx, instance)
	if err != nil {
		log.Error(err, "Failed to check child dependencies")
		// Fail-open: proceed without blocking on check errors
	}

	if hasChildren && !util.HasForceDeleteAnnotation(instance) {
		// Normal deletion: block on children
		log.Info("Deletion blocked by child dependencies", "children", children, "message", msg)
		util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDependenciesExist, msg)
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = msg
		if statusErr := c.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		c.Recorder.Eventf(instance, corev1.EventTypeWarning, "DeletionBlocked",
			"Deletion blocked: %s", msg)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if hasChildren && util.HasForceDeleteAnnotation(instance) {
		// Force-delete with children: require confirmation
		if !util.IsForceDeleteConfirmed(instance, children) {
			hash := util.ComputeDeletionHash(children)
			instance.Status.Phase = dbopsv1alpha1.PhasePendingDeletion
			instance.Status.Message = fmt.Sprintf(
				"Force-delete requires confirmation: %d child(ren) will be cascade-deleted. "+
					"Set annotation %s=%s to confirm.",
				len(children), util.AnnotationConfirmForceDelete, hash)
			instance.Status.DeletionConfirmation = &dbopsv1alpha1.DeletionConfirmation{
				Required:       true,
				Hash:           hash,
				Children:       children,
				RemainingCount: len(children),
				Message:        instance.Status.Message,
			}
			util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionFalse,
				util.ReasonPendingDeletionConfirmation, instance.Status.Message)
			if statusErr := c.Status().Update(ctx, instance); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			c.Recorder.Eventf(instance, corev1.EventTypeWarning, "ForceDeletePending",
				"Confirmation required: %d children will be cascade-deleted: %s",
				len(children), strings.Join(children, ", "))
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		// Confirmed — cascade delete children
		remaining, remainingNames, err := c.cascadeDeleteChildren(ctx, instance)
		if err != nil {
			return c.handleError(ctx, instance, err, "cascade delete children")
		}
		if remaining > 0 {
			log.Info("Waiting for children to be deleted", "remaining", remaining)
			instance.Status.Phase = dbopsv1alpha1.PhaseDeleting
			instance.Status.Message = fmt.Sprintf("Cascade-deleting: %d child(ren) remaining", remaining)
			instance.Status.DeletionConfirmation = &dbopsv1alpha1.DeletionConfirmation{
				Required:       false,
				Hash:           util.ComputeDeletionHash(children),
				Children:       remainingNames,
				RemainingCount: remaining,
				Message:        instance.Status.Message,
			}
			util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionFalse,
				util.ReasonCascadeDeleting, instance.Status.Message)
			if statusErr := c.Status().Update(ctx, instance); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		c.Recorder.Eventf(instance, corev1.EventTypeNormal, "CascadeDeleteComplete",
			"All %d children cascade-deleted", len(children))
	}

	// Clear deletion confirmation before final cleanup
	instance.Status.DeletionConfirmation = nil

	// Clean up metrics
	c.handler.CleanupMetrics(instance)

	// Remove finalizer
	controllerutil.RemoveFinalizer(instance, util.FinalizerDatabaseInstance)
	if err := c.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseInstance")
	return ctrl.Result{}, nil
}

// handleError handles errors during reconciliation.
func (c *Controller) handleError(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("instance", instance.Name, "namespace", instance.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)
	c.Recorder.Event(instance, corev1.EventTypeWarning, "ReconcileFailed",
		reconcilecontext.EventMessage(ctx, fmt.Sprintf("Failed to %s: %v", operation, err)))

	instance.Status.Phase = dbopsv1alpha1.PhaseFailed
	instance.Status.Message = err.Error()

	// Set appropriate condition based on operation
	switch operation {
	case "connect":
		util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonConnectionFailed, err.Error())
	default:
		util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonReconcileFailed, err.Error())
	}

	if statusErr := c.Status().Update(ctx, instance); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(instance)

	return reconcileutil.ClassifyRequeue(err)
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseInstance{}).
		Named("databaseinstance").
		WithPredicates(c.predicates...).
		Complete(c)
}

// hasChildDependencies checks if any child resources (Database, DatabaseUser, DatabaseRole)
// reference this instance. Returns true with a descriptive message if children exist.
func (c *Controller) hasChildDependencies(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (bool, string, []string, error) {
	var childTypes []string

	// Check Databases referencing this instance
	var databases dbopsv1alpha1.DatabaseList
	if err := c.List(ctx, &databases, client.InNamespace(instance.Namespace)); err != nil {
		return false, "", nil, fmt.Errorf("list databases: %w", err)
	}
	for _, db := range databases.Items {
		if db.Spec.InstanceRef != nil && db.Spec.InstanceRef.Name == instance.Name {
			ns := db.Spec.InstanceRef.Namespace
			if ns == "" || ns == instance.Namespace {
				childTypes = append(childTypes, "Database/"+db.Name)
			}
		}
	}

	// Check DatabaseUsers referencing this instance
	var users dbopsv1alpha1.DatabaseUserList
	if err := c.List(ctx, &users, client.InNamespace(instance.Namespace)); err != nil {
		return false, "", nil, fmt.Errorf("list users: %w", err)
	}
	for _, u := range users.Items {
		if u.Spec.InstanceRef != nil && u.Spec.InstanceRef.Name == instance.Name {
			ns := u.Spec.InstanceRef.Namespace
			if ns == "" || ns == instance.Namespace {
				childTypes = append(childTypes, "DatabaseUser/"+u.Name)
			}
		}
	}

	// Check DatabaseRoles referencing this instance
	var roles dbopsv1alpha1.DatabaseRoleList
	if err := c.List(ctx, &roles, client.InNamespace(instance.Namespace)); err != nil {
		return false, "", nil, fmt.Errorf("list roles: %w", err)
	}
	for _, r := range roles.Items {
		if r.Spec.InstanceRef != nil && r.Spec.InstanceRef.Name == instance.Name {
			ns := r.Spec.InstanceRef.Namespace
			if ns == "" || ns == instance.Namespace {
				childTypes = append(childTypes, "DatabaseRole/"+r.Name)
			}
		}
	}

	if len(childTypes) == 0 {
		return false, "", nil, nil
	}

	msg := fmt.Sprintf("cannot delete: %d child resource(s) still reference this instance: %s. "+
		"Delete children first or use force-delete annotation", len(childTypes), strings.Join(childTypes, ", "))
	return true, msg, childTypes, nil
}

// cascadeDeleteChildren deletes all child resources (Database, DatabaseUser, DatabaseRole)
// that reference this instance and returns the count of children still remaining.
func (c *Controller) cascadeDeleteChildren(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (int, []string, error) {
	var remaining []string

	// Delete Databases referencing this instance
	var databases dbopsv1alpha1.DatabaseList
	if err := c.List(ctx, &databases, client.InNamespace(instance.Namespace)); err != nil {
		return 0, nil, fmt.Errorf("list databases: %w", err)
	}
	for i := range databases.Items {
		db := &databases.Items[i]
		if db.Spec.InstanceRef != nil && db.Spec.InstanceRef.Name == instance.Name {
			ns := db.Spec.InstanceRef.Namespace
			if ns == "" || ns == instance.Namespace {
				if err := c.Delete(ctx, db); err != nil {
					if !apierrors.IsNotFound(err) {
						return 0, nil, fmt.Errorf("delete database %s: %w", db.Name, err)
					}
				}
				// Check if still exists (may have finalizers)
				if err := c.Get(ctx, client.ObjectKeyFromObject(db), db); err == nil {
					remaining = append(remaining, "Database/"+db.Name)
				}
			}
		}
	}

	// Delete DatabaseUsers referencing this instance
	var users dbopsv1alpha1.DatabaseUserList
	if err := c.List(ctx, &users, client.InNamespace(instance.Namespace)); err != nil {
		return 0, nil, fmt.Errorf("list users: %w", err)
	}
	for i := range users.Items {
		u := &users.Items[i]
		if u.Spec.InstanceRef != nil && u.Spec.InstanceRef.Name == instance.Name {
			ns := u.Spec.InstanceRef.Namespace
			if ns == "" || ns == instance.Namespace {
				if err := c.Delete(ctx, u); err != nil {
					if !apierrors.IsNotFound(err) {
						return 0, nil, fmt.Errorf("delete user %s: %w", u.Name, err)
					}
				}
				if err := c.Get(ctx, client.ObjectKeyFromObject(u), u); err == nil {
					remaining = append(remaining, "DatabaseUser/"+u.Name)
				}
			}
		}
	}

	// Delete DatabaseRoles referencing this instance
	var roles dbopsv1alpha1.DatabaseRoleList
	if err := c.List(ctx, &roles, client.InNamespace(instance.Namespace)); err != nil {
		return 0, nil, fmt.Errorf("list roles: %w", err)
	}
	for i := range roles.Items {
		r := &roles.Items[i]
		if r.Spec.InstanceRef != nil && r.Spec.InstanceRef.Name == instance.Name {
			ns := r.Spec.InstanceRef.Namespace
			if ns == "" || ns == instance.Namespace {
				if err := c.Delete(ctx, r); err != nil {
					if !apierrors.IsNotFound(err) {
						return 0, nil, fmt.Errorf("delete role %s: %w", r.Name, err)
					}
				}
				if err := c.Get(ctx, client.ObjectKeyFromObject(r), r); err == nil {
					remaining = append(remaining, "DatabaseRole/"+r.Name)
				}
			}
		}
	}

	return len(remaining), remaining, nil
}

// hasDeletionProtection checks if the instance has deletion protection enabled.
func (c *Controller) hasDeletionProtection(instance *dbopsv1alpha1.DatabaseInstance) bool {
	return instance.Spec.DeletionProtection
}
