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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	driftsvc "github.com/db-provision-operator/internal/service/drift"
)

// grantDriftableResource wraps DatabaseGrant to implement drift.DriftableResource.
type grantDriftableResource struct {
	*dbopsv1alpha1.DatabaseGrant
}

func (r *grantDriftableResource) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	return r.Spec.DriftPolicy
}

func (r *grantDriftableResource) SetDriftStatus(status *dbopsv1alpha1.DriftStatus) {
	r.Status.Drift = status
}

// grantDriftDetector wraps Handler to implement drift.Detector.
type grantDriftDetector struct {
	handler   *Handler
	spec      *dbopsv1alpha1.DatabaseGrantSpec
	namespace string
}

func (d *grantDriftDetector) DetectDrift(ctx context.Context, allowDestructive bool) (*driftsvc.Result, error) {
	return d.handler.DetectDrift(ctx, d.spec, d.namespace, allowDestructive)
}

func (d *grantDriftDetector) CorrectDrift(ctx context.Context, driftResult *driftsvc.Result, allowDestructive bool) (*driftsvc.CorrectionResult, error) {
	return d.handler.CorrectDrift(ctx, d.spec, d.namespace, driftResult, allowDestructive)
}
