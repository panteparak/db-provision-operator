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
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Metric namespace
	namespace = "dbops"

	// Label names
	labelInstance  = "instance"
	labelEngine    = "engine"
	labelStatus    = "status"
	labelOperation = "operation"
	labelDatabase  = "database"
	labelNamespace = "namespace"
	labelPhase     = "phase"
)

// Status values
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
)

// Operation values
const (
	OperationCreate  = "create"
	OperationUpdate  = "update"
	OperationDelete  = "delete"
	OperationConnect = "connect"
	OperationBackup  = "backup"
	OperationRestore = "restore"
)

var (
	// Connection metrics

	// ConnectionAttemptsTotal tracks the total number of database connection attempts
	ConnectionAttemptsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connection_attempts_total",
			Help:      "Total number of database connection attempts",
		},
		[]string{labelInstance, labelEngine, labelStatus, labelNamespace},
	)

	// ConnectionLatencySeconds tracks the latency of database connections
	ConnectionLatencySeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "connection_latency_seconds",
			Help:      "Latency of database connection attempts in seconds",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{labelInstance, labelEngine, labelNamespace},
	)

	// Instance health metrics

	// InstanceHealthy indicates whether a database instance is healthy (1) or not (0)
	InstanceHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "instance_healthy",
			Help:      "Whether the database instance is healthy (1) or unhealthy (0)",
		},
		[]string{labelInstance, labelEngine, labelNamespace},
	)

	// InstanceLastHealthCheckTimestamp records the timestamp of the last health check
	InstanceLastHealthCheckTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "instance_last_health_check_timestamp_seconds",
			Help:      "Unix timestamp of the last successful health check",
		},
		[]string{labelInstance, labelEngine, labelNamespace},
	)

	// Database operation metrics

	// DatabaseOperationsTotal tracks total database operations (create, update, delete)
	DatabaseOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "database_operations_total",
			Help:      "Total number of database operations",
		},
		[]string{labelOperation, labelEngine, labelStatus, labelNamespace},
	)

	// DatabaseOperationDurationSeconds tracks the duration of database operations
	DatabaseOperationDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "database_operation_duration_seconds",
			Help:      "Duration of database operations in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{labelOperation, labelEngine, labelNamespace},
	)

	// DatabaseSizeBytes tracks the size of databases
	DatabaseSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "database_size_bytes",
			Help:      "Size of the database in bytes",
		},
		[]string{labelDatabase, labelInstance, labelEngine, labelNamespace},
	)

	// User operation metrics

	// UserOperationsTotal tracks total user operations
	UserOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "user_operations_total",
			Help:      "Total number of user operations",
		},
		[]string{labelOperation, labelEngine, labelStatus, labelNamespace},
	)

	// Role operation metrics

	// RoleOperationsTotal tracks total role operations
	RoleOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "role_operations_total",
			Help:      "Total number of role operations",
		},
		[]string{labelOperation, labelEngine, labelStatus, labelNamespace},
	)

	// Grant operation metrics

	// GrantOperationsTotal tracks total grant operations
	GrantOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grant_operations_total",
			Help:      "Total number of grant operations",
		},
		[]string{labelOperation, labelEngine, labelStatus, labelNamespace},
	)

	// Backup metrics

	// BackupOperationsTotal tracks total backup operations
	BackupOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "backup_operations_total",
			Help:      "Total number of backup operations",
		},
		[]string{labelEngine, labelStatus, labelNamespace},
	)

	// BackupDurationSeconds tracks the duration of backup operations
	BackupDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "backup_duration_seconds",
			Help:      "Duration of backup operations in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
		},
		[]string{labelEngine, labelNamespace},
	)

	// BackupSizeBytes tracks the size of backups
	BackupSizeBytes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backup_size_bytes",
			Help:      "Size of the backup in bytes",
		},
		[]string{labelDatabase, labelEngine, labelNamespace},
	)

	// BackupLastSuccessTimestamp records the timestamp of the last successful backup
	BackupLastSuccessTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backup_last_success_timestamp_seconds",
			Help:      "Unix timestamp of the last successful backup",
		},
		[]string{labelDatabase, labelEngine, labelNamespace},
	)

	// Restore metrics

	// RestoreOperationsTotal tracks total restore operations
	RestoreOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "restore_operations_total",
			Help:      "Total number of restore operations",
		},
		[]string{labelEngine, labelStatus, labelNamespace},
	)

	// RestoreDurationSeconds tracks the duration of restore operations
	RestoreDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "restore_duration_seconds",
			Help:      "Duration of restore operations in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600},
		},
		[]string{labelEngine, labelNamespace},
	)

	// Schedule metrics

	// ScheduledBackupsTotal tracks total scheduled backups
	ScheduledBackupsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "scheduled_backups_total",
			Help:      "Total number of scheduled backups triggered",
		},
		[]string{labelStatus, labelNamespace},
	)

	// ScheduleNextBackupTimestamp records the next scheduled backup time
	ScheduleNextBackupTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "schedule_next_backup_timestamp_seconds",
			Help:      "Unix timestamp of the next scheduled backup",
		},
		[]string{labelDatabase, labelNamespace},
	)

	// Resource counts

	// ResourceCount tracks the current count of managed resources
	ResourceCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "resource_count",
			Help:      "Current count of managed resources by type and phase",
		},
		[]string{"resource_type", labelPhase, labelNamespace},
	)
)

