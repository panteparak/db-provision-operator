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

// DatabaseRoleSpec defines the desired state of DatabaseRole.
type DatabaseRoleSpec struct {
	// InstanceRef references the DatabaseInstance to use
	// +kubebuilder:validation:Required
	InstanceRef InstanceReference `json:"instanceRef"`

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
}

// DatabaseRoleStatus defines the observed state of DatabaseRole.
type DatabaseRoleStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Creating;Ready;Failed;Deleting
	Phase Phase `json:"phase,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// Role contains role-specific status information
	Role *RoleInfo `json:"role,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RoleInfo contains role status information
type RoleInfo struct {
	// Name is the actual role name
	Name string `json:"name,omitempty"`

	// CreatedAt is the role creation timestamp
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbr
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.roleName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseRole is the Schema for the databaseroles API.
type DatabaseRole struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseRoleSpec   `json:"spec,omitempty"`
	Status DatabaseRoleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseRoleList contains a list of DatabaseRole.
type DatabaseRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseRole `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseRole{}, &DatabaseRoleList{})
}
