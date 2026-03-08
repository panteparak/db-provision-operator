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

// backupOperation tracks an active backup operation
type backupOperation struct {
	cancel    context.CancelFunc
	progress  int32
	startTime time.Time
	completed bool
	err       error
}

// Backup performs a ClickHouse database backup using native BACKUP SQL.
// Uses BACKUP DATABASE ... TO Disk(...) syntax (available since ClickHouse 22.8).
func (a *Adapter) Backup(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)

	// Track the operation
	op := &backupOperation{
		cancel:    cancel,
		startTime: time.Now(),
	}
	a.activeOps.Store(opts.BackupID, op)
	defer func() {
		op.completed = true
		go func() {
			time.Sleep(5 * time.Minute)
			a.activeOps.Delete(opts.BackupID)
		}()
	}()

	// Build backup name
	backupName := opts.BackupName
	if backupName == "" {
		backupName = opts.BackupID
	}

	diskName := opts.DiskName
	if diskName == "" {
		diskName = "backups"
	}

	database := opts.Database
	if database == "" {
		op.err = fmt.Errorf("database name is required for backup")
		return nil, op.err
	}

	// Build BACKUP SQL
	// BACKUP DATABASE `db` TO Disk('diskName', 'backupName')
	query := fmt.Sprintf("BACKUP DATABASE %s TO Disk(%s, %s)",
		escapeIdentifier(database),
		escapeLiteral(diskName),
		escapeLiteral(backupName))

	// Add base backup for incremental backups
	if opts.BaseBackup != "" {
		query += fmt.Sprintf(" SETTINGS base_backup = Disk(%s, %s)",
			escapeLiteral(diskName),
			escapeLiteral(opts.BaseBackup))
	}

	atomic.StoreInt32(&op.progress, 10)

	// Execute the backup (this is a synchronous operation in ClickHouse)
	_, err = db.ExecContext(ctx, query)
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("backup failed for database %s: %w", database, err)
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	result := &types.BackupResult{
		BackupID:  opts.BackupID,
		Path:      fmt.Sprintf("Disk('%s', '%s')", diskName, backupName),
		Format:    "native",
		StartTime: op.startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	}

	return result, nil
}

// GetBackupProgress returns the progress of a backup operation
func (a *Adapter) GetBackupProgress(_ context.Context, backupID string) (int, error) {
	val, ok := a.activeOps.Load(backupID)
	if !ok {
		return 0, fmt.Errorf("backup operation %s not found", backupID)
	}

	op := val.(*backupOperation)
	return int(atomic.LoadInt32(&op.progress)), nil
}

// CancelBackup cancels a running backup operation
func (a *Adapter) CancelBackup(_ context.Context, backupID string) error {
	val, ok := a.activeOps.Load(backupID)
	if !ok {
		return fmt.Errorf("backup operation %s not found", backupID)
	}

	op := val.(*backupOperation)
	if op.completed {
		return fmt.Errorf("backup operation %s already completed", backupID)
	}

	op.cancel()
	return nil
}
