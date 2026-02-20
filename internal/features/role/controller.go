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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/reconcileutil"
	reconcilecontext "github.com/db-provision-operator/internal/shared/reconcile"
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

// NewController creates a new role controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:   cfg.Client,
		Scheme:   cfg.Scheme,
		Recorder: cfg.Recorder,
		handler:  cfg.Handler,
		logger:   cfg.Logger,
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
	c.performDriftDetection(ctx, role, instance)

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
	return ctrl.Result{RequeueAfter: c.getRequeueInterval(role, instance)}, nil
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

	// Check for grant dependencies (skip if force-delete)
	if !util.HasForceDeleteAnnotation(role) {
		hasGrants, msg, err := c.hasGrantDependencies(ctx, role)
		if err != nil {
			log.Error(err, "Failed to check grant dependencies")
			// Don't block on check errors — proceed with deletion
		} else if hasGrants {
			log.Info("Deletion blocked by grant dependencies", "message", msg)
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
	}

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
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseRole{}).
		Named("databaserole-feature").
		Complete(c)
}

// performDriftDetection detects and optionally corrects drift for the role.
// It uses the drift policy from the role spec or falls back to the instance's default policy.
func (c *Controller) performDriftDetection(ctx context.Context, role *dbopsv1alpha1.DatabaseRole, instance *dbopsv1alpha1.DatabaseInstance) {
	log := logf.FromContext(ctx).WithValues("role", role.Name, "namespace", role.Namespace)

	// 1. Get effective drift policy
	policy := c.getEffectiveDriftPolicy(role, instance)
	if policy.Mode == dbopsv1alpha1.DriftModeIgnore {
		log.V(1).Info("Drift detection disabled (mode=ignore)")
		return
	}

	// 2. Check if destructive drift corrections are allowed via annotation
	allowDestructive := c.hasDestructiveDriftAnnotation(role)

	// 3. Detect drift
	driftResult, err := c.handler.DetectDrift(ctx, &role.Spec, role.Namespace, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to detect drift")
		return
	}

	// 4. Update drift status
	if driftResult != nil {
		role.Status.Drift = driftResult.ToAPIStatus()

		// 5. Record event if drift detected
		if driftResult.HasDrift() {
			var driftFields []string
			for _, d := range driftResult.Diffs {
				driftFields = append(driftFields, d.Field)
			}
			c.Recorder.Eventf(role, corev1.EventTypeWarning, "DriftDetected",
				"Configuration drift detected in fields: %v", driftFields)
			log.Info("Drift detected", "fields", driftFields)

			// 6. Correct drift if mode is "correct"
			if policy.Mode == dbopsv1alpha1.DriftModeCorrect {
				c.correctDrift(ctx, role, driftResult, allowDestructive)
			}
		}
	}
}

// correctDrift attempts to correct detected drift.
func (c *Controller) correctDrift(ctx context.Context, role *dbopsv1alpha1.DatabaseRole, driftResult interface{}, allowDestructive bool) {
	log := logf.FromContext(ctx).WithValues("role", role.Name, "namespace", role.Namespace)

	// Get the drift result - we need to use the concrete type from the handler
	driftResultTyped, ok := driftResult.(interface {
		ToAPIStatus() *dbopsv1alpha1.DriftStatus
	})
	if !ok {
		log.Error(nil, "Invalid drift result type")
		return
	}

	// Get the actual drift.Result from the handler
	handlerDriftResult, err := c.handler.DetectDrift(ctx, &role.Spec, role.Namespace, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to re-detect drift for correction")
		return
	}

	// Skip if no drift to correct (could have been corrected elsewhere)
	if handlerDriftResult == nil || !handlerDriftResult.HasDrift() {
		log.V(1).Info("No drift to correct")
		return
	}

	correctionResult, err := c.handler.CorrectDrift(ctx, &role.Spec, role.Namespace, handlerDriftResult, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to correct drift")
		c.Recorder.Eventf(role, corev1.EventTypeWarning, "DriftCorrectionFailed",
			"Failed to correct drift: %v", err)
		return
	}

	if correctionResult != nil && correctionResult.HasCorrections() {
		var correctedFields []string
		for _, corr := range correctionResult.Corrected {
			correctedFields = append(correctedFields, corr.Diff.Field)
		}
		c.Recorder.Eventf(role, corev1.EventTypeNormal, "DriftCorrected",
			"Drift corrected for fields: %v", correctedFields)
		log.Info("Drift corrected", "fields", correctedFields)

		// Clear drift status after successful correction
		role.Status.Drift = driftResultTyped.ToAPIStatus()
		role.Status.Drift.Detected = false
		role.Status.Drift.Diffs = nil
	}

	// Log skipped corrections
	if correctionResult != nil && len(correctionResult.Skipped) > 0 {
		for _, s := range correctionResult.Skipped {
			log.V(1).Info("Drift correction skipped", "field", s.Diff.Field, "reason", s.Reason)
		}
	}

	// Log failed corrections
	if correctionResult != nil && correctionResult.HasFailures() {
		for _, f := range correctionResult.Failed {
			log.Error(f.Error, "Drift correction failed", "field", f.Diff.Field)
		}
	}
}

// getRequeueInterval returns the requeue interval based on the effective drift policy.
func (c *Controller) getRequeueInterval(role *dbopsv1alpha1.DatabaseRole, instance *dbopsv1alpha1.DatabaseInstance) time.Duration {
	policy := c.getEffectiveDriftPolicy(role, instance)
	return util.ParseDriftRequeueInterval(policy, RequeueAfterReady)
}

// getEffectiveDriftPolicy returns the effective drift policy for a role.
// It uses the role's drift policy if set, otherwise falls back to the instance's default.
func (c *Controller) getEffectiveDriftPolicy(role *dbopsv1alpha1.DatabaseRole, instance *dbopsv1alpha1.DatabaseInstance) dbopsv1alpha1.DriftPolicy {
	// Use role-level policy if set
	if role.Spec.DriftPolicy != nil {
		return *role.Spec.DriftPolicy
	}

	// Fall back to instance-level policy if available
	if instance != nil && instance.Spec.DriftPolicy != nil {
		return *instance.Spec.DriftPolicy
	}

	// Default policy: detect mode, 5 minute interval
	return dbopsv1alpha1.DriftPolicy{
		Mode:     dbopsv1alpha1.DriftModeDetect,
		Interval: "5m",
	}
}

// hasDestructiveDriftAnnotation checks if the role has the allow-destructive-drift annotation.
func (c *Controller) hasDestructiveDriftAnnotation(role *dbopsv1alpha1.DatabaseRole) bool {
	if role.Annotations == nil {
		return false
	}
	return role.Annotations[dbopsv1alpha1.AnnotationAllowDestructiveDrift] == "true"
}

// hasGrantDependencies checks if any DatabaseGrant resources reference this role.
// Returns true with a descriptive message if grants exist.
func (c *Controller) hasGrantDependencies(ctx context.Context, role *dbopsv1alpha1.DatabaseRole) (bool, string, error) {
	var childNames []string

	var grants dbopsv1alpha1.DatabaseGrantList
	if err := c.List(ctx, &grants, client.InNamespace(role.Namespace)); err != nil {
		return false, "", fmt.Errorf("list grants: %w", err)
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
		return false, "", nil
	}

	msg := fmt.Sprintf("cannot delete: %d DatabaseGrant(s) still reference this role: %s. "+
		"Delete grants first or use force-delete annotation", len(childNames), strings.Join(childNames, ", "))
	return true, msg, nil
}
