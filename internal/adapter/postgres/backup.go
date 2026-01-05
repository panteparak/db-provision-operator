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

package postgres

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/db-provision-operator/internal/adapter/types"
)

// backupOperation tracks an active backup operation
type backupOperation struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	progress  int32
	startTime time.Time
	completed bool
	err       error
}

// Backup performs a PostgreSQL database backup using pg_dump
func (a *Adapter) Backup(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
	if opts.Writer == nil {
		return nil, fmt.Errorf("backup writer is required")
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
		// Keep in map for a while for progress queries
		go func() {
			time.Sleep(5 * time.Minute)
			a.activeOps.Delete(opts.BackupID)
		}()
	}()

	// Build pg_dump command
	args := a.buildPgDumpArgs(opts)

	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	op.cmd = cmd

	// Set up environment for password
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("PGPASSWORD=%s", a.config.Password))

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to start pg_dump: %w", err)
	}

	// Track bytes written for progress
	var bytesWritten int64

	// Copy output to writer
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if _, writeErr := opts.Writer.Write(buf[:n]); writeErr != nil {
					return
				}
				bytesWritten += int64(n)
				// Update progress estimate (assume ~100MB database for progress calc)
				progress := int32(float64(bytesWritten) / (100 * 1024 * 1024) * 100)
				if progress > 99 {
					progress = 99
				}
				atomic.StoreInt32(&op.progress, progress)
			}
			if err != nil {
				return
			}
		}
	}()

	// Capture stderr for error messages
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrOutput.WriteString(scanner.Text())
			stderrOutput.WriteString("\n")
		}
	}()

	// Wait for completion
	if err := cmd.Wait(); err != nil {
		op.err = err
		errMsg := stderrOutput.String()
		if errMsg != "" {
			return nil, fmt.Errorf("pg_dump failed: %s", errMsg)
		}
		return nil, fmt.Errorf("pg_dump failed: %w", err)
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	result := &types.BackupResult{
		BackupID:  opts.BackupID,
		SizeBytes: bytesWritten,
		Format:    opts.Format,
		StartTime: op.startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	}

	return result, nil
}

// buildPgDumpArgs builds the arguments for pg_dump
func (a *Adapter) buildPgDumpArgs(opts types.BackupOptions) []string {
	args := []string{
		"-h", a.config.Host,
		"-p", fmt.Sprintf("%d", a.config.Port),
		"-U", a.config.Username,
		"-d", opts.Database,
	}

	// Format
	switch opts.Format {
	case "custom", "c":
		args = append(args, "-Fc")
	case "directory", "d":
		args = append(args, "-Fd")
	case "tar", "t":
		args = append(args, "-Ft")
	default:
		args = append(args, "-Fp") // plain text (default)
	}

	// Parallel jobs (only for directory format)
	if opts.Jobs > 0 && (opts.Format == "directory" || opts.Format == "d") {
		args = append(args, "-j", fmt.Sprintf("%d", opts.Jobs))
	}

	// Data only
	if opts.DataOnly {
		args = append(args, "-a")
	}

	// Schema only
	if opts.SchemaOnly {
		args = append(args, "-s")
	}

	// Include blobs
	if opts.Blobs {
		args = append(args, "-b")
	}

	// No owner
	if opts.NoOwner {
		args = append(args, "-O")
	}

	// No privileges
	if opts.NoPrivileges {
		args = append(args, "-x")
	}

	// Specific schemas
	for _, schema := range opts.Schemas {
		args = append(args, "-n", schema)
	}

	// Exclude schemas
	for _, schema := range opts.ExcludeSchemas {
		args = append(args, "-N", schema)
	}

	// Specific tables
	for _, table := range opts.Tables {
		args = append(args, "-t", table)
	}

	// Exclude tables
	for _, table := range opts.ExcludeTables {
		args = append(args, "-T", table)
	}

	// Lock wait timeout
	if opts.LockWaitTimeout != "" {
		args = append(args, "--lock-wait-timeout="+opts.LockWaitTimeout)
	}

	// Verbose output
	args = append(args, "-v")

	return args
}

// GetBackupProgress returns the progress of a backup operation
func (a *Adapter) GetBackupProgress(ctx context.Context, backupID string) (int, error) {
	val, ok := a.activeOps.Load(backupID)
	if !ok {
		return 0, fmt.Errorf("backup operation %s not found", backupID)
	}

	op := val.(*backupOperation)
	return int(atomic.LoadInt32(&op.progress)), nil
}

// CancelBackup cancels a running backup operation
func (a *Adapter) CancelBackup(ctx context.Context, backupID string) error {
	val, ok := a.activeOps.Load(backupID)
	if !ok {
		return fmt.Errorf("backup operation %s not found", backupID)
	}

	op := val.(*backupOperation)
	if op.completed {
		return fmt.Errorf("backup operation %s already completed", backupID)
	}

	op.cancel()

	// Wait briefly for cleanup
	time.Sleep(100 * time.Millisecond)

	if op.cmd != nil && op.cmd.Process != nil {
		_ = op.cmd.Process.Kill()
	}

	return nil
}
