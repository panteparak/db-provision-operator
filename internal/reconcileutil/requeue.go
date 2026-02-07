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

package reconcileutil

import (
	"errors"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/service"
	"github.com/db-provision-operator/internal/util"
)

const (
	// RequeueDefault is the standard requeue interval for transient errors.
	RequeueDefault = 30 * time.Second

	// RequeueConnection is the requeue interval for connection-related errors.
	RequeueConnection = 1 * time.Minute
)

// ErrorClass represents the classification of an error for requeue decisions.
type ErrorClass int

const (
	// ErrorClassTransient indicates a transient error that should be retried.
	ErrorClassTransient ErrorClass = iota

	// ErrorClassConnection indicates a connection error (longer backoff).
	ErrorClassConnection

	// ErrorClassPermanent indicates a permanent error that should not be retried.
	ErrorClassPermanent
)

// ClassifyError determines the error class for requeue decisions.
// Permanent errors (validation, unsupported engine) should not cause infinite retries.
// Connection errors get longer backoff intervals.
// All other errors are treated as transient with standard backoff.
func ClassifyError(err error) ErrorClass {
	if err == nil {
		return ErrorClassTransient
	}

	// Check for permanent errors — these will never succeed on retry
	if service.IsValidationError(err) {
		return ErrorClassPermanent
	}
	if errors.Is(err, service.ErrUnsupportedEngine) {
		return ErrorClassPermanent
	}
	if errors.Is(err, service.ErrAlreadyExists) {
		return ErrorClassPermanent
	}

	// Check for connection errors — need longer backoff
	if service.IsConnectionFailed(err) {
		return ErrorClassConnection
	}
	if service.IsTimeout(err) {
		return ErrorClassConnection
	}
	if util.IsRetryableError(err) {
		// Retryable errors from retry.go are transient network issues
		return ErrorClassConnection
	}

	return ErrorClassTransient
}

// ClassifyRequeue returns the appropriate ctrl.Result and error based on error classification.
// - Permanent errors: no requeue (return nil error so controller-runtime doesn't retry)
// - Connection errors: longer requeue interval
// - Transient errors: standard requeue interval
func ClassifyRequeue(err error) (ctrl.Result, error) {
	switch ClassifyError(err) {
	case ErrorClassPermanent:
		// Don't requeue permanent errors — they won't resolve on retry.
		// Return nil error so controller-runtime doesn't requeue.
		return ctrl.Result{}, nil
	case ErrorClassConnection:
		return ctrl.Result{RequeueAfter: RequeueConnection}, err
	default:
		return ctrl.Result{RequeueAfter: RequeueDefault}, err
	}
}
