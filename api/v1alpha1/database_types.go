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

// Annotations for force delete
const (
	AnnotationForceDelete        = "dbops.dbprovision.io/force-delete"
	AnnotationForceDeleteConfirm = "dbops.dbprovision.io/force-delete-confirm"
	ForceDeleteConfirmValue      = "I-UNDERSTAND-DATA-LOSS"
)

// DatabaseSpec defines the desired state of Database.
// +kubebuilder:validation:XValidation:rule="has(self.instanceRef) || has(self.clusterInstanceRef)",message="either instanceRef or clusterInstanceRef must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.instanceRef) && has(self.clusterInstanceRef))",message="instanceRef and clusterInstanceRef are mutually exclusive"
type DatabaseSpec struct {
	// InstanceRef references a namespaced DatabaseInstance (mutually exclusive with ClusterInstanceRef)
	// +optional
	InstanceRef *InstanceReference `json:"instanceRef,omitempty"`

	// ClusterInstanceRef references a cluster-scoped ClusterDatabaseInstance (mutually exclusive with InstanceRef)
	// +optional
	ClusterInstanceRef *ClusterInstanceReference `json:"clusterInstanceRef,omitempty"`

	// Name is the database name in the database server (immutable after creation)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="database name is immutable"
	Name string `json:"name"`

	// Owner specifies the database owner (role name). For role-based ownership patterns,
	// set this to a service role rather than a user to enable safe credential rotation.
	// If not specified, the database is owned by the connection user.
	// +optional
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z_][a-zA-Z0-9_]*$`
	Owner string `json:"owner,omitempty"`

	// DeletionPolicy defines what happens on CR deletion
	// +kubebuilder:validation:Enum=Retain;Delete;Snapshot
	// +kubebuilder:default=Retain
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`

	// DeletionProtection prevents accidental deletion
	// +kubebuilder:default=true
	DeletionProtection bool `json:"deletionProtection,omitempty"`

	// DriftPolicy overrides the instance-level drift policy for this database.
	// If not specified, the instance's drift policy is used.
	// +optional
	DriftPolicy *DriftPolicy `json:"driftPolicy,omitempty"`

	// PostgreSQL-specific configuration (required when instance engine is "postgres")
	// +optional
	Postgres *PostgresDatabaseConfig `json:"postgres,omitempty"`

	// MySQL-specific configuration (required when instance engine is "mysql")
	// +optional
	MySQL *MySQLDatabaseConfig `json:"mysql,omitempty"`
}

// DatabaseStatus defines the observed state of Database.
type DatabaseStatus struct {
	// Phase represents the current state of the database
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

	// Database contains database-specific status information
	Database *DatabaseInfo `json:"database,omitempty"`

	// Postgres contains PostgreSQL-specific status information
	// +optional
	Postgres *PostgresDatabaseStatus `json:"postgres,omitempty"`

	// MySQL contains MySQL-specific status information
	// +optional
	MySQL *MySQLDatabaseStatus `json:"mysql,omitempty"`

	// Drift contains drift detection status information
	// +optional
	Drift *DriftStatus `json:"drift,omitempty"`

	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatabaseInfo contains general database information
type DatabaseInfo struct {
	// Name is the actual database name
	Name string `json:"name,omitempty"`

	// Owner is the database owner
	Owner string `json:"owner,omitempty"`

	// SizeBytes is the database size in bytes
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// CreatedAt is the creation timestamp
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=db
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef.name`
// +kubebuilder:printcolumn:name="Database",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.status.database.sizeBytes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Database is the Schema for the databases API.
type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec   `json:"spec,omitempty"`
	Status DatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DatabaseList contains a list of Database.
type DatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Database `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Database{}, &DatabaseList{})
}
