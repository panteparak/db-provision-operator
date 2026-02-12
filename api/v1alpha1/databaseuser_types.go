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

// DatabaseUserSpec defines the desired state of DatabaseUser.
// +kubebuilder:validation:XValidation:rule="has(self.instanceRef) || has(self.clusterInstanceRef)",message="either instanceRef or clusterInstanceRef must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.instanceRef) && has(self.clusterInstanceRef))",message="instanceRef and clusterInstanceRef are mutually exclusive"
type DatabaseUserSpec struct {
	// InstanceRef references a namespaced DatabaseInstance (mutually exclusive with ClusterInstanceRef)
	// +optional
	InstanceRef *InstanceReference `json:"instanceRef,omitempty"`

	// ClusterInstanceRef references a cluster-scoped ClusterDatabaseInstance (mutually exclusive with InstanceRef)
	// +optional
	ClusterInstanceRef *ClusterInstanceReference `json:"clusterInstanceRef,omitempty"`

	// Username is the database username (immutable after creation)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="username is immutable"
	Username string `json:"username"`

	// PasswordSecret configures password generation and secret output
	// +optional
	PasswordSecret *PasswordConfig `json:"passwordSecret,omitempty"`

	// ExistingPasswordSecret references an existing secret containing the password
	// +optional
	ExistingPasswordSecret *ExistingPasswordSecret `json:"existingPasswordSecret,omitempty"`

	// PasswordRotation configures automatic password rotation
	// +optional
	PasswordRotation *PasswordRotationConfig `json:"passwordRotation,omitempty"`

	// PostgreSQL-specific configuration
	// +optional
	Postgres *PostgresUserConfig `json:"postgres,omitempty"`

	// MySQL-specific configuration
	// +optional
	MySQL *MySQLUserConfig `json:"mysql,omitempty"`

	// DriftPolicy overrides the instance-level drift policy for this user.
	// If not specified, the instance's drift policy is used.
	// +optional
	DriftPolicy *DriftPolicy `json:"driftPolicy,omitempty"`
}

// DatabaseUserStatus defines the observed state of DatabaseUser.
type DatabaseUserStatus struct {
	// Phase represents the current state
	// +kubebuilder:validation:Enum=Pending;Creating;Ready;Failed;Deleting
	Phase Phase `json:"phase,omitempty"`

	// ObservedGeneration is the last observed generation of the resource
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// User contains user-specific status information
	User *UserInfo `json:"user,omitempty"`

	// Secret contains generated secret information
	Secret *SecretInfo `json:"secret,omitempty"`

	// Drift contains drift detection status information
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// UserInfo contains user status information
type UserInfo struct {
	// Username is the actual database username
	Username string `json:"username,omitempty"`

	// CreatedAt is the user creation timestamp
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

// SecretInfo contains generated secret information
type SecretInfo struct {
	// Name is the secret name
	Name string `json:"name,omitempty"`

	// Namespace is the secret namespace
	Namespace string `json:"namespace,omitempty"`

	// LastRotatedAt is the last password rotation timestamp
	LastRotatedAt *metav1.Time `json:"lastRotatedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbu
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Username",type=string,JSONPath=`.spec.username`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.status.secret.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DatabaseUser is the Schema for the databaseusers API.
type DatabaseUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseUserSpec   `json:"spec,omitempty"`
	Status DatabaseUserStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseUserList contains a list of DatabaseUser.
type DatabaseUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseUser `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseUser{}, &DatabaseUserList{})
}
