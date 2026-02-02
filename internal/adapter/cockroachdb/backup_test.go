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
	"bytes"
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/db-provision-operator/internal/adapter/cockroachdb/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Backup Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(testutil.DefaultConnectionConfig())
	})

	Describe("Backup", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				result, err := adapter.Backup(ctx, types.BackupOptions{
					Database: "testdb",
					BackupID: "backup-001",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("buildBackupSQL", func() {
		It("should generate BACKUP DATABASE with nodelocal destination", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-001",
			}
			sql := buildCRDBBackupSQL(opts)
			Expect(sql).To(ContainSubstring("BACKUP DATABASE"))
			Expect(sql).To(ContainSubstring(`"testdb"`))
			Expect(sql).To(ContainSubstring("INTO"))
			Expect(sql).To(ContainSubstring("'nodelocal://1/backups/backup-001'"))
		})

		It("should include revision_history for schema-only backup", func() {
			opts := types.BackupOptions{
				Database:   "testdb",
				BackupID:   "backup-002",
				SchemaOnly: true,
			}
			sql := buildCRDBBackupSQL(opts)
			Expect(sql).To(ContainSubstring("WITH revision_history"))
		})

		It("should not include WITH clause when no options", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				BackupID: "backup-003",
			}
			sql := buildCRDBBackupSQL(opts)
			Expect(sql).NotTo(ContainSubstring("WITH"))
		})

		It("should escape special characters in database name", func() {
			opts := types.BackupOptions{
				Database: `test"db`,
				BackupID: "backup-004",
			}
			sql := buildCRDBBackupSQL(opts)
			Expect(sql).To(ContainSubstring(`"test""db"`))
		})

		It("should use backup ID in destination path", func() {
			opts := types.BackupOptions{
				Database: "testdb",
				BackupID: "my-custom-backup-2026",
			}
			sql := buildCRDBBackupSQL(opts)
			Expect(sql).To(ContainSubstring("'nodelocal://1/backups/my-custom-backup-2026'"))
		})
	})

	Describe("GetBackupProgress", func() {
		It("should return error for unknown backup", func() {
			progress, err := adapter.GetBackupProgress(ctx, "nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(progress).To(Equal(0))
		})
	})

	Describe("CancelBackup", func() {
		It("should return error for unknown backup", func() {
			err := adapter.CancelBackup(ctx, "nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Describe("With Mock Pool", func() {
		var (
			mock pgxmock.PgxPoolIface
		)

		BeforeEach(func() {
			var err error
			mock, err = pgxmock.NewPool()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			mock.Close()
		})

		Describe("Backup with mock", func() {
			It("should execute BACKUP and return result", func() {
				rows := pgxmock.NewRows([]string{
					"job_id", "status", "fraction_completed", "rows", "index_entries", "bytes",
				}).AddRow(int64(12345), "succeeded", float64(1.0), int64(100), int64(50), int64(8192))

				mock.ExpectQuery(`BACKUP DATABASE "testdb" INTO`).
					WillReturnRows(rows)

				result, err := backupWithCRDBMock(ctx, mock, types.BackupOptions{
					Database: "testdb",
					BackupID: "backup-001",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.BackupID).To(Equal("backup-001"))
				Expect(result.SizeBytes).To(Equal(int64(8192)))
				Expect(result.Format).To(Equal("cockroachdb-native"))
				Expect(result.StartTime).NotTo(BeEmpty())
				Expect(result.EndTime).NotTo(BeEmpty())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should write metadata to writer when provided", func() {
				rows := pgxmock.NewRows([]string{
					"job_id", "status", "fraction_completed", "rows", "index_entries", "bytes",
				}).AddRow(int64(12345), "succeeded", float64(1.0), int64(100), int64(50), int64(8192))

				mock.ExpectQuery(`BACKUP DATABASE "testdb" INTO`).
					WillReturnRows(rows)

				var buf bytes.Buffer
				result, err := backupWithCRDBMock(ctx, mock, types.BackupOptions{
					Database: "testdb",
					BackupID: "backup-001",
					Writer:   &buf,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())

				metadata := buf.String()
				Expect(metadata).To(ContainSubstring("backup_id=backup-001"))
				Expect(metadata).To(ContainSubstring("job_id=12345"))
				Expect(metadata).To(ContainSubstring("status=succeeded"))
				Expect(metadata).To(ContainSubstring("rows=100"))
				Expect(metadata).To(ContainSubstring("bytes=8192"))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on BACKUP failure", func() {
				mock.ExpectQuery(`BACKUP DATABASE "testdb" INTO`).
					WillReturnError(fmt.Errorf("backup destination already exists"))

				result, err := backupWithCRDBMock(ctx, mock, types.BackupOptions{
					Database: "testdb",
					BackupID: "backup-001",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to execute BACKUP"))
				Expect(result).To(BeNil())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should execute BACKUP with revision_history for schema-only", func() {
				rows := pgxmock.NewRows([]string{
					"job_id", "status", "fraction_completed", "rows", "index_entries", "bytes",
				}).AddRow(int64(12346), "succeeded", float64(1.0), int64(0), int64(0), int64(1024))

				mock.ExpectQuery(`BACKUP DATABASE "testdb" INTO .* WITH revision_history`).
					WillReturnRows(rows)

				result, err := backupWithCRDBMock(ctx, mock, types.BackupOptions{
					Database:   "testdb",
					BackupID:   "backup-schema",
					SchemaOnly: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("GetBackupProgress with mock", func() {
			It("should query crdb_internal.jobs for running backup", func() {
				rows := pgxmock.NewRows([]string{"fraction_completed"}).
					AddRow(float64(0.75))
				mock.ExpectQuery(`SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = \$1`).
					WithArgs(int64(12345)).
					WillReturnRows(rows)

				progress := getBackupProgressWithCRDBMock(ctx, mock, int64(12345))
				Expect(progress).To(Equal(75))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return 100 for completed backup", func() {
				rows := pgxmock.NewRows([]string{"fraction_completed"}).
					AddRow(float64(1.0))
				mock.ExpectQuery(`SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = \$1`).
					WithArgs(int64(12345)).
					WillReturnRows(rows)

				progress := getBackupProgressWithCRDBMock(ctx, mock, int64(12345))
				Expect(progress).To(Equal(100))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("CancelBackup with mock", func() {
			It("should execute CANCEL JOB", func() {
				mock.ExpectExec(`CANCEL JOB \$1`).
					WithArgs(int64(12345)).
					WillReturnResult(pgxmock.NewResult("CANCEL", 0))

				err := cancelBackupWithCRDBMock(ctx, mock, int64(12345))
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})
})

// SQL builder helper functions for CockroachDB backup operations

func buildCRDBBackupSQL(opts types.BackupOptions) string {
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

// Mock-based helper functions for CockroachDB backup operations

func backupWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.BackupOptions) (*types.BackupResult, error) {
	backupSQL := buildCRDBBackupSQL(opts)

	var jobID int64
	var status string
	var fractionCompleted float64
	var rows int64
	var indexEntries int64
	var bytesWritten int64

	err := pool.QueryRow(ctx, backupSQL).Scan(
		&jobID, &status, &fractionCompleted, &rows, &indexEntries, &bytesWritten,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute BACKUP: %w", err)
	}

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
		StartTime: "2026-01-01T00:00:00Z",
		EndTime:   "2026-01-01T00:01:00Z",
	}

	return result, nil
}

func getBackupProgressWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, jobID int64) int {
	var fractionCompleted float64
	err := pool.QueryRow(ctx,
		"SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = $1",
		jobID).Scan(&fractionCompleted)
	if err != nil {
		return 0
	}
	return int(fractionCompleted * 100)
}

func cancelBackupWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, jobID int64) error {
	_, err := pool.Exec(ctx, "CANCEL JOB $1", jobID)
	return err
}
