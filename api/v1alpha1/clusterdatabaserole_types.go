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

// ClusterDatabaseRoleSpec defines the desired state of ClusterDatabaseRole.
// This is a cluster-scoped role for shared service accounts and cross-namespace access.
type ClusterDatabaseRoleSpec struct {
	// ClusterInstanceRef references a cluster-scoped ClusterDatabaseInstance
	// +kubebuilder:validation:Required
	ClusterInstanceRef ClusterInstanceReference `json:"clusterInstanceRef"`

	// RoleName is the role name in the database (immutable after creation)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="roleName is immutable"
	RoleName string `json:"roleName"`

	// PostgreSQL-specific configuration
	// +optional
	Postgres *PostgresRoleConfig `json:"postgres,omitempty"`

	// MySQL-specific configuration
	// +optional
	MySQL *MySQLRoleConfig `json:"mysql,omitempty"`

	// DriftPolicy overrides the instance-level drift policy for this role.
	// If not specified, the instance's drift policy is used.
	// +optional
	DriftPolicy *DriftPolicy `json:"driftPolicy,omitempty"`

	// ManagedResourceComment is an informational comment applied to the role in the database.
	// This helps identify operator-managed resources at the database level.
	// Example: "managed-by:db-provision-operator,cr:orders-service"
	// +optional
	ManagedResourceComment string `json:"managedResourceComment,omitempty"`
}

// ClusterDatabaseRoleStatus defines the observed state of ClusterDatabaseRole.
type ClusterDatabaseRoleStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Creating;Ready;Failed;Deleting
	Phase Phase `json:"phase,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReconcileID is a unique identifier for the current reconciliation cycle.
	// Used for tracing through logs, events, and APM systems.
	// +optional
	ReconcileID string `json:"reconcileID,omitempty"`

	// LastReconcileTime is the timestamp of the last reconciliation
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Role contains role-specific status information
	Role *RoleInfo `json:"role,omitempty"`

	// Drift contains drift detection status information
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cdbr
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.clusterInstanceRef.name`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.roleName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterDatabaseRole is the Schema for the clusterdatabaseroles API.
// It represents a cluster-scoped database role for shared service accounts
// and cross-namespace access patterns.
type ClusterDatabaseRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDatabaseRoleSpec   `json:"spec,omitempty"`
	Status ClusterDatabaseRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterDatabaseRoleList contains a list of ClusterDatabaseRole.
type ClusterDatabaseRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDatabaseRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDatabaseRole{}, &ClusterDatabaseRoleList{})
}
