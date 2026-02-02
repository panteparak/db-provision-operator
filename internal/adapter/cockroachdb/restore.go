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

package cockroachdb

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/db-provision-operator/internal/adapter/types"
)

// restoreOperation tracks an active CockroachDB restore operation.
// Like backup, CockroachDB RESTORE runs as a distributed SQL job.
type restoreOperation struct {
	jobID     int64
	cancel    context.CancelFunc
	progress  int32
	startTime time.Time
	completed bool
	err       error
	warnings  []string
}

// Restore performs a CockroachDB database restore using the native RESTORE command.
//
// CockroachDB's RESTORE is fundamentally different from PostgreSQL's pg_restore:
//   - It's a SQL command, not an external tool
//   - It reads from a storage location (nodelocal, S3, GCS, Azure), not stdin
//   - It runs as a distributed job across the cluster
//   - It can restore to a point in time (AS OF SYSTEM TIME)
//
// The RestoreOptions.RestoreID is used to identify the backup source path.
func (a *Adapter) Restore(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
	pool, err := a.getPool()
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

	// Drop existing database if requested
	if opts.DropExisting {
		if dropErr := a.DropDatabase(ctx, opts.Database, types.DropDatabaseOptions{Force: true}); dropErr != nil {
			// Only fail if the database actually exists and we can't drop it
			if !strings.Contains(dropErr.Error(), "does not exist") {
				op.err = dropErr
				return nil, fmt.Errorf("failed to drop existing database: %w", dropErr)
			}
		}
	}

	// Build the RESTORE statement
	restoreSQL := a.buildRestoreSQL(opts)

	// Execute RESTORE and get the job ID
	var jobID int64
	var status string
	var fractionCompleted float64
	var rows int64
	var indexEntries int64
	var bytesRead int64

	err = pool.QueryRow(ctx, restoreSQL).Scan(
		&jobID, &status, &fractionCompleted, &rows, &indexEntries, &bytesRead,
	)
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to execute RESTORE: %w", err)
	}

	op.jobID = jobID
	atomic.StoreInt32(&op.progress, 100)

	endTime := time.Now()

	// Run ANALYZE if requested
	if opts.Analyze {
		if analyzeErr := a.analyzeDatabase(ctx, opts.Database); analyzeErr != nil {
			op.warnings = append(op.warnings, fmt.Sprintf("ANALYZE failed: %v", analyzeErr))
		}
	}

	result := &types.RestoreResult{
		RestoreID:      opts.RestoreID,
		TargetDatabase: opts.Database,
		TablesRestored: int32(rows),
		StartTime:      op.startTime.Format(time.RFC3339),
		EndTime:        endTime.Format(time.RFC3339),
		Warnings:       op.warnings,
	}

	return result, nil
}

// buildRestoreSQL constructs the CockroachDB RESTORE SQL statement.
//
// Syntax: RESTORE DATABASE <name> FROM '<source>' [WITH options...]
//
// Options supported:
//   - into_db: restore into a different database name
//   - skip_missing_foreign_keys: ignore missing FK references
//   - skip_missing_sequences: ignore missing sequences
//   - encryption_passphrase: decrypt an encrypted backup
func (a *Adapter) buildRestoreSQL(opts types.RestoreOptions) string {
	var sb strings.Builder

	sb.WriteString("RESTORE DATABASE ")
	sb.WriteString(escapeIdentifier(opts.Database))

	// Determine restore source
	source := fmt.Sprintf("nodelocal://1/backups/%s", opts.RestoreID)
	sb.WriteString(fmt.Sprintf(" FROM LATEST IN %s", escapeLiteral(source)))

	// Build WITH options
	var withOpts []string

	// Skip missing foreign keys if data-only restore
	if opts.DataOnly {
		withOpts = append(withOpts, "skip_missing_foreign_keys")
		withOpts = append(withOpts, "skip_missing_sequences")
	}

	if len(withOpts) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(withOpts, ", "))
	}

	return sb.String()
}

// analyzeDatabase runs table statistics collection on a CockroachDB database.
// CockroachDB uses CREATE STATISTICS instead of PostgreSQL's ANALYZE command,
// but also supports ANALYZE for compatibility.
func (a *Adapter) analyzeDatabase(ctx context.Context, database string) error {
	return a.execWithNewConnection(ctx, database, "ANALYZE")
}

// GetRestoreProgress returns the progress of a CockroachDB restore operation.
// Queries crdb_internal.jobs for the restore job status.
func (a *Adapter) GetRestoreProgress(ctx context.Context, restoreID string) (int, error) {
	val, ok := a.activeOps.Load(restoreID)
	if !ok {
		return 0, fmt.Errorf("restore operation %s not found", restoreID)
	}

	op := val.(*restoreOperation)

	// If we have a job ID and it's not completed, query the job status
	if op.jobID > 0 && !op.completed {
		pool, err := a.getPool()
		if err != nil {
			return int(atomic.LoadInt32(&op.progress)), nil
		}

		var fractionCompleted float64
		err = pool.QueryRow(ctx,
			"SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = $1",
			op.jobID).Scan(&fractionCompleted)
		if err == nil {
			progress := int32(fractionCompleted * 100)
			atomic.StoreInt32(&op.progress, progress)
		}
	}

	return int(atomic.LoadInt32(&op.progress)), nil
}

// CancelRestore cancels a running CockroachDB restore operation.
// Uses CANCEL JOB to stop the distributed restore job.
func (a *Adapter) CancelRestore(ctx context.Context, restoreID string) error {
	val, ok := a.activeOps.Load(restoreID)
	if !ok {
		return fmt.Errorf("restore operation %s not found", restoreID)
	}

	op := val.(*restoreOperation)
	if op.completed {
		return fmt.Errorf("restore operation %s already completed", restoreID)
	}

	// Cancel the context
	op.cancel()

	// Also cancel the CockroachDB job if we have a job ID
	if op.jobID > 0 {
		pool, err := a.getPool()
		if err == nil {
			_, _ = pool.Exec(ctx, "CANCEL JOB $1", op.jobID)
		}
	}

	return nil
}
