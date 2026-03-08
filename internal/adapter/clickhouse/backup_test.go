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
	"database/sql"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Backup Operations", func() {
	var (
		adapter *Adapter
		ctx     context.Context
		mockDB  *sql.DB
		mock    sqlmock.Sqlmock
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		mockDB, mock, err = sqlmock.New()
		Expect(err).NotTo(HaveOccurred())

		adapter = NewAdapter(defaultConfig())
		adapter.db = mockDB
	})

	AfterEach(func() {
		if mockDB != nil {
			_ = mockDB.Close()
		}
	})

	Describe("Backup", func() {
		It("executes native BACKUP SQL", func() {
			mock.ExpectExec("BACKUP DATABASE `testdb` TO Disk\\('backups', 'backup-001'\\)").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Backup(ctx, types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-001",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.BackupID).To(Equal("backup-001"))
			Expect(result.Format).To(Equal("native"))
			Expect(result.Path).To(ContainSubstring("backups"))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("uses custom disk name", func() {
			mock.ExpectExec("BACKUP DATABASE `testdb` TO Disk\\('custom_disk', 'backup-002'\\)").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Backup(ctx, types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-002",
				DiskName: "custom_disk",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Path).To(ContainSubstring("custom_disk"))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("uses custom backup name", func() {
			mock.ExpectExec("BACKUP DATABASE `testdb` TO Disk\\('backups', 'my-snapshot'\\)").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Backup(ctx, types.BackupOptions{
				Database:   "testdb",
				BackupID:   "backup-003",
				BackupName: "my-snapshot",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("supports incremental backups with base_backup", func() {
			mock.ExpectExec("BACKUP DATABASE `testdb` TO Disk\\('backups', 'backup-inc'\\) SETTINGS base_backup").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Backup(ctx, types.BackupOptions{
				Database:   "testdb",
				BackupID:   "backup-inc",
				BaseBackup: "backup-001",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error when database is empty", func() {
			result, err := adapter.Backup(ctx, types.BackupOptions{
				BackupID: "backup-empty",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database name is required"))
			Expect(result).To(BeNil())
		})

		It("returns error when not connected", func() {
			disconnectedAdapter := NewAdapter(defaultConfig())

			result, err := disconnectedAdapter.Backup(ctx, types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-err",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not connected"))
			Expect(result).To(BeNil())
		})

		It("returns error when backup fails", func() {
			mock.ExpectExec("BACKUP DATABASE `testdb` TO Disk").
				WillReturnError(fmt.Errorf("disk not configured"))

			result, err := adapter.Backup(ctx, types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-fail",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup failed"))
			Expect(result).To(BeNil())
		})

		It("sets start and end time in result", func() {
			mock.ExpectExec("BACKUP DATABASE `testdb` TO Disk").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Backup(ctx, types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-time",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.StartTime).NotTo(BeEmpty())
			Expect(result.EndTime).NotTo(BeEmpty())
		})
	})

	Describe("GetBackupProgress", func() {
		It("returns error when not found", func() {
			progress, err := adapter.GetBackupProgress(ctx, "unknown-backup-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup operation unknown-backup-id not found"))
			Expect(progress).To(Equal(0))
		})
	})

	Describe("CancelBackup", func() {
		It("returns error when not found", func() {
			err := adapter.CancelBackup(ctx, "unknown-backup-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup operation unknown-backup-id not found"))
		})

		It("returns error for completed backup", func() {
			op := &backupOperation{completed: true}
			adapter.activeOps.Store("completed-backup", op)

			err := adapter.CancelBackup(ctx, "completed-backup")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))

			adapter.activeOps.Delete("completed-backup")
		})
	})

	Describe("Active operation tracking", func() {
		It("stores and retrieves backup operations", func() {
			op := &backupOperation{
				completed: false,
				progress:  50,
			}
			adapter.activeOps.Store("active-backup", op)

			val, ok := adapter.activeOps.Load("active-backup")
			Expect(ok).To(BeTrue())
			retrievedOp := val.(*backupOperation)
			Expect(retrievedOp.completed).To(BeFalse())
			Expect(retrievedOp.progress).To(Equal(int32(50)))

			adapter.activeOps.Delete("active-backup")
		})
	})

	Describe("Restore Operations", func() {
		It("executes native RESTORE SQL", func() {
			mock.ExpectExec("RESTORE DATABASE `testdb` FROM Disk\\('backups', 'restore-001'\\)").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Restore(ctx, types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-001",
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RestoreID).To(Equal("restore-001"))
			Expect(result.TargetDatabase).To(Equal("testdb"))
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("drops existing database before restore when requested", func() {
			mock.ExpectExec("KILL QUERY WHERE current_database").
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("DROP DATABASE IF EXISTS `testdb`").
				WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectExec("RESTORE DATABASE `testdb` FROM Disk").
				WillReturnResult(sqlmock.NewResult(0, 0))

			result, err := adapter.Restore(ctx, types.RestoreOptions{
				Database:     "testdb",
				RestoreID:    "restore-drop",
				DropExisting: true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
		})

		It("returns error when database is empty", func() {
			result, err := adapter.Restore(ctx, types.RestoreOptions{
				RestoreID: "restore-empty",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("database name is required"))
			Expect(result).To(BeNil())
		})

		It("returns error when restore fails", func() {
			mock.ExpectExec("RESTORE DATABASE `testdb` FROM Disk").
				WillReturnError(fmt.Errorf("backup not found"))

			result, err := adapter.Restore(ctx, types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-fail",
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restore failed"))
			Expect(result).To(BeNil())
		})
	})

	Describe("GetRestoreProgress", func() {
		It("returns error when not found", func() {
			progress, err := adapter.GetRestoreProgress(ctx, "unknown-restore-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restore operation unknown-restore-id not found"))
			Expect(progress).To(Equal(0))
		})
	})

	Describe("CancelRestore", func() {
		It("returns error when not found", func() {
			err := adapter.CancelRestore(ctx, "unknown-restore-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restore operation unknown-restore-id not found"))
		})

		It("returns error for completed restore", func() {
			op := &restoreOperation{completed: true}
			adapter.activeOps.Store("completed-restore", op)

			err := adapter.CancelRestore(ctx, "completed-restore")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))

			adapter.activeOps.Delete("completed-restore")
		})
	})
})
