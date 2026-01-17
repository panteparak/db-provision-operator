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
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInMemoryBus(t *testing.T) {
	bus := NewInMemoryBus()
	assert.NotNil(t, bus)
	assert.NotNil(t, bus.handlers)
}

func TestNewInMemoryBus_WithOptions(t *testing.T) {
	logger := logr.Discard()
	middleware := LoggingMiddleware(logger)

	bus := NewInMemoryBus(
		WithLogger(logger),
		WithMiddleware(middleware),
	)

	assert.NotNil(t, bus)
	assert.Len(t, bus.middleware, 1)
}

func TestInMemoryBus_Subscribe(t *testing.T) {
	bus := NewInMemoryBus()

	handler := func(ctx context.Context, event Event) error {
		return nil
	}

	bus.Subscribe("TestEvent", "testHandler", handler)

	handlers := bus.Handlers("TestEvent")
	assert.Len(t, handlers, 1)
	assert.Equal(t, "testHandler", handlers[0].Name)
}

func TestInMemoryBus_Subscribe_DuplicateName(t *testing.T) {
	bus := NewInMemoryBus()

	handler1 := func(ctx context.Context, event Event) error { return nil }
	handler2 := func(ctx context.Context, event Event) error { return nil }

	bus.Subscribe("TestEvent", "sameHandler", handler1)
	bus.Subscribe("TestEvent", "sameHandler", handler2) // Should be ignored

	handlers := bus.Handlers("TestEvent")
	assert.Len(t, handlers, 1, "Duplicate handler should be ignored")
}

func TestInMemoryBus_Unsubscribe(t *testing.T) {
	bus := NewInMemoryBus()

	handler := func(ctx context.Context, event Event) error { return nil }

	bus.Subscribe("TestEvent", "testHandler", handler)
	assert.Len(t, bus.Handlers("TestEvent"), 1)

	bus.Unsubscribe("TestEvent", "testHandler")
	assert.Len(t, bus.Handlers("TestEvent"), 0)
}

func TestInMemoryBus_Unsubscribe_NonExistent(t *testing.T) {
	bus := NewInMemoryBus()

	// Should not panic
	bus.Unsubscribe("NonExistentEvent", "nonExistentHandler")
}

func TestInMemoryBus_Publish(t *testing.T) {
	bus := NewInMemoryBus()

	var receivedEvent Event
	handler := func(ctx context.Context, event Event) error {
		receivedEvent = event
		return nil
	}

	bus.Subscribe(EventDatabaseCreated, "testHandler", handler)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	require.NoError(t, err)
	assert.NotNil(t, receivedEvent)
	assert.Equal(t, EventDatabaseCreated, receivedEvent.EventName())
}

func TestInMemoryBus_Publish_MultipleHandlers(t *testing.T) {
	bus := NewInMemoryBus()

	var mu sync.Mutex
	callOrder := []string{}

	handler1 := func(ctx context.Context, event Event) error {
		mu.Lock()
		callOrder = append(callOrder, "handler1")
		mu.Unlock()
		return nil
	}

	handler2 := func(ctx context.Context, event Event) error {
		mu.Lock()
		callOrder = append(callOrder, "handler2")
		mu.Unlock()
		return nil
	}

	bus.Subscribe(EventDatabaseCreated, "handler1", handler1)
	bus.Subscribe(EventDatabaseCreated, "handler2", handler2)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	require.NoError(t, err)
	assert.Len(t, callOrder, 2)
	assert.Contains(t, callOrder, "handler1")
	assert.Contains(t, callOrder, "handler2")
}

func TestInMemoryBus_Publish_NoHandlers(t *testing.T) {
	bus := NewInMemoryBus()

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	// Should not return error when no handlers
	assert.NoError(t, err)
}

func TestInMemoryBus_Publish_HandlerError(t *testing.T) {
	bus := NewInMemoryBus()

	expectedErr := errors.New("handler failed")
	handler := func(ctx context.Context, event Event) error {
		return expectedErr
	}

	bus.Subscribe(EventDatabaseCreated, "failingHandler", handler)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler failed")
}

