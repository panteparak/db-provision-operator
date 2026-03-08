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

package clickhouse

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/db-provision-operator/internal/adapter/types"
)

// restoreOperation tracks an active restore operation
type restoreOperation struct {
	cancel    context.CancelFunc
	progress  int32
	startTime time.Time
	completed bool
	err       error
	warnings  []string
}

// Restore performs a ClickHouse database restore using native RESTORE SQL.
// Uses RESTORE DATABASE ... FROM Disk(...) syntax (available since ClickHouse 22.8).
func (a *Adapter) Restore(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)

	// Track the operation
	op := &restoreOperation{
		cancel:    cancel,
		startTime: time.Now(),
	}
	a.activeOps.Store(opts.RestoreID, op)
	defer func() {
		op.completed = true
		go func() {
			time.Sleep(5 * time.Minute)
			a.activeOps.Delete(opts.RestoreID)
		}()
	}()

	diskName := opts.DiskName
	if diskName == "" {
		diskName = "backups"
	}

	database := opts.Database
	if database == "" {
		op.err = fmt.Errorf("database name is required for restore")
		return nil, op.err
	}

	// Drop existing database if requested
	if opts.DropExisting {
		// Kill queries first to avoid conflicts
		killQuery := fmt.Sprintf("KILL QUERY WHERE current_database = %s", escapeLiteral(database))
		_, _ = db.ExecContext(ctx, killQuery)

		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", escapeIdentifier(database))
		if _, err := db.ExecContext(ctx, dropQuery); err != nil {
			op.err = err
			return nil, fmt.Errorf("failed to drop database before restore: %w", err)
		}
	}

	atomic.StoreInt32(&op.progress, 10)

	// Build RESTORE SQL
	// RESTORE DATABASE `db` FROM Disk('diskName', 'restoreID')
	backupName := opts.RestoreID
	query := fmt.Sprintf("RESTORE DATABASE %s FROM Disk(%s, %s)",
		escapeIdentifier(database),
		escapeLiteral(diskName),
		escapeLiteral(backupName))

	// Execute the restore
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("restore failed for database %s: %w", database, err)
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	result := &types.RestoreResult{
		RestoreID:      opts.RestoreID,
		TargetDatabase: database,
		StartTime:      op.startTime.Format(time.RFC3339),
		EndTime:        endTime.Format(time.RFC3339),
		Warnings:       op.warnings,
	}

	return result, nil
}

// GetRestoreProgress returns the progress of a restore operation
func (a *Adapter) GetRestoreProgress(_ context.Context, restoreID string) (int, error) {
	val, ok := a.activeOps.Load(restoreID)
	if !ok {
		return 0, fmt.Errorf("restore operation %s not found", restoreID)
	}

	op := val.(*restoreOperation)
	return int(atomic.LoadInt32(&op.progress)), nil
}

// CancelRestore cancels a running restore operation
func (a *Adapter) CancelRestore(_ context.Context, restoreID string) error {
	val, ok := a.activeOps.Load(restoreID)
	if !ok {
		return fmt.Errorf("restore operation %s not found", restoreID)
	}

	op := val.(*restoreOperation)
	if op.completed {
		return fmt.Errorf("restore operation %s already completed", restoreID)
	}

	op.cancel()
	return nil
}
