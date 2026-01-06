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

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordConnectionAttempt(t *testing.T) {
	// Reset metrics
	ConnectionAttemptsTotal.Reset()

	// Record some connection attempts
	RecordConnectionAttempt("instance1", "postgresql", "default", StatusSuccess)
	RecordConnectionAttempt("instance1", "postgresql", "default", StatusSuccess)
	RecordConnectionAttempt("instance1", "postgresql", "default", StatusFailure)
	RecordConnectionAttempt("instance2", "mysql", "production", StatusSuccess)

	// Verify counts
	successCount := testutil.ToFloat64(ConnectionAttemptsTotal.WithLabelValues("instance1", "postgresql", StatusSuccess, "default"))
	if successCount != 2 {
		t.Errorf("Expected 2 successful connections for instance1, got %v", successCount)
	}

	failureCount := testutil.ToFloat64(ConnectionAttemptsTotal.WithLabelValues("instance1", "postgresql", StatusFailure, "default"))
	if failureCount != 1 {
		t.Errorf("Expected 1 failed connection for instance1, got %v", failureCount)
	}

	mysqlCount := testutil.ToFloat64(ConnectionAttemptsTotal.WithLabelValues("instance2", "mysql", StatusSuccess, "production"))
	if mysqlCount != 1 {
		t.Errorf("Expected 1 successful connection for instance2, got %v", mysqlCount)
	}
}

func TestRecordConnectionLatency(t *testing.T) {
	// Reset metrics
	ConnectionLatencySeconds.Reset()

	// Record latencies
	RecordConnectionLatency("instance1", "postgresql", "default", 0.05)
	RecordConnectionLatency("instance1", "postgresql", "default", 0.1)
	RecordConnectionLatency("instance1", "postgresql", "default", 0.2)

	// Verify histogram count using CollectAndCount
	count := testutil.CollectAndCount(ConnectionLatencySeconds)
	if count != 1 { // 1 metric series with 3 observations
		t.Errorf("Expected 1 metric series, got %v", count)
	}
}

func TestSetInstanceHealth(t *testing.T) {
	// Reset metrics
	InstanceHealthy.Reset()

	// Set healthy
	SetInstanceHealth("instance1", "postgresql", "default", true)
	health := testutil.ToFloat64(InstanceHealthy.WithLabelValues("instance1", "postgresql", "default"))
	if health != 1 {
		t.Errorf("Expected health=1 for healthy instance, got %v", health)
	}

	// Set unhealthy
	SetInstanceHealth("instance1", "postgresql", "default", false)
	health = testutil.ToFloat64(InstanceHealthy.WithLabelValues("instance1", "postgresql", "default"))
	if health != 0 {
		t.Errorf("Expected health=0 for unhealthy instance, got %v", health)
	}
}

func TestSetInstanceLastHealthCheck(t *testing.T) {
	// Reset metrics
	InstanceLastHealthCheckTimestamp.Reset()

	timestamp := float64(1704067200) // 2024-01-01 00:00:00 UTC
	SetInstanceLastHealthCheck("instance1", "postgresql", "default", timestamp)

	recorded := testutil.ToFloat64(InstanceLastHealthCheckTimestamp.WithLabelValues("instance1", "postgresql", "default"))
	if recorded != timestamp {
		t.Errorf("Expected timestamp %v, got %v", timestamp, recorded)
	}
}

func TestRecordDatabaseOperation(t *testing.T) {
	// Reset metrics
	DatabaseOperationsTotal.Reset()

	// Record operations
	RecordDatabaseOperation(OperationCreate, "postgresql", "default", StatusSuccess)
	RecordDatabaseOperation(OperationCreate, "postgresql", "default", StatusFailure)
	RecordDatabaseOperation(OperationDelete, "postgresql", "default", StatusSuccess)

	// Verify counts
	createSuccess := testutil.ToFloat64(DatabaseOperationsTotal.WithLabelValues(OperationCreate, "postgresql", StatusSuccess, "default"))
	if createSuccess != 1 {
		t.Errorf("Expected 1 successful create, got %v", createSuccess)
	}

	createFailure := testutil.ToFloat64(DatabaseOperationsTotal.WithLabelValues(OperationCreate, "postgresql", StatusFailure, "default"))
	if createFailure != 1 {
		t.Errorf("Expected 1 failed create, got %v", createFailure)
	}

	deleteSuccess := testutil.ToFloat64(DatabaseOperationsTotal.WithLabelValues(OperationDelete, "postgresql", StatusSuccess, "default"))
	if deleteSuccess != 1 {
		t.Errorf("Expected 1 successful delete, got %v", deleteSuccess)
	}
}

