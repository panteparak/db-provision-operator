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

package util

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types for database resources
const (
	// ConditionTypeReady indicates whether the resource is ready
	ConditionTypeReady = "Ready"

	// ConditionTypeConnected indicates whether the database connection is established
	ConditionTypeConnected = "Connected"

	// ConditionTypeHealthy indicates whether the resource is healthy
	ConditionTypeHealthy = "Healthy"

	// ConditionTypeSynced indicates whether the resource is in sync with the database
	ConditionTypeSynced = "Synced"

	// ConditionTypeProgressing indicates whether the resource is being processed
	ConditionTypeProgressing = "Progressing"

	// ConditionTypeDegraded indicates whether the resource is in a degraded state
	ConditionTypeDegraded = "Degraded"
)

// Condition reasons
const (
	ReasonReconciling        = "Reconciling"
	ReasonReconcileSuccess   = "ReconcileSuccess"
	ReasonReconcileFailed    = "ReconcileFailed"
	ReasonConnecting         = "Connecting"
	ReasonConnectionSuccess  = "ConnectionSuccess"
	ReasonConnectionFailed   = "ConnectionFailed"
	ReasonCreating           = "Creating"
	ReasonCreated            = "Created"
	ReasonCreateFailed       = "CreateFailed"
	ReasonUpdating           = "Updating"
	ReasonUpdated            = "Updated"
	ReasonUpdateFailed       = "UpdateFailed"
	ReasonDeleting           = "Deleting"
	ReasonDeleted            = "Deleted"
	ReasonDeleteFailed       = "DeleteFailed"
	ReasonHealthCheckPassed  = "HealthCheckPassed"
	ReasonHealthCheckFailed  = "HealthCheckFailed"
	ReasonSecretNotFound     = "SecretNotFound"
	ReasonSecretInvalid      = "SecretInvalid"
	ReasonInstanceNotFound   = "InstanceNotFound"
	ReasonInstanceNotReady   = "InstanceNotReady"
	ReasonDatabaseNotFound   = "DatabaseNotFound"
	ReasonDatabaseNotReady   = "DatabaseNotReady"
	ReasonBackupInProgress   = "BackupInProgress"
	ReasonBackupCompleted    = "BackupCompleted"
	ReasonBackupFailed       = "BackupFailed"
	ReasonRestoreInProgress  = "RestoreInProgress"
	ReasonRestoreCompleted   = "RestoreCompleted"
	ReasonRestoreFailed      = "RestoreFailed"
	ReasonDeletionProtected  = "DeletionProtected"
)

// SetCondition adds or updates a condition in the conditions list
func SetCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus, reason, message string) {
	now := metav1.NewTime(time.Now())

	// Find existing condition
	for i, c := range *conditions {
		if c.Type == conditionType {
			// Only update if status, reason, or message changed
			if c.Status != status || c.Reason != reason || c.Message != message {
				(*conditions)[i] = metav1.Condition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					ObservedGeneration: c.ObservedGeneration,
				}
			}
			return
		}
	}

	// Add new condition
	*conditions = append(*conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

// GetCondition returns a condition by type
func GetCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// IsConditionTrue checks if a condition is true
func IsConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	cond := GetCondition(conditions, conditionType)
	return cond != nil && cond.Status == metav1.ConditionTrue
}

// IsConditionFalse checks if a condition is false
func IsConditionFalse(conditions []metav1.Condition, conditionType string) bool {
	cond := GetCondition(conditions, conditionType)
	return cond != nil && cond.Status == metav1.ConditionFalse
}

// RemoveCondition removes a condition by type
func RemoveCondition(conditions *[]metav1.Condition, conditionType string) {
	var newConditions []metav1.Condition
	for _, c := range *conditions {
		if c.Type != conditionType {
			newConditions = append(newConditions, c)
		}
	}
	*conditions = newConditions
}

// SetReadyCondition is a helper to set the Ready condition
func SetReadyCondition(conditions *[]metav1.Condition, status metav1.ConditionStatus, reason, message string) {
	SetCondition(conditions, ConditionTypeReady, status, reason, message)
}

// SetConnectedCondition is a helper to set the Connected condition
func SetConnectedCondition(conditions *[]metav1.Condition, status metav1.ConditionStatus, reason, message string) {
	SetCondition(conditions, ConditionTypeConnected, status, reason, message)
}

// SetHealthyCondition is a helper to set the Healthy condition
func SetHealthyCondition(conditions *[]metav1.Condition, status metav1.ConditionStatus, reason, message string) {
	SetCondition(conditions, ConditionTypeHealthy, status, reason, message)
}

// SetSyncedCondition is a helper to set the Synced condition
func SetSyncedCondition(conditions *[]metav1.Condition, status metav1.ConditionStatus, reason, message string) {
	SetCondition(conditions, ConditionTypeSynced, status, reason, message)
}
