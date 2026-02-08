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

package backupschedule

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
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/util"
)

const (
	// RequeueAfterError is the requeue duration after an error.
	RequeueAfterError = 30 * time.Second

	// MaxRequeueAfter is the maximum time to wait before requeuing.
	MaxRequeueAfter = 1 * time.Hour

	// MinRequeueAfter is the minimum time to wait before requeuing.
	MinRequeueAfter = 10 * time.Second
)

// Controller handles K8s reconciliation for DatabaseBackupSchedule resources.
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

// NewController creates a new backupschedule controller.
func NewController(cfg ControllerConfig) *Controller {
	return &Controller{
		Client:   cfg.Client,
		Scheme:   cfg.Scheme,
		Recorder: cfg.Recorder,
		handler:  cfg.Handler,
		logger:   cfg.Logger,
	}
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackupschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackupschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackupschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databasebackups,verbs=get;list;watch;create;update;patch;delete

// Reconcile implements the reconciliation loop for DatabaseBackupSchedule resources.
func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("schedule", req.NamespacedName)

	// Fetch the DatabaseBackupSchedule resource
	schedule := &dbopsv1alpha1.DatabaseBackupSchedule{}
	if err := c.Get(ctx, req.NamespacedName, schedule); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Unable to fetch DatabaseBackupSchedule")
		return ctrl.Result{}, err
	}

	// Check for skip-reconcile annotation
	if util.ShouldSkipReconcile(schedule) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(schedule) {
		return c.handleDeletion(ctx, schedule)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(schedule, util.FinalizerDatabaseBackupSchedule) {
		controllerutil.AddFinalizer(schedule, util.FinalizerDatabaseBackupSchedule)
		if err := c.Update(ctx, schedule); err != nil {
			return ctrl.Result{}, err
		}
	}

	return c.reconcile(ctx, schedule)
}

func (c *Controller) reconcile(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	// Handle paused schedule
	if schedule.Spec.Paused {
		return c.handlePaused(ctx, schedule)
	}

	// Evaluate the schedule
	evaluation, err := c.handler.EvaluateSchedule(ctx, schedule)
	if err != nil {
		return c.handleError(ctx, schedule, err, "evaluate schedule")
	}

	// Update next backup time in status and metrics
	nextBackupTime := metav1.NewTime(evaluation.NextBackupTime)
	schedule.Status.NextBackupTime = &nextBackupTime

	databaseName := schedule.Spec.Template.Spec.DatabaseRef.Name
	metrics.SetScheduleNextBackup(databaseName, schedule.Namespace, float64(evaluation.NextBackupTime.Unix()))

	// Handle concurrency if backup is due
	if evaluation.BackupDue && evaluation.RunningBackups > 0 {
		concurrencyResult, err := c.handler.HandleConcurrency(ctx, schedule)
		if err != nil {
			return c.handleError(ctx, schedule, err, "handle concurrency")
		}

		if !concurrencyResult.ShouldProceed {
			schedule.Status.Phase = dbopsv1alpha1.PhaseActive
			util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionTrue, "WaitingForCompletion", concurrencyResult.Message)
			if err := c.Status().Update(ctx, schedule); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: RequeueAfterError}, nil
		}
	}

	// Trigger backup if due
	if evaluation.BackupDue {
		result, err := c.handler.TriggerBackup(ctx, schedule)
		if err != nil {
			return c.handleError(ctx, schedule, err, "trigger backup")
		}

		if result.Triggered {
			// Update last backup info
			now := metav1.Now()
			schedule.Status.LastBackup = &dbopsv1alpha1.ScheduledBackupInfo{
				Name:      result.BackupName,
				Status:    string(dbopsv1alpha1.PhasePending),
				StartedAt: &now,
			}

			// Increment total backups in statistics
			if schedule.Status.Statistics == nil {
				schedule.Status.Statistics = &dbopsv1alpha1.BackupStatistics{}
			}
			schedule.Status.Statistics.TotalBackups++

			c.Recorder.Eventf(schedule, corev1.EventTypeNormal, "BackupTriggered", "Scheduled backup %s triggered", result.BackupName)
			log.Info("Triggered scheduled backup", "backup", result.BackupName)
		}
	}

	// Get all backups for this schedule
	backups, err := c.handler.ListBackupsForSchedule(ctx, schedule)
	if err != nil {
		log.Error(err, "Failed to list backups for schedule")
	} else {
		// Apply retention policy
		if schedule.Spec.Retention != nil {
			if _, err := c.handler.EnforceRetention(ctx, schedule); err != nil {
				log.Error(err, "Failed to enforce retention policy")
			}
		}

		// Update recent backups list
		schedule.Status.RecentBackups = c.handler.GetRecentBackupsList(schedule, backups)

		// Update statistics
		stats, err := c.handler.UpdateStatistics(ctx, schedule)
		if err != nil {
			log.Error(err, "Failed to update statistics")
		} else {
			schedule.Status.Statistics = stats
		}

		// Update last backup status from latest backup
		c.handler.UpdateLastBackupStatus(schedule, backups)
	}

	// Set status to active
	schedule.Status.Phase = dbopsv1alpha1.PhaseActive
	util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionTrue, "Active", "Schedule is active")

	if err := c.Status().Update(ctx, schedule); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(schedule)

	// Calculate requeue time
	requeueAfter := time.Until(evaluation.NextBackupTime)
	if requeueAfter < 0 {
		requeueAfter = MinRequeueAfter
	} else if requeueAfter > MaxRequeueAfter {
		requeueAfter = MaxRequeueAfter
	}

	log.V(1).Info("Reconciliation complete", "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (c *Controller) handlePaused(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	// Only record event when transitioning to paused
	if schedule.Status.Phase != dbopsv1alpha1.PhasePaused {
		c.Recorder.Eventf(schedule, corev1.EventTypeNormal, "Paused", "Backup schedule paused")
	}

	schedule.Status.Phase = dbopsv1alpha1.PhasePaused
	schedule.Status.NextBackupTime = nil
	util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionTrue, "Paused", "Schedule is paused")

	if err := c.Status().Update(ctx, schedule); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Update info metric for Grafana table views
	c.handler.UpdateInfoMetric(schedule)

	log.Info("Schedule is paused")
	return ctrl.Result{}, nil
}

