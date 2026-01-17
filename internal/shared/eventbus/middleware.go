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
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

// PublishFunc is the signature for event publishing.
type PublishFunc func(ctx context.Context, event Event, handlers []HandlerInfo) error

// Middleware wraps a PublishFunc to add behavior.
type Middleware func(next PublishFunc) PublishFunc

// LoggingMiddleware logs event publishing with timing.
func LoggingMiddleware(logger logr.Logger) Middleware {
	return func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) error {
			start := time.Now()

			logger.V(1).Info("Event publish started",
				"event", event.EventName(),
				"aggregateID", event.AggregateID(),
				"aggregateType", event.AggregateType(),
				"handlerCount", len(handlers))

			err := next(ctx, event, handlers)

			duration := time.Since(start)
			if err != nil {
				logger.Error(err, "Event publish completed with errors",
					"event", event.EventName(),
					"aggregateID", event.AggregateID(),
					"duration", duration)
			} else {
				logger.V(1).Info("Event publish completed",
					"event", event.EventName(),
					"aggregateID", event.AggregateID(),
					"duration", duration)
			}

			return err
		}
	}
}

// Metrics holds Prometheus metrics for the event bus.
type Metrics struct {
	eventsPublished  *prometheus.CounterVec
	eventDuration    *prometheus.HistogramVec
	handlerDuration  *prometheus.HistogramVec
	handlerErrors    *prometheus.CounterVec
	handlersExecuted *prometheus.CounterVec
}

// NewMetrics creates Prometheus metrics for the event bus.
func NewMetrics(namespace string) *Metrics {
	return &Metrics{
		eventsPublished: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "eventbus",
				Name:      "events_published_total",
				Help:      "Total number of events published",
			},
			[]string{"event", "aggregate_type", "status"},
		),
		eventDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "eventbus",
				Name:      "event_duration_seconds",
				Help:      "Duration of event processing including all handlers",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"event", "aggregate_type"},
		),
		handlerDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "eventbus",
				Name:      "handler_duration_seconds",
				Help:      "Duration of individual handler execution",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"event", "handler"},
		),
		handlerErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "eventbus",
				Name:      "handler_errors_total",
				Help:      "Total number of handler errors",
			},
			[]string{"event", "handler"},
		),
		handlersExecuted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "eventbus",
				Name:      "handlers_executed_total",
				Help:      "Total number of handlers executed",
			},
			[]string{"event", "handler", "status"},
		),
	}
}

// Register registers all metrics with a Prometheus registry.
func (m *Metrics) Register(registry prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		m.eventsPublished,
		m.eventDuration,
		m.handlerDuration,
		m.handlerErrors,
		m.handlersExecuted,
	}

	for _, c := range collectors {
		if err := registry.Register(c); err != nil {
			return err
		}
	}

	return nil
}

// MetricsMiddleware adds Prometheus metrics collection.
func MetricsMiddleware(metrics *Metrics) Middleware {
	return func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) error {
			start := time.Now()

			err := next(ctx, event, handlers)

			duration := time.Since(start).Seconds()

			status := "success"
			if err != nil {
				status = "error"
			}

			metrics.eventsPublished.WithLabelValues(
				event.EventName(),
				event.AggregateType(),
				status,
			).Inc()

			metrics.eventDuration.WithLabelValues(
				event.EventName(),
				event.AggregateType(),
			).Observe(duration)

			return err
		}
	}
}

// RecoveryMiddleware recovers from panics in handlers.
func RecoveryMiddleware(logger logr.Logger) Middleware {
	return func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) (err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error(nil, "Panic recovered in event handler",
						"event", event.EventName(),
						"panic", r)
					// Convert panic to error
					if e, ok := r.(error); ok {
						err = e
					}
				}
			}()

			return next(ctx, event, handlers)
		}
	}
}

// TracingMiddleware adds trace context to events.
// This is a placeholder for OpenTelemetry integration.
type TraceContext struct {
	TraceID string
	SpanID  string
}

// ContextKey for storing trace context.
type contextKey string

const traceContextKey contextKey = "eventbus.trace"

// WithTraceContext adds trace context to a context.
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey, tc)
}

// GetTraceContext retrieves trace context from a context.
func GetTraceContext(ctx context.Context) (TraceContext, bool) {
	tc, ok := ctx.Value(traceContextKey).(TraceContext)
	return tc, ok
}

// TracingMiddleware logs trace context with events.
func TracingMiddleware(logger logr.Logger) Middleware {
	return func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) error {
			if tc, ok := GetTraceContext(ctx); ok {
				logger.V(2).Info("Event trace context",
					"event", event.EventName(),
					"traceID", tc.TraceID,
					"spanID", tc.SpanID)
			}

			return next(ctx, event, handlers)
		}
	}
}
