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

package drift

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
	driftsvc "github.com/db-provision-operator/internal/service/drift"
	"github.com/db-provision-operator/internal/shared/instanceresolver"
)

// mockResource implements DriftableResource for testing.
type mockResource struct {
	metav1.ObjectMeta
	driftPolicy *dbopsv1alpha1.DriftPolicy
	driftStatus *dbopsv1alpha1.DriftStatus
}

func (m *mockResource) GetObjectKind() schema.ObjectKind { return schema.EmptyObjectKind }
func (m *mockResource) DeepCopyObject() runtime.Object   { return m }

func (m *mockResource) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	return m.driftPolicy
}

func (m *mockResource) SetDriftStatus(status *dbopsv1alpha1.DriftStatus) {
	m.driftStatus = status
}

// mockInstancePolicy implements InstanceDriftPolicy for testing.
type mockInstancePolicy struct {
	policy *dbopsv1alpha1.DriftPolicy
}

func (m *mockInstancePolicy) GetDriftPolicy() *dbopsv1alpha1.DriftPolicy {
	return m.policy
}

// mockDetector implements Detector for testing.
type mockDetector struct {
	detectDriftFunc  func(ctx context.Context, allowDestructive bool) (*driftsvc.Result, error)
	correctDriftFunc func(ctx context.Context, driftResult *driftsvc.Result, allowDestructive bool) (*driftsvc.CorrectionResult, error)
	detectCallCount  int
	correctCallCount int
}

func (m *mockDetector) DetectDrift(ctx context.Context, allowDestructive bool) (*driftsvc.Result, error) {
	m.detectCallCount++
	if m.detectDriftFunc != nil {
		return m.detectDriftFunc(ctx, allowDestructive)
	}
	return nil, nil
}

func (m *mockDetector) CorrectDrift(ctx context.Context, driftResult *driftsvc.Result, allowDestructive bool) (*driftsvc.CorrectionResult, error) {
	m.correctCallCount++
	if m.correctDriftFunc != nil {
		return m.correctDriftFunc(ctx, driftResult, allowDestructive)
	}
	return nil, nil
}

func newTestOrchestrator() (*Orchestrator, *record.FakeRecorder) {
	recorder := record.NewFakeRecorder(10)
	return &Orchestrator{
		Recorder:             recorder,
		DefaultDriftInterval: 5 * time.Minute,
	}, recorder
}

func newMockResource(name, namespace string, policy *dbopsv1alpha1.DriftPolicy, annotations map[string]string) *mockResource {
	return &mockResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			UID:         types.UID("test-uid"),
			Annotations: annotations,
		},
		driftPolicy: policy,
	}
}

func TestPerformDriftDetection_IgnoreMode(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeIgnore,
	}, nil)
	detector := &mockDetector{}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	assert.Equal(t, 0, detector.detectCallCount, "DetectDrift should not be called in ignore mode")
}

func TestPerformDriftDetection_NoDrift(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeDetect,
	}, nil)
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, _ bool) (*driftsvc.Result, error) {
			return driftsvc.NewResult("role", "test"), nil
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	assert.Equal(t, 1, detector.detectCallCount)
	require.NotNil(t, resource.driftStatus)
	assert.False(t, resource.driftStatus.Detected)
}

func TestPerformDriftDetection_DriftDetectedInDetectMode(t *testing.T) {
	orch, recorder := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeDetect,
	}, nil)
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, _ bool) (*driftsvc.Result, error) {
			r := driftsvc.NewResult("role", "test")
			r.AddDiff(driftsvc.Diff{Field: "password", Expected: "new", Actual: "old"})
			return r, nil
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	assert.Equal(t, 1, detector.detectCallCount, "Should detect but not correct in detect mode")
	assert.Equal(t, 0, detector.correctCallCount)
	require.NotNil(t, resource.driftStatus)
	assert.True(t, resource.driftStatus.Detected)

	// Check event was recorded
	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "DriftDetected")
	default:
		t.Error("Expected DriftDetected event")
	}
}