func init() {
	// Register all metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		// Connection metrics
		ConnectionAttemptsTotal,
		ConnectionLatencySeconds,

		// Instance health metrics
		InstanceHealthy,
		InstanceLastHealthCheckTimestamp,

		// Database operation metrics
		DatabaseOperationsTotal,
		DatabaseOperationDurationSeconds,
		DatabaseSizeBytes,

		// User operation metrics
		UserOperationsTotal,

		// Role operation metrics
		RoleOperationsTotal,

		// Grant operation metrics
		GrantOperationsTotal,

		// Backup metrics
		BackupOperationsTotal,
		BackupDurationSeconds,
		BackupSizeBytes,
		BackupLastSuccessTimestamp,

		// Restore metrics
		RestoreOperationsTotal,
		RestoreDurationSeconds,

		// Schedule metrics
		ScheduledBackupsTotal,
		ScheduleNextBackupTimestamp,

		// Resource counts
		ResourceCount,
	)
}

// RecordConnectionAttempt records a connection attempt with its status
func RecordConnectionAttempt(instance, engine, namespace, status string) {
	ConnectionAttemptsTotal.WithLabelValues(instance, engine, status, namespace).Inc()
}

// RecordConnectionLatency records connection latency
func RecordConnectionLatency(instance, engine, namespace string, seconds float64) {
	ConnectionLatencySeconds.WithLabelValues(instance, engine, namespace).Observe(seconds)
}

// SetInstanceHealth sets the health status of an instance
func SetInstanceHealth(instance, engine, namespace string, healthy bool) {
	value := float64(0)
	if healthy {
		value = 1
	}
	InstanceHealthy.WithLabelValues(instance, engine, namespace).Set(value)
}

// SetInstanceLastHealthCheck records the last health check timestamp
func SetInstanceLastHealthCheck(instance, engine, namespace string, timestamp float64) {
	InstanceLastHealthCheckTimestamp.WithLabelValues(instance, engine, namespace).Set(timestamp)
}

// RecordDatabaseOperation records a database operation
func RecordDatabaseOperation(operation, engine, namespace, status string) {
	DatabaseOperationsTotal.WithLabelValues(operation, engine, status, namespace).Inc()
}

// RecordDatabaseOperationDuration records the duration of a database operation
func RecordDatabaseOperationDuration(operation, engine, namespace string, seconds float64) {
	DatabaseOperationDurationSeconds.WithLabelValues(operation, engine, namespace).Observe(seconds)
}

