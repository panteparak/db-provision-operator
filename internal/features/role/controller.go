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
	"strings"
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

// Controller handles K8s reconciliation for DatabaseRole resources.
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

// NewController creates a new role controller.
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

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants,verbs=list

// Reconcile implements the reconciliation loop for DatabaseRole resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("role", req.NamespacedName)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

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

	return c.reconcile(ctx, role, reconcileID)
}

func (c *Controller) reconcile(ctx context.Context, role *dbopsv1alpha1.DatabaseRole, reconcileID string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("role", role.Name, "namespace", role.Namespace)

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
		c.Recorder.Eventf(role, corev1.EventTypeNormal, "Created", "Role %s created successfully", role.Spec.RoleName)
	} else {
		// Update role settings if role already exists
		if _, err := c.handler.Update(ctx, role.Spec.RoleName, &role.Spec, role.Namespace); err != nil {
			log.Error(err, "Failed to update role settings")
		}
	}

	// Get instance for drift detection
	instance, err := c.handler.GetInstance(ctx, &role.Spec, role.Namespace)
	if err != nil {
		log.Error(err, "Failed to get instance for drift detection")
		// Continue anyway, drift detection will use defaults
	}

	// Perform drift detection
	c.driftOrchestrator.PerformDriftDetection(ctx,
		&roleDriftableResource{role},
		&controllerdrift.NamespacedInstancePolicy{Instance: instance},
		&roleDriftDetector{handler: c.handler, spec: &role.Spec, namespace: role.Namespace},
	)

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

	// Set reconcileID for end-to-end tracing
	now := metav1.Now()
	role.Status.ReconcileID = reconcileID
	role.Status.LastReconcileTime = &now

	if err := c.Status().Update(ctx, role); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(role)

	log.Info("Successfully reconciled DatabaseRole", "roleName", role.Spec.RoleName)
	return ctrl.Result{RequeueAfter: c.driftOrchestrator.GetRequeueInterval(
		&roleDriftableResource{role},
		&controllerdrift.NamespacedInstancePolicy{Instance: instance},
	)}, nil
}

