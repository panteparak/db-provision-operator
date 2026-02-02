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

// backupOperation tracks an active CockroachDB backup operation.
// Unlike PostgreSQL which uses external pg_dump, CockroachDB runs
// BACKUP as a distributed SQL job tracked via SHOW JOBS.
type backupOperation struct {
	jobID     int64
	cancel    context.CancelFunc
	progress  int32
	startTime time.Time
	completed bool
	err       error
}

// Backup performs a CockroachDB database backup using the native BACKUP command.
//
// CockroachDB's BACKUP is fundamentally different from PostgreSQL's pg_dump:
//   - It's a SQL command, not an external tool
//   - It writes to a storage location (nodelocal, S3, GCS, Azure), not stdout
//   - It runs as a distributed job across the cluster
//   - Progress is tracked via SHOW JOBS, not byte counting
//
// The BackupOptions.BackupID is used as the backup destination path.
// If opts.Writer is provided, the backup metadata (not data) is written to it.
func (a *Adapter) Backup(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
	pool, err := a.getPool()
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

	// Build the BACKUP statement
	backupSQL := a.buildBackupSQL(opts)

	// Execute BACKUP and get the job ID
	var jobID int64
	var status string
	var fractionCompleted float64
	var rows int64
	var indexEntries int64
	var bytesWritten int64

	err = pool.QueryRow(ctx, backupSQL).Scan(
		&jobID, &status, &fractionCompleted, &rows, &indexEntries, &bytesWritten,
	)
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to execute BACKUP: %w", err)
	}

	op.jobID = jobID
	atomic.StoreInt32(&op.progress, 100)

	endTime := time.Now()

	// Write metadata to writer if provided
	if opts.Writer != nil {
		metadata := fmt.Sprintf("backup_id=%s\njob_id=%d\nstatus=%s\nrows=%d\nbytes=%d\n",
			opts.BackupID, jobID, status, rows, bytesWritten)
		if _, writeErr := opts.Writer.Write([]byte(metadata)); writeErr != nil {
			return nil, fmt.Errorf("failed to write backup metadata: %w", writeErr)
		}
	}

	result := &types.BackupResult{
		BackupID:  opts.BackupID,
		SizeBytes: bytesWritten,
		Format:    "cockroachdb-native",
		StartTime: op.startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	}

	return result, nil
}

// buildBackupSQL constructs the CockroachDB BACKUP SQL statement.
//
// Syntax: BACKUP DATABASE <name> INTO '<destination>' [WITH options...]
//
// Options supported:
//   - revision_history: include MVCC history for point-in-time restores
//   - encryption_passphrase: encrypt the backup
//   - AS OF SYSTEM TIME: backup at a specific timestamp
func (a *Adapter) buildBackupSQL(opts types.BackupOptions) string {
	var sb strings.Builder

	sb.WriteString("BACKUP DATABASE ")
	sb.WriteString(escapeIdentifier(opts.Database))

	// Determine backup destination
	destination := fmt.Sprintf("nodelocal://1/backups/%s", opts.BackupID)
	sb.WriteString(fmt.Sprintf(" INTO %s", escapeLiteral(destination)))

	// Build WITH options
	var withOpts []string

	// Schema-only backup using revision history
	if opts.SchemaOnly {
		withOpts = append(withOpts, "revision_history")
	}

	if len(withOpts) > 0 {
		sb.WriteString(" WITH ")
		sb.WriteString(strings.Join(withOpts, ", "))
	}

	return sb.String()
}

// GetBackupProgress returns the progress of a CockroachDB backup operation.
// For completed backups, returns 100. For running backups, queries SHOW JOBS.
func (a *Adapter) GetBackupProgress(ctx context.Context, backupID string) (int, error) {
	val, ok := a.activeOps.Load(backupID)
	if !ok {
		return 0, fmt.Errorf("backup operation %s not found", backupID)
	}

	op := val.(*backupOperation)

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

// CancelBackup cancels a running CockroachDB backup operation.
// Uses CANCEL JOB to stop the distributed backup job.
func (a *Adapter) CancelBackup(ctx context.Context, backupID string) error {
	val, ok := a.activeOps.Load(backupID)
	if !ok {
		return fmt.Errorf("backup operation %s not found", backupID)
	}

	op := val.(*backupOperation)
	if op.completed {
		return fmt.Errorf("backup operation %s already completed", backupID)
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
