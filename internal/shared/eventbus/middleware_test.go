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

package eventbus

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggingMiddleware(t *testing.T) {
	logger := logr.Discard()
	middleware := LoggingMiddleware(logger)

	nextCalled := false
	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		nextCalled = true
		return nil
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	err := wrapped(context.Background(), event, nil)

	require.NoError(t, err)
	assert.True(t, nextCalled)
}

func TestLoggingMiddleware_WithError(t *testing.T) {
	logger := logr.Discard()
	middleware := LoggingMiddleware(logger)

	expectedErr := errors.New("test error")
	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		return expectedErr
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	err := wrapped(context.Background(), event, nil)

	assert.Equal(t, expectedErr, err)
}

func TestMetricsMiddleware(t *testing.T) {
	metrics := NewMetrics("test")
	middleware := MetricsMiddleware(metrics)

	nextCalled := false
	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		nextCalled = true
		return nil
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	err := wrapped(context.Background(), event, nil)

	require.NoError(t, err)
	assert.True(t, nextCalled)
}

func TestMetrics_Register(t *testing.T) {
	metrics := NewMetrics("test")
	registry := prometheus.NewRegistry()

	err := metrics.Register(registry)
	require.NoError(t, err)

	// Registering again should fail (metrics already registered)
	err = metrics.Register(registry)
	assert.Error(t, err, "Re-registering should fail")
}

func TestRecoveryMiddleware(t *testing.T) {
	logger := logr.Discard()
	middleware := RecoveryMiddleware(logger)

	// Test recovery from panic
	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		panic("test panic")
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	// Should not panic
	err := wrapped(context.Background(), event, nil)
	assert.NoError(t, err) // Panic was recovered
}

func TestRecoveryMiddleware_WithErrorPanic(t *testing.T) {
	logger := logr.Discard()
	middleware := RecoveryMiddleware(logger)

	expectedErr := errors.New("panic error")
	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		panic(expectedErr)
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	err := wrapped(context.Background(), event, nil)
	assert.Equal(t, expectedErr, err)
}

func TestRecoveryMiddleware_NoPanic(t *testing.T) {
	logger := logr.Discard()
	middleware := RecoveryMiddleware(logger)

	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		return nil
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	err := wrapped(context.Background(), event, nil)
	assert.NoError(t, err)
}

func TestTracingMiddleware(t *testing.T) {
	logger := logr.Discard()
	middleware := TracingMiddleware(logger)

	next := func(ctx context.Context, event Event, handlers []HandlerInfo) error {
		return nil
	}

	wrapped := middleware(next)
	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")

	// Without trace context
	err := wrapped(context.Background(), event, nil)
	assert.NoError(t, err)

	// With trace context
	tc := TraceContext{TraceID: "trace-123", SpanID: "span-456"}
	ctx := WithTraceContext(context.Background(), tc)
	err = wrapped(ctx, event, nil)
	assert.NoError(t, err)
}

func TestTraceContext(t *testing.T) {
	ctx := context.Background()

	// No trace context
	tc, ok := GetTraceContext(ctx)
	assert.False(t, ok)
	assert.Equal(t, TraceContext{}, tc)

	// With trace context
	expected := TraceContext{TraceID: "trace-123", SpanID: "span-456"}
	ctx = WithTraceContext(ctx, expected)

	tc, ok = GetTraceContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, expected, tc)
}