func (c *Controller) handleDeletion(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("role", role.Name, "namespace", role.Namespace)

	if !controllerutil.ContainsFinalizer(role, util.FinalizerDatabaseRole) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseRole")

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

	// Check for grant dependencies
	hasGrants, msg, children, err := c.hasGrantDependencies(ctx, role)
	if err != nil {
		log.Error(err, "Failed to check grant dependencies")
		// Fail-open: proceed without blocking on check errors
	}

	if hasGrants && !util.HasForceDeleteAnnotation(role) {
		// Normal deletion: block on grants
		log.Info("Deletion blocked by grant dependencies", "children", children, "message", msg)
		util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDependenciesExist, msg)
		role.Status.Phase = dbopsv1alpha1.PhaseFailed
		role.Status.Message = msg
		if statusErr := c.Status().Update(ctx, role); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		c.Recorder.Eventf(role, corev1.EventTypeWarning, "DeletionBlocked",
			"Deletion blocked: %s", msg)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	if hasGrants && util.HasForceDeleteAnnotation(role) {
		// Force-delete with children: require confirmation
		if !util.IsForceDeleteConfirmed(role, children) {
			hash := util.ComputeDeletionHash(children)
			role.Status.Phase = dbopsv1alpha1.PhasePendingDeletion
			role.Status.Message = fmt.Sprintf(
				"Force-delete requires confirmation: %d child(ren) will be cascade-deleted. "+
					"Set annotation %s=%s to confirm.",
				len(children), util.AnnotationConfirmForceDelete, hash)
			role.Status.DeletionConfirmation = &dbopsv1alpha1.DeletionConfirmation{
				Required:       true,
				Hash:           hash,
				Children:       children,
				RemainingCount: len(children),
				Message:        role.Status.Message,
			}
			util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
				util.ReasonPendingDeletionConfirmation, role.Status.Message)
			if statusErr := c.Status().Update(ctx, role); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			c.Recorder.Eventf(role, corev1.EventTypeWarning, "ForceDeletePending",
				"Confirmation required: %d children will be cascade-deleted: %s",
				len(children), strings.Join(children, ", "))
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		// Confirmed — cascade delete children
		remaining, remainingNames, err := c.cascadeDeleteChildren(ctx, role)
		if err != nil {
			return c.handleError(ctx, role, err, "cascade delete children")
		}
		if remaining > 0 {
			log.Info("Waiting for children to be deleted", "remaining", remaining)
			role.Status.Phase = dbopsv1alpha1.PhaseDeleting
			role.Status.Message = fmt.Sprintf("Cascade-deleting: %d child(ren) remaining", remaining)
			role.Status.DeletionConfirmation = &dbopsv1alpha1.DeletionConfirmation{
				Required:       false,
				Hash:           util.ComputeDeletionHash(children),
				Children:       remainingNames,
				RemainingCount: remaining,
				Message:        role.Status.Message,
			}
			util.SetReadyCondition(&role.Status.Conditions, metav1.ConditionFalse,
				util.ReasonCascadeDeleting, role.Status.Message)
			if statusErr := c.Status().Update(ctx, role); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		c.Recorder.Eventf(role, corev1.EventTypeNormal, "CascadeDeleteComplete",
			"All %d children cascade-deleted", len(children))
	}

	// Clear deletion confirmation before proceeding
	role.Status.DeletionConfirmation = nil

	if deletionPolicy == string(dbopsv1alpha1.DeletionPolicyDelete) {
		force := util.HasForceDeleteAnnotation(role)
		if err := c.handler.Delete(ctx, role.Spec.RoleName, &role.Spec, role.Namespace, force); err != nil {
			// Log the error but continue with finalizer removal
			// This prevents the CR from being stuck if the database is unreachable
			log.Error(err, "Failed to delete role from database")
			c.Recorder.Eventf(role, corev1.EventTypeWarning, "DeleteFailed", "Failed to delete role: %v", err)
			if !errors.IsNotFound(err) && !force {
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
		} else {
			c.Recorder.Eventf(role, corev1.EventTypeNormal, "Deleted", "Role %s deleted successfully", role.Spec.RoleName)
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
	log := logf.FromContext(ctx).WithValues("role", role.Name, "namespace", role.Namespace)

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

func (c *Controller) updatePhase(ctx context.Context, role *dbopsv1alpha1.DatabaseRole, phase dbopsv1alpha1.Phase, message string) {
	role.Status.Phase = phase
	role.Status.Message = message
	if err := c.Status().Update(ctx, role); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// getDeletionPolicy returns the deletion policy from annotation or default to Retain.
func (c *Controller) getDeletionPolicy(role *dbopsv1alpha1.DatabaseRole) string {
	annotations := role.GetAnnotations()
	if annotations == nil {
		return string(dbopsv1alpha1.DeletionPolicyDelete)
	}
	policy := annotations[AnnotationDeletionPolicy]
	if policy == "" {
		return string(dbopsv1alpha1.DeletionPolicyDelete)
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
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseRole{}).
		Named("databaserole-feature").
		WithPredicates(c.predicates...).
		Complete(c)
}

// cascadeDeleteChildren deletes all DatabaseGrant CRs referencing this role.
// Returns the count of children still existing (pending finalizer removal by their controllers).
func (c *Controller) cascadeDeleteChildren(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (int, []string, error) {
	var remaining []string

	var grants dbopsv1alpha1.DatabaseGrantList
	if err := c.List(ctx, &grants, client.InNamespace(role.Namespace)); err != nil {
		return 0, nil, fmt.Errorf("list grants: %w", err)
	}
	for i := range grants.Items {
		g := &grants.Items[i]
		if g.Spec.RoleRef != nil && g.Spec.RoleRef.Name == role.Name {
			ns := g.Spec.RoleRef.Namespace
			if ns == "" || ns == role.Namespace {
				if err := c.Delete(ctx, g); err != nil {
					if !errors.IsNotFound(err) {
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

// hasGrantDependencies checks if any DatabaseGrant resources reference this role.
// Returns true with a descriptive message if grants exist.
func (c *Controller) hasGrantDependencies(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (bool, string, []string, error) {
	var childNames []string

	var grants dbopsv1alpha1.DatabaseGrantList
	if err := c.List(ctx, &grants, client.InNamespace(role.Namespace)); err != nil {
		return false, "", nil, fmt.Errorf("list grants: %w", err)
	}
	for _, g := range grants.Items {
		if g.Spec.RoleRef != nil && g.Spec.RoleRef.Name == role.Name {
			ns := g.Spec.RoleRef.Namespace
			if ns == "" || ns == role.Namespace {
				childNames = append(childNames, "DatabaseGrant/"+g.Name)
			}
		}
	}

	if len(childNames) == 0 {
		return false, "", nil, nil
	}

	msg := fmt.Sprintf("cannot delete: %d DatabaseGrant(s) still reference this role: %s. "+
		"Delete grants first or use force-delete annotation", len(childNames), strings.Join(childNames, ", "))
	return true, msg, childNames, nil
}
