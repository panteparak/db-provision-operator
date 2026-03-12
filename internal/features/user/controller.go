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

package user

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
	"github.com/db-provision-operator/internal/secret"
	reconcilecontext "github.com/db-provision-operator/internal/shared/reconcile"
	"github.com/db-provision-operator/internal/util"
)

const (
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second
)

// Controller handles K8s reconciliation for DatabaseUser resources.
type Controller struct {
	client.Client
	Scheme               *runtime.Scheme
	Recorder             record.EventRecorder
	handler              *Handler
	secretManager        *secret.Manager
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
	SecretManager        *secret.Manager
	DefaultDriftInterval time.Duration
	Predicates           []predicate.Predicate
	Logger               logr.Logger
}

// NewController creates a new user controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:               cfg.Client,
		Scheme:               cfg.Scheme,
		Recorder:             cfg.Recorder,
		handler:              cfg.Handler,
		secretManager:        cfg.SecretManager,
		defaultDriftInterval: cfg.DefaultDriftInterval,
		predicates:           cfg.Predicates,
		logger:               cfg.Logger,
		driftOrchestrator: &controllerdrift.Orchestrator{
			Recorder:             cfg.Recorder,
			DefaultDriftInterval: cfg.DefaultDriftInterval,
		},
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasegrants,verbs=list

// Reconcile implements the reconciliation loop for DatabaseUser resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate reconcileID for end-to-end tracing
	ctx, log, reconcileID := reconcilecontext.WithReconcileID(ctx)
	log = log.WithValues("user", req.NamespacedName)
	// Inject the enriched logger back into context for downstream functions
	ctx = logf.IntoContext(ctx, log)

	// Fetch the DatabaseUser resource
	user := &dbopsv1alpha1.DatabaseUser{}
	if err := c.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update reconcileID in status at the start of reconciliation
	c.updateReconcileID(ctx, user, reconcileID)

	if util.ShouldSkipReconcile(user) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	if util.IsMarkedForDeletion(user) {
		return c.handleDeletion(ctx, user)
	}

	if !controllerutil.ContainsFinalizer(user, util.FinalizerDatabaseUser) {
		controllerutil.AddFinalizer(user, util.FinalizerDatabaseUser)
		if err := c.Update(ctx, user); err != nil {
			return ctrl.Result{}, err
		}
	}

	return c.reconcile(ctx, user, reconcileID)
}

func (c *Controller) reconcile(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, reconcileID string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	// Check if user exists
	exists, err := c.handler.Exists(ctx, user.Spec.Username, &user.Spec, user.Namespace)
	if err != nil {
		return c.handleError(ctx, user, err, "check existence")
	}

	// Get or create password
	var password string
	secretName := user.Spec.Username + "-credentials"
	if user.Spec.PasswordSecret != nil && user.Spec.PasswordSecret.SecretName != "" {
		secretName = user.Spec.PasswordSecret.SecretName
	}

	existingSecret := &corev1.Secret{}
	err = c.Get(ctx, client.ObjectKey{Namespace: user.Namespace, Name: secretName}, existingSecret)
	if errors.IsNotFound(err) {
		// Generate new password using PasswordConfig if specified
		password, err = secret.GeneratePassword(user.Spec.PasswordSecret)
		if err != nil {
			return c.handleError(ctx, user, err, "generate password")
		}
	} else if err != nil {
		return c.handleError(ctx, user, err, "get secret")
	} else {
		password = string(existingSecret.Data["password"])
	}

	// Create user if not exists
	if !exists {
		log.Info("Creating user", "username", user.Spec.Username)
		c.updatePhase(ctx, user, dbopsv1alpha1.PhaseCreating, "Creating user")

		_, err := c.handler.Create(ctx, &user.Spec, user.Namespace, password)
		if err != nil {
			return c.handleError(ctx, user, err, "create user")
		}
		c.Recorder.Eventf(user, corev1.EventTypeNormal, "Created", "User %s created successfully", user.Spec.Username)
	}

	// Update user settings
	if _, err := c.handler.Update(ctx, user.Spec.Username, &user.Spec, user.Namespace); err != nil {
		log.Error(err, "Failed to update user settings")
	}

	// Create/update credentials secret
	if err := c.ensureCredentialsSecret(ctx, user, password, secretName); err != nil {
		return c.handleError(ctx, user, err, "ensure credentials secret")
	}

	// Get instance for drift detection
	instance, err := c.handler.GetInstance(ctx, &user.Spec, user.Namespace)
	if err != nil {
		log.Error(err, "Failed to get instance for drift detection")
		// Continue anyway, drift detection will use defaults
	}

	// Perform drift detection
	c.driftOrchestrator.PerformDriftDetection(ctx,
		&userDriftableResource{user},
		&controllerdrift.NamespacedInstancePolicy{Instance: instance},
		&userDriftDetector{handler: c.handler, spec: &user.Spec, namespace: user.Namespace},
	)

	// Update status
	user.Status.Phase = dbopsv1alpha1.PhaseReady
	user.Status.Message = "User is ready"
	if user.Status.Secret == nil {
		user.Status.Secret = &dbopsv1alpha1.SecretInfo{}
	}
	user.Status.Secret.Name = secretName
	user.Status.Secret.Namespace = user.Namespace

	util.SetSyncedCondition(&user.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "User is synced")
	util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "User is ready")

	// Set reconcileID for end-to-end tracing
	now := metav1.Now()
	user.Status.ReconcileID = reconcileID
	user.Status.LastReconcileTime = &now

	if err := c.Status().Update(ctx, user); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(user)

	log.Info("Successfully reconciled DatabaseUser", "username", user.Spec.Username)
	return ctrl.Result{RequeueAfter: c.driftOrchestrator.GetRequeueInterval(
		&userDriftableResource{user},
		&controllerdrift.NamespacedInstancePolicy{Instance: instance},
	)}, nil
}

