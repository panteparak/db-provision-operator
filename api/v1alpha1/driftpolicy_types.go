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

// Drift policy annotations
const (
	// AnnotationAllowDestructiveDrift allows destructive drift corrections when set to "true"
	AnnotationAllowDestructiveDrift = "dbops.dbprovision.io/allow-destructive-drift"

	// AnnotationAdoptDatabases contains comma-separated database names to adopt
	AnnotationAdoptDatabases = "dbops.dbprovision.io/adopt-databases"

	// AnnotationAdoptUsers contains comma-separated user names to adopt
	AnnotationAdoptUsers = "dbops.dbprovision.io/adopt-users"

	// AnnotationAdoptRoles contains comma-separated role names to adopt
	AnnotationAdoptRoles = "dbops.dbprovision.io/adopt-roles"

	// LabelAdopted indicates a resource was created via adoption
	LabelAdopted = "dbops.dbprovision.io/adopted"

	// LabelInstance indicates which instance owns this resource
	LabelInstance = "dbops.dbprovision.io/instance"
)

// DriftMode defines how drift is handled
// +kubebuilder:validation:Enum=ignore;detect;correct
type DriftMode string

const (
	// DriftModeIgnore disables drift detection entirely
	DriftModeIgnore DriftMode = "ignore"

	// DriftModeDetect detects drift and reports in status/events but does not auto-correct
	DriftModeDetect DriftMode = "detect"

	// DriftModeCorrect detects drift and automatically corrects it
	DriftModeCorrect DriftMode = "correct"
)

// DriftPolicy defines how drift detection and correction should be handled.
// This can be set at the instance level (default for all child resources)
// or overridden at the individual resource level.
type DriftPolicy struct {
	// Mode determines how drift is handled
	// +kubebuilder:validation:Enum=ignore;detect;correct
	// +kubebuilder:default=detect
	Mode DriftMode `json:"mode,omitempty"`

	// Interval specifies how often to check for drift (Go duration string)
	// This is only meaningful when mode is "detect" or "correct"
	// +kubebuilder:default="5m"
	// +optional
	Interval string `json:"interval,omitempty"`
}

// DriftStatus represents the current drift detection status for a resource.
type DriftStatus struct {
	// Detected indicates if drift was detected
	Detected bool `json:"detected,omitempty"`

	// LastChecked is when drift was last checked
	// +optional
	LastChecked *metav1.Time `json:"lastChecked,omitempty"`

	// Diffs contains the specific differences found
	// +optional
	Diffs []DriftDiff `json:"diffs,omitempty"`
}

// DriftDiff represents a single difference between desired and actual state.
type DriftDiff struct {
	// Field is the name of the field that differs
	Field string `json:"field"`

	// Expected is the expected value from the CR spec
	Expected string `json:"expected"`

	// Actual is the actual value in the database
	Actual string `json:"actual"`

	// Destructive indicates if correcting this drift would be destructive
	// +optional
	Destructive bool `json:"destructive,omitempty"`

	// Immutable indicates if this field cannot be changed after creation
	// +optional
	Immutable bool `json:"immutable,omitempty"`
}

// DiscoveryConfig defines configuration for resource discovery.
// When enabled, the operator will scan the database for resources
// that exist but are not managed by Kubernetes CRs.
type DiscoveryConfig struct {
	// Enabled enables resource discovery
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Interval specifies how often to scan for unmanaged resources (Go duration string)
	// +kubebuilder:default="30m"
	// +optional
	Interval string `json:"interval,omitempty"`
}

// DiscoveredResourcesStatus contains discovered unmanaged resources.
type DiscoveredResourcesStatus struct {
	// Databases contains discovered database resources
	// +optional
	Databases []DiscoveredResource `json:"databases,omitempty"`

	// Users contains discovered user resources
	// +optional
	Users []DiscoveredResource `json:"users,omitempty"`

	// Roles contains discovered role resources
	// +optional
	Roles []DiscoveredResource `json:"roles,omitempty"`

	// LastScan is when the last discovery scan was performed
	// +optional
	LastScan *metav1.Time `json:"lastScan,omitempty"`
}

// DiscoveredResource represents a resource found in the database
// that is not managed by a Kubernetes CR.
type DiscoveredResource struct {
	// Name is the name of the discovered resource
	Name string `json:"name"`

	// Discovered is when this resource was first discovered
	Discovered *metav1.Time `json:"discovered,omitempty"`

	// Adopted indicates if this resource has been adopted via annotation
	Adopted bool `json:"adopted,omitempty"`
}
