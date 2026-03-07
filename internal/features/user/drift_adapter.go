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

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	driftsvc "github.com/db-provision-operator/internal/service/drift"
)

// userDriftableResource wraps DatabaseUser to implement drift.DriftableResource.
type userDriftableResource struct {
	*dbopsv1alpha1.DatabaseUser
}

func (r *userDriftableResource) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	return r.Spec.DriftPolicy
}

func (r *userDriftableResource) SetDriftStatus(status *dbopsv1alpha1.DriftStatus) {
	r.Status.Drift = status
}

// userDriftDetector wraps Handler to implement drift.Detector.
type userDriftDetector struct {
	handler   *Handler
	spec      *dbopsv1alpha1.DatabaseUserSpec
	namespace string
}

func (d *userDriftDetector) DetectDrift(ctx context.Context, allowDestructive bool) (*driftsvc.Result, error) {
	return d.handler.DetectDrift(ctx, d.spec, d.namespace, allowDestructive)
}

func (d *userDriftDetector) CorrectDrift(ctx context.Context, driftResult *driftsvc.Result, allowDestructive bool) (*driftsvc.CorrectionResult, error) {
	return d.handler.CorrectDrift(ctx, d.spec, d.namespace, driftResult, allowDestructive)
}
