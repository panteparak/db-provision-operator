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

	// Info metrics - expose resource details as labels for Grafana table views
	// Following the kube-state-metrics pattern, value is always 1

	// InstanceInfo exposes database instance details
	InstanceInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "instance_info",
			Help:      "Information about database instances (value is always 1)",
		},
		[]string{labelInstance, labelNamespace, labelEngine, "version", "host", "port", labelPhase},
	)

	// DatabaseInfo exposes database details
	DatabaseInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "database_info",
			Help:      "Information about databases (value is always 1)",
		},
		[]string{labelDatabase, labelNamespace, "instance_ref", "db_name", labelPhase},
	)

	// UserInfo exposes database user details
	UserInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "user_info",
			Help:      "Information about database users (value is always 1)",
		},
		[]string{"user", labelNamespace, "instance_ref", "username", labelPhase},
	)

	// RoleInfo exposes database role details
	RoleInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "role_info",
			Help:      "Information about database roles (value is always 1)",
		},
		[]string{"role", labelNamespace, "instance_ref", "role_name", labelPhase},
	)

	// GrantInfo exposes database grant details
	GrantInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "grant_info",
			Help:      "Information about database grants (value is always 1)",
		},
		[]string{"grant", labelNamespace, "user_ref", "database_ref", "privileges", labelPhase},
	)

	// BackupInfo exposes database backup details
	BackupInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "backup_info",
			Help:      "Information about database backups (value is always 1)",
		},
		[]string{"backup", labelNamespace, "database_ref", "storage_type", labelPhase},
	)

	// ScheduleInfo exposes backup schedule details
	ScheduleInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "schedule_info",
			Help:      "Information about backup schedules (value is always 1)",
		},
		[]string{"schedule", labelNamespace, "database_ref", "cron", "paused"},
	)

	// RestoreInfo exposes database restore details
	RestoreInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "restore_info",
			Help:      "Information about database restores (value is always 1)",
		},
		[]string{"restore", labelNamespace, "backup_ref", "target_instance", labelPhase},
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

		// Info metrics for Grafana table views
		InstanceInfo,
		DatabaseInfo,
		UserInfo,
		RoleInfo,
		GrantInfo,
		BackupInfo,
		ScheduleInfo,
		RestoreInfo,
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

// Info metric helper functions
// These use DeletePartialMatch to handle changing labels (e.g., phase changes)

// SetInstanceInfo sets info metric for an instance (value is always 1)
// It first clears any existing info metric for this instance to handle phase changes
func SetInstanceInfo(instance, namespace, engine, version, host, port, phase string) {
	// Clear any existing info metric for this instance (handles phase changes)
	InstanceInfo.DeletePartialMatch(prometheus.Labels{
		labelInstance:  instance,
		labelNamespace: namespace,
	})
	InstanceInfo.WithLabelValues(instance, namespace, engine, version, host, port, phase).Set(1)
}

// DeleteInstanceInfo removes all info metrics for a deleted instance
func DeleteInstanceInfo(instance, namespace string) {
	InstanceInfo.DeletePartialMatch(prometheus.Labels{
		labelInstance:  instance,
		labelNamespace: namespace,
	})
}

// SetDatabaseInfo sets info metric for a database (value is always 1)
func SetDatabaseInfo(database, namespace, instanceRef, dbName, phase string) {
	DatabaseInfo.DeletePartialMatch(prometheus.Labels{
		labelDatabase:  database,
		labelNamespace: namespace,
	})
	DatabaseInfo.WithLabelValues(database, namespace, instanceRef, dbName, phase).Set(1)
}

// DeleteDatabaseInfo removes all info metrics for a deleted database
func DeleteDatabaseInfo(database, namespace string) {
	DatabaseInfo.DeletePartialMatch(prometheus.Labels{
		labelDatabase:  database,
		labelNamespace: namespace,
	})
}

