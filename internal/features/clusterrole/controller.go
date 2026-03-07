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

package clusterrole

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second

	// AnnotationDeletionPolicy specifies the deletion policy via annotation
	AnnotationDeletionPolicy = "dbops.dbprovision.io/deletion-policy"

	// AnnotationDeletionProtection enables deletion protection via annotation
	AnnotationDeletionProtection = "dbops.dbprovision.io/deletion-protection"
)

// Controller handles K8s reconciliation for ClusterDatabaseRole resources.
type Controller struct {
	client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	handler              *Handler
	defaultDriftInterval time.Duration
	predicates           []predicate.Predicate
	logger               logr.Logger
	driftOrchestrator    *controllerdrift.Orchestrator
}

// ControllerConfig holds dependencies for the controller.
type ControllerConfig struct {
	Client               client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	Handler              *Handler
	DefaultDriftInterval time.Duration
	Predicates           []predicate.Predicate
	Logger               logr.Logger
}

// NewController creates a new cluster role controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:               cfg.Client,
		Scheme:               cfg.Scheme,
		Recorder:             cfg.Recorder,
		handler:              cfg.Handler,
		defaultDriftInterval: cfg.DefaultDriftInterval,
		predicates:           cfg.Predicates,
		logger:               cfg.Logger,
		driftOrchestrator: &controllerdrift.Orchestrator{
			Recorder:             cfg.Recorder,
			DefaultDriftInterval: cfg.DefaultDriftInterval,
		},
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseroles/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for ClusterDatabaseRole resources.
// Note: ClusterDatabaseRole is cluster-scoped, so req.Name is used (no namespace).
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("clusterRole", req.Name)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

	// Fetch the ClusterDatabaseRole resource (cluster-scoped, no namespace)
	role := &dbopsv1alpha1.ClusterDatabaseRole{}
	if err := c.Get(ctx, client.ObjectKey{Name: req.Name}, role); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if util.ShouldSkipReconcile(role) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	if util.IsMarkedForDeletion(role) {
		return c.handleDeletion(ctx, role)
	}

	if !controllerutil.ContainsFinalizer(role, util.FinalizerClusterDatabaseRole) {
		controllerutil.AddFinalizer(role, util.FinalizerClusterDatabaseRole)
		if err := c.Update(ctx, role); err != nil {
			return ctrl.Result{}, err
		}
	}

	return c.reconcile(ctx, role, reconcileID)
}

