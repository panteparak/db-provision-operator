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

	// Roles defines role memberships for this user
	// The user will be granted membership in these roles
	// +optional
	Roles []string `json:"roles,omitempty"`

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

	// ReconcileID is the unique identifier for the last reconciliation.
	// Used for end-to-end tracing across logs, events, and status updates.
	// +optional
	ReconcileID string `json:"reconcileID,omitempty"`

	// LastReconcileTime is when the last reconciliation occurred
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// User contains user-specific status information
	User *UserInfo `json:"user,omitempty"`

	// Secret contains generated secret information
	Secret *SecretInfo `json:"secret,omitempty"`

	// Rotation contains rotation status information
	// +optional
	Rotation *RotationStatus `json:"rotation,omitempty"`

	// OwnershipBlock contains information when deletion is blocked due to object ownership
	// +optional
	OwnershipBlock *OwnershipBlockStatus `json:"ownershipBlock,omitempty"`

	// Drift contains drift detection status information
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// OwnershipBlockStatus contains information about deletion blocking due to object ownership.
type OwnershipBlockStatus struct {
	// Blocked indicates if deletion is currently blocked
	Blocked bool `json:"blocked"`

	// LastCheckedAt is when the ownership check was performed
	LastCheckedAt *metav1.Time `json:"lastCheckedAt,omitempty"`

	// OwnedObjects lists database objects owned by this user
	OwnedObjects []OwnedObject `json:"ownedObjects,omitempty"`

	// Resolution provides the command to resolve ownership issues
	Resolution string `json:"resolution,omitempty"`

	// Message provides human-readable context about the block
	Message string `json:"message,omitempty"`
}

// RotationStatus contains the current rotation state
type RotationStatus struct {
	// Enabled indicates if rotation is enabled
	Enabled bool `json:"enabled,omitempty"`

	// Strategy is the rotation strategy being used
	Strategy RotationStrategy `json:"strategy,omitempty"`

	// LastRotatedAt is when the last rotation occurred
	LastRotatedAt *metav1.Time `json:"lastRotatedAt,omitempty"`

	// NextRotationAt is when the next rotation is scheduled
	NextRotationAt *metav1.Time `json:"nextRotationAt,omitempty"`

	// ActiveUser is the currently active database username
	ActiveUser string `json:"activeUser,omitempty"`

	// ServiceRole is the service role used for PostgreSQL role-inheritance
	ServiceRole string `json:"serviceRole,omitempty"`

	// PendingDeletion contains users scheduled for deletion
	// +optional
	PendingDeletion []PendingDeletionInfo `json:"pendingDeletion,omitempty"`
}

// PendingDeletionInfo contains information about a user pending deletion
type PendingDeletionInfo struct {
	// User is the database username pending deletion
	User string `json:"user"`

	// DeprecatedAt is when the user was marked as deprecated
	DeprecatedAt *metav1.Time `json:"deprecatedAt,omitempty"`

	// DeleteAfter is when the user should be deleted
	DeleteAfter *metav1.Time `json:"deleteAfter,omitempty"`

	// Status is the current deletion status
	// +kubebuilder:validation:Enum=pending;blocked;deleted
	Status string `json:"status,omitempty"`

	// BlockedReason explains why deletion is blocked (if applicable)
	BlockedReason string `json:"blockedReason,omitempty"`

	// OwnedObjects lists database objects owned by this user
	// +optional
	OwnedObjects []OwnedObject `json:"ownedObjects,omitempty"`

	// Resolution provides the command to resolve ownership issues
	Resolution string `json:"resolution,omitempty"`

	// ReconcileID is the reconcile ID when this status was last updated
	ReconcileID string `json:"reconcileID,omitempty"`
}

// OwnedObject represents a database object owned by a user
type OwnedObject struct {
	// Schema is the object's schema
	Schema string `json:"schema,omitempty"`

	// Name is the object's name
	Name string `json:"name"`

	// Type is the object type (table, sequence, function, etc.)
	Type string `json:"type"`
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
