//go:build !integration && !e2e && !envtest

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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/db-provision-operator/internal/service"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorClass
	}{
		{
			name:     "nil error is transient",
			err:      nil,
			expected: ErrorClassTransient,
		},
		{
			name:     "validation error is permanent",
			err:      &service.ValidationError{Field: "name", Message: "required"},
			expected: ErrorClassPermanent,
		},
		{
			name:     "wrapped validation error is permanent",
			err:      fmt.Errorf("failed: %w", &service.ValidationError{Field: "name", Message: "bad"}),
			expected: ErrorClassPermanent,
		},
		{
			name:     "unsupported engine is permanent",
			err:      service.ErrUnsupportedEngine,
			expected: ErrorClassPermanent,
		},
		{
			name:     "wrapped unsupported engine is permanent",
			err:      fmt.Errorf("create: %w", service.ErrUnsupportedEngine),
			expected: ErrorClassPermanent,
		},
		{
			name:     "already exists is permanent",
			err:      service.ErrAlreadyExists,
			expected: ErrorClassPermanent,
		},
		{
			name:     "connection error is connection class",
			err:      service.NewConnectionError("localhost", 5432, fmt.Errorf("refused")),
			expected: ErrorClassConnection,
		},
		{
			name:     "wrapped connection error is connection class",
			err:      fmt.Errorf("connect: %w", service.NewConnectionError("host", 5432, fmt.Errorf("timeout"))),
			expected: ErrorClassConnection,
		},
		{
			name:     "connection failed sentinel is connection class",
			err:      service.ErrConnectionFailed,
			expected: ErrorClassConnection,
		},
		{
			name:     "timeout error is connection class",
			err:      service.NewTimeoutError("create", "mydb", "30s", fmt.Errorf("deadline")),
			expected: ErrorClassConnection,
		},
		{
			name:     "timeout sentinel is connection class",
			err:      service.ErrTimeout,
			expected: ErrorClassConnection,
		},
		{
			name:     "retryable network error is connection class",
			err:      fmt.Errorf("connection refused"),
			expected: ErrorClassConnection,
		},
		{
			name:     "retryable i/o timeout is connection class",
			err:      fmt.Errorf("i/o timeout"),
			expected: ErrorClassConnection,
		},
		{
			name:     "generic error is transient",
			err:      fmt.Errorf("something went wrong"),
			expected: ErrorClassTransient,
		},
		{
			name:     "not found is transient",
			err:      service.ErrNotFound,
			expected: ErrorClassTransient,
		},
		{
			name:     "not ready is transient",
			err:      service.ErrNotReady,
			expected: ErrorClassTransient,
		},
		{
			name:     "database error is transient",
			err:      service.NewDatabaseError("create", "mydb", fmt.Errorf("internal")),
			expected: ErrorClassTransient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestClassifyRequeue(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedResult ctrl.Result
		expectedErr    bool
	}{
		{
			name:           "permanent error: no requeue, nil error",
			err:            &service.ValidationError{Field: "name", Message: "required"},
			expectedResult: ctrl.Result{},
			expectedErr:    false,
		},
		{
			name:           "connection error: longer requeue",
			err:            service.ErrConnectionFailed,
			expectedResult: ctrl.Result{RequeueAfter: RequeueConnection},
			expectedErr:    true,
		},
		{
			name:           "transient error: standard requeue",
			err:            fmt.Errorf("something went wrong"),
			expectedResult: ctrl.Result{RequeueAfter: RequeueDefault},
			expectedErr:    true,
		},
		{
			name:           "unsupported engine: no requeue",
			err:            fmt.Errorf("create: %w", service.ErrUnsupportedEngine),
			expectedResult: ctrl.Result{},
			expectedErr:    false,
		},
		{
			name:           "timeout error: connection-level requeue",
			err:            service.NewTimeoutError("backup", "mydb", "30s", fmt.Errorf("ctx deadline")),
			expectedResult: ctrl.Result{RequeueAfter: RequeueConnection},
			expectedErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ClassifyRequeue(tt.err)
			assert.Equal(t, tt.expectedResult, result)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