func TestPerformDriftDetection_DriftCorrected(t *testing.T) {
	orch, recorder := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}, nil)
	diff := driftsvc.Diff{Field: "password", Expected: "new", Actual: "old"}
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, _ bool) (*driftsvc.Result, error) {
			r := driftsvc.NewResult("role", "test")
			r.AddDiff(diff)
			return r, nil
		},
		correctDriftFunc: func(_ context.Context, _ *driftsvc.Result, _ bool) (*driftsvc.CorrectionResult, error) {
			cr := driftsvc.NewCorrectionResult("test")
			cr.AddCorrected(diff)
			return cr, nil
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	assert.Equal(t, 2, detector.detectCallCount, "DetectDrift called twice: initial + re-detect for correction")
	assert.Equal(t, 1, detector.correctCallCount)
	require.NotNil(t, resource.driftStatus)
	assert.False(t, resource.driftStatus.Detected, "Drift status should be cleared after correction")

	// Check events
	events := collectEvents(recorder)
	assert.Len(t, events, 2)
	assert.Contains(t, events[0], "DriftDetected")
	assert.Contains(t, events[1], "DriftCorrected")
}

func TestPerformDriftDetection_DetectError(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeDetect,
	}, nil)
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, _ bool) (*driftsvc.Result, error) {
			return nil, errors.New("connection failed")
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	assert.Nil(t, resource.driftStatus, "Status should not be set on error")
}

func TestPerformDriftDetection_DestructiveAnnotation(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeDetect,
	}, map[string]string{
		dbopsv1alpha1.AnnotationAllowDestructiveDrift: "true",
	})

	var capturedAllowDestructive bool
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, allowDestructive bool) (*driftsvc.Result, error) {
			capturedAllowDestructive = allowDestructive
			return driftsvc.NewResult("role", "test"), nil
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	assert.True(t, capturedAllowDestructive)
}

func TestPerformDriftDetection_FallbackToInstancePolicy(t *testing.T) {
	orch, _ := newTestOrchestrator()
	// Resource has no drift policy
	resource := newMockResource("test", "default", nil, nil)
	// Instance has detect mode
	instance := &mockInstancePolicy{
		policy: &dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeDetect,
			Interval: "10m",
		},
	}
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, _ bool) (*driftsvc.Result, error) {
			return driftsvc.NewResult("role", "test"), nil
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, instance, detector)

	assert.Equal(t, 1, detector.detectCallCount, "Should use instance policy when resource has none")
}

func TestPerformDriftDetection_FallbackToInstanceIgnore(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", nil, nil)
	instance := &mockInstancePolicy{
		policy: &dbopsv1alpha1.DriftPolicy{
			Mode: dbopsv1alpha1.DriftModeIgnore,
		},
	}
	detector := &mockDetector{}

	orch.PerformDriftDetection(context.Background(), resource, instance, detector)

	assert.Equal(t, 0, detector.detectCallCount, "Should not detect when instance policy is ignore")
}

func TestPerformDriftDetection_CorrectionFailed(t *testing.T) {
	orch, recorder := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode: dbopsv1alpha1.DriftModeCorrect,
	}, nil)
	diff := driftsvc.Diff{Field: "owner", Expected: "new_owner", Actual: "old_owner"}
	detector := &mockDetector{
		detectDriftFunc: func(_ context.Context, _ bool) (*driftsvc.Result, error) {
			r := driftsvc.NewResult("database", "testdb")
			r.AddDiff(diff)
			return r, nil
		},
		correctDriftFunc: func(_ context.Context, _ *driftsvc.Result, _ bool) (*driftsvc.CorrectionResult, error) {
			return nil, errors.New("permission denied")
		},
	}

	orch.PerformDriftDetection(context.Background(), resource, nil, detector)

	events := collectEvents(recorder)
	assert.Contains(t, events[0], "DriftDetected")
	assert.Contains(t, events[1], "DriftCorrectionFailed")
}

func TestGetRequeueInterval_ResourcePolicy(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", &dbopsv1alpha1.DriftPolicy{
		Mode:     dbopsv1alpha1.DriftModeDetect,
		Interval: "10m",
	}, nil)

	interval := orch.GetRequeueInterval(resource, nil)
	assert.Equal(t, 10*time.Minute, interval)
}

func TestGetRequeueInterval_InstanceFallback(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", nil, nil)
	instance := &mockInstancePolicy{
		policy: &dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeDetect,
			Interval: "15m",
		},
	}

	interval := orch.GetRequeueInterval(resource, instance)
	assert.Equal(t, 15*time.Minute, interval)
}

