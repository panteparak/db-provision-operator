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

package eventbus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseEvent(t *testing.T) {
	event := NewBaseEvent("TestEvent", "test-123", "TestAggregate")

	assert.Equal(t, "TestEvent", event.EventName())
	assert.Equal(t, "test-123", event.AggregateID())
	assert.Equal(t, "TestAggregate", event.AggregateType())
	assert.WithinDuration(t, time.Now(), event.EventTime(), time.Second)
}

func TestInstanceConnected(t *testing.T) {
	event := NewInstanceConnected("my-instance", "default", "mysql", "8.0", "localhost", 3306)

	assert.Equal(t, EventInstanceConnected, event.EventName())
	assert.Equal(t, "my-instance", event.AggregateID())
	assert.Equal(t, "DatabaseInstance", event.AggregateType())
	assert.Equal(t, "my-instance", event.InstanceName)
	assert.Equal(t, "default", event.Namespace)
	assert.Equal(t, "mysql", event.Engine)
	assert.Equal(t, "8.0", event.Version)
	assert.Equal(t, "localhost", event.Host)
	assert.Equal(t, 3306, event.Port)
}

func TestInstanceDisconnected(t *testing.T) {
	event := NewInstanceDisconnected("my-instance", "default", "connection timeout")

	assert.Equal(t, EventInstanceDisconnected, event.EventName())
	assert.Equal(t, "my-instance", event.AggregateID())
	assert.Equal(t, "connection timeout", event.Reason)
}

func TestInstanceHealthy(t *testing.T) {
	event := NewInstanceHealthy("my-instance", "default")

	assert.Equal(t, EventInstanceHealthy, event.EventName())
	assert.Equal(t, "my-instance", event.InstanceName)
	assert.Equal(t, "default", event.Namespace)
}

func TestInstanceUnhealthy(t *testing.T) {
	event := NewInstanceUnhealthy("my-instance", "default", "health check failed")

	assert.Equal(t, EventInstanceUnhealthy, event.EventName())
	assert.Equal(t, "health check failed", event.Reason)
}

func TestDatabaseCreated(t *testing.T) {
	event := NewDatabaseCreated("mydb", "instance1", "default", "postgres")

	assert.Equal(t, EventDatabaseCreated, event.EventName())
	assert.Equal(t, "mydb", event.AggregateID())
	assert.Equal(t, "Database", event.AggregateType())
	assert.Equal(t, "mydb", event.DatabaseName)
	assert.Equal(t, "instance1", event.InstanceRef)
	assert.Equal(t, "default", event.Namespace)
	assert.Equal(t, "postgres", event.Engine)
}

func TestDatabaseDeleted(t *testing.T) {
	event := NewDatabaseDeleted("mydb", "instance1", "default")

	assert.Equal(t, EventDatabaseDeleted, event.EventName())
	assert.Equal(t, "mydb", event.DatabaseName)
}

func TestDatabaseUpdated(t *testing.T) {
	changes := []string{"charset", "collation"}
	event := NewDatabaseUpdated("mydb", "instance1", "default", changes)

	assert.Equal(t, EventDatabaseUpdated, event.EventName())
	assert.Equal(t, changes, event.Changes)
}

func TestUserCreated(t *testing.T) {
	event := NewUserCreated("testuser", "instance1", "default", "testuser-credentials")

	assert.Equal(t, EventUserCreated, event.EventName())
	assert.Equal(t, "testuser", event.AggregateID())
	assert.Equal(t, "DatabaseUser", event.AggregateType())
	assert.Equal(t, "testuser", event.Username)
	assert.Equal(t, "instance1", event.InstanceRef)
	assert.Equal(t, "testuser-credentials", event.SecretName)
}

func TestUserDeleted(t *testing.T) {
	event := NewUserDeleted("testuser", "instance1", "default")

	assert.Equal(t, EventUserDeleted, event.EventName())
	assert.Equal(t, "testuser", event.Username)
}

func TestUserUpdated(t *testing.T) {
	changes := []string{"connection_limit"}
	event := NewUserUpdated("testuser", "instance1", "default", changes)

	assert.Equal(t, EventUserUpdated, event.EventName())
	assert.Equal(t, changes, event.Changes)
}

func TestPasswordRotated(t *testing.T) {
	event := NewPasswordRotated("testuser", "default", "testuser-credentials", "scheduled")

	assert.Equal(t, EventPasswordRotated, event.EventName())
	assert.Equal(t, "testuser", event.Username)
	assert.Equal(t, "testuser-credentials", event.SecretName)
	assert.Equal(t, "scheduled", event.Reason)
	assert.WithinDuration(t, time.Now(), event.RotatedAt, time.Second)
}

