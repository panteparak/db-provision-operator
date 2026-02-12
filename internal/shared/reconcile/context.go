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

// Package reconcile provides utilities for tracking reconciliation context,
// including unique reconcileID for end-to-end tracing across logs, events, and status.
package reconcile

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// reconcileIDKey is the context key for storing the reconcileID.
type reconcileIDKey struct{}

// NewID generates a new unique reconcile ID (8-character hex string).
// This ID is used to correlate logs, events, and status updates for a single reconciliation.
func NewID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if random fails
		return "00000000"
	}
	return hex.EncodeToString(b)
}

// WithReconcileID adds a new reconcileID to the context and returns the updated context
// along with a logger that includes the reconcileID.
// This should be called at the start of each reconciliation.
func WithReconcileID(ctx context.Context) (context.Context, logr.Logger, string) {
	reconcileID := NewID()
	ctx = context.WithValue(ctx, reconcileIDKey{}, reconcileID)

	// Get the logger from context and add reconcileID
	log := logf.FromContext(ctx).WithValues("reconcileID", reconcileID)

	return ctx, log, reconcileID
}

// WithExistingReconcileID adds an existing reconcileID to the context.
// This is useful when resuming a reconciliation or passing through handlers.
func WithExistingReconcileID(ctx context.Context, reconcileID string) context.Context {
	return context.WithValue(ctx, reconcileIDKey{}, reconcileID)
}

// FromContext extracts the reconcileID from the context.
// Returns an empty string if no reconcileID is set.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(reconcileIDKey{}).(string); ok {
		return id
	}
	return ""
}

// LoggerFromContext returns a logger with reconcileID added if present.
// If no reconcileID is in the context, returns the base logger from context.
func LoggerFromContext(ctx context.Context) logr.Logger {
	log := logf.FromContext(ctx)
	if id := FromContext(ctx); id != "" {
		return log.WithValues("reconcileID", id)
	}
	return log
}

// EventMessage formats an event message with the reconcileID prefix.
// Format: "[reconcileID] message"
func EventMessage(ctx context.Context, message string) string {
	if id := FromContext(ctx); id != "" {
		return "[" + id + "] " + message
	}
	return message
}
