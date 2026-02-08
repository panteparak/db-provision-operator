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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
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
	// DefaultHealthCheckInterval is the default interval for health checks.
	DefaultHealthCheckInterval = 60 * time.Second

	// RequeueAfterError is the requeue duration after an error.
	RequeueAfterError = 30 * time.Second
)

// Controller handles K8s reconciliation for DatabaseInstance resources.
type Controller struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	handler  *Handler
	logger   logr.Logger
}

// ControllerConfig holds dependencies for the controller.
type ControllerConfig struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Handler  *Handler
	Logger   logr.Logger
}

// NewController creates a new instance controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:   cfg.Client,
		Scheme:   cfg.Scheme,
		Recorder: cfg.Recorder,
		handler:  cfg.Handler,
		logger:   cfg.Logger,
	}
}

// Reconcile implements the reconciliation loop for DatabaseInstance resources.
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("instance", req.NamespacedName)

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
	return c.reconcile(ctx, instance)
}

// reconcile handles the main reconciliation logic.
func (c *Controller) reconcile(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (ctrl.Result, error) {
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
		For(&dbopsv1alpha1.DatabaseInstance{}).
		Named("databaseinstance").
		Complete(c)
}

// hasDeletionProtection checks if the instance has deletion protection enabled.
func (c *Controller) hasDeletionProtection(instance *dbopsv1alpha1.DatabaseInstance) bool {
	return instance.Spec.DeletionProtection
}
