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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseInstanceSpec defines the desired state of DatabaseInstance.
type DatabaseInstanceSpec struct {
	// Engine type (required, immutable)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=postgres;mysql;mariadb;cockroachdb
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="engine is immutable"
	Engine EngineType `json:"engine"`

	// Connection configuration
	// +kubebuilder:validation:Required
	Connection ConnectionConfig `json:"connection"`

	// TLS configuration
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// Health check configuration
	// +optional
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`

	// DriftPolicy defines the default drift detection policy for resources using this instance.
	// Individual resources can override this policy.
	// +optional
	DriftPolicy *DriftPolicy `json:"driftPolicy,omitempty"`

	// Discovery enables scanning for database resources not managed by Kubernetes CRs.
	// Discovered resources can be adopted via annotations.
	// +optional
	Discovery *DiscoveryConfig `json:"discovery,omitempty"`

	// PostgreSQL-specific options (only valid when engine is "postgres")
	// +optional
	Postgres *PostgresInstanceConfig `json:"postgres,omitempty"`

	// MySQL-specific options (only valid when engine is "mysql")
	// +optional
	MySQL *MySQLInstanceConfig `json:"mysql,omitempty"`

	// ResourceTracking configures how managed resources are tagged in the database.
	// This enables identifying operator-managed resources at the database level
	// via COMMENT ON (PostgreSQL/CockroachDB) or user attributes/metadata tables (MySQL).
	// +optional
	ResourceTracking *ResourceTrackingConfig `json:"resourceTracking,omitempty"`

	// DeletionProtection prevents accidental deletion
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`
}

// ConnectionConfig defines the database connection settings
type ConnectionConfig struct {
	// Host is the database server hostname or IP
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// Port is the database server port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Database is the admin database name for initial connection
	// +kubebuilder:validation:Required
	Database string `json:"database"`

	// SecretRef references a secret containing credentials (mutually exclusive with ExistingSecret)
	// +optional
	SecretRef *CredentialSecretRef `json:"secretRef,omitempty"`

	// ExistingSecret references an existing secret with custom keys (mutually exclusive with SecretRef)
	// +optional
	ExistingSecret *CredentialSecretRef `json:"existingSecret,omitempty"`
}

// DatabaseInstanceStatus defines the observed state of DatabaseInstance.
type DatabaseInstanceStatus struct {
	// Phase represents the current state of the instance
	// +kubebuilder:validation:Enum=Pending;Ready;Failed
	Phase Phase `json:"phase,omitempty"`

	// Version is the detected database server version
	Version string `json:"version,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// LastCheckedAt is the timestamp of the last health check
	LastCheckedAt *metav1.Time `json:"lastCheckedAt,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReconcileID is the unique identifier for the last reconciliation.
	// Used for end-to-end tracing across logs, events, and status updates.
	// +optional
	ReconcileID string `json:"reconcileID,omitempty"`

	// LastReconcileTime is when the last reconciliation occurred
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// DiscoveredResources contains resources found in the database that are not managed by CRs.
	// Only populated when discovery is enabled.
	// +optional
	DiscoveredResources *DiscoveredResourcesStatus `json:"discoveredResources,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbi
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.connection.host`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.spec.connection.port`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseInstance is the Schema for the databaseinstances API.
type DatabaseInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseInstanceSpec   `json:"spec,omitempty"`
	Status DatabaseInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseInstanceList contains a list of DatabaseInstance.
type DatabaseInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseInstance{}, &DatabaseInstanceList{})
}
