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

package clusterinstance

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
	"github.com/db-provision-operator/internal/util"
)

const (
	// FinalizerClusterDatabaseInstance is the finalizer for cluster database instances.
	FinalizerClusterDatabaseInstance = "dbops.dbprovision.io/clusterdatabaseinstance-finalizer"

	// DefaultHealthCheckInterval is the default interval for health checks.
	DefaultHealthCheckInterval = 60 * time.Second

	// RequeueAfterError is the requeue duration after an error.
	RequeueAfterError = 30 * time.Second
)

// Controller handles K8s reconciliation for ClusterDatabaseInstance resources.
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

// NewController creates a new cluster instance controller.
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

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=list;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=list;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles,verbs=list;delete

// Reconcile implements the reconciliation loop for ClusterDatabaseInstance resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Note: For cluster-scoped resources, req.Namespace is empty
	log := logf.FromContext(ctx).WithValues("clusterinstance", req.Name)

	// 1. Fetch the ClusterDatabaseInstance resource
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{}
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
	if !controllerutil.ContainsFinalizer(instance, FinalizerClusterDatabaseInstance) {
		controllerutil.AddFinalizer(instance, FinalizerClusterDatabaseInstance)
		if err := c.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 5. Reconcile the instance
	return c.reconcile(ctx, instance)
}

// reconcile handles the main reconciliation logic.
func (c *Controller) reconcile(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterinstance", instance.Name)

	// Attempt connection
	result, err := c.handler.Connect(ctx, instance.Name)
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
		util.ReasonReconcileSuccess, "Cluster instance is ready")

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

	log.Info("Successfully reconciled ClusterDatabaseInstance",
		"engine", instance.Spec.Engine,
		"version", result.Version)

	// Determine health check interval
	healthCheckInterval := DefaultHealthCheckInterval
	if instance.Spec.HealthCheck != nil && instance.Spec.HealthCheck.IntervalSeconds > 0 {
		healthCheckInterval = time.Duration(instance.Spec.HealthCheck.IntervalSeconds) * time.Second
	}

	return ctrl.Result{RequeueAfter: healthCheckInterval}, nil
}

// handleDeletion handles the deletion of a ClusterDatabaseInstance resource.
func (c *Controller) handleDeletion(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterinstance", instance.Name)

	if !controllerutil.ContainsFinalizer(instance, FinalizerClusterDatabaseInstance) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of ClusterDatabaseInstance")

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

	// Clear deletion confirmation before proceeding
	instance.Status.DeletionConfirmation = nil

	// Clean up metrics
	c.handler.CleanupMetrics(instance)

	// Remove finalizer
	controllerutil.RemoveFinalizer(instance, FinalizerClusterDatabaseInstance)
	if err := c.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of ClusterDatabaseInstance")
	return ctrl.Result{}, nil
}

// handleError handles errors during reconciliation.
func (c *Controller) handleError(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterinstance", instance.Name)

	log.Error(err, "Reconciliation failed", "operation", operation)
	c.Recorder.Eventf(instance, corev1.EventTypeWarning, "ReconcileFailed", "Failed to %s: %v", operation, err)

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
		For(&dbopsv1alpha1.ClusterDatabaseInstance{}).
		Named("clusterdatabaseinstance").
		WithPredicates(c.predicates...).
		Complete(c)
}

// hasDeletionProtection checks if the instance has deletion protection enabled.
func (c *Controller) hasDeletionProtection(instance *dbopsv1alpha1.ClusterDatabaseInstance) bool {
	return instance.Spec.DeletionProtection
}

