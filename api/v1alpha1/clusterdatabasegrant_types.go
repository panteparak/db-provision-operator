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

// ClusterDatabaseGrantSpec defines the desired state of ClusterDatabaseGrant.
// ClusterDatabaseGrant is a cluster-scoped grant resource for cross-namespace access control.
type ClusterDatabaseGrantSpec struct {
	// ClusterInstanceRef references the ClusterDatabaseInstance.
	// Only ClusterDatabaseInstance is supported (cluster-scoped grants require cluster-scoped instances).
	// +kubebuilder:validation:Required
	ClusterInstanceRef ClusterInstanceReference `json:"clusterInstanceRef"`

	// UserRef references the DatabaseUser to grant permissions to.
	// The namespace field enables cross-namespace access.
	// Either userRef or roleRef must be specified, but not both.
	// +optional
	UserRef *NamespacedUserReference `json:"userRef,omitempty"`

	// RoleRef references a ClusterDatabaseRole or DatabaseRole to grant permissions to.
	// If namespace is empty, it refers to a ClusterDatabaseRole.
	// If namespace is specified, it refers to a DatabaseRole in that namespace.
	// Either userRef or roleRef must be specified, but not both.
	// +optional
	RoleRef *NamespacedRoleReference `json:"roleRef,omitempty"`

	// PostgreSQL-specific grants
	// +optional
	Postgres *PostgresGrantConfig `json:"postgres,omitempty"`

	// MySQL-specific grants
	// +optional
	MySQL *MySQLGrantConfig `json:"mysql,omitempty"`

	// DriftPolicy overrides the instance-level drift policy for this grant.
	// If not specified, the instance's drift policy is used.
	// +optional
	DriftPolicy *DriftPolicy `json:"driftPolicy,omitempty"`

	// DeletionProtection prevents accidental deletion
	// +optional
	DeletionProtection bool `json:"deletionProtection,omitempty"`
}

// NamespacedUserReference references a DatabaseUser with an explicit namespace for cross-namespace access.
type NamespacedUserReference struct {
	// Name of the DatabaseUser resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the DatabaseUser (required for cross-namespace access)
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

// NamespacedRoleReference references either a ClusterDatabaseRole or a DatabaseRole.
// If namespace is empty, it refers to a ClusterDatabaseRole (cluster-scoped).
// If namespace is provided, it refers to a DatabaseRole in that namespace.
type NamespacedRoleReference struct {
	// Name of the role resource
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the role.
	// If empty, refers to a ClusterDatabaseRole (cluster-scoped).
	// If provided, refers to a DatabaseRole in the specified namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ClusterDatabaseGrantStatus defines the observed state of ClusterDatabaseGrant.
type ClusterDatabaseGrantStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Creating;Ready;Failed;Deleting
	Phase Phase `json:"phase,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ReconcileID is the unique ID for the current reconciliation.
	// Used for observability and tracing.
	// +optional
	ReconcileID string `json:"reconcileID,omitempty"`

	// LastReconcileTime is the timestamp of the last reconciliation.
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// AppliedGrants contains information about applied grants
	AppliedGrants *ClusterAppliedGrantsInfo `json:"appliedGrants,omitempty"`

	// TargetInfo contains resolved information about the target user or role.
	// +optional
	TargetInfo *GrantTargetInfo `json:"targetInfo,omitempty"`

	// Drift contains drift detection status information
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ClusterAppliedGrantsInfo contains information about applied grants for cluster-scoped grants.
type ClusterAppliedGrantsInfo struct {
	// Applied is the count of grants successfully applied
	Applied int32 `json:"applied,omitempty"`

	// Failed is the count of grants that failed to apply
	Failed int32 `json:"failed,omitempty"`

	// Roles lists assigned roles (if granting role membership)
	Roles []string `json:"roles,omitempty"`

	// DirectGrants is the count of direct privilege grants applied
	DirectGrants int32 `json:"directGrants,omitempty"`

	// DefaultPrivileges is the count of default privileges applied
	DefaultPrivileges int32 `json:"defaultPrivileges,omitempty"`

	// Details contains detailed information about each grant
	// +optional
	Details []GrantDetail `json:"details,omitempty"`
}

// GrantDetail contains information about a specific grant.
type GrantDetail struct {
	// Database is the target database
	Database string `json:"database,omitempty"`

	// Schema is the target schema
	// +optional
	Schema string `json:"schema,omitempty"`

	// Privileges lists the granted privileges
	Privileges []string `json:"privileges,omitempty"`

	// AppliedAt is when the grant was applied
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// Status indicates if this specific grant succeeded
	Status string `json:"status,omitempty"`

	// Message provides details about this grant
	// +optional
	Message string `json:"message,omitempty"`
}

// GrantTargetInfo contains resolved information about the grant target.
type GrantTargetInfo struct {
	// Type is the target type: "user" or "role"
	Type string `json:"type,omitempty"`

	// Name is the resolved name of the target
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the target (empty for cluster-scoped roles)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// ResolvedUsername is the actual database username (may differ from K8s resource name)
	// +optional
	ResolvedUsername string `json:"resolvedUsername,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cdbg
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.status.targetInfo.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.targetInfo.type`
// +kubebuilder:printcolumn:name="Applied",type=integer,JSONPath=`.status.appliedGrants.applied`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterDatabaseGrant is the Schema for the clusterdatabasegrants API.
// It provides cluster-scoped grant management for cross-namespace database access control.
type ClusterDatabaseGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterDatabaseGrantSpec   `json:"spec,omitempty"`
	Status ClusterDatabaseGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterDatabaseGrantList contains a list of ClusterDatabaseGrant.
type ClusterDatabaseGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDatabaseGrant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDatabaseGrant{}, &ClusterDatabaseGrantList{})
}
