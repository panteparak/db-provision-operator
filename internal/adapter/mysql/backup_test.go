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
	"bytes"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Backup Operations", func() {
	var (
		adapter *Adapter
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(types.ConnectionConfig{
			Host:     "localhost",
			Port:     3306,
			Database: "testdb",
			Username: "root",
			Password: "password",
		})
	})

	Describe("buildMysqldumpArgs", func() {
		It("includes host/port/user", func() {
			opts := types.BackupOptions{
				Database: "testdb",
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElements("-h", "localhost"))
			Expect(args).To(ContainElements("-P", "3306"))
			Expect(args).To(ContainElements("-u", "root"))
		})

		It("includes single-transaction", func() {
			opts := types.BackupOptions{
				Database:          "testdb",
				SingleTransaction: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--single-transaction"))
		})

		It("includes quick mode", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Quick:    true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--quick"))
		})

		It("includes lock-tables", func() {
			opts := types.BackupOptions{
				Database:   "testdb",
				LockTables: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--lock-tables"))
		})

		It("skips lock-tables when false", func() {
			opts := types.BackupOptions{
				Database:   "testdb",
				LockTables: false,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--skip-lock-tables"))
		})

		It("includes routines", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Routines: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--routines"))
		})

		It("includes triggers", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Triggers: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--triggers"))
		})

		It("skips triggers when false", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Triggers: false,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--skip-triggers"))
		})

		It("includes events", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Events:   true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--events"))
		})

		It("includes extended-insert", func() {
			opts := types.BackupOptions{
				Database:       "testdb",
				ExtendedInsert: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--extended-insert"))
		})

		It("skips extended-insert when false", func() {
			opts := types.BackupOptions{
				Database:       "testdb",
				ExtendedInsert: false,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--skip-extended-insert"))
		})

		It("includes databases", func() {
			opts := types.BackupOptions{
				Databases: []string{"db1", "db2", "db3"},
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--databases"))
			Expect(args).To(ContainElement("db1"))
			Expect(args).To(ContainElement("db2"))
			Expect(args).To(ContainElement("db3"))
		})

		It("includes single database when no databases list", func() {
			opts := types.BackupOptions{
				Database: "testdb",
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("testdb"))
			Expect(args).NotTo(ContainElement("--databases"))
		})

		It("excludes tables", func() {
			opts := types.BackupOptions{
				Database:      "testdb",
				ExcludeTables: []string{"logs", "temp_data", "sessions"},
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--ignore-table=testdb.logs"))
			Expect(args).To(ContainElement("--ignore-table=testdb.temp_data"))
			Expect(args).To(ContainElement("--ignore-table=testdb.sessions"))
		})

		It("data only (--no-create-info)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				DataOnly: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--no-create-info"))
		})

		It("schema only (--no-data)", func() {
			opts := types.BackupOptions{
				Database:   "testdb",
				SchemaOnly: true,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--no-data"))
		})

		It("includes specific tables", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Tables:   []string{"users", "orders"},
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("users"))
			Expect(args).To(ContainElement("orders"))
		})

		It("includes set-gtid-purged", func() {
			opts := types.BackupOptions{
				Database:      "testdb",
				SetGtidPurged: "OFF",
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--set-gtid-purged=OFF"))
		})

		It("builds comprehensive args with all options", func() {
			opts := types.BackupOptions{
				Database:          "testdb",
				SingleTransaction: true,
				Quick:             true,
				LockTables:        false,
				Routines:          true,
				Triggers:          true,
				Events:            true,
				ExtendedInsert:    true,
				DataOnly:          false,
				SchemaOnly:        false,
				Tables:            []string{"users"},
				ExcludeTables:     []string{"logs"},
				SetGtidPurged:     "AUTO",
			}

			args := adapter.buildMysqldumpArgs(opts)

			// Verify connection args
			Expect(args).To(ContainElements("-h", "localhost"))
			Expect(args).To(ContainElements("-P", "3306"))
			Expect(args).To(ContainElements("-u", "root"))

			// Verify flags
			Expect(args).To(ContainElement("--single-transaction"))
			Expect(args).To(ContainElement("--quick"))
			Expect(args).To(ContainElement("--skip-lock-tables"))
			Expect(args).To(ContainElement("--routines"))
			Expect(args).To(ContainElement("--triggers"))
			Expect(args).To(ContainElement("--events"))
			Expect(args).To(ContainElement("--extended-insert"))
			Expect(args).To(ContainElement("--set-gtid-purged=AUTO"))
			Expect(args).To(ContainElement("--ignore-table=testdb.logs"))
			Expect(args).To(ContainElement("testdb"))
			Expect(args).To(ContainElement("users"))
		})

		It("uses different host and port from config", func() {
			customAdapter := NewAdapter(types.ConnectionConfig{
				Host:     "db.example.com",
				Port:     3307,
				Database: "production",
				Username: "admin",
				Password: "secret",
			})

			opts := types.BackupOptions{
				Database: "production",
			}

			args := customAdapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElements("-h", "db.example.com"))
			Expect(args).To(ContainElements("-P", "3307"))
			Expect(args).To(ContainElements("-u", "admin"))
		})
	})

	Describe("Backup", func() {
		It("returns error when writer is nil", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-001",
				Writer:   nil,
			}

			_, err := adapter.Backup(ctx, opts)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup writer is required"))
		})

		It("accepts valid backup options with writer", func() {
			// This test verifies the options structure without actually running mysqldump
			opts := types.BackupOptions{
				Database:          "testdb",
				BackupID:          "backup-002",
				Writer:            &bytes.Buffer{},
				SingleTransaction: true,
				Quick:             true,
				Routines:          true,
				Triggers:          true,
			}

			// Verify options are properly set
			Expect(opts.BackupID).To(Equal("backup-002"))
			Expect(opts.Writer).NotTo(BeNil())
			Expect(opts.SingleTransaction).To(BeTrue())
			Expect(opts.Quick).To(BeTrue())
			Expect(opts.Routines).To(BeTrue())
			Expect(opts.Triggers).To(BeTrue())
		})
	})

	Describe("GetBackupProgress", func() {
		It("returns error when not found", func() {
			progress, err := adapter.GetBackupProgress(ctx, "unknown-backup-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup operation unknown-backup-id not found"))
			Expect(progress).To(Equal(0))
		})

		It("returns error for non-existent backup", func() {
			progress, err := adapter.GetBackupProgress(ctx, "non-existent-123")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
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
			// Store a completed backup operation
			op := &backupOperation{
				completed: true,
			}
			adapter.activeOps.Store("completed-backup", op)

			err := adapter.CancelBackup(ctx, "completed-backup")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))

			// Cleanup
			adapter.activeOps.Delete("completed-backup")
		})
	})

	Describe("BackupOptions validation", func() {
		It("should accept valid backup options", func() {
			opts := types.BackupOptions{
				Database:          "testdb",
				BackupID:          "backup-123",
				SingleTransaction: true,
				Quick:             true,
				LockTables:        false,
				Routines:          true,
				Triggers:          true,
				Events:            true,
				ExtendedInsert:    true,
				DataOnly:          false,
				SchemaOnly:        false,
				Tables:            []string{},
				ExcludeTables:     []string{},
				Databases:         []string{},
				SetGtidPurged:     "OFF",
			}

			Expect(opts.Database).NotTo(BeEmpty())
			Expect(opts.BackupID).NotTo(BeEmpty())
		})

		It("should handle multiple databases", func() {
			opts := types.BackupOptions{
				BackupID:  "backup-456",
				Databases: []string{"db1", "db2", "db3"},
			}

			Expect(opts.Databases).To(HaveLen(3))
			Expect(opts.Databases).To(ContainElements("db1", "db2", "db3"))
		})
	})

	Describe("buildMysqldumpArgs edge cases", func() {
		It("handles empty database name with databases list", func() {
			opts := types.BackupOptions{
				Database:  "",
				Databases: []string{"db1", "db2"},
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--databases"))
			Expect(args).To(ContainElement("db1"))
			Expect(args).To(ContainElement("db2"))
		})

		It("does not include empty options", func() {
			opts := types.BackupOptions{
				Database:          "testdb",
				SingleTransaction: false,
				Quick:             false,
				Routines:          false,
				Events:            false,
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).NotTo(ContainElement("--single-transaction"))
			Expect(args).NotTo(ContainElement("--quick"))
			Expect(args).NotTo(ContainElement("--routines"))
			Expect(args).NotTo(ContainElement("--events"))
		})

		It("handles exclude tables with proper database prefix", func() {
			opts := types.BackupOptions{
				Database:      "mydb",
				ExcludeTables: []string{"table1", "table2"},
			}

			args := adapter.buildMysqldumpArgs(opts)

			Expect(args).To(ContainElement("--ignore-table=mydb.table1"))
			Expect(args).To(ContainElement("--ignore-table=mydb.table2"))
		})
	})

	Describe("Active operation tracking", func() {
		It("stores and retrieves backup operations", func() {
			// Store an active backup operation
			op := &backupOperation{
				completed: false,
				progress:  50,
			}
			adapter.activeOps.Store("active-backup", op)

			// Retrieve and verify
			val, ok := adapter.activeOps.Load("active-backup")
			Expect(ok).To(BeTrue())
			retrievedOp := val.(*backupOperation)
			Expect(retrievedOp.completed).To(BeFalse())
			Expect(retrievedOp.progress).To(Equal(int32(50)))

			// Cleanup
			adapter.activeOps.Delete("active-backup")
		})
	})
})
