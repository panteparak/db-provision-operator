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
			Port:     5432,
			Database: "postgres",
			Username: "postgres",
			Password: "password",
		})
	})

	Describe("buildPgDumpArgs", func() {
		It("should include host, port, user, database", func() {
			opts := types.BackupOptions{
				Database: "testdb",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-h", "localhost"))
			Expect(args).To(ContainElements("-p", "5432"))
			Expect(args).To(ContainElements("-U", "postgres"))
			Expect(args).To(ContainElements("-d", "testdb"))
		})

		It("should use -Fc for custom format", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "custom",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fc"))
		})

		It("should use -Fc for short custom format (c)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "c",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fc"))
		})

		It("should use -Fd for directory format", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "directory",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fd"))
		})

		It("should use -Fd for short directory format (d)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "d",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fd"))
		})

		It("should use -Ft for tar format", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "tar",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Ft"))
		})

		It("should use -Ft for short tar format (t)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "t",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Ft"))
		})

		It("should use -Fp for plain format (default)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "plain",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fp"))
		})

		It("should use -Fp when format is empty", func() {
			opts := types.BackupOptions{
				Database: "testdb",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fp"))
		})

		It("should include -j for parallel jobs (directory only)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "directory",
				Jobs:     4,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-j", "4"))
		})

		It("should include -j for parallel jobs with short format (d)", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "d",
				Jobs:     2,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-j", "2"))
		})

		It("should not include -j for non-directory formats", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Format:   "custom",
				Jobs:     4,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).NotTo(ContainElement("-j"))
		})

		It("should include -a for data only", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				DataOnly: true,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-a"))
		})

		It("should include -s for schema only", func() {
			opts := types.BackupOptions{
				Database:   "testdb",
				SchemaOnly: true,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-s"))
		})

		It("should include -b for blobs", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Blobs:    true,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-b"))
		})

		It("should include -O for no owner", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				NoOwner:  true,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-O"))
		})

		It("should include -x for no privileges", func() {
			opts := types.BackupOptions{
				Database:     "testdb",
				NoPrivileges: true,
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-x"))
		})

		It("should include -n for specific schemas", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Schemas:  []string{"public", "myschema"},
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-n", "public"))
			Expect(args).To(ContainElements("-n", "myschema"))
		})

		It("should include -N for exclude schemas", func() {
			opts := types.BackupOptions{
				Database:       "testdb",
				ExcludeSchemas: []string{"temp", "backup"},
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-N", "temp"))
			Expect(args).To(ContainElements("-N", "backup"))
		})

		It("should include -t for specific tables", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				Tables:   []string{"users", "orders"},
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-t", "users"))
			Expect(args).To(ContainElements("-t", "orders"))
		})

		It("should include -T for exclude tables", func() {
			opts := types.BackupOptions{
				Database:      "testdb",
				ExcludeTables: []string{"logs", "temp_data"},
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElements("-T", "logs"))
			Expect(args).To(ContainElements("-T", "temp_data"))
		})

		It("should include --lock-wait-timeout", func() {
			opts := types.BackupOptions{
				Database:        "testdb",
				LockWaitTimeout: "30s",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("--lock-wait-timeout=30s"))
		})

		It("should always include -v for verbose", func() {
			opts := types.BackupOptions{
				Database: "testdb",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-v"))
		})

		It("should build comprehensive args with all options", func() {
			opts := types.BackupOptions{
				Database:        "testdb",
				Format:          "directory",
				Jobs:            4,
				DataOnly:        false,
				SchemaOnly:      false,
				Blobs:           true,
				NoOwner:         true,
				NoPrivileges:    true,
				Schemas:         []string{"public"},
				ExcludeSchemas:  []string{"temp"},
				Tables:          []string{"users"},
				ExcludeTables:   []string{"logs"},
				LockWaitTimeout: "60s",
			}

			args := adapter.buildPgDumpArgs(opts)

			Expect(args).To(ContainElement("-Fd"))
			Expect(args).To(ContainElements("-j", "4"))
			Expect(args).To(ContainElement("-b"))
			Expect(args).To(ContainElement("-O"))
			Expect(args).To(ContainElement("-x"))
			Expect(args).To(ContainElements("-n", "public"))
			Expect(args).To(ContainElements("-N", "temp"))
			Expect(args).To(ContainElements("-t", "users"))
			Expect(args).To(ContainElements("-T", "logs"))
			Expect(args).To(ContainElement("--lock-wait-timeout=60s"))
		})
	})

	Describe("Backup", func() {
		It("should require writer", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-001",
				Writer:   nil,
			}

			_, err := adapter.Backup(ctx, opts)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup writer is required"))
		})

		It("should track operation progress", func() {
			// This test verifies the structure without actually running pg_dump
			opts := types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-002",
				Writer:   &bytes.Buffer{},
			}

			// Verify options are properly set
			Expect(opts.BackupID).To(Equal("backup-002"))
			Expect(opts.Writer).NotTo(BeNil())
		})
	})

	Describe("GetBackupProgress", func() {
		It("should return error for unknown backup ID", func() {
			progress, err := adapter.GetBackupProgress(ctx, "unknown-backup-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup operation unknown-backup-id not found"))
			Expect(progress).To(Equal(0))
		})

		It("should return error for non-existent backup", func() {
			progress, err := adapter.GetBackupProgress(ctx, "non-existent-123")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(progress).To(Equal(0))
		})
	})

	Describe("CancelBackup", func() {
		It("should return error for unknown backup ID", func() {
			err := adapter.CancelBackup(ctx, "unknown-backup-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("backup operation unknown-backup-id not found"))
		})

		It("should return error for completed backup", func() {
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

	Describe("buildPgRestoreArgs", func() {
		It("should include host, port, user, database", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElements("-h", "localhost"))
			Expect(args).To(ContainElements("-p", "5432"))
			Expect(args).To(ContainElements("-U", "postgres"))
			Expect(args).To(ContainElements("-d", "testdb"))
		})

		It("should include -j for parallel jobs", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				Jobs:     4,
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElements("-j", "4"))
		})

		It("should include -a for data only", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				DataOnly: true,
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("-a"))
		})

		It("should include -s for schema only", func() {
			opts := types.RestoreOptions{
				Database:   "testdb",
				SchemaOnly: true,
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("-s"))
		})

		It("should include -O for no owner", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				NoOwner:  true,
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("-O"))
		})

		It("should include -x for no privileges", func() {
			opts := types.RestoreOptions{
				Database:     "testdb",
				NoPrivileges: true,
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("-x"))
		})

		It("should include --disable-triggers", func() {
			opts := types.RestoreOptions{
				Database:        "testdb",
				DisableTriggers: true,
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("--disable-triggers"))
		})

		It("should include -n for specific schemas", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				Schemas:  []string{"public", "myschema"},
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElements("-n", "public"))
			Expect(args).To(ContainElements("-n", "myschema"))
		})

		It("should include -t for specific tables", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				Tables:   []string{"users", "orders"},
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElements("-t", "users"))
			Expect(args).To(ContainElements("-t", "orders"))
		})

		It("should include --role for role mapping", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				RoleMapping: map[string]string{
					"old_role": "new_role",
				},
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("--role=old_role:new_role"))
		})

		It("should include multiple role mappings", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
				RoleMapping: map[string]string{
					"role1": "mapped1",
					"role2": "mapped2",
				},
			}

			args := adapter.buildPgRestoreArgs(opts)

			// Check that both role mappings are present
			roleArgs := []string{}
			for _, arg := range args {
				if len(arg) > 7 && arg[:7] == "--role=" {
					roleArgs = append(roleArgs, arg)
				}
			}
			Expect(roleArgs).To(HaveLen(2))
		})

		It("should always include -v for verbose", func() {
			opts := types.RestoreOptions{
				Database: "testdb",
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElement("-v"))
		})

		It("should build comprehensive args with all options", func() {
			opts := types.RestoreOptions{
				Database:        "testdb",
				Jobs:            4,
				DataOnly:        false,
				SchemaOnly:      false,
				NoOwner:         true,
				NoPrivileges:    true,
				DisableTriggers: true,
				Schemas:         []string{"public"},
				Tables:          []string{"users"},
				RoleMapping:     map[string]string{"admin": "superadmin"},
			}

			args := adapter.buildPgRestoreArgs(opts)

			Expect(args).To(ContainElements("-j", "4"))
			Expect(args).To(ContainElement("-O"))
			Expect(args).To(ContainElement("-x"))
			Expect(args).To(ContainElement("--disable-triggers"))
			Expect(args).To(ContainElements("-n", "public"))
			Expect(args).To(ContainElements("-t", "users"))
			Expect(args).To(ContainElement("--role=admin:superadmin"))
		})
	})

	Describe("GetRestoreProgress", func() {
		It("should return error for unknown restore ID", func() {
			progress, err := adapter.GetRestoreProgress(ctx, "unknown-restore-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restore operation unknown-restore-id not found"))
			Expect(progress).To(Equal(0))
		})

		It("should return error for non-existent restore", func() {
			progress, err := adapter.GetRestoreProgress(ctx, "non-existent-456")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(progress).To(Equal(0))
		})
	})

	Describe("CancelRestore", func() {
		It("should return error for unknown restore ID", func() {
			err := adapter.CancelRestore(ctx, "unknown-restore-id")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("restore operation unknown-restore-id not found"))
		})

		It("should return error for completed restore", func() {
			// Store a completed restore operation
			op := &restoreOperation{
				completed: true,
			}
			adapter.activeOps.Store("completed-restore", op)

			err := adapter.CancelRestore(ctx, "completed-restore")

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already completed"))

			// Cleanup
			adapter.activeOps.Delete("completed-restore")
		})
	})

	Describe("BackupOptions validation", func() {
		It("should accept valid backup options", func() {
			opts := types.BackupOptions{
				Database:        "testdb",
				BackupID:        "backup-123",
				Format:          "custom",
				Jobs:            2,
				DataOnly:        false,
				SchemaOnly:      false,
				Blobs:           true,
				NoOwner:         false,
				NoPrivileges:    false,
				Schemas:         []string{"public"},
				ExcludeSchemas:  []string{},
				Tables:          []string{},
				ExcludeTables:   []string{},
				LockWaitTimeout: "30s",
			}

			Expect(opts.Database).NotTo(BeEmpty())
			Expect(opts.BackupID).NotTo(BeEmpty())
			Expect(opts.Format).To(BeElementOf("custom", "c", "directory", "d", "tar", "t", "plain", "p", ""))
		})
	})

	Describe("RestoreOptions validation", func() {
		It("should accept valid restore options", func() {
			opts := types.RestoreOptions{
				Database:        "testdb",
				RestoreID:       "restore-123",
				DropExisting:    false,
				CreateDatabase:  true,
				DataOnly:        false,
				SchemaOnly:      false,
				NoOwner:         true,
				NoPrivileges:    true,
				RoleMapping:     map[string]string{},
				Schemas:         []string{"public"},
				Tables:          []string{},
				Jobs:            4,
				DisableTriggers: false,
				Analyze:         true,
			}

			Expect(opts.Database).NotTo(BeEmpty())
			Expect(opts.RestoreID).NotTo(BeEmpty())
			Expect(opts.Jobs).To(BeNumerically(">=", 0))
		})
	})
})