func (c *Controller) ensureCredentialsSecret(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, password, secretName string) error {
	log := logf.FromContext(ctx)

	// Get instance for connection info
	instance, err := c.handler.GetInstance(ctx, &user.Spec, user.Namespace)
	if err != nil {
		return err
	}

	// Build base labels
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "db-provision-operator",
		"dbops.dbprovision.io/user":    user.Name,
	}

	// Build base annotations
	annotations := map[string]string{}

	// Determine secret type
	secretType := corev1.SecretTypeOpaque

	// Merge custom labels, annotations, and type from SecretTemplate
	if user.Spec.PasswordSecret != nil && user.Spec.PasswordSecret.SecretTemplate != nil {
		template := user.Spec.PasswordSecret.SecretTemplate

		// Merge custom labels (custom labels override defaults)
		for k, v := range template.Labels {
			labels[k] = v
		}

		// Merge custom annotations
		for k, v := range template.Annotations {
			annotations[k] = v
		}

		// Use custom secret type if specified
		if template.Type != "" {
			secretType = template.Type
		}
	}

	// Build template data for potential template rendering
	tmplData := secret.TemplateData{
		Username:  user.Spec.Username,
		Password:  password,
		Host:      instance.Spec.Connection.Host,
		Port:      instance.Spec.Connection.Port,
		Database:  instance.Spec.Connection.Database,
		Namespace: user.Namespace,
		Name:      user.Name,
	}

	// Populate SSLMode and TLS certs from instance
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tmplData.SSLMode = instance.Spec.TLS.Mode
		tlsCreds, tlsErr := c.secretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if tlsErr != nil {
			log.Error(tlsErr, "Failed to load TLS credentials for template")
		} else if tlsCreds != nil {
			tmplData.CA = string(tlsCreds.CA)
			tmplData.TLSCert = string(tlsCreds.Cert)
			tmplData.TLSKey = string(tlsCreds.Key)
		}
	}

	// Determine secret data: use SecretTemplate.Data if provided, else defaults
	var secretData map[string]string
	if hasSecretTemplateData(user) {
		rendered, renderErr := secret.RenderSecretTemplate(user.Spec.PasswordSecret.SecretTemplate, tmplData)
		if renderErr != nil {
			return fmt.Errorf("render secret template: %w", renderErr)
		}
		secretData = make(map[string]string, len(rendered))
		for k, v := range rendered {
			secretData[k] = string(v)
		}
	} else {
		// Default keys (backward compatible)
		secretData = map[string]string{
			"username": user.Spec.Username,
			"password": password,
			"host":     instance.Spec.Connection.Host,
			"port":     fmt.Sprintf("%d", instance.Spec.Connection.Port),
		}
	}

	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secretName,
			Namespace:   user.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type:       secretType,
		StringData: secretData,
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(user, credSecret, c.Scheme); err != nil {
		return err
	}

	existing := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: credSecret.Namespace, Name: credSecret.Name}, existing); err != nil {
		if errors.IsNotFound(err) {
			return c.Create(ctx, credSecret)
		}
		return err
	}

	// Update if exists - update StringData, Labels, Annotations, and Type
	existing.StringData = credSecret.StringData
	existing.Labels = credSecret.Labels
	existing.Annotations = credSecret.Annotations
	existing.Type = credSecret.Type
	return c.Update(ctx, existing)
}

