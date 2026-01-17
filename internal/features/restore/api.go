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

// Package restore provides the DatabaseRestore feature module for managing database restores.
package restore

import (
	"context"
	"time"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// API defines the public interface for the restore module.
type API interface {
	// Execute performs a database restore operation.
	Execute(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) (*Result, error)

	// ValidateSpec validates the restore specification.
	ValidateSpec(ctx context.Context, restore *dbopsv1alpha1.DatabaseRestore) error

	// CheckDeadline checks if the restore has exceeded its deadline.
	CheckDeadline(restore *dbopsv1alpha1.DatabaseRestore) (bool, string)

	// IsTerminal checks if the restore is in a terminal state (completed or failed).
	IsTerminal(restore *dbopsv1alpha1.DatabaseRestore) bool
}

// Result represents the result of a restore operation.
type Result struct {
	// TargetDatabase is the name of the restored database.
	TargetDatabase string

	// TargetInstance is the name of the target instance.
	TargetInstance string

	// SourceBackup is the name of the source backup (if using BackupRef).
	SourceBackup string

	// TablesRestored is the number of tables restored.
	TablesRestored int32

	// Duration is the time taken for the restore.
	Duration time.Duration

	// Warnings contains any warnings from the restore process.
	Warnings []string
}

// ValidationError represents a validation error for restore specification.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
