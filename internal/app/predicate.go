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

package app

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// InstanceIDPredicate filters events so that each operator instance only
// reconciles resources assigned to it via the operator-instance-id label.
//
// Matching rules:
//   - Label matches InstanceID → accept
//   - Label absent AND InstanceID == "default" → accept (backward compat)
//   - Otherwise → reject
type InstanceIDPredicate struct {
	predicate.Funcs
	InstanceID string
}

// NewInstanceIDPredicate creates a predicate for the given operator instance ID.
func NewInstanceIDPredicate(instanceID string) InstanceIDPredicate {
	return InstanceIDPredicate{InstanceID: instanceID}
}

func (p InstanceIDPredicate) matches(obj client.Object) bool {
	if obj == nil {
		return false
	}
	labels := obj.GetLabels()
	val, exists := labels[dbopsv1alpha1.LabelOperatorInstanceID]
	if !exists {
		// Unlabeled resources belong to the "default" operator
		return p.InstanceID == "default"
	}
	return val == p.InstanceID
}

// Create filters create events.
func (p InstanceIDPredicate) Create(e event.CreateEvent) bool {
	return p.matches(e.Object)
}

// Update filters update events.
func (p InstanceIDPredicate) Update(e event.UpdateEvent) bool {
	return p.matches(e.ObjectNew)
}

// Delete filters delete events.
func (p InstanceIDPredicate) Delete(e event.DeleteEvent) bool {
	return p.matches(e.Object)
}

// Generic filters generic events.
func (p InstanceIDPredicate) Generic(e event.GenericEvent) bool {
	return p.matches(e.Object)
}
