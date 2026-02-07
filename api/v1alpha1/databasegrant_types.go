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

// DatabaseGrantSpec defines the desired state of DatabaseGrant.
type DatabaseGrantSpec struct {
	// UserRef references the DatabaseUser to grant permissions to
	// +kubebuilder:validation:Required
	UserRef UserReference `json:"userRef"`

	// DatabaseRef references the Database for context (optional)
	// +optional
	DatabaseRef *DatabaseReference `json:"databaseRef,omitempty"`

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
}

// DatabaseGrantStatus defines the observed state of DatabaseGrant.
type DatabaseGrantStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Creating;Ready;Failed;Deleting
	Phase Phase `json:"phase,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// AppliedGrants contains information about applied grants
	AppliedGrants *AppliedGrantsInfo `json:"appliedGrants,omitempty"`

	// Drift contains drift detection status information
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// AppliedGrantsInfo contains information about applied grants
type AppliedGrantsInfo struct {
	// Roles lists assigned roles
	Roles []string `json:"roles,omitempty"`

	// DirectGrants is the count of direct grants applied
	DirectGrants int32 `json:"directGrants,omitempty"`

	// DefaultPrivileges is the count of default privileges applied
	DefaultPrivileges int32 `json:"defaultPrivileges,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbg
// +kubebuilder:printcolumn:name="User",type=string,JSONPath=`.spec.userRef.name`
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.databaseRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseGrant is the Schema for the databasegrants API.
type DatabaseGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseGrantSpec   `json:"spec,omitempty"`
	Status DatabaseGrantStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseGrantList contains a list of DatabaseGrant.
type DatabaseGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseGrant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseGrant{}, &DatabaseGrantList{})
}