func TestGetRequeueInterval_DefaultFallback(t *testing.T) {
	orch, _ := newTestOrchestrator()
	resource := newMockResource("test", "default", nil, nil)

	interval := orch.GetRequeueInterval(resource, nil)
	assert.Equal(t, 5*time.Minute, interval)
}

func TestGetEffectiveDriftPolicy_Precedence(t *testing.T) {
	defaultInterval := 5 * time.Minute

	tests := []struct {
		name           string
		resourcePolicy *dbopsv1alpha1.DriftPolicy
		instancePolicy *dbopsv1alpha1.DriftPolicy
		expectedMode   dbopsv1alpha1.DriftMode
	}{
		{
			name:           "resource policy takes precedence",
			resourcePolicy: &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect},
			instancePolicy: &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeIgnore},
			expectedMode:   dbopsv1alpha1.DriftModeCorrect,
		},
		{
			name:           "falls back to instance policy",
			resourcePolicy: nil,
			instancePolicy: &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeIgnore},
			expectedMode:   dbopsv1alpha1.DriftModeIgnore,
		},
		{
			name:           "defaults to detect mode",
			resourcePolicy: nil,
			instancePolicy: nil,
			expectedMode:   dbopsv1alpha1.DriftModeDetect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := newMockResource("test", "default", tt.resourcePolicy, nil)
			var instance InstanceDriftPolicy
			if tt.instancePolicy != nil {
				instance = &mockInstancePolicy{policy: tt.instancePolicy}
			}
			policy := getEffectiveDriftPolicy(resource, instance, defaultInterval)
			assert.Equal(t, tt.expectedMode, policy.Mode)
		})
	}
}

func TestNamespacedInstancePolicy(t *testing.T) {
	t.Run("nil instance", func(t *testing.T) {
		p := &NamespacedInstancePolicy{Instance: nil}
		assert.Nil(t, p.GetDriftPolicy())
	})

	t.Run("with policy", func(t *testing.T) {
		p := &NamespacedInstancePolicy{
			Instance: &dbopsv1alpha1.DatabaseInstance{
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect},
				},
			},
		}
		require.NotNil(t, p.GetDriftPolicy())
		assert.Equal(t, dbopsv1alpha1.DriftModeCorrect, p.GetDriftPolicy().Mode)
	})
}

func TestClusterInstancePolicy(t *testing.T) {
	t.Run("nil instance", func(t *testing.T) {
		p := &ClusterInstancePolicy{Instance: nil}
		assert.Nil(t, p.GetDriftPolicy())
	})

	t.Run("with policy", func(t *testing.T) {
		p := &ClusterInstancePolicy{
			Instance: &dbopsv1alpha1.ClusterDatabaseInstance{
				Spec: dbopsv1alpha1.DatabaseInstanceSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeDetect},
				},
			},
		}
		require.NotNil(t, p.GetDriftPolicy())
		assert.Equal(t, dbopsv1alpha1.DriftModeDetect, p.GetDriftPolicy().Mode)
	})
}

func TestResolvedInstancePolicy(t *testing.T) {
	t.Run("nil resolved", func(t *testing.T) {
		p := &ResolvedInstancePolicy{Resolved: nil}
		assert.Nil(t, p.GetDriftPolicy())
	})

	t.Run("nil spec", func(t *testing.T) {
		p := &ResolvedInstancePolicy{Resolved: &instanceresolver.ResolvedInstance{}}
		assert.Nil(t, p.GetDriftPolicy())
	})

	t.Run("with policy", func(t *testing.T) {
		p := &ResolvedInstancePolicy{
			Resolved: &instanceresolver.ResolvedInstance{
				Spec: &dbopsv1alpha1.DatabaseInstanceSpec{
					DriftPolicy: &dbopsv1alpha1.DriftPolicy{Mode: dbopsv1alpha1.DriftModeCorrect},
				},
			},
		}
		require.NotNil(t, p.GetDriftPolicy())
		assert.Equal(t, dbopsv1alpha1.DriftModeCorrect, p.GetDriftPolicy().Mode)
	})
}

func collectEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case event := <-recorder.Events:
			events = append(events, event)
		default:
			return events
		}
	}
}
