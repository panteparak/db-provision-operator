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

package clustergrant

import (
	"context"
	"errors"
	"fmt"
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
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second
)

// Controller handles K8s reconciliation for ClusterDatabaseGrant resources.
type Controller struct {
	client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	handler              *Handler
	defaultDriftInterval time.Duration
	predicates           []predicate.Predicate
	logger               logr.Logger
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

// NewController creates a new cluster grant controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:               cfg.Client,
		Scheme:               cfg.Scheme,
		Recorder:             cfg.Recorder,
		handler:              cfg.Handler,
		defaultDriftInterval: cfg.DefaultDriftInterval,
		predicates:           cfg.Predicates,
		logger:               cfg.Logger,
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabasegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabasegrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabasegrants/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=clusterdatabaseroles,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for ClusterDatabaseGrant resources.
// Note: ClusterDatabaseGrant is cluster-scoped, so req.Name is used (no namespace).
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("clusterGrant", req.Name)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

	// Fetch the ClusterDatabaseGrant resource (cluster-scoped, no namespace)
	grant := &dbopsv1alpha1.ClusterDatabaseGrant{}
	if err := c.Get(ctx, client.ObjectKey{Name: req.Name}, grant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if util.ShouldSkipReconcile(grant) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	if util.IsMarkedForDeletion(grant) {
		return c.handleDeletion(ctx, grant)
	}

	if !controllerutil.ContainsFinalizer(grant, util.FinalizerClusterDatabaseGrant) {
		controllerutil.AddFinalizer(grant, util.FinalizerClusterDatabaseGrant)
		if err := c.Update(ctx, grant); err != nil {
			return ctrl.Result{}, err
		}
	}

	return c.reconcile(ctx, grant, reconcileID)
}

func (c *Controller) reconcile(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant, reconcileID string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterGrant", grant.Name)

	// Check if the referenced ClusterDatabaseInstance exists and is ready
	instance, err := c.handler.GetInstance(ctx, &grant.Spec)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for ClusterDatabaseInstance %s", grant.Spec.ClusterInstanceRef.Name))
		}
		return c.handleError(ctx, grant, err, "get cluster instance")
	}

	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for ClusterDatabaseInstance %s to be ready", instance.Name))
	}

	// Resolve the target (user or role) and check readiness
	target, err := c.handler.ResolveTarget(ctx, &grant.Spec)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for target: %v", err))
		}
		return c.handleError(ctx, grant, err, "resolve target")
	}

	// Apply grants
	log.Info("Applying cluster grants", "target", target.DatabaseName, "targetType", target.Type)
	c.updatePhase(ctx, grant, dbopsv1alpha1.PhaseCreating, "Applying grants")

	result, err := c.handler.Apply(ctx, &grant.Spec)
	if err != nil {
		return c.handleError(ctx, grant, err, "apply grants")
	}

	// Perform drift detection
	c.performDriftDetection(ctx, grant, instance)

	// Update status
	now := metav1.Now()
	grant.Status.Phase = dbopsv1alpha1.PhaseReady
	grant.Status.Message = "Grants applied successfully"
	grant.Status.ObservedGeneration = grant.Generation
	grant.Status.ReconcileID = reconcileID
	grant.Status.LastReconcileTime = &now
	grant.Status.AppliedGrants = &dbopsv1alpha1.ClusterAppliedGrantsInfo{
		Applied:           result.DirectGrants + int32(len(result.Roles)),
		Roles:             result.Roles,
		DirectGrants:      result.DirectGrants,
		DefaultPrivileges: result.DefaultPrivileges,
	}
	grant.Status.TargetInfo = &dbopsv1alpha1.GrantTargetInfo{
		Type:             target.Type,
		Name:             target.Name,
		Namespace:        target.Namespace,
		ResolvedUsername: target.DatabaseName,
	}
	c.Recorder.Eventf(grant, corev1.EventTypeNormal, "Applied", "Grants applied successfully: %d roles, %d direct grants", len(result.Roles), result.DirectGrants)

	util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Grants are synced")
	util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Grants are ready")

	if err := c.Status().Update(ctx, grant); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(grant)

	log.Info("Successfully reconciled ClusterDatabaseGrant",
		"roles", len(result.Roles),
		"directGrants", result.DirectGrants,
		"defaultPrivileges", result.DefaultPrivileges)
	return ctrl.Result{RequeueAfter: c.getRequeueInterval(grant, instance)}, nil
}

