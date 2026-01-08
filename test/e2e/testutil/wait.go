//go:build e2e

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
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
)

const (
	// DefaultPollInterval is the default interval between polling attempts
	DefaultPollInterval = 2 * time.Second
)

// WaitForPhase waits for a custom resource to reach the expected phase.
// It polls the resource until the phase matches or the timeout is reached.
func WaitForPhase(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace, name string,
	expectedPhase string,
	timeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(ctx, DefaultPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		resource, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil // Resource not found yet, keep polling
			}
			return false, fmt.Errorf("failed to get resource %s/%s: %w", namespace, name, err)
		}

		status, found, err := unstructuredNestedMap(resource.Object, "status")
		if err != nil {
			return false, fmt.Errorf("failed to get status from resource: %w", err)
		}
		if !found {
			return false, nil // Status not set yet, keep polling
		}

		phase, found, err := unstructuredNestedString(status, "phase")
		if err != nil {
			return false, fmt.Errorf("failed to get phase from status: %w", err)
		}
		if !found {
			return false, nil // Phase not set yet, keep polling
		}

		if phase == expectedPhase {
			return true, nil // Phase matches, we're done
		}

		// If phase is "Failed", return an error to stop waiting
		if phase == "Failed" {
			message, _, _ := unstructuredNestedString(status, "message")
			return false, fmt.Errorf("resource %s/%s reached Failed phase: %s", namespace, name, message)
		}

		return false, nil // Keep polling
	})
}

// WaitForCondition waits for a custom resource to have a specific condition with the expected status.
func WaitForCondition(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace, name string,
	condType string,
	condStatus string,
	timeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(ctx, DefaultPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		resource, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil // Resource not found yet, keep polling
			}
			return false, fmt.Errorf("failed to get resource %s/%s: %w", namespace, name, err)
		}

		status, found, err := unstructuredNestedMap(resource.Object, "status")
		if err != nil {
			return false, fmt.Errorf("failed to get status from resource: %w", err)
		}
		if !found {
			return false, nil // Status not set yet, keep polling
		}

		conditions, found, err := unstructuredNestedSlice(status, "conditions")
		if err != nil {
			return false, fmt.Errorf("failed to get conditions from status: %w", err)
		}
		if !found {
			return false, nil // Conditions not set yet, keep polling
		}

		for _, c := range conditions {
			condMap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}

			cType, _, _ := unstructuredNestedString(condMap, "type")
			cStatus, _, _ := unstructuredNestedString(condMap, "status")

			if cType == condType && cStatus == condStatus {
				return true, nil // Condition matches, we're done
			}
		}

		return false, nil // Condition not found or status doesn't match, keep polling
	})
}

// WaitForDeletion waits for a custom resource to be deleted.
func WaitForDeletion(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace, name string,
	timeout time.Duration,
) error {
	return wait.PollUntilContextTimeout(ctx, DefaultPollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil // Resource is gone, we're done
			}
			return false, fmt.Errorf("failed to check resource %s/%s: %w", namespace, name, err)
		}
		return false, nil // Resource still exists, keep polling
	})
}

// Helper functions for unstructured access

func unstructuredNestedMap(obj map[string]interface{}, fields ...string) (map[string]interface{}, bool, error) {
	val, found, err := unstructuredNestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, false, fmt.Errorf("expected map, got %T", val)
	}
	return m, true, nil
}

func unstructuredNestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	val, found, err := unstructuredNestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return "", found, err
	}
	s, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("expected string, got %T", val)
	}
	return s, true, nil
}

func unstructuredNestedSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	val, found, err := unstructuredNestedFieldNoCopy(obj, fields...)
	if !found || err != nil {
		return nil, found, err
	}
	s, ok := val.([]interface{})
	if !ok {
		return nil, false, fmt.Errorf("expected slice, got %T", val)
	}
	return s, true, nil
}

func unstructuredNestedFieldNoCopy(obj map[string]interface{}, fields ...string) (interface{}, bool, error) {
	var val interface{} = obj

	for i, field := range fields {
		m, ok := val.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("value at %v is not a map", fields[:i])
		}
		val, ok = m[field]
		if !ok {
			return nil, false, nil
		}
	}
	return val, true, nil
}
