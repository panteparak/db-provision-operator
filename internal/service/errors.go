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

package service

import (
	"errors"
	"fmt"
)

// Common service errors
var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists indicates a resource already exists
	ErrAlreadyExists = errors.New("already exists")

	// ErrConnectionFailed indicates a connection to the database failed
	ErrConnectionFailed = errors.New("connection failed")

	// ErrNotReady indicates a resource is not ready for the requested operation
	ErrNotReady = errors.New("not ready")

	// ErrUnsupportedEngine indicates an unsupported database engine
	ErrUnsupportedEngine = errors.New("unsupported engine")

	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")
)

// ValidationError represents a validation error for a specific field.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// DatabaseError wraps errors from database operations.
type DatabaseError struct {
	Operation string
	Resource  string
	Err       error
}

func (e *DatabaseError) Error() string {
	return fmt.Sprintf("database error during %s on %s: %v", e.Operation, e.Resource, e.Err)
}

func (e *DatabaseError) Unwrap() error {
	return e.Err
}

// NewDatabaseError creates a new DatabaseError.
func NewDatabaseError(operation, resource string, err error) *DatabaseError {
	return &DatabaseError{
		Operation: operation,
		Resource:  resource,
		Err:       err,
	}
}

// ConnectionError wraps connection-related errors.
type ConnectionError struct {
	Host string
	Port int32
	Err  error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection to %s:%d failed: %v", e.Host, e.Port, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// NewConnectionError creates a new ConnectionError.
func NewConnectionError(host string, port int32, err error) *ConnectionError {
	return &ConnectionError{
		Host: host,
		Port: port,
		Err:  err,
	}
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists checks if an error is an already exists error.
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsConnectionFailed checks if an error is a connection failed error.
func IsConnectionFailed(err error) bool {
	var connErr *ConnectionError
	return errors.Is(err, ErrConnectionFailed) || errors.As(err, &connErr)
}

// TimeoutError wraps timeout-related errors with operation context.
type TimeoutError struct {
	Operation string
	Resource  string
	Timeout   string
	Err       error
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("operation %s on %s timed out after %s: %v", e.Operation, e.Resource, e.Timeout, e.Err)
}

func (e *TimeoutError) Unwrap() error {
	return e.Err
}

// NewTimeoutError creates a new TimeoutError.
func NewTimeoutError(operation, resource, timeout string, err error) *TimeoutError {
	return &TimeoutError{
		Operation: operation,
		Resource:  resource,
		Timeout:   timeout,
		Err:       err,
	}
}

// IsTimeout checks if an error is a timeout error.
func IsTimeout(err error) bool {
	var timeoutErr *TimeoutError
	return errors.Is(err, ErrTimeout) || errors.As(err, &timeoutErr)
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	var valErr *ValidationError
	return errors.As(err, &valErr)
}

// IsDatabaseError checks if an error is a database error.
func IsDatabaseError(err error) bool {
	var dbErr *DatabaseError
	return errors.As(err, &dbErr)
}