func (c *Controller) handleDeletion(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterGrant", grant.Name)

	if !controllerutil.ContainsFinalizer(grant, util.FinalizerClusterDatabaseGrant) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of ClusterDatabaseGrant")

	// Check deletion protection
	if c.hasDeletionProtection(grant) && !util.HasForceDeleteAnnotation(grant) {
		c.Recorder.Eventf(grant, corev1.EventTypeWarning, "DeletionBlocked", "Deletion blocked by deletion protection")
		grant.Status.Phase = dbopsv1alpha1.PhaseFailed
		grant.Status.Message = "Deletion blocked by deletion protection"
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, grant); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Always revoke grants on deletion (default policy for grants is Delete)
	c.updatePhase(ctx, grant, dbopsv1alpha1.PhaseDeleting, "Revoking grants")
	force := util.HasForceDeleteAnnotation(grant)

	if err := c.handler.Revoke(ctx, &grant.Spec); err != nil {
		// If the target user/role is not found or not ready, treat as
		// success during deletion — the target is gone or being deleted,
		// and any granted privileges will be implicitly revoked when the
		// database user/role is dropped.
		var targetErr *TargetResolutionError
		if errors.As(err, &targetErr) {
			log.Info("Target not resolvable during deletion, skipping revocation", "reason", targetErr.Error())
			c.Recorder.Eventf(grant, corev1.EventTypeNormal, "RevokeSkipped",
				"Skipped revocation: target not resolvable (%s)", targetErr.Error())
		} else {
			log.Error(err, "Failed to revoke grants")
			c.Recorder.Eventf(grant, corev1.EventTypeWarning, "RevokeFailed", "Failed to revoke grants: %v", err)
			if !force {
				// Don't remove finalizer if revoke fails — prevents privilege leaks.
				// The resource will be retried until the revoke succeeds.
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
			// Force delete: continue with finalizer removal despite failure
		}
	} else {
		log.Info("Grants revoked successfully")
		c.Recorder.Eventf(grant, corev1.EventTypeNormal, "Revoked", "Grants revoked successfully")
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(grant)

	controllerutil.RemoveFinalizer(grant, util.FinalizerClusterDatabaseGrant)
	if err := c.Update(ctx, grant); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of ClusterDatabaseGrant")
	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterGrant", grant.Name)

	log.Error(err, "Reconciliation failed", "operation", operation)
	c.Recorder.Event(grant, corev1.EventTypeWarning, "ReconcileFailed",
		reconcilecontext.EventMessage(ctx, fmt.Sprintf("Failed to %s: %v", operation, err)))

	grant.Status.Phase = dbopsv1alpha1.PhaseFailed
	grant.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, grant); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(grant)

	return reconcileutil.ClassifyRequeue(err)
}

func (c *Controller) handlePending(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant, message string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("clusterGrant", grant.Name)

	log.V(1).Info("Grant pending", "reason", message)

	grant.Status.Phase = dbopsv1alpha1.PhasePending
	grant.Status.Message = message
	util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconciling, message)

	if err := c.Status().Update(ctx, grant); err != nil {
		log.Error(err, "Failed to update status")
	}

	return ctrl.Result{RequeueAfter: RequeueAfterPending}, nil
}

func (c *Controller) updatePhase(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant, phase dbopsv1alpha1.Phase, message string) {
	grant.Status.Phase = phase
	grant.Status.Message = message
	if err := c.Status().Update(ctx, grant); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// hasDeletionProtection checks if the grant has deletion protection enabled.
func (c *Controller) hasDeletionProtection(grant *dbopsv1alpha1.ClusterDatabaseGrant) bool {
	return grant.Spec.DeletionProtection
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.ClusterDatabaseGrant{}).
		Named("clusterdatabasegrant-feature").
		WithPredicates(c.predicates...).
		Complete(c)
}

