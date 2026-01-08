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

package testutil

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/db-provision-operator/internal/storage"
)

// MethodCall represents a recorded method call for verification
type MethodCall struct {
	Method    string
	Args      []interface{}
	Timestamp time.Time
}

// MockBackend implements the storage.Backend interface for testing
type MockBackend struct {
	mu sync.RWMutex

	// data stores the mock storage data (path -> content)
	data map[string][]byte

	// metadata stores object metadata (path -> ObjectInfo)
	metadata map[string]storage.ObjectInfo

	// calls records all method calls for verification
	calls []MethodCall

	// errors configures errors to return for specific methods
	errors map[string]error

	// errorOnPath configures errors for specific paths
	errorOnPath map[string]error

	// closed tracks if Close() has been called
	closed bool

	// latency adds artificial latency to operations
	latency time.Duration
}

// NewMockBackend creates a new MockBackend for testing
func NewMockBackend() *MockBackend {
	return &MockBackend{
		data:        make(map[string][]byte),
		metadata:    make(map[string]storage.ObjectInfo),
		calls:       make([]MethodCall, 0),
		errors:      make(map[string]error),
		errorOnPath: make(map[string]error),
	}
}

// NewMockBackendWithData creates a MockBackend pre-populated with test data
func NewMockBackendWithData(data map[string][]byte) *MockBackend {
	mock := NewMockBackend()
	for path, content := range data {
		mock.data[path] = content
		mock.metadata[path] = storage.ObjectInfo{
			Path:         path,
			Size:         int64(len(content)),
			LastModified: time.Now().Unix(),
			Checksum:     fmt.Sprintf("%x", md5.Sum(content)),
		}
	}
	return mock
}

// SetError configures an error to be returned for a specific method
func (m *MockBackend) SetError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors[method] = err
}

// SetErrorOnPath configures an error to be returned for operations on a specific path
func (m *MockBackend) SetErrorOnPath(path string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errorOnPath[path] = err
}

// ClearErrors removes all configured errors
func (m *MockBackend) ClearErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errors = make(map[string]error)
	m.errorOnPath = make(map[string]error)
}

// SetLatency configures artificial latency for all operations
func (m *MockBackend) SetLatency(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = d
}

// GetCalls returns all recorded method calls
func (m *MockBackend) GetCalls() []MethodCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MethodCall, len(m.calls))
	copy(result, m.calls)
	return result
}

// GetCallsForMethod returns calls for a specific method
func (m *MockBackend) GetCallsForMethod(method string) []MethodCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []MethodCall
	for _, call := range m.calls {
		if call.Method == method {
			result = append(result, call)
		}
	}
	return result
}

// GetCallCount returns the number of times a method was called
func (m *MockBackend) GetCallCount(method string) int {
	return len(m.GetCallsForMethod(method))
}

// ClearCalls clears all recorded method calls
func (m *MockBackend) ClearCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]MethodCall, 0)
}

// Reset clears all data, calls, and errors
func (m *MockBackend) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string][]byte)
	m.metadata = make(map[string]storage.ObjectInfo)
	m.calls = make([]MethodCall, 0)
	m.errors = make(map[string]error)
	m.errorOnPath = make(map[string]error)
	m.closed = false
	m.latency = 0
}

// GetData returns a copy of the stored data
func (m *MockBackend) GetData() map[string][]byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string][]byte)
	for k, v := range m.data {
		copied := make([]byte, len(v))
		copy(copied, v)
		result[k] = copied
	}
	return result
}

// GetDataForPath returns the data stored at a specific path
func (m *MockBackend) GetDataForPath(path string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, exists := m.data[path]
	if !exists {
		return nil, false
	}
	copied := make([]byte, len(data))
	copy(copied, data)
	return copied, true
}

// IsClosed returns whether Close() has been called
func (m *MockBackend) IsClosed() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closed
}

// recordCall records a method call for later verification
func (m *MockBackend) recordCall(method string, args ...interface{}) {
	m.calls = append(m.calls, MethodCall{
		Method:    method,
		Args:      args,
		Timestamp: time.Now(),
	})
}

// checkError checks for configured errors
func (m *MockBackend) checkError(method, path string) error {
	if err, ok := m.errors[method]; ok {
		return err
	}
	if path != "" {
		if err, ok := m.errorOnPath[path]; ok {
			return err
		}
	}
	return nil
}

