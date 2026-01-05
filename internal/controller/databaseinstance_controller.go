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

package controller

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/adapter"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/util"
)

// DatabaseInstanceReconciler reconciles a DatabaseInstance object
type DatabaseInstanceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile reconciles a DatabaseInstance object
func (r *DatabaseInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseInstance
	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(instance) {
		log.Info("Skipping reconciliation due to annotation")
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if util.IsMarkedForDeletion(instance) {
		return r.handleDeletion(ctx, instance)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(instance, util.FinalizerDatabaseInstance) {
		controllerutil.AddFinalizer(instance, util.FinalizerDatabaseInstance)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile the instance
	result, err := r.reconcileInstance(ctx, instance)
	if err != nil {
		log.Error(err, "Failed to reconcile instance")
		// Update status with error
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = err.Error()
		util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonReconcileFailed, err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return result, err
	}

	return result, nil
}

// reconcileInstance handles the main reconciliation logic
func (r *DatabaseInstanceReconciler) reconcileInstance(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get credentials
	creds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = fmt.Sprintf("Failed to get credentials: %v", err)
		util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonSecretNotFound, err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Get TLS credentials if enabled
	var tlsCreds *secret.TLSCredentials
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err = r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if err != nil {
			instance.Status.Phase = dbopsv1alpha1.PhaseFailed
			instance.Status.Message = fmt.Sprintf("Failed to get TLS credentials: %v", err)
			util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionFalse,
				util.ReasonSecretNotFound, err.Error())
			if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, err
		}
	}

	// Build connection config
	var tlsCA, tlsCert, tlsKey []byte
	if tlsCreds != nil {
		tlsCA = tlsCreds.CA
		tlsCert = tlsCreds.Cert
		tlsKey = tlsCreds.Key
	}
	config := adapter.BuildConnectionConfig(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)

	// Create adapter
	dbAdapter, err := adapter.NewAdapter(instance.Spec.Engine, config)
	if err != nil {
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = fmt.Sprintf("Failed to create adapter: %v", err)
		util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonConnectionFailed, err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	defer dbAdapter.Close()

	// Connect to database
	if err := dbAdapter.Connect(ctx); err != nil {
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = fmt.Sprintf("Failed to connect: %v", err)
		util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonConnectionFailed, err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Ping to verify connection
	if err := dbAdapter.Ping(ctx); err != nil {
		instance.Status.Phase = dbopsv1alpha1.PhaseFailed
		instance.Status.Message = fmt.Sprintf("Failed to ping: %v", err)
		util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionFalse,
			util.ReasonConnectionFailed, err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	// Get version
	version, err := dbAdapter.GetVersion(ctx)
	if err != nil {
		log.Error(err, "Failed to get version")
	}

	// Update status
	now := metav1.Now()
	instance.Status.Phase = dbopsv1alpha1.PhaseReady
	instance.Status.Message = "Connected successfully"
	instance.Status.Version = version
	instance.Status.LastCheckedAt = &now

	// Set conditions
	util.SetConnectedCondition(&instance.Status.Conditions, metav1.ConditionTrue,
		util.ReasonConnectionSuccess, "Successfully connected to database")
	util.SetHealthyCondition(&instance.Status.Conditions, metav1.ConditionTrue,
		util.ReasonHealthCheckPassed, "Database is healthy")
	util.SetReadyCondition(&instance.Status.Conditions, metav1.ConditionTrue,
		util.ReasonReconcileSuccess, "Instance is ready")

	if err := r.Status().Update(ctx, instance); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled DatabaseInstance",
		"name", instance.Name,
		"engine", instance.Spec.Engine,
		"version", version)

	// Requeue for health check
	healthCheckInterval := 60 * time.Second
	if instance.Spec.HealthCheck != nil && instance.Spec.HealthCheck.IntervalSeconds > 0 {
		healthCheckInterval = time.Duration(instance.Spec.HealthCheck.IntervalSeconds) * time.Second
	}

	return ctrl.Result{RequeueAfter: healthCheckInterval}, nil
}

// handleDeletion handles the deletion of a DatabaseInstance
func (r *DatabaseInstanceReconciler) handleDeletion(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(instance, util.FinalizerDatabaseInstance) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of DatabaseInstance", "name", instance.Name)

	// Remove finalizer
	controllerutil.RemoveFinalizer(instance, util.FinalizerDatabaseInstance)
	if err := r.Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted DatabaseInstance", "name", instance.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize secret manager if not set
	if r.SecretManager == nil {
		r.SecretManager = secret.NewManager(r.Client)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&dbopsv1alpha1.DatabaseInstance{}).
		Named("databaseinstance").
		Complete(r)
}
