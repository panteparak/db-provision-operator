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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cdbi
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.connection.host`
// +kubebuilder:printcolumn:name="Port",type=integer,JSONPath=`.spec.connection.port`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.version`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ClusterDatabaseInstance is the Schema for the clusterdatabaseinstances API.
// It represents a cluster-scoped database instance that can be referenced by
// resources in any namespace. Use this for shared database infrastructure
// managed by platform teams.
type ClusterDatabaseInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of the ClusterDatabaseInstance.
	// Uses the same spec as DatabaseInstance.
	Spec DatabaseInstanceSpec `json:"spec,omitempty"`

	// Status defines the observed state of the ClusterDatabaseInstance.
	// Uses the same status as DatabaseInstance.
	Status DatabaseInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterDatabaseInstanceList contains a list of ClusterDatabaseInstance.
type ClusterDatabaseInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterDatabaseInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterDatabaseInstance{}, &ClusterDatabaseInstanceList{})
}