// SetDatabaseSize sets the size of a database
func SetDatabaseSize(database, instance, engine, namespace string, bytes float64) {
	DatabaseSizeBytes.WithLabelValues(database, instance, engine, namespace).Set(bytes)
}

// RecordUserOperation records a user operation
func RecordUserOperation(operation, engine, namespace, status string) {
	UserOperationsTotal.WithLabelValues(operation, engine, status, namespace).Inc()
}

// RecordRoleOperation records a role operation
func RecordRoleOperation(operation, engine, namespace, status string) {
	RoleOperationsTotal.WithLabelValues(operation, engine, status, namespace).Inc()
}

// RecordGrantOperation records a grant operation
func RecordGrantOperation(operation, engine, namespace, status string) {
	GrantOperationsTotal.WithLabelValues(operation, engine, status, namespace).Inc()
}

// RecordBackupOperation records a backup operation
func RecordBackupOperation(engine, namespace, status string) {
	BackupOperationsTotal.WithLabelValues(engine, status, namespace).Inc()
}

// RecordBackupDuration records the duration of a backup operation
func RecordBackupDuration(engine, namespace string, seconds float64) {
	BackupDurationSeconds.WithLabelValues(engine, namespace).Observe(seconds)
}

// SetBackupSize sets the size of a backup
func SetBackupSize(database, engine, namespace string, bytes float64) {
	BackupSizeBytes.WithLabelValues(database, engine, namespace).Set(bytes)
}

// SetBackupLastSuccess records the last successful backup timestamp
func SetBackupLastSuccess(database, engine, namespace string, timestamp float64) {
	BackupLastSuccessTimestamp.WithLabelValues(database, engine, namespace).Set(timestamp)
}

// RecordRestoreOperation records a restore operation
func RecordRestoreOperation(engine, namespace, status string) {
	RestoreOperationsTotal.WithLabelValues(engine, status, namespace).Inc()
}

// RecordRestoreDuration records the duration of a restore operation
func RecordRestoreDuration(engine, namespace string, seconds float64) {
	RestoreDurationSeconds.WithLabelValues(engine, namespace).Observe(seconds)
}

// RecordScheduledBackup records a scheduled backup trigger
func RecordScheduledBackup(namespace, status string) {
	ScheduledBackupsTotal.WithLabelValues(status, namespace).Inc()
}

// SetScheduleNextBackup sets the next scheduled backup timestamp
func SetScheduleNextBackup(database, namespace string, timestamp float64) {
	ScheduleNextBackupTimestamp.WithLabelValues(database, namespace).Set(timestamp)
}

// SetResourceCount sets the count of resources by type and phase
func SetResourceCount(resourceType, phase, namespace string, count float64) {
	ResourceCount.WithLabelValues(resourceType, phase, namespace).Set(count)
}

// DeleteInstanceMetrics removes all metrics for a deleted instance
func DeleteInstanceMetrics(instance, engine, namespace string) {
	InstanceHealthy.DeleteLabelValues(instance, engine, namespace)
	InstanceLastHealthCheckTimestamp.DeleteLabelValues(instance, engine, namespace)
	ConnectionLatencySeconds.DeleteLabelValues(instance, engine, namespace)
}

// DeleteDatabaseMetrics removes all metrics for a deleted database
func DeleteDatabaseMetrics(database, instance, engine, namespace string) {
	DatabaseSizeBytes.DeleteLabelValues(database, instance, engine, namespace)
}

// DeleteBackupMetrics removes all metrics for a deleted backup
func DeleteBackupMetrics(database, engine, namespace string) {
	BackupSizeBytes.DeleteLabelValues(database, engine, namespace)
	BackupLastSuccessTimestamp.DeleteLabelValues(database, engine, namespace)
}

// DeleteScheduleMetrics removes all metrics for a deleted schedule
func DeleteScheduleMetrics(database, namespace string) {
	ScheduleNextBackupTimestamp.DeleteLabelValues(database, namespace)
}
