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

// restoreOperation tracks an active restore operation
type restoreOperation struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	progress  int32
	startTime time.Time
	completed bool
	err       error
	warnings  []string
}

// Restore performs a PostgreSQL database restore using pg_restore or psql
func (a *Adapter) Restore(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
	if opts.Reader == nil {
		return nil, fmt.Errorf("restore reader is required")
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
		pool, err := a.getPool()
		if err != nil {
			return nil, err
		}

		// Terminate existing connections
		terminateQuery := `
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()`
		_, _ = pool.Exec(ctx, terminateQuery, opts.Database)

		// Drop database
		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", escapeIdentifier(opts.Database))
		if _, err := pool.Exec(ctx, dropQuery); err != nil {
			op.err = err
			return nil, fmt.Errorf("failed to drop database: %w", err)
		}
	}

	// Create database if requested
	if opts.CreateDatabase {
		pool, err := a.getPool()
		if err != nil {
			return nil, err
		}

		createQuery := fmt.Sprintf("CREATE DATABASE %s", escapeIdentifier(opts.Database))
		if _, err := pool.Exec(ctx, createQuery); err != nil {
			// Ignore if database already exists
			if !strings.Contains(err.Error(), "already exists") {
				op.err = err
				return nil, fmt.Errorf("failed to create database: %w", err)
			}
		}
	}

	// Use pg_restore for custom/directory/tar format, psql for plain SQL
	// For now, we'll use pg_restore which auto-detects format
	args := a.buildPgRestoreArgs(opts)

	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	op.cmd = cmd

	// Set up environment for password
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("PGPASSWORD=%s", a.config.Password))

	// Connect stdin to reader
	cmd.Stdin = opts.Reader

	stderr, err := cmd.StderrPipe()
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		// If pg_restore fails, try with psql (for plain SQL dumps)
		return a.restoreWithPsql(ctx, opts, op)
	}

	// Capture stderr for warnings and errors
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		lineCount := 0
		for scanner.Scan() {
			line := scanner.Text()
			stderrOutput.WriteString(line)
			stderrOutput.WriteString("\n")

			// Track progress based on output lines
			lineCount++
			progress := int32(lineCount / 10) // rough estimate
			if progress > 99 {
				progress = 99
			}
			atomic.StoreInt32(&op.progress, progress)

			// Capture warnings
			if strings.Contains(strings.ToLower(line), "warning") {
				op.warnings = append(op.warnings, line)
			}
		}
	}()

	// Wait for completion
	if err := cmd.Wait(); err != nil {
		op.err = err
		// Check if it's just warnings
		errMsg := stderrOutput.String()
		if !strings.Contains(strings.ToLower(errMsg), "error") {
			// Just warnings, not errors
			atomic.StoreInt32(&op.progress, 100)
		} else {
			return nil, fmt.Errorf("pg_restore failed: %s", errMsg)
		}
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	// Analyze if requested
	if opts.Analyze {
		if err := a.analyzeDatabase(ctx, opts.Database); err != nil {
			op.warnings = append(op.warnings, fmt.Sprintf("ANALYZE failed: %v", err))
		}
	}

	result := &types.RestoreResult{
		RestoreID:      opts.RestoreID,
		TargetDatabase: opts.Database,
		StartTime:      op.startTime.Format(time.RFC3339),
		EndTime:        endTime.Format(time.RFC3339),
		Warnings:       op.warnings,
	}

	return result, nil
}

// buildPgRestoreArgs builds the arguments for pg_restore
func (a *Adapter) buildPgRestoreArgs(opts types.RestoreOptions) []string {
	args := []string{
		"-h", a.config.Host,
		"-p", fmt.Sprintf("%d", a.config.Port),
		"-U", a.config.Username,
		"-d", opts.Database,
	}

	// Parallel jobs
	if opts.Jobs > 0 {
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

	// No owner
	if opts.NoOwner {
		args = append(args, "-O")
	}

	// No privileges
	if opts.NoPrivileges {
		args = append(args, "-x")
	}

	// Disable triggers (useful for data-only restores)
	if opts.DisableTriggers {
		args = append(args, "--disable-triggers")
	}

	// Specific schemas
	for _, schema := range opts.Schemas {
		args = append(args, "-n", schema)
	}

	// Specific tables
	for _, table := range opts.Tables {
		args = append(args, "-t", table)
	}

	// Role mapping
	for from, to := range opts.RoleMapping {
		args = append(args, fmt.Sprintf("--role=%s:%s", from, to))
	}

	// Verbose output
	args = append(args, "-v")

	return args
}

// restoreWithPsql restores a plain SQL dump using psql
func (a *Adapter) restoreWithPsql(ctx context.Context, opts types.RestoreOptions, op *restoreOperation) (*types.RestoreResult, error) {
	args := []string{
		"-h", a.config.Host,
		"-p", fmt.Sprintf("%d", a.config.Port),
		"-U", a.config.Username,
		"-d", opts.Database,
		"-v", // verbose
		"-X", // no .psqlrc
	}

	// Single transaction
	if !opts.DataOnly {
		args = append(args, "--single-transaction")
	}

	cmd := exec.CommandContext(ctx, "psql", args...)
	op.cmd = cmd

	cmd.Env = append(cmd.Environ(), fmt.Sprintf("PGPASSWORD=%s", a.config.Password))
	cmd.Stdin = opts.Reader

	stderr, err := cmd.StderrPipe()
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to start psql: %w", err)
	}

	// Capture stderr
	var stderrOutput strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrOutput.WriteString(line)
			stderrOutput.WriteString("\n")

			if strings.Contains(strings.ToLower(line), "warning") {
				op.warnings = append(op.warnings, line)
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		op.err = err
		errMsg := stderrOutput.String()
		if strings.Contains(strings.ToLower(errMsg), "error") {
			return nil, fmt.Errorf("psql restore failed: %s", errMsg)
		}
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	// Analyze if requested
	if opts.Analyze {
		if err := a.analyzeDatabase(ctx, opts.Database); err != nil {
			op.warnings = append(op.warnings, fmt.Sprintf("ANALYZE failed: %v", err))
		}
	}

	result := &types.RestoreResult{
		RestoreID:      opts.RestoreID,
		TargetDatabase: opts.Database,
		StartTime:      op.startTime.Format(time.RFC3339),
		EndTime:        endTime.Format(time.RFC3339),
		Warnings:       op.warnings,
	}

	return result, nil
}

// analyzeDatabase runs ANALYZE on a database
func (a *Adapter) analyzeDatabase(ctx context.Context, database string) error {
	return a.execWithNewConnection(ctx, database, "ANALYZE")
}

// GetRestoreProgress returns the progress of a restore operation
func (a *Adapter) GetRestoreProgress(ctx context.Context, restoreID string) (int, error) {
	val, ok := a.activeOps.Load(restoreID)
	if !ok {
		return 0, fmt.Errorf("restore operation %s not found", restoreID)
	}

	op := val.(*restoreOperation)
	return int(atomic.LoadInt32(&op.progress)), nil
}

// CancelRestore cancels a running restore operation
func (a *Adapter) CancelRestore(ctx context.Context, restoreID string) error {
	val, ok := a.activeOps.Load(restoreID)
	if !ok {
		return fmt.Errorf("restore operation %s not found", restoreID)
	}

	op := val.(*restoreOperation)
	if op.completed {
		return fmt.Errorf("restore operation %s already completed", restoreID)
	}

	op.cancel()

	time.Sleep(100 * time.Millisecond)

	if op.cmd != nil && op.cmd.Process != nil {
		_ = op.cmd.Process.Kill()
	}

	return nil
}
