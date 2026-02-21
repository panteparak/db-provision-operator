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

package logging

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ControllerBuilder wraps controller-runtime's builder to apply standard
// middleware (reconcileID, etc.) to every controller globally.
//
// Usage:
//
//	return logging.BuildController(mgr).
//	    For(&dbopsv1alpha1.DatabaseInstance{}).
//	    Named("databaseinstance").
//	    Complete(c)
type ControllerBuilder struct {
	mgr        ctrl.Manager
	obj        client.Object
	name       string
	owns       []client.Object
	predicates []predicate.Predicate
}

// BuildController creates a builder that auto-applies all standard middleware.
// This is the SINGLE POINT where cross-cutting reconciler concerns are applied.
func BuildController(mgr ctrl.Manager) *ControllerBuilder {
	return &ControllerBuilder{mgr: mgr}
}

// For sets the primary resource this controller reconciles.
func (b *ControllerBuilder) For(obj client.Object) *ControllerBuilder {
	b.obj = obj
	return b
}

// Named sets the controller name used for logging and metrics.
func (b *ControllerBuilder) Named(name string) *ControllerBuilder {
	b.name = name
	return b
}

// Owns registers a resource type as owned by the primary resource.
// The controller will watch for changes to owned resources and
// enqueue the owning resource for reconciliation.
func (b *ControllerBuilder) Owns(obj client.Object) *ControllerBuilder {
	b.owns = append(b.owns, obj)
	return b
}

// WithPredicates adds event predicates that filter which events trigger reconciliation.
func (b *ControllerBuilder) WithPredicates(predicates ...predicate.Predicate) *ControllerBuilder {
	b.predicates = append(b.predicates, predicates...)
	return b
}

// WithEventFilter adds a single event predicate that filters which events trigger reconciliation.
func (b *ControllerBuilder) WithEventFilter(p predicate.Predicate) *ControllerBuilder {
	b.predicates = append(b.predicates, p)
	return b
}

// Complete registers the controller with all standard middleware applied.
// Middleware chain (applied in order):
//  1. ReconcileID injection — unique correlation ID per reconciliation cycle
//
// Add future middleware here — tracing, metrics, audit, etc.
func (b *ControllerBuilder) Complete(r reconcile.Reconciler) error {
	builder := ctrl.NewControllerManagedBy(b.mgr).
		For(b.obj).
		Named(b.name)
	for _, o := range b.owns {
		builder = builder.Owns(o)
	}
	for _, p := range b.predicates {
		builder = builder.WithEventFilter(p)
	}
	return builder.Complete(&withReconcileID{inner: r})
}