func TestInMemoryBus_Publish_ContinuesOnError(t *testing.T) {
	bus := NewInMemoryBus()

	handler1Called := false
	handler2Called := false

	handler1 := func(ctx context.Context, event Event) error {
		handler1Called = true
		return errors.New("handler1 failed")
	}

	handler2 := func(ctx context.Context, event Event) error {
		handler2Called = true
		return nil
	}

	bus.Subscribe(EventDatabaseCreated, "handler1", handler1)
	bus.Subscribe(EventDatabaseCreated, "handler2", handler2)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	// Error returned but both handlers were called
	assert.Error(t, err)
	assert.True(t, handler1Called)
	assert.True(t, handler2Called)
}

func TestInMemoryBus_PublishAsync(t *testing.T) {
	bus := NewInMemoryBus()

	var wg sync.WaitGroup
	wg.Add(1)

	handler := func(ctx context.Context, event Event) error {
		defer wg.Done()
		return nil
	}

	bus.Subscribe(EventDatabaseCreated, "asyncHandler", handler)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	bus.PublishAsync(context.Background(), event)

	// Wait for async handler with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Async handler did not complete in time")
	}
}

func TestInMemoryBus_Handlers(t *testing.T) {
	bus := NewInMemoryBus()

	handler1 := func(ctx context.Context, event Event) error { return nil }
	handler2 := func(ctx context.Context, event Event) error { return nil }

	bus.Subscribe("Event1", "handler1", handler1)
	bus.Subscribe("Event1", "handler2", handler2)
	bus.Subscribe("Event2", "handler3", handler1)

	handlers1 := bus.Handlers("Event1")
	assert.Len(t, handlers1, 2)

	handlers2 := bus.Handlers("Event2")
	assert.Len(t, handlers2, 1)

	handlers3 := bus.Handlers("NonExistent")
	assert.Len(t, handlers3, 0)
}

func TestInMemoryBus_ConcurrentAccess(t *testing.T) {
	bus := NewInMemoryBus()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent subscribes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			handler := func(ctx context.Context, event Event) error { return nil }
			bus.Subscribe("TestEvent", "handler"+string(rune(id)), handler)
		}(i)
	}

	// Concurrent publishes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
			_ = bus.Publish(context.Background(), event)
		}()
	}

	wg.Wait()
}

func TestInMemoryBus_WithMiddleware(t *testing.T) {
	var middlewareCalled bool

	middleware := func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) error {
			middlewareCalled = true
			return next(ctx, event, handlers)
		}
	}

	bus := NewInMemoryBus(WithMiddleware(middleware))

	handler := func(ctx context.Context, event Event) error { return nil }
	bus.Subscribe(EventDatabaseCreated, "testHandler", handler)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	require.NoError(t, err)
	assert.True(t, middlewareCalled)
}

func TestInMemoryBus_MiddlewareChainOrder(t *testing.T) {
	var order []string

	middleware1 := func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) error {
			order = append(order, "m1-before")
			err := next(ctx, event, handlers)
			order = append(order, "m1-after")
			return err
		}
	}

	middleware2 := func(next PublishFunc) PublishFunc {
		return func(ctx context.Context, event Event, handlers []HandlerInfo) error {
			order = append(order, "m2-before")
			err := next(ctx, event, handlers)
			order = append(order, "m2-after")
			return err
		}
	}

	bus := NewInMemoryBus(WithMiddleware(middleware1, middleware2))

	handler := func(ctx context.Context, event Event) error {
		order = append(order, "handler")
		return nil
	}
	bus.Subscribe(EventDatabaseCreated, "testHandler", handler)

	event := NewDatabaseCreated("testdb", "instance1", "default", "mysql")
	err := bus.Publish(context.Background(), event)

	require.NoError(t, err)
	assert.Equal(t, []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}, order)
}