func (c *Controller) reconcile(ctx context.Context, role *dbopsv1alpha1.ClusterDatabaseRole, reconcileID string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterRole", role.Name)

	// Check if role exists
	exists, err := c.handler.Exists(ctx, role.Spec.RoleName, &role.Spec)
	if err != nil {
		return c.handleError(ctx, role, err, "check existence")
	}

	// Create role if not exists
	if !exists {
		log.Info("Creating cluster role", "roleName", role.Spec.RoleName)
		c.updatePhase(ctx, role, dbopsv1alpha1.PhaseCreating, "Creating cluster role")

		_, err := c.handler.Create(ctx, &role.Spec)
		if err != nil {
			return c.handleError(ctx, role, err, "create cluster role")
		}
		c.Recorder.Eventf(role, corev1.EventTypeNormal, "Created", "ClusterRole %s created successfully", role.Spec.RoleName)
	} else {
		// Update role settings if role already exists
		if _, err := c.handler.Update(ctx, role.Spec.RoleName, &role.Spec); err != nil {
			log.Error(err, "Failed to update cluster role settings")
		}
	}

	// Get instance for drift detection
	instance, err := c.handler.GetInstance(ctx, &role.Spec)
	if err != nil {
		log.Error(err, "Failed to get instance for drift detection")
		// Continue anyway, drift detection will use defaults
	}

	// Perform drift detection
	c.driftOrchestrator.PerformDriftDetection(ctx,
		&clusterRoleDriftableResource{role},
		&controllerdrift.ClusterInstancePolicy{Instance: instance},
		&clusterRoleDriftDetector{handler: c.handler, spec: &role.Spec},
	)

	// Update status
	now := metav1.Now()
	role.Status.Phase = dbopsv1alpha1.PhaseReady
	role.Status.Message = "Cluster role is ready"
	role.Status.ObservedGeneration = role.Generation
	role.Status.ReconcileID = reconcileID
	role.Status.LastReconcileTime = &now

	if role.Status.Role == nil {
		role.Status.Role = &dbopsv1alpha1.RoleInfo{}
	}
	role.Status.Role.Name = role.Spec.RoleName
	if role.Status.Role.CreatedAt == nil {
		role.Status.Role.CreatedAt = &now
	}

	util.SetSyncedCondition(&role.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Cluster role is synced")
	util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Cluster role is ready")

	if err := c.Status().Update(ctx, role); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(role)

	log.Info("Successfully reconciled ClusterDatabaseRole", "roleName", role.Spec.RoleName)
	return ctrl.Result{RequeueAfter: c.driftOrchestrator.GetRequeueInterval(
		&clusterRoleDriftableResource{role},
		&controllerdrift.ClusterInstancePolicy{Instance: instance},
	)}, nil
}

func (c *Controller) handleDeletion(ctx context.Context, role *dbopsv1alpha1.ClusterDatabaseRole) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterRole", role.Name)

	if !controllerutil.ContainsFinalizer(role, util.FinalizerClusterDatabaseRole) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of ClusterDatabaseRole")

	// Get deletion policy from annotation (default to Retain)
	deletionPolicy := c.getDeletionPolicy(role)

	// Check deletion protection
	if c.hasDeletionProtection(role) && !util.HasForceDeleteAnnotation(role) {
		c.Recorder.Eventf(role, corev1.EventTypeWarning, "DeletionBlocked", "Deletion blocked by deletion protection annotation")
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
		if err := c.handler.Delete(ctx, role.Spec.RoleName, &role.Spec, force); err != nil {
			// Log the error but continue with finalizer removal
			// This prevents the CR from being stuck if the database is unreachable
			log.Error(err, "Failed to delete cluster role from database")
			c.Recorder.Eventf(role, corev1.EventTypeWarning, "DeleteFailed", "Failed to delete cluster role: %v", err)
			if !errors.IsNotFound(err) && !force {
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
		} else {
			c.Recorder.Eventf(role, corev1.EventTypeNormal, "Deleted", "ClusterRole %s deleted successfully", role.Spec.RoleName)
		}
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(role)

	controllerutil.RemoveFinalizer(role, util.FinalizerClusterDatabaseRole)
	if err := c.Update(ctx, role); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of ClusterDatabaseRole")
	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, role *dbopsv1alpha1.ClusterDatabaseRole, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterRole", role.Name)

	log.Error(err, "Reconciliation failed", "operation", operation)
	c.Recorder.Event(role, corev1.EventTypeWarning, "ReconcileFailed",
		reconcilecontext.EventMessage(ctx, fmt.Sprintf("Failed to %s: %v", operation, err)))

	role.Status.Phase = dbopsv1alpha1.PhaseFailed
	role.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, role); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(role)

	return reconcileutil.ClassifyRequeue(err)
}

func (c *Controller) updatePhase(ctx context.Context, role *dbopsv1alpha1.ClusterDatabaseRole, phase dbopsv1alpha1.Phase, message string) {
	role.Status.Phase = phase
	role.Status.Message = message
	if err := c.Status().Update(ctx, role); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// getDeletionPolicy returns the deletion policy from annotation or default to Retain.
func (c *Controller) getDeletionPolicy(role *dbopsv1alpha1.ClusterDatabaseRole) string {
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
func (c *Controller) hasDeletionProtection(role *dbopsv1alpha1.ClusterDatabaseRole) bool {
	annotations := role.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationDeletionProtection] == "true"
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.ClusterDatabaseRole{}).
		Named("clusterdatabaserole-feature").
		WithPredicates(c.predicates...).
		Complete(c)
}
