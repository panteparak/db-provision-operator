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
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// reconcileIDKey is the unexported context key for storing the reconcile ID.
type reconcileIDKey struct{}

// GenerateID returns a random 8-character lowercase hex string suitable
// for log correlation. Uses crypto/rand for uniqueness.
func GenerateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// withReconcileID is a reconciler decorator that injects a unique reconcileID
// into the context logger before each reconciliation. This enables filtering
// all log lines from a single reconciliation cycle.
type withReconcileID struct {
	inner reconcile.Reconciler
}

// Reconcile implements reconcile.Reconciler by enriching the context with
// a unique reconcileID, then delegating to the inner reconciler.
func (w *withReconcileID) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	id := GenerateID()
	log := logf.FromContext(ctx).WithValues("reconcileID", id)
	ctx = logr.NewContext(ctx, log)
	ctx = context.WithValue(ctx, reconcileIDKey{}, id)
	return w.inner.Reconcile(ctx, req)
}

// IDFromContext retrieves the reconcileID from context.
// Returns an empty string if no reconcileID is present.
func IDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(reconcileIDKey{}).(string); ok {
		return id
	}
	return ""
}