// hasChildDependencies checks if any child resources (Database, DatabaseUser, DatabaseRole)
// reference this cluster instance via clusterInstanceRef. Lists across all namespaces.
func (c *Controller) hasChildDependencies(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (bool, string, []string, error) {
	var childTypes []string

	var databases dbopsv1alpha1.DatabaseList
	if err := c.List(ctx, &databases); err != nil {
		return false, "", nil, fmt.Errorf("list databases: %w", err)
	}
	for _, db := range databases.Items {
		if db.Spec.ClusterInstanceRef != nil && db.Spec.ClusterInstanceRef.Name == instance.Name {
			childTypes = append(childTypes, "Database/"+db.Namespace+"/"+db.Name)
		}
	}

	var users dbopsv1alpha1.DatabaseUserList
	if err := c.List(ctx, &users); err != nil {
		return false, "", nil, fmt.Errorf("list users: %w", err)
	}
	for _, u := range users.Items {
		if u.Spec.ClusterInstanceRef != nil && u.Spec.ClusterInstanceRef.Name == instance.Name {
			childTypes = append(childTypes, "DatabaseUser/"+u.Namespace+"/"+u.Name)
		}
	}

	var roles dbopsv1alpha1.DatabaseRoleList
	if err := c.List(ctx, &roles); err != nil {
		return false, "", nil, fmt.Errorf("list roles: %w", err)
	}
	for _, r := range roles.Items {
		if r.Spec.ClusterInstanceRef != nil && r.Spec.ClusterInstanceRef.Name == instance.Name {
			childTypes = append(childTypes, "DatabaseRole/"+r.Namespace+"/"+r.Name)
		}
	}

	if len(childTypes) == 0 {
		return false, "", nil, nil
	}

	msg := fmt.Sprintf("cannot delete: %d child resource(s) still reference this cluster instance: %s. "+
		"Delete children first or use force-delete annotation", len(childTypes), strings.Join(childTypes, ", "))
	return true, msg, childTypes, nil
}

// cascadeDeleteChildren deletes all child CRs referencing this cluster instance.
func (c *Controller) cascadeDeleteChildren(ctx context.Context, instance *dbopsv1alpha1.ClusterDatabaseInstance) (int, []string, error) {
	var remaining []string

	var databases dbopsv1alpha1.DatabaseList
	if err := c.List(ctx, &databases); err != nil {
		return 0, nil, fmt.Errorf("list databases: %w", err)
	}
	for i := range databases.Items {
		db := &databases.Items[i]
		if db.Spec.ClusterInstanceRef != nil && db.Spec.ClusterInstanceRef.Name == instance.Name {
			if err := c.Delete(ctx, db); err != nil {
				if !apierrors.IsNotFound(err) {
					return 0, nil, fmt.Errorf("delete database %s/%s: %w", db.Namespace, db.Name, err)
				}
			}
			if err := c.Get(ctx, client.ObjectKeyFromObject(db), db); err == nil {
				remaining = append(remaining, "Database/"+db.Namespace+"/"+db.Name)
			}
		}
	}

	var users dbopsv1alpha1.DatabaseUserList
	if err := c.List(ctx, &users); err != nil {
		return 0, nil, fmt.Errorf("list users: %w", err)
	}
	for i := range users.Items {
		u := &users.Items[i]
		if u.Spec.ClusterInstanceRef != nil && u.Spec.ClusterInstanceRef.Name == instance.Name {
			if err := c.Delete(ctx, u); err != nil {
				if !apierrors.IsNotFound(err) {
					return 0, nil, fmt.Errorf("delete user %s/%s: %w", u.Namespace, u.Name, err)
				}
			}
			if err := c.Get(ctx, client.ObjectKeyFromObject(u), u); err == nil {
				remaining = append(remaining, "DatabaseUser/"+u.Namespace+"/"+u.Name)
			}
		}
	}

	var roles dbopsv1alpha1.DatabaseRoleList
	if err := c.List(ctx, &roles); err != nil {
		return 0, nil, fmt.Errorf("list roles: %w", err)
	}
	for i := range roles.Items {
		r := &roles.Items[i]
		if r.Spec.ClusterInstanceRef != nil && r.Spec.ClusterInstanceRef.Name == instance.Name {
			if err := c.Delete(ctx, r); err != nil {
				if !apierrors.IsNotFound(err) {
					return 0, nil, fmt.Errorf("delete role %s/%s: %w", r.Namespace, r.Name, err)
				}
			}
			if err := c.Get(ctx, client.ObjectKeyFromObject(r), r); err == nil {
				remaining = append(remaining, "DatabaseRole/"+r.Namespace+"/"+r.Name)
			}
		}
	}

	return len(remaining), remaining, nil
}
