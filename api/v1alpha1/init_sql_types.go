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

// InitSQLFailurePolicy defines what happens when init SQL execution fails.
// +kubebuilder:validation:Enum=Continue;Block
type InitSQLFailurePolicy string

const (
	// InitSQLFailurePolicyContinue allows the database to reach Ready with a Synced=False condition.
	InitSQLFailurePolicyContinue InitSQLFailurePolicy = "Continue"

	// InitSQLFailurePolicyBlock keeps the database in Failed phase and requeues.
	InitSQLFailurePolicyBlock InitSQLFailurePolicy = "Block"
)

// InitSQLConfig defines SQL statements to execute once after database creation.
// Exactly one of inline, configMapRef, or secretRef must be specified.
// +kubebuilder:validation:XValidation:rule="(has(self.inline) ? 1 : 0) + (has(self.configMapRef) ? 1 : 0) + (has(self.secretRef) ? 1 : 0) == 1",message="exactly one of inline, configMapRef, or secretRef must be specified"
type InitSQLConfig struct {
	// Inline SQL statements to execute in order.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	Inline []string `json:"inline,omitempty"`

	// ConfigMapRef references a ConfigMap key containing SQL statements separated by '---'.
	// +optional
	ConfigMapRef *ConfigMapKeySelector `json:"configMapRef,omitempty"`

	// SecretRef references a Secret key containing SQL statements separated by '---'.
	// +optional
	SecretRef *SecretKeySelector `json:"secretRef,omitempty"`

	// FailurePolicy controls behavior when init SQL execution fails.
	// Block (default): database stays in Failed phase and requeues.
	// Continue: database reaches Ready with a Synced=False condition.
	// +kubebuilder:validation:Enum=Continue;Block
	// +kubebuilder:default=Block
	// +optional
	FailurePolicy InitSQLFailurePolicy `json:"failurePolicy,omitempty"`
}

// InitSQLStatus tracks the execution state of init SQL.
type InitSQLStatus struct {
	// Applied indicates whether init SQL executed successfully.
	Applied bool `json:"applied"`

	// AppliedAt is when init SQL was last successfully executed.
	// +optional
	AppliedAt *metav1.Time `json:"appliedAt,omitempty"`

	// Hash is the SHA-256 of the resolved SQL content. Changes trigger re-execution.
	Hash string `json:"hash,omitempty"`

	// Error contains the last error message (empty on success).
	// +optional
	Error string `json:"error,omitempty"`

	// StatementsExecuted is the number of statements successfully executed.
	StatementsExecuted int32 `json:"statementsExecuted"`
}