// SetUserInfo sets info metric for a user (value is always 1)
func SetUserInfo(user, namespace, instanceRef, username, phase string) {
	UserInfo.DeletePartialMatch(prometheus.Labels{
		"user":         user,
		labelNamespace: namespace,
	})
	UserInfo.WithLabelValues(user, namespace, instanceRef, username, phase).Set(1)
}

// DeleteUserInfo removes all info metrics for a deleted user
func DeleteUserInfo(user, namespace string) {
	UserInfo.DeletePartialMatch(prometheus.Labels{
		"user":         user,
		labelNamespace: namespace,
	})
}

// SetRoleInfo sets info metric for a role (value is always 1)
func SetRoleInfo(role, namespace, instanceRef, roleName, phase string) {
	RoleInfo.DeletePartialMatch(prometheus.Labels{
		"role":         role,
		labelNamespace: namespace,
	})
	RoleInfo.WithLabelValues(role, namespace, instanceRef, roleName, phase).Set(1)
}

// DeleteRoleInfo removes all info metrics for a deleted role
func DeleteRoleInfo(role, namespace string) {
	RoleInfo.DeletePartialMatch(prometheus.Labels{
		"role":         role,
		labelNamespace: namespace,
	})
}

// SetGrantInfo sets info metric for a grant (value is always 1)
func SetGrantInfo(grant, namespace, userRef, databaseRef, privileges, phase string) {
	GrantInfo.DeletePartialMatch(prometheus.Labels{
		"grant":        grant,
		labelNamespace: namespace,
	})
	GrantInfo.WithLabelValues(grant, namespace, userRef, databaseRef, privileges, phase).Set(1)
}

// DeleteGrantInfo removes all info metrics for a deleted grant
func DeleteGrantInfo(grant, namespace string) {
	GrantInfo.DeletePartialMatch(prometheus.Labels{
		"grant":        grant,
		labelNamespace: namespace,
	})
}

// SetBackupInfo sets info metric for a backup (value is always 1)
func SetBackupInfo(backup, namespace, databaseRef, storageType, phase string) {
	BackupInfo.DeletePartialMatch(prometheus.Labels{
		"backup":       backup,
		labelNamespace: namespace,
	})
	BackupInfo.WithLabelValues(backup, namespace, databaseRef, storageType, phase).Set(1)
}

// DeleteBackupInfo removes all info metrics for a deleted backup
func DeleteBackupInfo(backup, namespace string) {
	BackupInfo.DeletePartialMatch(prometheus.Labels{
		"backup":       backup,
		labelNamespace: namespace,
	})
}

// SetScheduleInfo sets info metric for a backup schedule (value is always 1)
func SetScheduleInfo(schedule, namespace, databaseRef, cron, paused string) {
	ScheduleInfo.DeletePartialMatch(prometheus.Labels{
		"schedule":     schedule,
		labelNamespace: namespace,
	})
	ScheduleInfo.WithLabelValues(schedule, namespace, databaseRef, cron, paused).Set(1)
}

// DeleteScheduleInfo removes all info metrics for a deleted schedule
func DeleteScheduleInfo(schedule, namespace string) {
	ScheduleInfo.DeletePartialMatch(prometheus.Labels{
		"schedule":     schedule,
		labelNamespace: namespace,
	})
}

// SetRestoreInfo sets info metric for a restore (value is always 1)
func SetRestoreInfo(restore, namespace, backupRef, targetInstance, phase string) {
	RestoreInfo.DeletePartialMatch(prometheus.Labels{
		"restore":      restore,
		labelNamespace: namespace,
	})
	RestoreInfo.WithLabelValues(restore, namespace, backupRef, targetInstance, phase).Set(1)
}

// DeleteRestoreInfo removes all info metrics for a deleted restore
func DeleteRestoreInfo(restore, namespace string) {
	RestoreInfo.DeletePartialMatch(prometheus.Labels{
		"restore":      restore,
		labelNamespace: namespace,
	})
}
