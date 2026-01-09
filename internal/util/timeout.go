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

package util

import (
	"context"
	"time"
)

// TimeoutConfig defines timeout settings for database operations.
// Zero values mean no timeout (use parent context's deadline).
type TimeoutConfig struct {
	// ConnectTimeout is the timeout for establishing connections
	ConnectTimeout time.Duration

	// OperationTimeout is the default timeout for standard operations
	// (create, update, delete, exists checks)
	OperationTimeout time.Duration

	// QueryTimeout is the timeout for read-only queries (get, list)
	QueryTimeout time.Duration

	// LongOperationTimeout is the timeout for operations that may take longer
	// (backup, restore, schema migrations)
	LongOperationTimeout time.Duration
}

// DefaultTimeoutConfig returns sensible default timeouts for database operations.
// These are conservative defaults that work well for most scenarios:
//   - Connect: 30s - enough for slow networks but catches stuck connections
//   - Operation: 60s - sufficient for most DDL/DML operations
//   - Query: 30s - read operations should be fast
//   - LongOperation: 10m - backups/restores need more time
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		ConnectTimeout:       30 * time.Second,
		OperationTimeout:     60 * time.Second,
		QueryTimeout:         30 * time.Second,
		LongOperationTimeout: 10 * time.Minute,
	}
}

// FastTimeoutConfig returns shorter timeouts suitable for testing
// or environments where operations should be quick.
func FastTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		ConnectTimeout:       10 * time.Second,
		OperationTimeout:     30 * time.Second,
		QueryTimeout:         15 * time.Second,
		LongOperationTimeout: 2 * time.Minute,
	}
}

// NoTimeoutConfig returns a config with no timeouts.
// Use this when the caller manages their own context deadlines.
func NoTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{}
}

// WithTimeout wraps a context with a timeout if the duration is positive.
// If duration is zero or negative, returns the original context and a no-op cancel function.
// Always call the returned cancel function to release resources.
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// WithConnectTimeout wraps a context with the connect timeout from config.
func (c TimeoutConfig) WithConnectTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(ctx, c.ConnectTimeout)
}

// WithOperationTimeout wraps a context with the operation timeout from config.
func (c TimeoutConfig) WithOperationTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(ctx, c.OperationTimeout)
}

// WithQueryTimeout wraps a context with the query timeout from config.
func (c TimeoutConfig) WithQueryTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(ctx, c.QueryTimeout)
}

// WithLongOperationTimeout wraps a context with the long operation timeout from config.
func (c TimeoutConfig) WithLongOperationTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return WithTimeout(ctx, c.LongOperationTimeout)
}

// IsTimeoutError checks if an error is a context deadline exceeded error.
func IsTimeoutError(err error) bool {
	return err == context.DeadlineExceeded
}

// IsCanceled checks if an error is a context canceled error.
func IsCanceled(err error) bool {
	return err == context.Canceled
}

// IsContextError checks if an error is a context-related error (timeout or canceled).
func IsContextError(err error) bool {
	return IsTimeoutError(err) || IsCanceled(err)
}
