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

package instanceresolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = dbopsv1alpha1.AddToScheme(scheme)
	return scheme
}

func newNamespacedInstance(name, namespace string, engine dbopsv1alpha1.EngineType, phase dbopsv1alpha1.Phase) *dbopsv1alpha1.DatabaseInstance {
	return &dbopsv1alpha1.DatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: engine,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host:     "db.example.com",
				Port:     5432,
				Database: "admin",
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name: "db-credentials",
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase:   phase,
			Version: "15.4",
		},
	}
}

func newClusterInstance(name string, engine dbopsv1alpha1.EngineType, phase dbopsv1alpha1.Phase, secretNamespace string) *dbopsv1alpha1.ClusterDatabaseInstance {
	var secretRef *dbopsv1alpha1.CredentialSecretRef
	if secretNamespace != "" {
		secretRef = &dbopsv1alpha1.CredentialSecretRef{
			Name:      "cluster-db-credentials",
			Namespace: secretNamespace,
		}
	}

	return &dbopsv1alpha1.ClusterDatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: engine,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host:      "cluster-db.example.com",
				Port:      5432,
				Database:  "admin",
				SecretRef: secretRef,
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase:   phase,
			Version: "16.1",
		},
	}
}

func TestResolve_NamespacedInstance_Ready(t *testing.T) {
	scheme := newTestScheme()
	instance := newNamespacedInstance("my-instance", "default", dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.PhaseReady)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	ref := &dbopsv1alpha1.InstanceReference{Name: "my-instance"}

	resolved, err := resolver.Resolve(context.Background(), ref, nil, "default")

	require.NoError(t, err)
	assert.Equal(t, "my-instance", resolved.Name)
	assert.Equal(t, "default", resolved.CredentialNamespace)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, resolved.Phase)
	assert.False(t, resolved.IsClusterScoped)
	assert.Equal(t, "15.4", resolved.Version)
	assert.Equal(t, dbopsv1alpha1.EngineTypePostgres, resolved.Spec.Engine)
	assert.Equal(t, "db.example.com", resolved.Spec.Connection.Host)
	assert.True(t, resolved.IsReady())
	assert.Equal(t, "postgres", resolved.Engine())
}

func TestResolve_NamespacedInstance_NotReady(t *testing.T) {
	scheme := newTestScheme()
	instance := newNamespacedInstance("my-instance", "default", dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.PhasePending)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	ref := &dbopsv1alpha1.InstanceReference{Name: "my-instance"}

	resolved, err := resolver.Resolve(context.Background(), ref, nil, "default")

	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhasePending, resolved.Phase)
	assert.False(t, resolved.IsReady())
}

func TestResolve_NamespacedInstance_NotFound(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resolver := New(k8sClient)
	ref := &dbopsv1alpha1.InstanceReference{Name: "nonexistent"}

	resolved, err := resolver.Resolve(context.Background(), ref, nil, "default")

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "get DatabaseInstance default/nonexistent")
}

func TestResolve_NamespacedInstance_ExplicitNamespace(t *testing.T) {
	scheme := newTestScheme()
	instance := newNamespacedInstance("my-instance", "other-ns", dbopsv1alpha1.EngineTypeMySQL, dbopsv1alpha1.PhaseReady)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	ref := &dbopsv1alpha1.InstanceReference{
		Name:      "my-instance",
		Namespace: "other-ns",
	}

	resolved, err := resolver.Resolve(context.Background(), ref, nil, "default")

	require.NoError(t, err)
	assert.Equal(t, "my-instance", resolved.Name)
	assert.Equal(t, "other-ns", resolved.CredentialNamespace)
	assert.Equal(t, dbopsv1alpha1.EngineTypeMySQL, resolved.Spec.Engine)
	assert.Equal(t, "mysql", resolved.Engine())
}

func TestResolve_NamespacedInstance_ExplicitNamespace_OverridesDefault(t *testing.T) {
	scheme := newTestScheme()
	instance := newNamespacedInstance("my-instance", "explicit-ns", dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.PhaseReady)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	ref := &dbopsv1alpha1.InstanceReference{
		Name:      "my-instance",
		Namespace: "explicit-ns",
	}

	// The default namespace should be ignored when ref.Namespace is set
	resolved, err := resolver.Resolve(context.Background(), ref, nil, "wrong-default-ns")

	require.NoError(t, err)
	assert.Equal(t, "explicit-ns", resolved.CredentialNamespace)
}

func TestResolve_ClusterInstance_Ready(t *testing.T) {
	scheme := newTestScheme()
	instance := newClusterInstance("shared-db", dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.PhaseReady, "infra")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	clusterRef := &dbopsv1alpha1.ClusterInstanceReference{Name: "shared-db"}

	resolved, err := resolver.Resolve(context.Background(), nil, clusterRef, "default")

	require.NoError(t, err)
	assert.Equal(t, "shared-db", resolved.Name)
	assert.Equal(t, "infra", resolved.CredentialNamespace)
	assert.Equal(t, dbopsv1alpha1.PhaseReady, resolved.Phase)
	assert.True(t, resolved.IsClusterScoped)
	assert.Equal(t, "16.1", resolved.Version)
	assert.Equal(t, "cluster-db.example.com", resolved.Spec.Connection.Host)
	assert.True(t, resolved.IsReady())
	assert.Equal(t, "postgres", resolved.Engine())
}

