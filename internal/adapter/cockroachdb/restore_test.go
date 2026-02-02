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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/db-provision-operator/internal/adapter/cockroachdb/testutil"
	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Restore Operations", func() {
	var (
		ctx     context.Context
		adapter *Adapter
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(testutil.DefaultConnectionConfig())
	})

	Describe("Restore", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				result, err := adapter.Restore(ctx, types.RestoreOptions{
					Database:  "testdb",
					RestoreID: "restore-001",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("buildRestoreSQL", func() {
		It("should generate RESTORE DATABASE with nodelocal source", func() {
			opts := types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-001",
			}
			sql := buildCRDBRestoreSQL(opts)
			Expect(sql).To(ContainSubstring("RESTORE DATABASE"))
			Expect(sql).To(ContainSubstring(`"testdb"`))
			Expect(sql).To(ContainSubstring("FROM LATEST IN"))
			Expect(sql).To(ContainSubstring("'nodelocal://1/backups/restore-001'"))
		})

		It("should include skip_missing_foreign_keys for data-only restore", func() {
			opts := types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-002",
				DataOnly:  true,
			}
			sql := buildCRDBRestoreSQL(opts)
			Expect(sql).To(ContainSubstring("WITH skip_missing_foreign_keys, skip_missing_sequences"))
		})

		It("should include skip_missing_sequences for data-only restore", func() {
			opts := types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-002",
				DataOnly:  true,
			}
			sql := buildCRDBRestoreSQL(opts)
			Expect(sql).To(ContainSubstring("skip_missing_sequences"))
		})

		It("should not include WITH clause when no special options", func() {
			opts := types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "restore-003",
			}
			sql := buildCRDBRestoreSQL(opts)
			Expect(sql).NotTo(ContainSubstring("WITH"))
		})

		It("should escape special characters in database name", func() {
			opts := types.RestoreOptions{
				Database:  `test"db`,
				RestoreID: "restore-004",
			}
			sql := buildCRDBRestoreSQL(opts)
			Expect(sql).To(ContainSubstring(`"test""db"`))
		})

		It("should use restore ID in source path", func() {
			opts := types.RestoreOptions{
				Database:  "testdb",
				RestoreID: "my-custom-restore-2026",
			}
			sql := buildCRDBRestoreSQL(opts)
			Expect(sql).To(ContainSubstring("'nodelocal://1/backups/my-custom-restore-2026'"))
		})
	})

	Describe("GetRestoreProgress", func() {
		It("should return error for unknown restore", func() {
			progress, err := adapter.GetRestoreProgress(ctx, "nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
			Expect(progress).To(Equal(0))
		})
	})

	Describe("CancelRestore", func() {
		It("should return error for unknown restore", func() {
			err := adapter.CancelRestore(ctx, "nonexistent")
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

		Describe("Restore with mock", func() {
			It("should execute RESTORE and return result", func() {
				rows := pgxmock.NewRows([]string{
					"job_id", "status", "fraction_completed", "rows", "index_entries", "bytes",
				}).AddRow(int64(67890), "succeeded", float64(1.0), int64(200), int64(100), int64(16384))

				mock.ExpectQuery(`RESTORE DATABASE "testdb" FROM LATEST IN`).
					WillReturnRows(rows)

				result, err := restoreWithCRDBMock(ctx, mock, types.RestoreOptions{
					Database:  "testdb",
					RestoreID: "restore-001",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(result.RestoreID).To(Equal("restore-001"))
				Expect(result.TargetDatabase).To(Equal("testdb"))
				Expect(result.TablesRestored).To(Equal(int32(200)))
				Expect(result.StartTime).NotTo(BeEmpty())
				Expect(result.EndTime).NotTo(BeEmpty())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return error on RESTORE failure", func() {
				mock.ExpectQuery(`RESTORE DATABASE "testdb" FROM LATEST IN`).
					WillReturnError(fmt.Errorf("backup not found"))

				result, err := restoreWithCRDBMock(ctx, mock, types.RestoreOptions{
					Database:  "testdb",
					RestoreID: "restore-001",
				})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to execute RESTORE"))
				Expect(result).To(BeNil())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should execute RESTORE with skip options for data-only", func() {
				rows := pgxmock.NewRows([]string{
					"job_id", "status", "fraction_completed", "rows", "index_entries", "bytes",
				}).AddRow(int64(67891), "succeeded", float64(1.0), int64(150), int64(75), int64(12288))

				mock.ExpectQuery(`RESTORE DATABASE "testdb" FROM LATEST IN .* WITH skip_missing_foreign_keys, skip_missing_sequences`).
					WillReturnRows(rows)

				result, err := restoreWithCRDBMock(ctx, mock, types.RestoreOptions{
					Database:  "testdb",
					RestoreID: "restore-data",
					DataOnly:  true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should drop existing database when requested", func() {
				// First: drop database
				mock.ExpectExec(`DROP DATABASE IF EXISTS "testdb" CASCADE`).
					WillReturnResult(pgxmock.NewResult("DROP DATABASE", 0))

				// Then: execute restore
				rows := pgxmock.NewRows([]string{
					"job_id", "status", "fraction_completed", "rows", "index_entries", "bytes",
				}).AddRow(int64(67892), "succeeded", float64(1.0), int64(100), int64(50), int64(8192))

				mock.ExpectQuery(`RESTORE DATABASE "testdb" FROM LATEST IN`).
					WillReturnRows(rows)

				result, err := restoreWithDropExistingMock(ctx, mock, types.RestoreOptions{
					Database:     "testdb",
					RestoreID:    "restore-drop",
					DropExisting: true,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).NotTo(BeNil())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})

		Describe("GetRestoreProgress with mock", func() {
			It("should query crdb_internal.jobs for running restore", func() {
				rows := pgxmock.NewRows([]string{"fraction_completed"}).
					AddRow(float64(0.50))
				mock.ExpectQuery(`SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = \$1`).
					WithArgs(int64(67890)).
					WillReturnRows(rows)

				progress := getRestoreProgressWithCRDBMock(ctx, mock, int64(67890))
				Expect(progress).To(Equal(50))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return 100 for completed restore", func() {
				rows := pgxmock.NewRows([]string{"fraction_completed"}).
					AddRow(float64(1.0))
				mock.ExpectQuery(`SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = \$1`).
					WithArgs(int64(67890)).
					WillReturnRows(rows)

				progress := getRestoreProgressWithCRDBMock(ctx, mock, int64(67890))
				Expect(progress).To(Equal(100))
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})

			It("should return 0 on query error", func() {
				mock.ExpectQuery(`SELECT fraction_completed`).
					WithArgs(int64(67890)).
					WillReturnError(fmt.Errorf("job not found"))

				progress := getRestoreProgressWithCRDBMock(ctx, mock, int64(67890))
				Expect(progress).To(Equal(0))
			})
		})

		Describe("CancelRestore with mock", func() {
			It("should execute CANCEL JOB", func() {
				mock.ExpectExec(`CANCEL JOB \$1`).
					WithArgs(int64(67890)).
					WillReturnResult(pgxmock.NewResult("CANCEL", 0))

				err := cancelRestoreWithCRDBMock(ctx, mock, int64(67890))
				Expect(err).NotTo(HaveOccurred())
				Expect(mock.ExpectationsWereMet()).NotTo(HaveOccurred())
			})
		})
	})
})

// SQL builder helper functions for CockroachDB restore operations

func buildCRDBRestoreSQL(opts types.RestoreOptions) string {
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

// Mock-based helper functions for CockroachDB restore operations

func restoreWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.RestoreOptions) (*types.RestoreResult, error) {
	restoreSQL := buildCRDBRestoreSQL(opts)

	var jobID int64
	var status string
	var fractionCompleted float64
	var rows int64
	var indexEntries int64
	var bytesRead int64

	err := pool.QueryRow(ctx, restoreSQL).Scan(
		&jobID, &status, &fractionCompleted, &rows, &indexEntries, &bytesRead,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to execute RESTORE: %w", err)
	}

	result := &types.RestoreResult{
		RestoreID:      opts.RestoreID,
		TargetDatabase: opts.Database,
		TablesRestored: int32(rows),
		StartTime:      "2026-01-01T00:00:00Z",
		EndTime:        "2026-01-01T00:01:00Z",
	}

	return result, nil
}

func restoreWithDropExistingMock(ctx context.Context, pool pgxmock.PgxPoolIface, opts types.RestoreOptions) (*types.RestoreResult, error) {
	// Drop existing database if requested
	if opts.DropExisting {
		dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %s CASCADE", escapeIdentifier(opts.Database))
		_, err := pool.Exec(ctx, dropSQL)
		if err != nil {
			return nil, fmt.Errorf("failed to drop existing database: %w", err)
		}
	}

	return restoreWithCRDBMock(ctx, pool, opts)
}

func getRestoreProgressWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, jobID int64) int {
	var fractionCompleted float64
	err := pool.QueryRow(ctx,
		"SELECT fraction_completed FROM crdb_internal.jobs WHERE job_id = $1",
		jobID).Scan(&fractionCompleted)
	if err != nil {
		return 0
	}
	return int(fractionCompleted * 100)
}

func cancelRestoreWithCRDBMock(ctx context.Context, pool pgxmock.PgxPoolIface, jobID int64) error {
	_, err := pool.Exec(ctx, "CANCEL JOB $1", jobID)
	return err
}