func TestRecordDatabaseOperationDuration(t *testing.T) {
	// Reset metrics
	DatabaseOperationDurationSeconds.Reset()

	RecordDatabaseOperationDuration(OperationCreate, "postgresql", "default", 1.5)
	RecordDatabaseOperationDuration(OperationCreate, "postgresql", "default", 2.0)

	// Verify histogram was recorded
	count := testutil.CollectAndCount(DatabaseOperationDurationSeconds)
	if count != 1 { // 1 metric series with 2 observations
		t.Errorf("Expected 1 metric series, got %v", count)
	}
}

func TestSetDatabaseSize(t *testing.T) {
	// Reset metrics
	DatabaseSizeBytes.Reset()

	SetDatabaseSize("mydb", "instance1", "postgresql", "default", 1073741824) // 1GB

	size := testutil.ToFloat64(DatabaseSizeBytes.WithLabelValues("mydb", "instance1", "postgresql", "default"))
	if size != 1073741824 {
		t.Errorf("Expected size 1073741824, got %v", size)
	}
}

func TestRecordUserOperation(t *testing.T) {
	// Reset metrics
	UserOperationsTotal.Reset()

	RecordUserOperation(OperationCreate, "postgresql", "default", StatusSuccess)
	RecordUserOperation(OperationUpdate, "mysql", "production", StatusFailure)

	createSuccess := testutil.ToFloat64(UserOperationsTotal.WithLabelValues(OperationCreate, "postgresql", StatusSuccess, "default"))
	if createSuccess != 1 {
		t.Errorf("Expected 1 successful user create, got %v", createSuccess)
	}

	updateFailure := testutil.ToFloat64(UserOperationsTotal.WithLabelValues(OperationUpdate, "mysql", StatusFailure, "production"))
	if updateFailure != 1 {
		t.Errorf("Expected 1 failed user update, got %v", updateFailure)
	}
}

func TestRecordRoleOperation(t *testing.T) {
	// Reset metrics
	RoleOperationsTotal.Reset()

	RecordRoleOperation(OperationCreate, "postgresql", "default", StatusSuccess)

	createSuccess := testutil.ToFloat64(RoleOperationsTotal.WithLabelValues(OperationCreate, "postgresql", StatusSuccess, "default"))
	if createSuccess != 1 {
		t.Errorf("Expected 1 successful role create, got %v", createSuccess)
	}
}

func TestRecordGrantOperation(t *testing.T) {
	// Reset metrics
	GrantOperationsTotal.Reset()

	RecordGrantOperation(OperationCreate, "postgresql", "default", StatusSuccess)
	RecordGrantOperation(OperationDelete, "postgresql", "default", StatusSuccess)

	createSuccess := testutil.ToFloat64(GrantOperationsTotal.WithLabelValues(OperationCreate, "postgresql", StatusSuccess, "default"))
	if createSuccess != 1 {
		t.Errorf("Expected 1 successful grant create, got %v", createSuccess)
	}
}

func TestRecordBackupOperation(t *testing.T) {
	// Reset metrics
	BackupOperationsTotal.Reset()

	RecordBackupOperation("postgresql", "default", StatusSuccess)
	RecordBackupOperation("postgresql", "default", StatusFailure)
	RecordBackupOperation("mysql", "production", StatusSuccess)

	pgSuccess := testutil.ToFloat64(BackupOperationsTotal.WithLabelValues("postgresql", StatusSuccess, "default"))
	if pgSuccess != 1 {
		t.Errorf("Expected 1 successful postgresql backup, got %v", pgSuccess)
	}

	pgFailure := testutil.ToFloat64(BackupOperationsTotal.WithLabelValues("postgresql", StatusFailure, "default"))
	if pgFailure != 1 {
		t.Errorf("Expected 1 failed postgresql backup, got %v", pgFailure)
	}
}

