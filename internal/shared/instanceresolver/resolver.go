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

// Package instanceresolver provides unified resolution of DatabaseInstance
// and ClusterDatabaseInstance resources for child resources.
package instanceresolver

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// ResolvedInstance contains the resolved instance information needed
// for database operations. This provides a unified view regardless of
// whether the source was a DatabaseInstance or ClusterDatabaseInstance.
type ResolvedInstance struct {
	// Spec is the DatabaseInstanceSpec (same type for both instance types)
	Spec *dbopsv1alpha1.DatabaseInstanceSpec

	// CredentialNamespace is the namespace where credentials should be looked up
	// For namespaced instances, this is the instance namespace
	// For cluster instances, this is the namespace specified in secretRef
	CredentialNamespace string

	// Phase is the current phase of the instance
	Phase dbopsv1alpha1.Phase

	// IsClusterScoped indicates if this was resolved from a ClusterDatabaseInstance
	IsClusterScoped bool

	// Name is the instance name
	Name string

	// Version is the database version from status
	Version string
}

// Resolver resolves DatabaseInstance or ClusterDatabaseInstance references.
type Resolver struct {
	client client.Client
}

// New creates a new instance resolver.
func New(c client.Client) *Resolver {
	return &Resolver{client: c}
}

// Resolve resolves an instance reference and returns the unified ResolvedInstance.
// Either instanceRef or clusterInstanceRef must be provided, but not both.
// The defaultNamespace is used when instanceRef doesn't specify a namespace.
func (r *Resolver) Resolve(
	ctx context.Context,
	instanceRef *dbopsv1alpha1.InstanceReference,
	clusterInstanceRef *dbopsv1alpha1.ClusterInstanceReference,
	defaultNamespace string,
) (*ResolvedInstance, error) {
	// Validate that exactly one reference is provided
	if instanceRef == nil && clusterInstanceRef == nil {
		return nil, fmt.Errorf("either instanceRef or clusterInstanceRef must be specified")
	}
	if instanceRef != nil && clusterInstanceRef != nil {
		return nil, fmt.Errorf("instanceRef and clusterInstanceRef are mutually exclusive")
	}

	if clusterInstanceRef != nil {
		return r.resolveClusterInstance(ctx, clusterInstanceRef)
	}

	return r.resolveNamespacedInstance(ctx, instanceRef, defaultNamespace)
}

// resolveNamespacedInstance resolves a namespaced DatabaseInstance.
func (r *Resolver) resolveNamespacedInstance(
	ctx context.Context,
	ref *dbopsv1alpha1.InstanceReference,
	defaultNamespace string,
) (*ResolvedInstance, error) {
	namespace := defaultNamespace
	if ref.Namespace != "" {
		namespace = ref.Namespace
	}

	instance := &dbopsv1alpha1.DatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      ref.Name,
	}, instance); err != nil {
		return nil, fmt.Errorf("get DatabaseInstance %s/%s: %w", namespace, ref.Name, err)
	}

	return &ResolvedInstance{
		Spec:                &instance.Spec,
		CredentialNamespace: instance.Namespace,
		Phase:               instance.Status.Phase,
		IsClusterScoped:     false,
		Name:                instance.Name,
		Version:             instance.Status.Version,
	}, nil
}

// resolveClusterInstance resolves a cluster-scoped ClusterDatabaseInstance.
func (r *Resolver) resolveClusterInstance(
	ctx context.Context,
	ref *dbopsv1alpha1.ClusterInstanceReference,
) (*ResolvedInstance, error) {
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{}
	if err := r.client.Get(ctx, types.NamespacedName{
		Name: ref.Name,
		// No namespace for cluster-scoped resources
	}, instance); err != nil {
		return nil, fmt.Errorf("get ClusterDatabaseInstance %s: %w", ref.Name, err)
	}

	// For cluster-scoped instances, the credential namespace must come from secretRef
	credNamespace := ""
	if instance.Spec.Connection.SecretRef != nil {
		credNamespace = instance.Spec.Connection.SecretRef.Namespace
	}
	if credNamespace == "" {
		return nil, fmt.Errorf("ClusterDatabaseInstance %s: secretRef.namespace is required for cluster-scoped instances", ref.Name)
	}

	return &ResolvedInstance{
		Spec:                &instance.Spec,
		CredentialNamespace: credNamespace,
		Phase:               instance.Status.Phase,
		IsClusterScoped:     true,
		Name:                instance.Name,
		Version:             instance.Status.Version,
	}, nil
}

// IsReady checks if the resolved instance is in Ready phase.
func (ri *ResolvedInstance) IsReady() bool {
	return ri.Phase == dbopsv1alpha1.PhaseReady
}

// Engine returns the database engine type.
func (ri *ResolvedInstance) Engine() string {
	return string(ri.Spec.Engine)
}
