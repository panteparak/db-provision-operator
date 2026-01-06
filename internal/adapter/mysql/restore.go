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

// Restore performs a MySQL database restore using mysql client
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

	db, err := a.getDB()
	if err != nil {
		return nil, err
	}

	// Drop existing database if requested
	if opts.DropExisting {
		dropQuery := fmt.Sprintf("DROP DATABASE IF EXISTS %s", escapeIdentifier(opts.Database))
		if _, err := db.ExecContext(ctx, dropQuery); err != nil {
			op.err = err
			return nil, fmt.Errorf("failed to drop database: %w", err)
		}
	}

	// Create database if requested
	if opts.CreateDatabase {
		createQuery := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", escapeIdentifier(opts.Database))
		if _, err := db.ExecContext(ctx, createQuery); err != nil {
			op.err = err
			return nil, fmt.Errorf("failed to create database: %w", err)
		}
	}

	// Build mysql command
	args := a.buildMysqlRestoreArgs(opts)

	cmd := exec.CommandContext(ctx, "mysql", args...)
	op.cmd = cmd

	// Set up environment for password
	cmd.Env = append(cmd.Environ(), fmt.Sprintf("MYSQL_PWD=%s", a.config.Password))

	// Connect stdin to reader
	cmd.Stdin = opts.Reader

	stderr, err := cmd.StderrPipe()
	if err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		op.err = err
		return nil, fmt.Errorf("failed to start mysql: %w", err)
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

			lineCount++
			progress := int32(lineCount / 10)
			if progress > 99 {
				progress = 99
			}
			atomic.StoreInt32(&op.progress, progress)

			if strings.Contains(strings.ToLower(line), "warning") {
				op.warnings = append(op.warnings, line)
			}
		}
	}()

	// Wait for completion
	if err := cmd.Wait(); err != nil {
		op.err = err
		errMsg := stderrOutput.String()
		if strings.Contains(strings.ToLower(errMsg), "error") {
			return nil, fmt.Errorf("mysql restore failed: %s", errMsg)
		}
	}

	atomic.StoreInt32(&op.progress, 100)
	endTime := time.Now()

	// Analyze tables if requested
	if opts.Analyze {
		if err := a.analyzeTables(ctx, opts.Database); err != nil {
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

// buildMysqlRestoreArgs builds the arguments for mysql restore
func (a *Adapter) buildMysqlRestoreArgs(opts types.RestoreOptions) []string {
	args := []string{
		"-h", a.config.Host,
		"-P", fmt.Sprintf("%d", a.config.Port),
		"-u", a.config.Username,
	}

	// Target database
	if opts.Database != "" {
		args = append(args, "-D", opts.Database)
	}

	// Disable foreign key checks during restore
	if opts.DisableForeignKeyChecks {
		args = append(args, "-e", "SET FOREIGN_KEY_CHECKS=0;")
	}

	// Disable binary logging during restore
	if opts.DisableBinlog {
		args = append(args, "-e", "SET SQL_LOG_BIN=0;")
	}

	return args
}

// analyzeTables runs ANALYZE TABLE on all tables in a database
func (a *Adapter) analyzeTables(ctx context.Context, database string) error {
	db, err := a.getDB()
	if err != nil {
		return err
	}

	// Get all tables in the database
	rows, err := db.QueryContext(ctx,
		"SELECT table_name FROM information_schema.tables WHERE table_schema = ?",
		database)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return err
		}
		tables = append(tables, table)
	}

	// Analyze each table
	for _, table := range tables {
		query := fmt.Sprintf("ANALYZE TABLE %s.%s",
			escapeIdentifier(database),
			escapeIdentifier(table))
		_, _ = db.ExecContext(ctx, query)
	}

	return nil
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