func TestRecordBackupDuration(t *testing.T) {
	// Reset metrics
	BackupDurationSeconds.Reset()

	RecordBackupDuration("postgresql", "default", 120.5)

	count := testutil.CollectAndCount(BackupDurationSeconds)
	if count != 1 {
		t.Errorf("Expected 1 metric series, got %v", count)
	}
}

func TestSetBackupSize(t *testing.T) {
	// Reset metrics
	BackupSizeBytes.Reset()

	SetBackupSize("mydb", "postgresql", "default", 536870912) // 512MB

	size := testutil.ToFloat64(BackupSizeBytes.WithLabelValues("mydb", "postgresql", "default"))
	if size != 536870912 {
		t.Errorf("Expected backup size 536870912, got %v", size)
	}
}

func TestSetBackupLastSuccess(t *testing.T) {
	// Reset metrics
	BackupLastSuccessTimestamp.Reset()

	timestamp := float64(1704067200)
	SetBackupLastSuccess("mydb", "postgresql", "default", timestamp)

	recorded := testutil.ToFloat64(BackupLastSuccessTimestamp.WithLabelValues("mydb", "postgresql", "default"))
	if recorded != timestamp {
		t.Errorf("Expected timestamp %v, got %v", timestamp, recorded)
	}
}

func TestRecordRestoreOperation(t *testing.T) {
	// Reset metrics
	RestoreOperationsTotal.Reset()

	RecordRestoreOperation("postgresql", "default", StatusSuccess)
	RecordRestoreOperation("postgresql", "default", StatusFailure)

	success := testutil.ToFloat64(RestoreOperationsTotal.WithLabelValues("postgresql", StatusSuccess, "default"))
	if success != 1 {
		t.Errorf("Expected 1 successful restore, got %v", success)
	}

	failure := testutil.ToFloat64(RestoreOperationsTotal.WithLabelValues("postgresql", StatusFailure, "default"))
	if failure != 1 {
		t.Errorf("Expected 1 failed restore, got %v", failure)
	}
}

func TestRecordRestoreDuration(t *testing.T) {
	// Reset metrics
	RestoreDurationSeconds.Reset()

	RecordRestoreDuration("postgresql", "default", 300.0)

	count := testutil.CollectAndCount(RestoreDurationSeconds)
	if count != 1 {
		t.Errorf("Expected 1 metric series, got %v", count)
	}
}

func TestRecordScheduledBackup(t *testing.T) {
	// Reset metrics
	ScheduledBackupsTotal.Reset()

	RecordScheduledBackup("default", StatusSuccess)
	RecordScheduledBackup("default", StatusSuccess)
	RecordScheduledBackup("default", StatusFailure)

	success := testutil.ToFloat64(ScheduledBackupsTotal.WithLabelValues(StatusSuccess, "default"))
	if success != 2 {
		t.Errorf("Expected 2 successful scheduled backups, got %v", success)
	}

	failure := testutil.ToFloat64(ScheduledBackupsTotal.WithLabelValues(StatusFailure, "default"))
	if failure != 1 {
		t.Errorf("Expected 1 failed scheduled backup, got %v", failure)
	}
}

func TestSetScheduleNextBackup(t *testing.T) {
	// Reset metrics
	ScheduleNextBackupTimestamp.Reset()

	timestamp := float64(1704153600) // Next day
	SetScheduleNextBackup("mydb", "default", timestamp)

	recorded := testutil.ToFloat64(ScheduleNextBackupTimestamp.WithLabelValues("mydb", "default"))
	if recorded != timestamp {
		t.Errorf("Expected timestamp %v, got %v", timestamp, recorded)
	}
}

