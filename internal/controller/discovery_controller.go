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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	"github.com/db-provision-operator/internal/logging"
	"github.com/db-provision-operator/internal/metrics"
	"github.com/db-provision-operator/internal/secret"
	"github.com/db-provision-operator/internal/service"
	"github.com/db-provision-operator/internal/service/discovery"
	"github.com/db-provision-operator/internal/util"
)

// DiscoveryReconciler reconciles DatabaseInstance objects to discover
// unmanaged database resources and optionally adopt them.
//
// This controller runs separately from the main DatabaseInstance controller
// to keep concerns separated. It:
// - Discovers databases, users, and roles that exist in the database but not in K8s
// - Updates the instance status with discovered resources
// - Handles adoption via annotations
type DiscoveryReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	SecretManager *secret.Manager
	Recorder      record.EventRecorder
}

// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances,verbs=get;list;watch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databases,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseusers,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=dbops.dbprovision.io,resources=databaseroles,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile runs discovery for a DatabaseInstance.
// Discovery is only performed if explicitly enabled in the instance spec.
func (r *DiscoveryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the DatabaseInstance
	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Skip if discovery is not enabled
	if instance.Spec.Discovery == nil || !instance.Spec.Discovery.Enabled {
		return ctrl.Result{}, nil
	}

	// Skip if instance is not ready
	if instance.Status.Phase != dbopsv1alpha1.PhaseReady {
		log.V(1).Info("Skipping discovery - instance not ready")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if reconciliation should be skipped
	if util.ShouldSkipReconcile(instance) {
		log.Info("Skipping discovery due to annotation")
		return ctrl.Result{}, nil
	}

	// Run discovery
	result, err := r.runDiscovery(ctx, instance)
	if err != nil {
		log.Error(err, "Discovery failed")
		// Don't fail hard - discovery is best-effort
		return result, nil
	}

	return result, nil
}

// runDiscovery performs the actual discovery and adoption workflow.
func (r *DiscoveryReconciler) runDiscovery(ctx context.Context, instance *dbopsv1alpha1.DatabaseInstance) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Running resource discovery", "instance", instance.Name)

	// Get credentials
	creds, err := r.SecretManager.GetCredentials(ctx, instance.Namespace, instance.Spec.Connection.SecretRef)
	if err != nil {
		log.Error(err, "Failed to get credentials for discovery")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Get TLS credentials if enabled
	var tlsCA, tlsCert, tlsKey []byte
	if instance.Spec.TLS != nil && instance.Spec.TLS.Enabled {
		tlsCreds, err := r.SecretManager.GetTLSCredentials(ctx, instance.Namespace, instance.Spec.TLS)
		if err == nil {
			tlsCA = tlsCreds.CA
			tlsCert = tlsCreds.Cert
			tlsKey = tlsCreds.Key
		}
	}

	// Build service config and create adapter
	cfg := service.ConfigFromInstance(&instance.Spec, creds.Username, creds.Password, tlsCA, tlsCert, tlsKey)
	cfg.Logger = log

	// Create instance service to get adapter
	svc, err := service.NewInstanceService(cfg)
	if err != nil {
		log.Error(err, "Failed to create instance service for discovery")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	defer func() { _ = svc.Close() }()

	// Connect
	if err := svc.Connect(ctx); err != nil {
		log.Error(err, "Failed to connect for discovery")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Create discovery service
	discoveryCfg := discovery.NewConfig(cfg, instance.Name, instance.Namespace, instance.Spec.Engine)
	discoverySvc := discovery.NewService(svc.Adapter(), r.Client, discoveryCfg)

	// Run discovery
	discoverResult, err := discoverySvc.Discover(ctx)
	if err != nil {
		log.Error(err, "Discovery scan failed")
		// Record failed discovery scan metric
		metrics.RecordDiscoveryScan(instance.Name, instance.Namespace, metrics.StatusFailure)
		// Emit event for discovery failure
		if r.Recorder != nil {
			r.Recorder.Eventf(instance, corev1.EventTypeWarning, "DiscoveryFailed",
				"Failed to discover resources: %v", err)
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Record successful discovery scan metric
	metrics.RecordDiscoveryScan(instance.Name, instance.Namespace, metrics.StatusSuccess)

	// Update status with discovered resources
	// Use the built-in ToAPIStatus() method which properly handles all fields including LastScan
	instance.Status.DiscoveredResources = discoverResult.ToAPIStatus()

	// Record discovered orphans metrics
	metrics.SetDiscoveredOrphans(instance.Name, "database", instance.Namespace, float64(len(discoverResult.Databases)))
	metrics.SetDiscoveredOrphans(instance.Name, "user", instance.Namespace, float64(len(discoverResult.Users)))
	metrics.SetDiscoveredOrphans(instance.Name, "role", instance.Namespace, float64(len(discoverResult.Roles)))

	// Emit event if new resources discovered
	if discoverResult.HasDiscoveries() {
		totalDiscovered := len(discoverResult.Databases) + len(discoverResult.Users) + len(discoverResult.Roles)
		if r.Recorder != nil {
			r.Recorder.Eventf(instance, corev1.EventTypeNormal, "ResourcesDiscovered",
				"Discovered %d unmanaged resources (%d databases, %d users, %d roles)",
				totalDiscovered,
				len(discoverResult.Databases),
				len(discoverResult.Users),
				len(discoverResult.Roles))
		}
		log.Info("Resources discovered",
			"databases", len(discoverResult.Databases),
			"users", len(discoverResult.Users),
			"roles", len(discoverResult.Roles))
	}

	// Handle adoption annotations
	r.processAdoptionAnnotations(ctx, instance, discoverySvc)

	// Update status
	if err := r.Status().Update(ctx, instance); err != nil {
		log.Error(err, "Failed to update instance status")
		return ctrl.Result{}, err
	}

	// Calculate next discovery interval
	interval := r.getDiscoveryInterval(instance)
	return ctrl.Result{RequeueAfter: interval}, nil
}

// processAdoptionAnnotations checks for adoption annotations and adopts resources.
func (r *DiscoveryReconciler) processAdoptionAnnotations(
	ctx context.Context,
	instance *dbopsv1alpha1.DatabaseInstance,
	discoverySvc *discovery.Service,
) {
	log := logf.FromContext(ctx)
	annotations := instance.Annotations
	if annotations == nil {
		return
	}

	// Handle database adoption
	if adoptDBs := annotations[dbopsv1alpha1.AnnotationAdoptDatabases]; adoptDBs != "" {
		databases := parseCSV(adoptDBs)
		if len(databases) > 0 {
			log.Info("Adopting databases", "count", len(databases))
			result, err := discoverySvc.AdoptDatabases(ctx, databases)
			if err != nil {
				log.Error(err, "Failed to adopt databases")
			} else {
				for _, adopted := range result.Adopted {
					metrics.RecordAdoption(adopted.Type, instance.Namespace, metrics.StatusSuccess)
					if r.Recorder != nil {
						r.Recorder.Eventf(instance, corev1.EventTypeNormal, "ResourceAdopted",
							"Adopted %s: %s (created CR: %s)", adopted.Type, adopted.Name, adopted.CRName)
					}
				}
				for _, failed := range result.Failed {
					metrics.RecordAdoption(failed.Type, instance.Namespace, metrics.StatusFailure)
					if r.Recorder != nil {
						r.Recorder.Eventf(instance, corev1.EventTypeWarning, "AdoptionFailed",
							"Failed to adopt %s %s: %v", failed.Type, failed.Name, failed.Error)
					}
				}
			}
			// Remove annotation after processing
			r.removeAdoptionAnnotation(ctx, instance, dbopsv1alpha1.AnnotationAdoptDatabases)
		}
	}

	// Handle user adoption
	if adoptUsers := annotations[dbopsv1alpha1.AnnotationAdoptUsers]; adoptUsers != "" {
		users := parseCSV(adoptUsers)
		if len(users) > 0 {
			log.Info("Adopting users", "count", len(users))
			result, err := discoverySvc.AdoptUsers(ctx, users)
			if err != nil {
				log.Error(err, "Failed to adopt users")
			} else {
				for _, adopted := range result.Adopted {
					metrics.RecordAdoption(adopted.Type, instance.Namespace, metrics.StatusSuccess)
					if r.Recorder != nil {
						r.Recorder.Eventf(instance, corev1.EventTypeNormal, "ResourceAdopted",
							"Adopted %s: %s (created CR: %s)", adopted.Type, adopted.Name, adopted.CRName)
					}
				}
				for _, failed := range result.Failed {
					metrics.RecordAdoption(failed.Type, instance.Namespace, metrics.StatusFailure)
				}
			}
			r.removeAdoptionAnnotation(ctx, instance, dbopsv1alpha1.AnnotationAdoptUsers)
		}
	}

	// Handle role adoption
	if adoptRoles := annotations[dbopsv1alpha1.AnnotationAdoptRoles]; adoptRoles != "" {
		roles := parseCSV(adoptRoles)
		if len(roles) > 0 {
			log.Info("Adopting roles", "count", len(roles))
			result, err := discoverySvc.AdoptRoles(ctx, roles)
			if err != nil {
				log.Error(err, "Failed to adopt roles")
			} else {
				for _, adopted := range result.Adopted {
					metrics.RecordAdoption(adopted.Type, instance.Namespace, metrics.StatusSuccess)
					if r.Recorder != nil {
						r.Recorder.Eventf(instance, corev1.EventTypeNormal, "ResourceAdopted",
							"Adopted %s: %s (created CR: %s)", adopted.Type, adopted.Name, adopted.CRName)
					}
				}
				for _, failed := range result.Failed {
					metrics.RecordAdoption(failed.Type, instance.Namespace, metrics.StatusFailure)
				}
			}
			r.removeAdoptionAnnotation(ctx, instance, dbopsv1alpha1.AnnotationAdoptRoles)
		}
	}
}

// removeAdoptionAnnotation removes an adoption annotation after it has been processed.
func (r *DiscoveryReconciler) removeAdoptionAnnotation(
	ctx context.Context,
	instance *dbopsv1alpha1.DatabaseInstance,
	annotation string,
) {
	log := logf.FromContext(ctx)

	if instance.Annotations == nil {
		return
	}

	delete(instance.Annotations, annotation)

	// Update the instance to remove the annotation
	if err := r.Update(ctx, instance); err != nil {
		log.Error(err, "Failed to remove adoption annotation", "annotation", annotation)
	}
}

// getDiscoveryInterval returns the discovery interval from instance spec or default.
func (r *DiscoveryReconciler) getDiscoveryInterval(instance *dbopsv1alpha1.DatabaseInstance) time.Duration {
	// Default interval
	defaultInterval := 30 * time.Minute

	if instance.Spec.Discovery == nil || instance.Spec.Discovery.Interval == "" {
		return defaultInterval
	}

	interval, err := time.ParseDuration(instance.Spec.Discovery.Interval)
	if err != nil {
		return defaultInterval
	}

	// Ensure minimum interval of 1 minute
	if interval < time.Minute {
		return time.Minute
	}

	return interval
}

// parseCSV parses a comma-separated string into a slice.
func parseCSV(csv string) []string {
	if csv == "" {
		return nil
	}

	parts := strings.Split(csv, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
func (r *DiscoveryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.SecretManager == nil {
		r.SecretManager = secret.NewManager(r.Client)
	}
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("discovery-controller")
	}

	return logging.BuildController(mgr).
		For(&dbopsv1alpha1.DatabaseInstance{}).
		Named("discovery").
		Complete(r)
}
