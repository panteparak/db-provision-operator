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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Finalizer names
const (
	FinalizerDatabaseInstance       = "dbops.dbprovision.io/instance-protection"
	FinalizerDatabase               = "dbops.dbprovision.io/database-protection"
	FinalizerDatabaseUser           = "dbops.dbprovision.io/user-protection"
	FinalizerDatabaseRole           = "dbops.dbprovision.io/role-protection"
	FinalizerDatabaseGrant          = "dbops.dbprovision.io/grant-protection"
	FinalizerDatabaseBackup         = "dbops.dbprovision.io/backup-protection"
	FinalizerDatabaseRestore        = "dbops.dbprovision.io/restore-protection"
	FinalizerDatabaseBackupSchedule = "dbops.dbprovision.io/schedule-protection"
)

// Annotation keys
const (
	// AnnotationForceDelete allows bypassing deletion protection
	AnnotationForceDelete = "dbops.dbprovision.io/force-delete"

	// AnnotationSkipReconcile temporarily skips reconciliation
	AnnotationSkipReconcile = "dbops.dbprovision.io/skip-reconcile"

	// AnnotationPauseReconcile pauses reconciliation
	AnnotationPauseReconcile = "dbops.dbprovision.io/pause-reconcile"
)

// AddFinalizer adds a finalizer to an object if it doesn't exist
func AddFinalizer(obj client.Object, finalizer string) bool {
	return controllerutil.AddFinalizer(obj, finalizer)
}

// RemoveFinalizer removes a finalizer from an object
func RemoveFinalizer(obj client.Object, finalizer string) bool {
	return controllerutil.RemoveFinalizer(obj, finalizer)
}

// HasFinalizer checks if an object has a specific finalizer
func HasFinalizer(obj client.Object, finalizer string) bool {
	return controllerutil.ContainsFinalizer(obj, finalizer)
}

// IsMarkedForDeletion checks if an object is marked for deletion
func IsMarkedForDeletion(obj client.Object) bool {
	return !obj.GetDeletionTimestamp().IsZero()
}

// HasForceDeleteAnnotation checks if the force-delete annotation is set to "true"
func HasForceDeleteAnnotation(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationForceDelete] == "true"
}

// ShouldSkipReconcile checks if reconciliation should be skipped
func ShouldSkipReconcile(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}
	return annotations[AnnotationSkipReconcile] == "true" ||
		annotations[AnnotationPauseReconcile] == "true"
}
