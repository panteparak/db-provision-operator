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

package mysql

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

// Backup performs a MySQL database backup using mysqldump
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
		go func() {
			time.Sleep(5 * time.Minute)
			a.activeOps.Delete(opts.BackupID)
		}()
	}()

	// Build mysqldump command
	args := a.buildMysqldumpArgs(opts)

	cmd := exec.CommandContext(ctx, "mysqldump", args...)
	op.cmd = cmd

	// Set up environment for password
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("MYSQL_PWD=%s", a.config.Password))

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
		return nil, fmt.Errorf("failed to start mysqldump: %w", err)
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
				// Update progress estimate
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
			return nil, fmt.Errorf("mysqldump failed: %s", errMsg)
		}
		return nil, fmt.Errorf("mysqldump failed: %w", err)
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	result := &types.BackupResult{
		BackupID:  opts.BackupID,
		SizeBytes: bytesWritten,
		Format:    "sql",
		StartTime: op.startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	}

	return result, nil
}

// buildMysqldumpArgs builds the arguments for mysqldump
func (a *Adapter) buildMysqldumpArgs(opts types.BackupOptions) []string {
	args := []string{
		"-h", a.config.Host,
		"-P", fmt.Sprintf("%d", a.config.Port),
		"-u", a.config.Username,
	}

	// Single transaction for consistent backup
	if opts.SingleTransaction {
		args = append(args, "--single-transaction")
	}

	// Quick mode for large tables
	if opts.Quick {
		args = append(args, "--quick")
	}

	// Lock tables
	if opts.LockTables {
		args = append(args, "--lock-tables")
	} else {
		args = append(args, "--skip-lock-tables")
	}

	// Include routines (stored procedures and functions)
	if opts.Routines {
		args = append(args, "--routines")
	}

	// Include triggers
	if opts.Triggers {
		args = append(args, "--triggers")
	} else {
		args = append(args, "--skip-triggers")
	}

	// Include events
	if opts.Events {
		args = append(args, "--events")
	}

	// Extended insert for faster imports
	if opts.ExtendedInsert {
		args = append(args, "--extended-insert")
	} else {
		args = append(args, "--skip-extended-insert")
	}

	// GTID handling
	if opts.SetGtidPurged != "" {
		args = append(args, fmt.Sprintf("--set-gtid-purged=%s", opts.SetGtidPurged))
	}

	// Data only
	if opts.DataOnly {
		args = append(args, "--no-create-info")
	}

	// Schema only
	if opts.SchemaOnly {
		args = append(args, "--no-data")
	}

	// Add databases
	if len(opts.Databases) > 0 {
		args = append(args, "--databases")
		args = append(args, opts.Databases...)
	} else if opts.Database != "" {
		args = append(args, opts.Database)
	}

	// Specific tables
	if len(opts.Tables) > 0 {
		args = append(args, opts.Tables...)
	}

	// Exclude tables
	for _, table := range opts.ExcludeTables {
		args = append(args, fmt.Sprintf("--ignore-table=%s.%s", opts.Database, table))
	}

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

	time.Sleep(100 * time.Millisecond)

	if op.cmd != nil && op.cmd.Process != nil {
		_ = op.cmd.Process.Kill()
	}

	return nil
}