func TestCredentialsSynced(t *testing.T) {
	event := NewCredentialsSynced("testuser", "default", "testuser-credentials", "v2")

	assert.Equal(t, EventCredentialsSynced, event.EventName())
	assert.Equal(t, "v2", event.SecretVersion)
}

func TestRoleCreated(t *testing.T) {
	event := NewRoleCreated("admin_role", "instance1", "default")

	assert.Equal(t, EventRoleCreated, event.EventName())
	assert.Equal(t, "admin_role", event.AggregateID())
	assert.Equal(t, "DatabaseRole", event.AggregateType())
	assert.Equal(t, "admin_role", event.RoleName)
}

func TestRoleDeleted(t *testing.T) {
	event := NewRoleDeleted("admin_role", "instance1", "default")

	assert.Equal(t, EventRoleDeleted, event.EventName())
	assert.Equal(t, "admin_role", event.RoleName)
}

func TestRoleUpdated(t *testing.T) {
	changes := []string{"grants"}
	event := NewRoleUpdated("admin_role", "instance1", "default", changes)

	assert.Equal(t, EventRoleUpdated, event.EventName())
	assert.Equal(t, changes, event.Changes)
}

func TestGrantApplied(t *testing.T) {
	privileges := []string{"SELECT", "INSERT"}
	event := NewGrantApplied("grant1", "testuser", "mydb", "default", privileges)

	assert.Equal(t, EventGrantApplied, event.EventName())
	assert.Equal(t, "grant1", event.AggregateID())
	assert.Equal(t, "DatabaseGrant", event.AggregateType())
	assert.Equal(t, "testuser", event.UserRef)
	assert.Equal(t, "mydb", event.DatabaseRef)
	assert.Equal(t, privileges, event.Privileges)
}

func TestGrantRevoked(t *testing.T) {
	privileges := []string{"SELECT"}
	event := NewGrantRevoked("grant1", "testuser", "mydb", "default", privileges)

	assert.Equal(t, EventGrantRevoked, event.EventName())
	assert.Equal(t, privileges, event.Privileges)
}

func TestBackupStarted(t *testing.T) {
	event := NewBackupStarted("backup-20260114", "mydb", "default", "s3")

	assert.Equal(t, EventBackupStarted, event.EventName())
	assert.Equal(t, "backup-20260114", event.AggregateID())
	assert.Equal(t, "DatabaseBackup", event.AggregateType())
	assert.Equal(t, "mydb", event.DatabaseRef)
	assert.Equal(t, "s3", event.StorageType)
	assert.WithinDuration(t, time.Now(), event.StartedAt, time.Second)
}

func TestBackupCompleted(t *testing.T) {
	duration := 5 * time.Minute
	event := NewBackupCompleted("backup-20260114", "mydb", "default", "s3://bucket/backup.sql", "sha256:abc123", 1024*1024, duration)

	assert.Equal(t, EventBackupCompleted, event.EventName())
	assert.Equal(t, "s3://bucket/backup.sql", event.StoragePath)
	assert.Equal(t, "sha256:abc123", event.Checksum)
	assert.Equal(t, int64(1024*1024), event.SizeBytes)
	assert.Equal(t, duration, event.Duration)
}

func TestBackupFailed(t *testing.T) {
	event := NewBackupFailed("backup-20260114", "mydb", "default", "disk full")

	assert.Equal(t, EventBackupFailed, event.EventName())
	assert.Equal(t, "disk full", event.Error)
	assert.WithinDuration(t, time.Now(), event.FailedAt, time.Second)
}

func TestRestoreStarted(t *testing.T) {
	event := NewRestoreStarted("restore-20260114", "backup-20260114", "mydb", "default")

	assert.Equal(t, EventRestoreStarted, event.EventName())
	assert.Equal(t, "restore-20260114", event.AggregateID())
	assert.Equal(t, "DatabaseRestore", event.AggregateType())
	assert.Equal(t, "backup-20260114", event.BackupRef)
	assert.WithinDuration(t, time.Now(), event.StartedAt, time.Second)
}

func TestRestoreCompleted(t *testing.T) {
	duration := 10 * time.Minute
	event := NewRestoreCompleted("restore-20260114", "backup-20260114", "mydb", "default", duration)

	assert.Equal(t, EventRestoreCompleted, event.EventName())
	assert.Equal(t, duration, event.Duration)
}