// applyLatency applies configured latency
func (m *MockBackend) applyLatency(ctx context.Context) error {
	if m.latency > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(m.latency):
		}
	}
	return nil
}

// Write implements storage.Backend.Write
func (m *MockBackend) Write(ctx context.Context, path string, reader io.Reader) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Write", path)

	if err := m.checkError("Write", path); err != nil {
		return err
	}

	if err := m.applyLatency(ctx); err != nil {
		return err
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read data: %w", err)
	}

	m.data[path] = data
	m.metadata[path] = storage.ObjectInfo{
		Path:         path,
		Size:         int64(len(data)),
		LastModified: time.Now().Unix(),
		Checksum:     fmt.Sprintf("%x", md5.Sum(data)),
	}

	return nil
}

// mockReadCloser wraps a bytes.Reader to implement io.ReadCloser
type mockReadCloser struct {
	*bytes.Reader
}

func (m *mockReadCloser) Close() error {
	return nil
}

// Read implements storage.Backend.Read
func (m *MockBackend) Read(ctx context.Context, path string) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("Read", path)

	if err := m.checkError("Read", path); err != nil {
		return nil, err
	}

	if err := m.applyLatency(ctx); err != nil {
		return nil, err
	}

	data, exists := m.data[path]
	if !exists {
		return nil, fmt.Errorf("object not found: %s", path)
	}

	// Return a copy to prevent modification
	copied := make([]byte, len(data))
	copy(copied, data)

	return &mockReadCloser{bytes.NewReader(copied)}, nil
}

// Delete implements storage.Backend.Delete
func (m *MockBackend) Delete(ctx context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Delete", path)

	if err := m.checkError("Delete", path); err != nil {
		return err
	}

	if err := m.applyLatency(ctx); err != nil {
		return err
	}

	delete(m.data, path)
	delete(m.metadata, path)

	return nil
}

// Exists implements storage.Backend.Exists
func (m *MockBackend) Exists(ctx context.Context, path string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("Exists", path)

	if err := m.checkError("Exists", path); err != nil {
		return false, err
	}

	if err := m.applyLatency(ctx); err != nil {
		return false, err
	}

	_, exists := m.data[path]
	return exists, nil
}

// List implements storage.Backend.List
func (m *MockBackend) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("List", prefix)

	if err := m.checkError("List", prefix); err != nil {
		return nil, err
	}

	if err := m.applyLatency(ctx); err != nil {
		return nil, err
	}

	var result []storage.ObjectInfo
	for path, info := range m.metadata {
		if strings.HasPrefix(path, prefix) {
			result = append(result, info)
		}
	}

	// Sort by path for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})

	return result, nil
}

// GetSize implements storage.Backend.GetSize
func (m *MockBackend) GetSize(ctx context.Context, path string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	m.recordCall("GetSize", path)

	if err := m.checkError("GetSize", path); err != nil {
		return 0, err
	}

	if err := m.applyLatency(ctx); err != nil {
		return 0, err
	}

	info, exists := m.metadata[path]
	if !exists {
		return 0, fmt.Errorf("object not found: %s", path)
	}

	return info.Size, nil
}

// Close implements storage.Backend.Close
func (m *MockBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.recordCall("Close")

	if err := m.checkError("Close", ""); err != nil {
		return err
	}

	m.closed = true
	return nil
}

// AssertCalled verifies that a method was called
func (m *MockBackend) AssertCalled(method string) bool {
	return m.GetCallCount(method) > 0
}

// AssertCalledWith verifies that a method was called with specific arguments
func (m *MockBackend) AssertCalledWith(method string, args ...interface{}) bool {
	calls := m.GetCallsForMethod(method)
	for _, call := range calls {
		if len(call.Args) != len(args) {
			continue
		}
		match := true
		for i, arg := range args {
			if arg != call.Args[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// AssertNotCalled verifies that a method was not called
func (m *MockBackend) AssertNotCalled(method string) bool {
	return m.GetCallCount(method) == 0
}

// AssertCallCount verifies the number of times a method was called
func (m *MockBackend) AssertCallCount(method string, count int) bool {
	return m.GetCallCount(method) == count
}

// Verify that MockBackend implements storage.Backend
var _ storage.Backend = (*MockBackend)(nil)
