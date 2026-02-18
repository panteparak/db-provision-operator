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

package grant

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
)

// Controller handles K8s reconciliation for DatabaseGrant resources.
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

// NewController creates a new grant controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:   cfg.Client,
		Scheme:   cfg.Scheme,
		Recorder: cfg.Recorder,
		handler:  cfg.Handler,
		logger:   cfg.Logger,
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for DatabaseGrant resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("grant", req.NamespacedName)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

	// Fetch the DatabaseGrant resource
	grant := &dbopsv1alpha1.DatabaseGrant{}
	if err := c.Get(ctx, req.NamespacedName, grant); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if util.ShouldSkipReconcile(grant) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	if util.IsMarkedForDeletion(grant) {
		return c.handleDeletion(ctx, grant)
	}

	if !controllerutil.ContainsFinalizer(grant, util.FinalizerDatabaseGrant) {
		controllerutil.AddFinalizer(grant, util.FinalizerDatabaseGrant)
		if err := c.Update(ctx, grant); err != nil {
			return ctrl.Result{}, err
		}
	}

	return c.reconcile(ctx, grant, reconcileID)
}

func (c *Controller) reconcile(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant, reconcileID string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("grant", grant.Name, "namespace", grant.Namespace)

	// Check if the referenced user exists and is ready
	user, err := c.getUser(ctx, grant)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for DatabaseUser %s", grant.Spec.UserRef.Name))
		}
		return c.handleError(ctx, grant, err, "get user")
	}

	if user.Status.Phase != dbopsv1alpha1.PhaseReady {
		return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for DatabaseUser %s to be ready", user.Name))
	}

	// Check if the referenced instance is ready
	instance, err := c.getInstance(ctx, user)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for DatabaseInstance %s", user.Spec.InstanceRef.Name))
		}
		return c.handleError(ctx, grant, err, "get instance")
	}

	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return c.handlePending(ctx, grant, fmt.Sprintf("Waiting for DatabaseInstance %s to be ready", instance.Name))
	}

	// Apply grants
	log.Info("Applying grants", "user", user.Spec.Username)
	c.updatePhase(ctx, grant, dbopsv1alpha1.PhaseCreating, "Applying grants")

	result, err := c.handler.Apply(ctx, &grant.Spec, grant.Namespace)
	if err != nil {
		return c.handleError(ctx, grant, err, "apply grants")
	}

	// Perform drift detection
	c.performDriftDetection(ctx, grant, instance)

	// Update status
	grant.Status.Phase = dbopsv1alpha1.PhaseReady
	grant.Status.Message = "Grants applied successfully"
	grant.Status.ObservedGeneration = grant.Generation
	grant.Status.AppliedGrants = &dbopsv1alpha1.AppliedGrantsInfo{
		Roles:             result.Roles,
		DirectGrants:      result.DirectGrants,
		DefaultPrivileges: result.DefaultPrivileges,
	}
	c.Recorder.Eventf(grant, corev1.EventTypeNormal, "Applied", "Grants applied successfully: %d roles, %d direct grants", len(result.Roles), result.DirectGrants)

	util.SetSyncedCondition(&grant.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Grants are synced")
	util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Grants are ready")

	// Set reconcileID for end-to-end tracing
	now := metav1.Now()
	grant.Status.ReconcileID = reconcileID
	grant.Status.LastReconcileTime = &now

	if err := c.Status().Update(ctx, grant); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(grant)

	log.Info("Successfully reconciled DatabaseGrant",
		"roles", len(result.Roles),
		"directGrants", result.DirectGrants,
		"defaultPrivileges", result.DefaultPrivileges)
	return ctrl.Result{RequeueAfter: RequeueAfterReady}, nil
}

