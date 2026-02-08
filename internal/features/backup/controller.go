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

package backup

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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/reconcileutil"
	"github.com/db-provision-operator/internal/util"
)

const (
	RequeueAfterError   = 30 * time.Second
	RequeueAfterPending = 10 * time.Second
)

// Controller handles K8s reconciliation for DatabaseBackup resources.
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

// NewController creates a new backup controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:   cfg.Client,
		Scheme:   cfg.Scheme,
		Recorder: cfg.Recorder,
		handler:  cfg.Handler,
		logger:   cfg.Logger,
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for DatabaseBackup resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("backup", req.NamespacedName)

	// Fetch the DatabaseBackup resource
	backup := &dbopsv1alpha1.DatabaseBackup{}
	if err := c.Get(ctx, req.NamespacedName, backup); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseBackup")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(backup) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(backup) {
		return c.handleDeletion(ctx, backup)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(backup, util.FinalizerDatabaseBackup) {
		controllerutil.AddFinalizer(backup, util.FinalizerDatabaseBackup)
		if err := c.Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if backup is already completed - handle expiration
	if backup.Status.Phase == dbopsv1alpha1.PhaseCompleted {
		return c.handleCompletedBackup(ctx, backup)
	}

	// Don't retry failed backups automatically
	if backup.Status.Phase == dbopsv1alpha1.PhaseFailed {
		return ctrl.Result{}, nil
	}

	return c.reconcile(ctx, backup)
}

func (c *Controller) reconcile(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("backup", backup.Name, "namespace", backup.Namespace)

	// Set initial phase
	if backup.Status.Phase == "" {
		backup.Status.Phase = dbopsv1alpha1.PhasePending
		backup.Status.Message = "Initializing backup"
		if err := c.Status().Update(ctx, backup); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Check if database exists and is ready
	database, err := c.handler.GetDatabase(ctx, backup)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handleError(ctx, backup, fmt.Errorf("database %s not found", backup.Spec.DatabaseRef.Name), "DatabaseNotFound")
		}
		return c.handleError(ctx, backup, err, "get database")
	}

	if database.Status.Phase != dbopsv1alpha1.PhaseReady {
		return c.handlePending(ctx, backup, fmt.Sprintf("Waiting for Database %s to be ready", database.Name))
	}

	// Check if instance is ready
	instance, err := c.handler.GetInstance(ctx, database)
	if err != nil {
		if errors.IsNotFound(err) {
			return c.handleError(ctx, backup, fmt.Errorf("instance %s not found", database.Spec.InstanceRef.Name), "InstanceNotFound")
		}
		return c.handleError(ctx, backup, err, "get instance")
	}

	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		return c.handlePending(ctx, backup, fmt.Sprintf("Waiting for DatabaseInstance %s to be ready", instance.Name))
	}

	// Check instance connectivity
	if !c.handler.IsInstanceConnected(instance.Namespace, instance.Name) {
		return c.handlePending(ctx, backup, fmt.Sprintf("Waiting for DatabaseInstance %s to reconnect", instance.Name))
	}

	// Update status to Running
	now := metav1.Now()
	backup.Status.Phase = dbopsv1alpha1.PhaseRunning
	backup.Status.Message = "Backup in progress"
	backup.Status.StartedAt = &now
	c.Recorder.Eventf(backup, corev1.EventTypeNormal, "Started", "Backup started for database %s", database.Spec.Name)
	backup.Status.Source = &dbopsv1alpha1.BackupSourceInfo{
		Instance:  instance.Name,
		Database:  database.Spec.Name,
		Engine:    string(instance.Spec.Engine),
		Version:   instance.Status.Version,
		Timestamp: &now,
	}
	if err := c.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Execute backup via handler
	result, err := c.handler.Execute(ctx, backup)
	if err != nil {
		return c.handleError(ctx, backup, err, "execute backup")
	}

	// Calculate expiration time from TTL
	var expiresAt *metav1.Time
	if backup.Spec.TTL != "" {
		ttlDuration, err := time.ParseDuration(backup.Spec.TTL)
		if err == nil {
			expTime := metav1.NewTime(time.Now().Add(ttlDuration))
			expiresAt = &expTime
		}
	}

	// Update status to Completed
	completedAt := metav1.Now()
	backup.Status.Phase = dbopsv1alpha1.PhaseCompleted
	backup.Status.Message = result.Message
	backup.Status.CompletedAt = &completedAt
	backup.Status.ExpiresAt = expiresAt
	backup.Status.Backup = &dbopsv1alpha1.BackupInfo{
		Path:                result.Path,
		SizeBytes:           result.SizeBytes,
		CompressedSizeBytes: result.CompressedSizeBytes,
		Checksum:            result.Checksum,
		Format:              result.Format,
	}
	util.SetSyncedCondition(&backup.Status.Conditions, metav1.ConditionTrue, "BackupCompleted", "Backup completed successfully")
	util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionTrue, "Ready", "DatabaseBackup is ready")

	if err := c.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(backup)

	c.Recorder.Eventf(backup, corev1.EventTypeNormal, "Completed", "Backup completed successfully, size: %d bytes", result.SizeBytes)

	log.Info("DatabaseBackup completed successfully",
		"path", result.Path,
		"size", result.SizeBytes,
		"duration", result.Duration)

	// Requeue to handle expiration
	if expiresAt != nil {
		return ctrl.Result{RequeueAfter: time.Until(expiresAt.Time)}, nil
	}

	return ctrl.Result{}, nil
}