func (c *Controller) handleDeletion(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	if !controllerutil.ContainsFinalizer(schedule, util.FinalizerDatabaseBackupSchedule) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseBackupSchedule")

	// Check deletion protection
	if c.hasDeletionProtection(schedule) && !util.HasForceDeleteAnnotation(schedule) {
		c.Recorder.Eventf(schedule, corev1.EventTypeWarning, "DeletionBlocked", "Deletion blocked by deletion protection")
		schedule.Status.Phase = dbopsv1alpha1.PhaseFailed
		util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionFalse,
			util.ReasonDeletionProtected, "Deletion protection is enabled")
		if err := c.Status().Update(ctx, schedule); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, fmt.Errorf("deletion protection enabled")
	}

	// Clean up schedule metrics
	databaseName := schedule.Spec.Template.Spec.DatabaseRef.Name
	metrics.DeleteScheduleMetrics(databaseName, schedule.Namespace)

	// Clean up info metric
	c.handler.CleanupInfoMetric(schedule)

	// Remove finalizer
	controllerutil.RemoveFinalizer(schedule, util.FinalizerDatabaseBackupSchedule)
	if err := c.Update(ctx, schedule); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled deletion of DatabaseBackupSchedule")
	return ctrl.Result{}, nil
}

func (c *Controller) handleError(ctx context.Context, schedule *dbopsv1alpha1.DatabaseBackupSchedule, err error, operation string) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("schedule", schedule.Name, "namespace", schedule.Namespace)

	log.Error(err, "Reconciliation failed", "operation", operation)
	c.Recorder.Eventf(schedule, corev1.EventTypeWarning, "ReconcileFailed", "Failed to %s: %v", operation, err)

	schedule.Status.Phase = dbopsv1alpha1.PhaseActive // Keep Active but set error condition
	util.SetReadyCondition(&schedule.Status.Conditions, metav1.ConditionFalse, "Error",
		fmt.Sprintf("Failed to %s: %v", operation, err))

	if statusErr := c.Status().Update(ctx, schedule); statusErr != nil {
		log.Error(statusErr, "Failed to update status")
	}

	// Update info metric for Grafana table views (even on error)
	c.handler.UpdateInfoMetric(schedule)

	return ctrl.Result{RequeueAfter: RequeueAfterError}, err
}

// SetupWithManager registers the controller with the manager.
func (c *Controller) SetupWithManager(mgr ctrl.Manager) error {
	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseBackupSchedule{}).
		Owns(&dbopsv1alpha1.DatabaseBackup{}).
		Named("databasebackupschedule").
		Complete(c)
}

// hasDeletionProtection checks if the schedule has deletion protection enabled.
func (c *Controller) hasDeletionProtection(schedule *dbopsv1alpha1.DatabaseBackupSchedule) bool {
	return schedule.Spec.DeletionProtection
}
