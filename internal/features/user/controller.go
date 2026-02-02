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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

const (
	RequeueAfterReady   = 5 * time.Minute
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second
)

// Controller handles K8s reconciliation for DatabaseUser resources.
type Controller struct {
	client.Client
	Scheme        *runtime.Scheme
	handler       *Handler
	secretManager *secret.Manager
	logger        logr.Logger
}

// ControllerConfig holds dependencies for the controller.
type ControllerConfig struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	Handler       *Handler
	SecretManager *secret.Manager
	Logger        logr.Logger
}

// NewController creates a new user controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:        cfg.Client,
		Scheme:        cfg.Scheme,
		handler:       cfg.Handler,
		secretManager: cfg.SecretManager,
		logger:        cfg.Logger,
	}
}

// Reconcile implements the reconciliation loop for DatabaseUser resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("user", req.NamespacedName)

	// Fetch the DatabaseUser resource
	user := &dbopsv1alpha1.DatabaseUser{}
	if err := c.Get(ctx, req.NamespacedName, user); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

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

	return c.reconcile(ctx, user)
}

func (c *Controller) reconcile(ctx context.Context, user *dbopsv1alpha1.DatabaseUser) (ctrl.Result, error) {
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
	}

	// Update user settings
	if _, err := c.handler.Update(ctx, user.Spec.Username, &user.Spec, user.Namespace); err != nil {
		log.Error(err, "Failed to update user settings")
	}

	// Create/update credentials secret
	if err := c.ensureCredentialsSecret(ctx, user, password, secretName); err != nil {
		return c.handleError(ctx, user, err, "ensure credentials secret")
	}

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

	if err := c.Status().Update(ctx, user); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(user)

	log.Info("Successfully reconciled DatabaseUser", "username", user.Spec.Username)
	return ctrl.Result{RequeueAfter: RequeueAfterReady}, nil
}

func (c *Controller) ensureCredentialsSecret(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, password, secretName string) error {
	// Get instance for connection info
	instance, err := c.handler.repo.GetInstance(ctx, &user.Spec, user.Namespace)
	if err != nil {
		return err
	}

	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: user.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "db-provision-operator",
				"dbops.dbprovision.io/user":    user.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": user.Spec.Username,
			"password": password,
			"host":     instance.Spec.Connection.Host,
			"port":     fmt.Sprintf("%d", instance.Spec.Connection.Port),
		},
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

	// Update if exists
	existing.StringData = credSecret.StringData
	existing.Labels = credSecret.Labels
	return c.Update(ctx, existing)
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
		user.Status.Phase = dbopsv1alpha1.PhaseFailed
		user.Status.Message = "Deletion blocked by deletion protection"
		util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, user); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	if deletionPolicy == dbopsv1alpha1.DeletionPolicyDelete {
		force := util.HasForceDeleteAnnotation(user)
		if err := c.handler.Delete(ctx, user.Spec.Username, &user.Spec, user.Namespace, force); err != nil {
			log.Error(err, "Failed to delete user")
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

func (c *Controller) handleError(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("user", user.Name, "namespace", user.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)

	user.Status.Phase = dbopsv1alpha1.PhaseFailed
	user.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetReadyCondition(&user.Status.Conditions, metav1.ConditionFalse,
		util.ReasonReconcileFailed, err.Error())

	if statusErr := c.Status().Update(ctx, user); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(user)

	return ctrl.Result{RequeueAfter: RequeueAfterError}, err
}

func (c *Controller) updatePhase(ctx context.Context, user *dbopsv1alpha1.DatabaseUser, phase dbopsv1alpha1.Phase, message string) {
	user.Status.Phase = phase
	user.Status.Message = message
	if err := c.Status().Update(ctx, user); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update phase", "phase", phase)
	}
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseUser{}).
		Owns(&corev1.Secret{}).
		Named("databaseuser").
		Complete(c)
}