func TestResolve_ClusterInstance_NotReady(t *testing.T) {
	scheme := newTestScheme()
	instance := newClusterInstance("shared-db", dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.PhaseFailed, "infra")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	clusterRef := &dbopsv1alpha1.ClusterInstanceReference{Name: "shared-db"}

	resolved, err := resolver.Resolve(context.Background(), nil, clusterRef, "default")

	require.NoError(t, err)
	assert.Equal(t, dbopsv1alpha1.PhaseFailed, resolved.Phase)
	assert.False(t, resolved.IsReady())
}

func TestResolve_ClusterInstance_NotFound(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resolver := New(k8sClient)
	clusterRef := &dbopsv1alpha1.ClusterInstanceReference{Name: "nonexistent"}

	resolved, err := resolver.Resolve(context.Background(), nil, clusterRef, "default")

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "get ClusterDatabaseInstance nonexistent")
}

func TestResolve_ClusterInstance_MissingSecretNamespace(t *testing.T) {
	scheme := newTestScheme()

	// Create a cluster instance with no secretRef at all
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-secret-ns",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host:     "cluster-db.example.com",
				Port:     5432,
				Database: "admin",
				// SecretRef is nil
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	clusterRef := &dbopsv1alpha1.ClusterInstanceReference{Name: "no-secret-ns"}

	resolved, err := resolver.Resolve(context.Background(), nil, clusterRef, "default")

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "secretRef.namespace is required for cluster-scoped instances")
}

func TestResolve_ClusterInstance_EmptySecretNamespace(t *testing.T) {
	scheme := newTestScheme()

	// Create a cluster instance with secretRef but empty namespace
	instance := &dbopsv1alpha1.ClusterDatabaseInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: "empty-secret-ns",
		},
		Spec: dbopsv1alpha1.DatabaseInstanceSpec{
			Engine: dbopsv1alpha1.EngineTypePostgres,
			Connection: dbopsv1alpha1.ConnectionConfig{
				Host:     "cluster-db.example.com",
				Port:     5432,
				Database: "admin",
				SecretRef: &dbopsv1alpha1.CredentialSecretRef{
					Name:      "creds",
					Namespace: "", // Empty namespace
				},
			},
		},
		Status: dbopsv1alpha1.DatabaseInstanceStatus{
			Phase: dbopsv1alpha1.PhaseReady,
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	clusterRef := &dbopsv1alpha1.ClusterInstanceReference{Name: "empty-secret-ns"}

	resolved, err := resolver.Resolve(context.Background(), nil, clusterRef, "default")

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "secretRef.namespace is required for cluster-scoped instances")
}

func TestResolve_BothRefsSet_Error(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resolver := New(k8sClient)
	ref := &dbopsv1alpha1.InstanceReference{Name: "my-instance"}
	clusterRef := &dbopsv1alpha1.ClusterInstanceReference{Name: "shared-db"}

	resolved, err := resolver.Resolve(context.Background(), ref, clusterRef, "default")

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "instanceRef and clusterInstanceRef are mutually exclusive")
}

func TestResolve_NeitherRefSet_Error(t *testing.T) {
	scheme := newTestScheme()
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	resolver := New(k8sClient)

	resolved, err := resolver.Resolve(context.Background(), nil, nil, "default")

	require.Error(t, err)
	assert.Nil(t, resolved)
	assert.Contains(t, err.Error(), "either instanceRef or clusterInstanceRef must be specified")
}

func TestIsReady_True(t *testing.T) {
	ri := &ResolvedInstance{
		Phase: dbopsv1alpha1.PhaseReady,
		Spec:  &dbopsv1alpha1.DatabaseInstanceSpec{},
	}
	assert.True(t, ri.IsReady())
}

func TestIsReady_False(t *testing.T) {
	tests := []struct {
		name  string
		phase dbopsv1alpha1.Phase
	}{
		{"Pending", dbopsv1alpha1.PhasePending},
		{"Failed", dbopsv1alpha1.PhaseFailed},
		{"PendingDeletion", dbopsv1alpha1.PhasePendingDeletion},
		{"Empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ri := &ResolvedInstance{
				Phase: tt.phase,
				Spec:  &dbopsv1alpha1.DatabaseInstanceSpec{},
			}
			assert.False(t, ri.IsReady())
		})
	}
}

func TestEngine_ReturnsCorrectType(t *testing.T) {
	tests := []struct {
		name     string
		engine   dbopsv1alpha1.EngineType
		expected string
	}{
		{"Postgres", dbopsv1alpha1.EngineTypePostgres, "postgres"},
		{"MySQL", dbopsv1alpha1.EngineTypeMySQL, "mysql"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ri := &ResolvedInstance{
				Spec: &dbopsv1alpha1.DatabaseInstanceSpec{
					Engine: tt.engine,
				},
			}
			assert.Equal(t, tt.expected, ri.Engine())
		})
	}
}

func TestResolve_NamespacedInstance_UsesDefaultNamespace(t *testing.T) {
	scheme := newTestScheme()
	instance := newNamespacedInstance("my-instance", "my-default-ns", dbopsv1alpha1.EngineTypePostgres, dbopsv1alpha1.PhaseReady)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(instance).
		WithStatusSubresource(instance).
		Build()

	resolver := New(k8sClient)
	// InstanceReference with no explicit namespace
	ref := &dbopsv1alpha1.InstanceReference{Name: "my-instance"}

	resolved, err := resolver.Resolve(context.Background(), ref, nil, "my-default-ns")

	require.NoError(t, err)
	assert.Equal(t, "my-default-ns", resolved.CredentialNamespace)
}
