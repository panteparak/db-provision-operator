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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// GetResourcePhase retrieves the current phase of a custom resource.
// Returns an empty string if the phase is not set.
func GetResourcePhase(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace, name string,
) (string, error) {
	resource, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get resource %s/%s: %w", namespace, name, err)
	}

	status, found, err := unstructuredNestedMap(resource.Object, "status")
	if err != nil {
		return "", fmt.Errorf("failed to get status from resource: %w", err)
	}
	if !found {
		return "", nil
	}

	phase, found, err := unstructuredNestedString(status, "phase")
	if err != nil {
		return "", fmt.Errorf("failed to get phase from status: %w", err)
	}
	if !found {
		return "", nil
	}

	return phase, nil
}

// ResourceExists checks if a custom resource exists.
func ResourceExists(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	gvr schema.GroupVersionResource,
	namespace, name string,
) (bool, error) {
	_, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check resource %s/%s: %w", namespace, name, err)
	}
	return true, nil
}

// SecretExists checks if a Kubernetes Secret exists.
func SecretExists(
	ctx context.Context,
	k8sClient kubernetes.Interface,
	namespace, name string,
) (bool, error) {
	_, err := k8sClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check secret %s/%s: %w", namespace, name, err)
	}
	return true, nil
}

// SecretContainsKeys checks if a Kubernetes Secret contains all the specified keys.
func SecretContainsKeys(
	ctx context.Context,
	k8sClient kubernetes.Interface,
	namespace, name string,
	keys ...string,
) (bool, error) {
	secret, err := k8sClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get secret %s/%s: %w", namespace, name, err)
	}

	for _, key := range keys {
		if _, exists := secret.Data[key]; !exists {
			return false, nil
		}
	}

	return true, nil
}
