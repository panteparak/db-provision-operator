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

package role

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
	RequeueAfterReady   = 5 * time.Minute
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second

	// AnnotationDeletionPolicy specifies the deletion policy via annotation
	AnnotationDeletionPolicy = "dbops.dbprovision.io/deletion-policy"

	// AnnotationDeletionProtection enables deletion protection via annotation
	AnnotationDeletionProtection = "dbops.dbprovision.io/deletion-protection"
)

// Controller handles K8s reconciliation for DatabaseRole resources.
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

// NewController creates a new role controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:  cfg.Client,
		Scheme:  cfg.Scheme,
		handler: cfg.Handler,
		logger:  cfg.Logger,
	}
}

// Reconcile implements the reconciliation loop for DatabaseRole resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := c.logger.WithValues("role", req.NamespacedName)

	// Fetch the DatabaseRole resource
	role := &dbopsv1alpha1.DatabaseRole{}
	if err := c.Get(ctx, req.NamespacedName, role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if util.ShouldSkipReconcile(role) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	if util.IsMarkedForDeletion(role) {
		return c.handleDeletion(ctx, role)
	}

	if !controllerutil.ContainsFinalizer(role, util.FinalizerDatabaseRole) {
		controllerutil.AddFinalizer(role, util.FinalizerDatabaseRole)
		if err := c.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
	}

	return c.reconcile(ctx, role)
}

func (c *Controller) reconcile(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (ctrl.Result, error) {
	log := c.logger.WithValues("role", role.Name, "namespace", role.Namespace)

	// Check if role exists
	exists, err := c.handler.Exists(ctx, role.Spec.RoleName, &role.Spec, role.Namespace)
	if err != nil {
		return c.handleError(ctx, role, err, "check existence")
	}

	// Create role if not exists
	if !exists {
		log.Info("Creating role", "roleName", role.Spec.RoleName)
		c.updatePhase(ctx, role, dbopsv1alpha1.PhaseCreating, "Creating role")

		_, err := c.handler.Create(ctx, &role.Spec, role.Namespace)
		if err != nil {
			return c.handleError(ctx, role, err, "create role")
		}
	} else {
		// Update role settings if role already exists
		if _, err := c.handler.Update(ctx, role.Spec.RoleName, &role.Spec, role.Namespace); err != nil {
			log.Error(err, "Failed to update role settings")
		}
	}

	// Update status
	role.Status.Phase = dbopsv1alpha1.PhaseReady
	role.Status.Message = "Role is ready"
	role.Status.ObservedGeneration = role.Generation

	if role.Status.Role == nil {
		role.Status.Role = &dbopsv1alpha1.RoleInfo{}
	}
	role.Status.Role.Name = role.Spec.RoleName
	if role.Status.Role.CreatedAt == nil {
		now := metav1.Now()
		role.Status.Role.CreatedAt = &now
	}

	util.SetSyncedCondition(&role.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Role is synced")
	util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Role is ready")

	if err := c.Status().Update(ctx, role); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(role)

	log.Info("Successfully reconciled DatabaseRole", "roleName", role.Spec.RoleName)
	return ctrl.Result{RequeueAfter: RequeueAfterReady}, nil
}

func (c *Controller) handleDeletion(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (ctrl.Result, error) {
	log := c.logger.WithValues("role", role.Name, "namespace", role.Namespace)

	if !controllerutil.ContainsFinalizer(role, util.FinalizerDatabaseRole) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseRole")

	// Get deletion policy from annotation (default to Retain)
	deletionPolicy := c.getDeletionPolicy(role)

	// Check deletion protection
	if c.hasDeletionProtection(role) && !util.HasForceDeleteAnnotation(role) {
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = "Deletion blocked by deletion protection"
		util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, role); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	if deletionPolicy == string(dbopsv1alpha1.DeletionPolicyDelete) {
		force := util.HasForceDeleteAnnotation(role)
		if err := c.handler.Delete(ctx, role.Spec.RoleName, &role.Spec, role.Namespace, force); err != nil {
			// Log the error but continue with finalizer removal
			// This prevents the CR from being stuck if the database is unreachable
			log.Error(err, "Failed to delete role from database")
			if !errors.IsNotFound(err) && !force {
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
		}
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(role)

	controllerutil.RemoveFinalizer(role, util.FinalizerDatabaseRole)
	if err := c.Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseRole")
	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, role *dbopsv1alpha1.DatabaseRole, err error, operation string) (ctrl.Result, error) {
	log := c.logger.WithValues("role", role.Name, "namespace", role.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)

	role.Status.Phase = dbopsv1alpha1.PhaseFailed
	role.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, role); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(role)

	return ctrl.Result{RequeueAfter: RequeueAfterError}, err
}

func (c *Controller) updatePhase(ctx context.Context, role *dbopsv1alpha1.DatabaseRole, phase dbopsv1alpha1.Phase, message string) {
	role.Status.Phase = phase
	role.Status.Message = message
	if err := c.Status().Update(ctx, role); err != nil {
		c.logger.Error(err, "Failed to update phase", "phase", phase)
	}
}

// getDeletionPolicy returns the deletion policy from annotation or default to Retain.
func (c *Controller) getDeletionPolicy(role *dbopsv1alpha1.DatabaseRole) string {
	annotations := role.GetAnnotations()
	if annotations == nil {
		return string(dbopsv1alpha1.DeletionPolicyRetain)
	}
	policy := annotations[AnnotationDeletionPolicy]
	if policy == "" {
		return string(dbopsv1alpha1.DeletionPolicyRetain)
	}
	return policy
}

// hasDeletionProtection checks if deletion protection is enabled via annotation.
func (c *Controller) hasDeletionProtection(role *dbopsv1alpha1.DatabaseRole) bool {
	annotations := role.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationDeletionProtection] == "true"
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseRole{}).
		Named("databaserole-feature").
		Complete(c)
}