func (c *Controller) handleCompletedBackup(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("backup", backup.Name, "namespace", backup.Namespace)

	// Check if expired
	if c.handler.IsExpired(backup) {
		log.Info("Backup has expired, deleting", "expiresAt", backup.Status.ExpiresAt)
		if err := c.Delete(ctx, backup); err != nil {
			log.Error(err, "Failed to delete expired backup")
			return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	// Requeue to check expiration later
	if backup.Status.ExpiresAt != nil {
		requeueAfter := time.Until(backup.Status.ExpiresAt.Time)
		if requeueAfter > 0 {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	return ctrl.Result{}, nil
}

func (c *Controller) handleDeletion(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("backup", backup.Name, "namespace", backup.Namespace)

	if !controllerutil.ContainsFinalizer(backup, util.FinalizerDatabaseBackup) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseBackup")

	// Update status to Deleting
	backup.Status.Phase = dbopsv1alpha1.PhaseDeleting
	backup.Status.Message = "Cleaning up backup"
	if err := c.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update status")
	}

	// Delete backup via handler
	if err := c.handler.Delete(ctx, backup); err != nil {
		log.Error(err, "Failed to delete backup")
		// Continue with finalizer removal even if backup deletion fails
	}

	// Clean up info metric
	c.handler.CleanupInfoMetric(backup)

	// Remove finalizer
	controllerutil.RemoveFinalizer(backup, util.FinalizerDatabaseBackup)
	if err := c.Update(ctx, backup); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseBackup")
	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("backup", backup.Name, "namespace", backup.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)
	c.Recorder.Eventf(backup, corev1.EventTypeWarning, "Failed", "Backup failed to %s: %v", operation, err)

	backup.Status.Phase = dbopsv1alpha1.PhaseFailed
	backup.Status.Message = fmt.Sprintf("Failed to %s: %v", operation, err)
	util.SetSyncedCondition(&backup.Status.Conditions, metav1.ConditionFalse, "BackupFailed", backup.Status.Message)
	util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, "BackupFailed", backup.Status.Message)

	if statusErr := c.Status().Update(ctx, backup); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(backup)

	return reconcileutil.ClassifyRequeue(err)
}

func (c *Controller) handlePending(ctx context.Context, backup *dbopsv1alpha1.DatabaseBackup, message string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("backup", backup.Name, "namespace", backup.Namespace)

	log.V(1).Info("Backup pending", "reason", message)

	backup.Status.Phase = dbopsv1alpha1.PhasePending
	backup.Status.Message = message
	util.SetReadyCondition(&backup.Status.Conditions, metav1.ConditionFalse, util.ReasonReconciling, message)

	if err := c.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update status")
	}

	return ctrl.Result{RequeueAfter: RequeueAfterPending}, nil
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseBackup{}).
		Named("databasebackup-feature").
		Complete(c)
}
