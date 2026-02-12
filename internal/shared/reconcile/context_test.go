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

package reconcile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
	// Generate multiple IDs and verify they're unique
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewID()
		assert.Len(t, id, 8, "ID should be 8 characters")
		assert.False(t, ids[id], "IDs should be unique")
		ids[id] = true
	}
}

func TestWithReconcileID(t *testing.T) {
	ctx := context.Background()
	newCtx, log, reconcileID := WithReconcileID(ctx)

	// Verify reconcileID is set
	require.NotEmpty(t, reconcileID)
	assert.Len(t, reconcileID, 8)

	// Verify it can be retrieved from context
	retrieved := FromContext(newCtx)
	assert.Equal(t, reconcileID, retrieved)

	// Verify logger is not nil
	assert.NotNil(t, log)
}

func TestWithExistingReconcileID(t *testing.T) {
	ctx := context.Background()
	existingID := "abc12345"

	newCtx := WithExistingReconcileID(ctx, existingID)
	retrieved := FromContext(newCtx)

	assert.Equal(t, existingID, retrieved)
}

func TestFromContext_NoID(t *testing.T) {
	ctx := context.Background()
	id := FromContext(ctx)
	assert.Empty(t, id)
}

func TestEventMessage(t *testing.T) {
	tests := []struct {
		name        string
		reconcileID string
		message     string
		expected    string
	}{
		{
			name:        "with reconcileID",
			reconcileID: "abc12345",
			message:     "User created successfully",
			expected:    "[abc12345] User created successfully",
		},
		{
			name:        "without reconcileID",
			reconcileID: "",
			message:     "User created successfully",
			expected:    "User created successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.reconcileID != "" {
				ctx = WithExistingReconcileID(ctx, tt.reconcileID)
			}

			result := EventMessage(ctx, tt.message)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoggerFromContext(t *testing.T) {
	ctx := context.Background()

	// Without reconcileID
	log1 := LoggerFromContext(ctx)
	assert.NotNil(t, log1)

	// With reconcileID
	ctx = WithExistingReconcileID(ctx, "test1234")
	log2 := LoggerFromContext(ctx)
	assert.NotNil(t, log2)
}
