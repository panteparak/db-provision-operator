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

package drift

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	driftsvc "github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
	"github.com/db-provision-operator/internal/util"
)

// Detector abstracts the handler's DetectDrift/CorrectDrift methods.
// Each controller provides an adapter that wraps its typed handler.
type Detector interface {
	DetectDrift(ctx context.Context, allowDestructive bool) (*driftsvc.Result, error)
	CorrectDrift(ctx context.Context, driftResult *driftsvc.Result, allowDestructive bool) (*driftsvc.CorrectionResult, error)
}

// DriftableResource abstracts access to the resource's drift policy and status.
// Each controller wraps its typed resource to implement this.
type DriftableResource interface {
	client.Object
	GetDriftPolicy() *dbopsv1alpha1.DriftPolicy
	SetDriftStatus(status *dbopsv1alpha1.DriftStatus)
}

// InstanceDriftPolicy provides the fallback drift policy from the parent instance.
type InstanceDriftPolicy interface {
	GetDriftPolicy() *dbopsv1alpha1.DriftPolicy
}

// Orchestrator performs drift detection and correction, replacing duplicated
// logic across 6 controllers with a single implementation.
type Orchestrator struct {
	Recorder             record.EventRecorder
	DefaultDriftInterval time.Duration
}

// PerformDriftDetection detects and optionally corrects drift for a resource.
func (o *Orchestrator) PerformDriftDetection(ctx context.Context, resource DriftableResource, instance InstanceDriftPolicy, detector Detector) {
	log := logf.FromContext(ctx)

	policy := getEffectiveDriftPolicy(resource, instance, o.DefaultDriftInterval)
	if policy.Mode == dbopsv1alpha1.DriftModeIgnore {
		log.V(1).Info("Drift detection disabled (mode=ignore)")
		return
	}

	allowDestructive := hasDestructiveDriftAnnotation(resource)

	driftResult, err := detector.DetectDrift(ctx, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to detect drift")
		return
	}

	if driftResult == nil {
		return
	}

	resource.SetDriftStatus(driftResult.ToAPIStatus())

	if driftResult.HasDrift() {
		var driftFields []string
		for _, d := range driftResult.Diffs {
			driftFields = append(driftFields, d.Field)
		}
		o.Recorder.Eventf(resource, corev1.EventTypeWarning, "DriftDetected",
			"Configuration drift detected in fields: %v", driftFields)
		log.Info("Drift detected", "fields", driftFields)

		if policy.Mode == dbopsv1alpha1.DriftModeCorrect {
			o.correctDrift(ctx, resource, driftResult, detector, allowDestructive)
		}
	}
}

// GetRequeueInterval returns the requeue interval for the resource based on drift policy.
func (o *Orchestrator) GetRequeueInterval(resource DriftableResource, instance InstanceDriftPolicy) time.Duration {
	policy := getEffectiveDriftPolicy(resource, instance, o.DefaultDriftInterval)
	return util.ParseDriftRequeueInterval(policy, o.DefaultDriftInterval)
}

func (o *Orchestrator) correctDrift(ctx context.Context, resource DriftableResource, driftResult *driftsvc.Result, detector Detector, allowDestructive bool) {
	log := logf.FromContext(ctx)

	// Re-detect drift to get fresh state for correction
	handlerDriftResult, err := detector.DetectDrift(ctx, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to re-detect drift for correction")
		return
	}

	if handlerDriftResult == nil || !handlerDriftResult.HasDrift() {
		log.V(1).Info("No drift to correct")
		return
	}

	correctionResult, err := detector.CorrectDrift(ctx, handlerDriftResult, allowDestructive)
	if err != nil {
		log.Error(err, "Failed to correct drift")
		o.Recorder.Eventf(resource, corev1.EventTypeWarning, "DriftCorrectionFailed",
			"Failed to correct drift: %v", err)
		return
	}

	if correctionResult != nil && correctionResult.HasCorrections() {
		var correctedFields []string
		for _, corr := range correctionResult.Corrected {
			correctedFields = append(correctedFields, corr.Diff.Field)
		}
		o.Recorder.Eventf(resource, corev1.EventTypeNormal, "DriftCorrected",
			"Drift corrected for fields: %v", correctedFields)
		log.Info("Drift corrected", "fields", correctedFields)

		// Clear drift status after successful correction
		resource.SetDriftStatus(driftResult.ToAPIStatus())
		status := driftResult.ToAPIStatus()
		status.Detected = false
		status.Diffs = nil
		resource.SetDriftStatus(status)
	}

	if correctionResult != nil && len(correctionResult.Skipped) > 0 {
		for _, s := range correctionResult.Skipped {
			log.V(1).Info("Drift correction skipped", "field", s.Diff.Field, "reason", s.Reason)
		}
	}

	if correctionResult != nil && correctionResult.HasFailures() {
		for _, f := range correctionResult.Failed {
			log.Error(f.Error, "Drift correction failed", "field", f.Diff.Field)
		}
	}
}

func getEffectiveDriftPolicy(resource DriftableResource, instance InstanceDriftPolicy, defaultInterval time.Duration) dbopsv1alpha1.DriftPolicy {
	if p := resource.GetDriftPolicy(); p != nil {
		return *p
	}

	if instance != nil {
		if p := instance.GetDriftPolicy(); p != nil {
			return *p
		}
	}

	return dbopsv1alpha1.DriftPolicy{
		Mode:     dbopsv1alpha1.DriftModeDetect,
		Interval: defaultInterval.String(),
	}
}

func hasDestructiveDriftAnnotation(resource DriftableResource) bool {
	annotations := resource.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[dbopsv1alpha1.AnnotationAllowDestructiveDrift] == "true"
}

// NamespacedInstancePolicy wraps DatabaseInstance to implement InstanceDriftPolicy.
type NamespacedInstancePolicy struct {
	Instance *dbopsv1alpha1.DatabaseInstance
}

func (p *NamespacedInstancePolicy) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	if p.Instance != nil {
		return p.Instance.Spec.DriftPolicy
	}
	return nil
}

// ClusterInstancePolicy wraps ClusterDatabaseInstance to implement InstanceDriftPolicy.
type ClusterInstancePolicy struct {
	Instance *dbopsv1alpha1.ClusterDatabaseInstance
}

func (p *ClusterInstancePolicy) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	if p.Instance != nil {
		return p.Instance.Spec.DriftPolicy
	}
	return nil
}

// ResolvedInstancePolicy wraps instanceresolver.ResolvedInstance to implement InstanceDriftPolicy.
type ResolvedInstancePolicy struct {
	Resolved *instanceresolver.ResolvedInstance
}

func (p *ResolvedInstancePolicy) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	if p.Resolved != nil && p.Resolved.Spec != nil {
		return p.Resolved.Spec.DriftPolicy
	}
	return nil
}