func TestSetResourceCount(t *testing.T) {
	// Reset metrics
	ResourceCount.Reset()

	SetResourceCount("Database", "Ready", "default", 5)
	SetResourceCount("Database", "Failed", "default", 2)
	SetResourceCount("DatabaseUser", "Ready", "default", 10)

	dbReady := testutil.ToFloat64(ResourceCount.WithLabelValues("Database", "Ready", "default"))
	if dbReady != 5 {
		t.Errorf("Expected 5 ready databases, got %v", dbReady)
	}

	dbFailed := testutil.ToFloat64(ResourceCount.WithLabelValues("Database", "Failed", "default"))
	if dbFailed != 2 {
		t.Errorf("Expected 2 failed databases, got %v", dbFailed)
	}

	userReady := testutil.ToFloat64(ResourceCount.WithLabelValues("DatabaseUser", "Ready", "default"))
	if userReady != 10 {
		t.Errorf("Expected 10 ready users, got %v", userReady)
	}
}

func TestDeleteInstanceMetrics(t *testing.T) {
	// Set up metrics
	InstanceHealthy.Reset()
	InstanceLastHealthCheckTimestamp.Reset()
	ConnectionLatencySeconds.Reset()

	SetInstanceHealth("instance1", "postgresql", "default", true)
	SetInstanceLastHealthCheck("instance1", "postgresql", "default", 1704067200)
	RecordConnectionLatency("instance1", "postgresql", "default", 0.1)

	// Verify metrics exist
	health := testutil.ToFloat64(InstanceHealthy.WithLabelValues("instance1", "postgresql", "default"))
	if health != 1 {
		t.Errorf("Expected health=1 before deletion, got %v", health)
	}

	// Delete metrics
	DeleteInstanceMetrics("instance1", "postgresql", "default")

	// After deletion, WithLabelValues creates a new metric with zero value
	// The original label set should be removed from the metric
	// We can verify by checking the metric count
}

func TestDeleteDatabaseMetrics(t *testing.T) {
	// Set up metrics
	DatabaseSizeBytes.Reset()

	SetDatabaseSize("mydb", "instance1", "postgresql", "default", 1073741824)

	// Verify metric exists
	size := testutil.ToFloat64(DatabaseSizeBytes.WithLabelValues("mydb", "instance1", "postgresql", "default"))
	if size != 1073741824 {
		t.Errorf("Expected size before deletion, got %v", size)
	}

	// Delete metrics
	DeleteDatabaseMetrics("mydb", "instance1", "postgresql", "default")
}

func TestDeleteBackupMetrics(t *testing.T) {
	// Set up metrics
	BackupSizeBytes.Reset()
	BackupLastSuccessTimestamp.Reset()

	SetBackupSize("mydb", "postgresql", "default", 536870912)
	SetBackupLastSuccess("mydb", "postgresql", "default", 1704067200)

	// Delete metrics
	DeleteBackupMetrics("mydb", "postgresql", "default")
}

func TestDeleteScheduleMetrics(t *testing.T) {
	// Set up metrics
	ScheduleNextBackupTimestamp.Reset()

	SetScheduleNextBackup("mydb", "default", 1704153600)

	// Delete metrics
	DeleteScheduleMetrics("mydb", "default")
}

func TestOperationConstants(t *testing.T) {
	// Verify operation constants are defined correctly
	if OperationCreate != "create" {
		t.Errorf("OperationCreate should be 'create', got %s", OperationCreate)
	}
	if OperationUpdate != "update" {
		t.Errorf("OperationUpdate should be 'update', got %s", OperationUpdate)
	}
	if OperationDelete != "delete" {
		t.Errorf("OperationDelete should be 'delete', got %s", OperationDelete)
	}
	if OperationConnect != "connect" {
		t.Errorf("OperationConnect should be 'connect', got %s", OperationConnect)
	}
	if OperationBackup != "backup" {
		t.Errorf("OperationBackup should be 'backup', got %s", OperationBackup)
	}
	if OperationRestore != "restore" {
		t.Errorf("OperationRestore should be 'restore', got %s", OperationRestore)
	}
}

func TestStatusConstants(t *testing.T) {
	// Verify status constants are defined correctly
	if StatusSuccess != "success" {
		t.Errorf("StatusSuccess should be 'success', got %s", StatusSuccess)
	}
	if StatusFailure != "failure" {
		t.Errorf("StatusFailure should be 'failure', got %s", StatusFailure)
	}
}
