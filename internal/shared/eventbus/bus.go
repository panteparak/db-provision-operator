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
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// Event is the base interface for all domain events.
// Events are immutable value objects that describe something that happened.
type Event interface {
	// EventName returns the unique name identifying this event type.
	EventName() string

	// EventTime returns when the event occurred.
	EventTime() time.Time

	// AggregateID returns the identifier of the aggregate that produced this event.
	AggregateID() string

	// AggregateType returns the type of aggregate (e.g., "Database", "User").
	AggregateType() string
}

// Handler processes events of a specific type.
// Handlers should be idempotent as events may be delivered more than once.
type Handler func(ctx context.Context, event Event) error

// HandlerInfo contains metadata about a registered handler.
type HandlerInfo struct {
	Name    string
	Handler Handler
}

// Bus manages event publishing and subscriptions.
// It provides a decoupled communication mechanism between feature modules.
type Bus interface {
	// Publish sends an event to all registered handlers synchronously.
	// Returns an error if any handler fails (but continues executing all handlers).
	Publish(ctx context.Context, event Event) error

	// PublishAsync sends an event without waiting for handlers to complete.
	// Errors are logged but not returned.
	PublishAsync(ctx context.Context, event Event)

	// Subscribe registers a named handler for a specific event type.
	// The handlerName should be unique and descriptive for debugging.
	Subscribe(eventName string, handlerName string, handler Handler)

	// Unsubscribe removes a handler by name from a specific event type.
	Unsubscribe(eventName string, handlerName string)

	// Handlers returns all registered handlers for an event type.
	Handlers(eventName string) []HandlerInfo
}

// InMemoryBus is a synchronous in-process event bus implementation.
// It's suitable for modular monoliths where all modules run in the same process.
type InMemoryBus struct {
	mu         sync.RWMutex
	handlers   map[string][]HandlerInfo
	logger     logr.Logger
	middleware []Middleware
}

// BusOption configures the InMemoryBus.
type BusOption func(*InMemoryBus)

// WithLogger sets the logger for the bus.
func WithLogger(logger logr.Logger) BusOption {
	return func(b *InMemoryBus) {
		b.logger = logger
	}
}

// WithMiddleware adds middleware to the bus.
func WithMiddleware(middleware ...Middleware) BusOption {
	return func(b *InMemoryBus) {
		b.middleware = append(b.middleware, middleware...)
	}
}

// NewInMemoryBus creates a new in-memory event bus.
func NewInMemoryBus(opts ...BusOption) *InMemoryBus {
	bus := &InMemoryBus{
		handlers: make(map[string][]HandlerInfo),
		logger:   logr.Discard(),
	}

	for _, opt := range opts {
		opt(bus)
	}

	return bus
}

// Publish sends an event to all registered handlers.
// All handlers are executed even if some fail. Errors are collected and returned.
func (b *InMemoryBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := make([]HandlerInfo, len(b.handlers[event.EventName()]))
	copy(handlers, b.handlers[event.EventName()])
	b.mu.RUnlock()

	if len(handlers) == 0 {
		b.logger.V(2).Info("No handlers registered for event",
			"event", event.EventName(),
			"aggregate", event.AggregateID())
		return nil
	}

	b.logger.V(1).Info("Publishing event",
		"event", event.EventName(),
		"aggregate", event.AggregateID(),
		"aggregateType", event.AggregateType(),
		"handlerCount", len(handlers))

	// Apply middleware chain
	publish := b.buildMiddlewareChain(b.executeHandlers)

	return publish(ctx, event, handlers)
}

// executeHandlers runs all handlers for an event.
func (b *InMemoryBus) executeHandlers(ctx context.Context, event Event, handlers []HandlerInfo) error {
	var errs []error

	for _, hi := range handlers {
		start := time.Now()

		if err := hi.Handler(ctx, event); err != nil {
			b.logger.Error(err, "Handler failed",
				"event", event.EventName(),
				"handler", hi.Name,
				"duration", time.Since(start))
			errs = append(errs, fmt.Errorf("handler %s: %w", hi.Name, err))
		} else {
			b.logger.V(2).Info("Handler completed",
				"event", event.EventName(),
				"handler", hi.Name,
				"duration", time.Since(start))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("event %s: %d handler(s) failed: %v", event.EventName(), len(errs), errs)
	}

	return nil
}

// buildMiddlewareChain creates the middleware execution chain.
func (b *InMemoryBus) buildMiddlewareChain(final PublishFunc) PublishFunc {
	// Build chain from right to left
	chain := final
	for i := len(b.middleware) - 1; i >= 0; i-- {
		chain = b.middleware[i](chain)
	}
	return chain
}

// PublishAsync sends an event asynchronously.
// The event is processed in a goroutine and errors are only logged.
func (b *InMemoryBus) PublishAsync(ctx context.Context, event Event) {
	go func() {
		// Create a new context that won't be cancelled when the parent is
		asyncCtx := context.Background()

		if err := b.Publish(asyncCtx, event); err != nil {
			b.logger.Error(err, "Async event publish failed",
				"event", event.EventName(),
				"aggregate", event.AggregateID())
		}
	}()
}

// Subscribe registers a handler for an event type.
// The handlerName must be unique within the event type.
func (b *InMemoryBus) Subscribe(eventName string, handlerName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check for duplicate handler names
	for _, hi := range b.handlers[eventName] {
		if hi.Name == handlerName {
			b.logger.Info("Handler already registered, skipping",
				"event", eventName,
				"handler", handlerName)
			return
		}
	}

	b.handlers[eventName] = append(b.handlers[eventName], HandlerInfo{
		Name:    handlerName,
		Handler: handler,
	})

	b.logger.V(1).Info("Handler subscribed",
		"event", eventName,
		"handler", handlerName,
		"totalHandlers", len(b.handlers[eventName]))
}

// Unsubscribe removes a handler by name.
func (b *InMemoryBus) Unsubscribe(eventName string, handlerName string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	handlers := b.handlers[eventName]
	for i, hi := range handlers {
		if hi.Name == handlerName {
			b.handlers[eventName] = append(handlers[:i], handlers[i+1:]...)
			b.logger.V(1).Info("Handler unsubscribed",
				"event", eventName,
				"handler", handlerName)
			return
		}
	}
}

// Handlers returns all registered handlers for an event type.
func (b *InMemoryBus) Handlers(eventName string) []HandlerInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	handlers := make([]HandlerInfo, len(b.handlers[eventName]))
	copy(handlers, b.handlers[eventName])
	return handlers
}

// Ensure InMemoryBus implements Bus interface.
var _ Bus = (*InMemoryBus)(nil)