// performDriftDetection detects and optionally corrects drift for the grant.
// It uses the drift policy from the grant spec or falls back to the instance's default policy.
func (c *Controller) performDriftDetection(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant, instance *dbopsv1alpha1.ClusterDatabaseInstance) {
	log := logf.FromContext(ctx).WithValues("clusterGrant", grant.Name)

	// 1. Get effective drift policy
	policy := c.getEffectiveDriftPolicy(grant, instance)
	if policy.Mode == dbopsv1alpha1.DriftModeIgnore {
		log.V(1).Info("Drift detection disabled (mode=ignore)")
		return
	}

	// 2. Check if destructive drift corrections are allowed via annotation
	allowDestructive := c.hasDestructiveDriftAnnotation(grant)

	// 3. Detect drift
	driftResult, err := c.handler.DetectDrift(ctx, &grant.Spec, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to detect drift")
		return
	}

	// 4. Update drift status
	if driftResult != nil {
		grant.Status.Drift = driftResult.ToAPIStatus()

		// 5. Record event if drift detected
		if driftResult.HasDrift() {
			var driftFields []string
			for _, d := range driftResult.Diffs {
				driftFields = append(driftFields, d.Field)
			}
			c.Recorder.Eventf(grant, corev1.EventTypeWarning, "DriftDetected",
				"Configuration drift detected in fields: %v", driftFields)
			log.Info("Drift detected", "fields", driftFields)

			// 6. Correct drift if mode is "correct"
			if policy.Mode == dbopsv1alpha1.DriftModeCorrect {
				c.correctDrift(ctx, grant, driftResult, allowDestructive)
			}
		}
	}
}

// correctDrift attempts to correct detected drift.
func (c *Controller) correctDrift(ctx context.Context, grant *dbopsv1alpha1.ClusterDatabaseGrant, driftResult interface{}, allowDestructive bool) {
	log := logf.FromContext(ctx).WithValues("clusterGrant", grant.Name)

	// Get the drift result - we need to use the concrete type from the handler
	driftResultTyped, ok := driftResult.(interface {
		ToAPIStatus() *dbopsv1alpha1.DriftStatus
	})
	if !ok {
		log.Error(nil, "Invalid drift result type")
		return
	}

	// Get the actual drift.Result from the handler
	handlerDriftResult, err := c.handler.DetectDrift(ctx, &grant.Spec, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to re-detect drift for correction")
		return
	}

	// Skip if no drift to correct (could have been corrected elsewhere)
	if handlerDriftResult == nil || !handlerDriftResult.HasDrift() {
		log.V(1).Info("No drift to correct")
		return
	}

	correctionResult, err := c.handler.CorrectDrift(ctx, &grant.Spec, handlerDriftResult, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to correct drift")
		c.Recorder.Eventf(grant, corev1.EventTypeWarning, "DriftCorrectionFailed",
			"Failed to correct drift: %v", err)
		return
	}

	if correctionResult != nil && correctionResult.HasCorrections() {
		var correctedFields []string
		for _, corr := range correctionResult.Corrected {
			correctedFields = append(correctedFields, corr.Diff.Field)
		}
		c.Recorder.Eventf(grant, corev1.EventTypeNormal, "DriftCorrected",
			"Drift corrected for fields: %v", correctedFields)
		log.Info("Drift corrected", "fields", correctedFields)

		// Clear drift status after successful correction
		grant.Status.Drift = driftResultTyped.ToAPIStatus()
		grant.Status.Drift.Detected = false
		grant.Status.Drift.Diffs = nil
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
func (c *Controller) getRequeueInterval(grant *dbopsv1alpha1.ClusterDatabaseGrant, instance *dbopsv1alpha1.ClusterDatabaseInstance) time.Duration {
	policy := c.getEffectiveDriftPolicy(grant, instance)
	return util.ParseDriftRequeueInterval(policy, c.defaultDriftInterval)
}

// getEffectiveDriftPolicy returns the effective drift policy for a cluster grant.
// It uses the grant's drift policy if set, otherwise falls back to the instance's default.
func (c *Controller) getEffectiveDriftPolicy(grant *dbopsv1alpha1.ClusterDatabaseGrant, instance *dbopsv1alpha1.ClusterDatabaseInstance) dbopsv1alpha1.DriftPolicy {
	// Use grant-level policy if set
	if grant.Spec.DriftPolicy != nil {
		return *grant.Spec.DriftPolicy
	}

	// Fall back to instance-level policy if available
	if instance != nil && instance.Spec.DriftPolicy != nil {
		return *instance.Spec.DriftPolicy
	}

	// Default policy: detect mode, operator-configured interval
	return dbopsv1alpha1.DriftPolicy{
		Mode:     dbopsv1alpha1.DriftModeDetect,
		Interval: c.defaultDriftInterval.String(),
	}
}

// hasDestructiveDriftAnnotation checks if the grant has the allow-destructive-drift annotation.
func (c *Controller) hasDestructiveDriftAnnotation(grant *dbopsv1alpha1.ClusterDatabaseGrant) bool {
	if grant.Annotations == nil {
		return false
	}
	return grant.Annotations[dbopsv1alpha1.AnnotationAllowDestructiveDrift] == "true"
}