// hasSecretTemplateData returns true if the user has SecretTemplate.Data with entries.
func hasSecretTemplateData(user *dbopsv1alpha1.DatabaseUser) bool {
	return user.Spec.PasswordSecret != nil &&
		user.Spec.PasswordSecret.SecretTemplate != nil &&
		len(user.Spec.PasswordSecret.SecretTemplate.Data) > 0
}

func (c *Controller) handleDeletion(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	if !controllerutil.ContainsFinalizer(user, util.FinalizerDatabaseUser) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseUser")

	// Get deletion policy from annotation (default to Retain for users)
	deletionPolicy := dbopsv1alpha1.DeletionPolicyRetain
	if policy, ok := user.Annotations["dbops.dbprovision.io/deletion-policy"]; ok {
		deletionPolicy = dbopsv1alpha1.DeletionPolicy(policy)
	}

	// Check deletion protection annotation
	deletionProtection := user.Annotations["dbops.dbprovision.io/deletion-protection"] == "true"
	if deletionProtection && !util.HasForceDeleteAnnotation(user) {
		c.Recorder.Eventf(user, corev1.EventTypeWarning, "DeletionBlocked", "Deletion blocked by deletion protection annotation")
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = "Deletion blocked by deletion protection"
		util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, user); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Check for grant dependencies (skip if force-delete)
	if !util.HasForceDeleteAnnotation(user) {
		hasGrants, msg, children, err := c.hasGrantDependencies(ctx, user)
		if err != nil {
			log.Error(err, "Failed to check grant dependencies")
			// Don't block on check errors — proceed with deletion
		} else if hasGrants {
			log.Info("Deletion blocked by grant dependencies", "children", children, "message", msg)
			util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
				util.ReasonDependenciesExist, msg)
			user.Status.Phase = dbopsv1alpha1.PhaseFailed
			user.Status.Message = msg
			if statusErr := c.Status().Update(ctx, user); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			c.Recorder.Eventf(user, corev1.EventTypeWarning, "DeletionBlocked",
				"Deletion blocked: %s", msg)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	if deletionPolicy == dbopsv1alpha1.DeletionPolicyDelete {
		force := util.HasForceDeleteAnnotation(user)

		// Pre-deletion ownership check (skip if force-delete is set)
		if !force {
			blocked, err := c.checkOwnershipBeforeDelete(ctx, user)
			if err != nil {
				log.Error(err, "Failed to check ownership")
				// Don't block deletion on check failure, proceed with deletion
			} else if blocked {
				// Ownership blocks deletion - status already updated by checkOwnershipBeforeDelete
				return ctrl.Result{RequeueAfter: RequeueAfterError}, fmt.Errorf("deletion blocked: user owns database objects")
			}
		}

		if err := c.handler.Delete(ctx, user.Spec.Username, &user.Spec, user.Namespace, force); err != nil {
			log.Error(err, "Failed to delete user")
			c.Recorder.Eventf(user, corev1.EventTypeWarning, "DeleteFailed", "Failed to delete user: %v", err)
			if !force {
				return ctrl.Result{RequeueAfter: RequeueAfterError}, err
			}
		} else {
			c.Recorder.Eventf(user, corev1.EventTypeNormal, "Deleted", "User %s deleted successfully", user.Spec.Username)
			// Clear ownership block status on successful deletion
			user.Status.OwnershipBlock = nil
		}
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(user)

	controllerutil.RemoveFinalizer(user, util.FinalizerDatabaseUser)
	if err := c.Update(ctx, user); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseUser")
	return ctrl.Result{}, nil
}

// checkOwnershipBeforeDelete checks if the user owns any database objects.
// Returns true if deletion should be blocked, false if deletion can proceed.
func (c *Controller) checkOwnershipBeforeDelete(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (bool, error) {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	ownershipResult, err := c.handler.GetOwnedObjects(ctx, user.Spec.Username, &user.Spec, user.Namespace)
	if err != nil {
		return false, fmt.Errorf("check owned objects: %w", err)
	}

	if !ownershipResult.BlocksDeletion {
		// No owned objects, clear any previous ownership block status
		if user.Status.OwnershipBlock != nil {
			user.Status.OwnershipBlock = nil
			if err := c.Status().Update(ctx, user); err != nil {
				log.Error(err, "Failed to clear ownership block status")
			}
		}
		return false, nil
	}

	// User owns objects - block deletion and update status
	log.Info("Deletion blocked: user owns database objects",
		"objectCount", len(ownershipResult.OwnedObjects),
		"resolution", ownershipResult.Resolution)

	now := metav1.Now()

	// Convert handler's OwnedObject to API OwnedObject
	apiOwnedObjects := make([]dbopsv1alpha1.OwnedObject, len(ownershipResult.OwnedObjects))
	for i, obj := range ownershipResult.OwnedObjects {
		apiOwnedObjects[i] = dbopsv1alpha1.OwnedObject{
			Schema: obj.Schema,
			Name:   obj.Name,
			Type:   obj.Type,
		}
	}

	user.Status.OwnershipBlock = &dbopsv1alpha1.OwnershipBlockStatus{
		Blocked:       true,
		LastCheckedAt: &now,
		OwnedObjects:  apiOwnedObjects,
		Resolution:    ownershipResult.Resolution,
		Message:       fmt.Sprintf("User owns %d database objects. Run the resolution command before deleting.", len(ownershipResult.OwnedObjects)),
	}
	user.Status.Phase = dbopsv1alpha1.PhaseFailed
	user.Status.Message = fmt.Sprintf("Deletion blocked: user owns %d database objects", len(ownershipResult.OwnedObjects))

	util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
		"OwnershipBlocks", fmt.Sprintf("User owns %d objects, cannot delete", len(ownershipResult.OwnedObjects)))

	if err := c.Status().Update(ctx, user); err != nil {
		log.Error(err, "Failed to update ownership block status")
	}

	c.Recorder.Event(user, corev1.EventTypeWarning, "DeletionBlocked",
		reconcilecontext.EventMessage(ctx, fmt.Sprintf("Deletion blocked: user owns %d database objects. Resolution: %s",
			len(ownershipResult.OwnedObjects), ownershipResult.Resolution)))

	return true, nil
}

func (c *Controller) handleError(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)

	c.Recorder.Event(user, corev1.EventTypeWarning, "ReconcileFailed",
		reconcilecontext.EventMessage(ctx, fmt.Sprintf("Failed to %s: %v", operation, err)))

	user.Status.Phase = dbopsv1alpha1.PhaseFailed
	user.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, user); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(user)

	return reconcileutil.ClassifyRequeue(err)
}