func (c *Controller) getUser(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant) (*dbopsv1alpha1.DatabaseUser, error) {
	user := &dbopsv1alpha1.DatabaseUser{}
	userNamespace := grant.Namespace
	if grant.Spec.UserRef.Namespace != "" {
		userNamespace = grant.Spec.UserRef.Namespace
	}

	if err := c.Get(ctx, types.NamespacedName{
		Namespace: userNamespace,
		Name:      grant.Spec.UserRef.Name,
	}, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (c *Controller) getInstance(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (*dbopsv1alpha1.DatabaseInstance, error) {
	instance := &dbopsv1alpha1.DatabaseInstance{}
	instanceNamespace := user.Namespace
	if user.Spec.InstanceRef.Namespace != "" {
		instanceNamespace = user.Spec.InstanceRef.Namespace
	}

	if err := c.Get(ctx, types.NamespacedName{
		Namespace: instanceNamespace,
		Name:      user.Spec.InstanceRef.Name,
	}, instance); err != nil {
		return nil, err
	}

	return instance, nil
}

func (c *Controller) handleDeletion(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("grant", grant.Name, "namespace", grant.Namespace)

	if !controllerutil.ContainsFinalizer(grant, util.FinalizerDatabaseGrant) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseGrant")

	// Check deletion protection
	if c.hasDeletionProtection(grant) && !util.HasForceDeleteAnnotation(grant) {
		c.Recorder.Eventf(grant, corev1.EventTypeWarning, "DeletionBlocked", "Deletion blocked by deletion protection annotation")
		grant.Status.Phase = dbopsv1alpha1.PhaseFailed
		grant.Status.Message = "Deletion blocked by deletion protection"
		util.SetReadyCondition(&grant.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, grant); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Check deletion policy - only revoke if policy is Delete
	if c.getDeletionPolicy(grant) == dbopsv1alpha1.DeletionPolicyDelete {
		c.updatePhase(ctx, grant, dbopsv1alpha1.PhaseDeleting, "Revoking grants")
		force := util.HasForceDeleteAnnotation(grant)

		if err := c.handler.Revoke(ctx, &grant.Spec, grant.Namespace); err != nil {
			log.Error(err, "Failed to revoke grants")
			c.Recorder.Eventf(grant, corev1.EventTypeWarning, "RevokeFailed", "Failed to revoke grants: %v", err)
			if !force {
				// Don't remove finalizer if revoke fails — prevents privilege leaks.
				// The resource will be retried until the revoke succeeds.
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
			// Force delete: continue with finalizer removal despite failure
		} else {
			log.Info("Grants revoked successfully")
			c.Recorder.Eventf(grant, corev1.EventTypeNormal, "Revoked", "Grants revoked successfully")
		}
	} else {
		log.Info("Deletion policy is Retain, skipping grant revocation")
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(grant)

	controllerutil.RemoveFinalizer(grant, util.FinalizerDatabaseGrant)
	if err := c.Update(ctx, grant); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseGrant")
	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("grant", grant.Name, "namespace", grant.Namespace)

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

func (c *Controller) handlePending(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant, message string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("grant", grant.Name, "namespace", grant.Namespace)

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

func (c *Controller) updatePhase(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant, phase dbopsv1alpha1.Phase, message string) {
	grant.Status.Phase = phase
	grant.Status.Message = message
	if err := c.Status().Update(ctx, grant); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// hasDeletionProtection checks if the grant has deletion protection enabled.
func (c *Controller) hasDeletionProtection(grant *dbopsv1alpha1.DatabaseGrant) bool {
	return grant.Spec.DeletionProtection
}

// getDeletionPolicy returns the deletion policy for the grant.
// DatabaseGrant doesn't have DeletionPolicy field in the spec, so default to Retain.
func (c *Controller) getDeletionPolicy(_ *dbopsv1alpha1.DatabaseGrant) dbopsv1alpha1.DeletionPolicy {
	// Default to Delete for grants - revoke permissions when grant is deleted
	return dbopsv1alpha1.DeletionPolicyDelete
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseGrant{}).
		Named("databasegrant-feature").
		Complete(c)
}

// performDriftDetection detects and optionally corrects drift for the grant.
// It uses the drift policy from the grant spec or falls back to the instance's default policy.
func (c *Controller) performDriftDetection(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant, instance *dbopsv1alpha1.DatabaseInstance) {
	log := logf.FromContext(ctx).WithValues("grant", grant.Name, "namespace", grant.Namespace)

	// 1. Get effective drift policy
	policy := c.getEffectiveDriftPolicy(grant, instance)
	if policy.Mode == dbopsv1alpha1.DriftModeIgnore {
		log.V(1).Info("Drift detection disabled (mode=ignore)")
		return
	}

	// 2. Check if destructive drift corrections are allowed via annotation
	allowDestructive := c.hasDestructiveDriftAnnotation(grant)

	// 3. Detect drift
	driftResult, err := c.handler.DetectDrift(ctx, &grant.Spec, grant.Namespace, allowDestructive)
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
func (c *Controller) correctDrift(ctx context.Context, grant *dbopsv1alpha1.DatabaseGrant, driftResult interface{}, allowDestructive bool) {
	log := logf.FromContext(ctx).WithValues("grant", grant.Name, "namespace", grant.Namespace)

	// Get the drift result - we need to use the concrete type from the handler
	driftResultTyped, ok := driftResult.(interface {
		ToAPIStatus() *dbopsv1alpha1.DriftStatus
	})
	if !ok {
		log.Error(nil, "Invalid drift result type")
		return
	}

	// Get the actual drift.Result from the handler
	handlerDriftResult, err := c.handler.DetectDrift(ctx, &grant.Spec, grant.Namespace, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to re-detect drift for correction")
		return
	}

	// Skip if no drift to correct (could have been corrected elsewhere)
	if handlerDriftResult == nil || !handlerDriftResult.HasDrift() {
		log.V(1).Info("No drift to correct")
		return
	}

	correctionResult, err := c.handler.CorrectDrift(ctx, &grant.Spec, grant.Namespace, handlerDriftResult, allowDestructive)
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

// getEffectiveDriftPolicy returns the effective drift policy for a grant.
// It uses the grant's drift policy if set, otherwise falls back to the instance's default.
func (c *Controller) getEffectiveDriftPolicy(grant *dbopsv1alpha1.DatabaseGrant, instance *dbopsv1alpha1.DatabaseInstance) dbopsv1alpha1.DriftPolicy {
	// Use grant-level policy if set
	if grant.Spec.DriftPolicy != nil {
		return *grant.Spec.DriftPolicy
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

// hasDestructiveDriftAnnotation checks if the grant has the allow-destructive-drift annotation.
func (c *Controller) hasDestructiveDriftAnnotation(grant *dbopsv1alpha1.DatabaseGrant) bool {
	if grant.Annotations == nil {
		return false
	}
	return grant.Annotations[dbopsv1alpha1.AnnotationAllowDestructiveDrift] == "true"
}