func (c *Controller) updatePhase(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, phase dbopsv1alpha1.Phase, message string) {
	user.Status.Phase = phase
	user.Status.Message = message
	if err := c.Status().Update(ctx, user); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// updateReconcileID updates the reconcileID and lastReconcileTime in the status.
// This enables end-to-end tracing across logs, events, and status updates.
func (c *Controller) updateReconcileID(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, reconcileID string) {
	now := metav1.Now()
	user.Status.ReconcileID = reconcileID
	user.Status.LastReconcileTime = &now

	// Note: We don't update the status here to avoid a separate API call.
	// The reconcileID will be persisted when the status is updated at the end of reconciliation.
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseUser{}).
		Owns(&corev1.Secret{}).
		Named("databaseuser").
		WithPredicates(c.predicates...).
		Complete(c)
}

// hasGrantDependencies checks if any DatabaseGrant resources reference this user.
// Returns true with a descriptive message if grants exist.
func (c *Controller) hasGrantDependencies(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (bool, string, []string, error) {
	var childNames []string

	var grants dbopsv1alpha1.DatabaseGrantList
	if err := c.List(ctx, &grants, client.InNamespace(user.Namespace)); err != nil {
		return false, "", nil, fmt.Errorf("list grants: %w", err)
	}
	for _, g := range grants.Items {
		if g.Spec.UserRef != nil && g.Spec.UserRef.Name == user.Name {
			ns := g.Spec.UserRef.Namespace
			if ns == "" || ns == user.Namespace {
				childNames = append(childNames, "DatabaseGrant/"+g.Name)
			}
		}
	}

	if len(childNames) == 0 {
		return false, "", nil, nil
	}

	msg := fmt.Sprintf("cannot delete: %d DatabaseGrant(s) still reference this user: %s. "+
		"Delete grants first or use force-delete annotation", len(childNames), strings.Join(childNames, ", "))
	return true, msg, childNames, nil
}
