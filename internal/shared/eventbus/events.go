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
	"time"
)

// Event names as constants for type safety and documentation.
const (
	// Instance events
	EventInstanceConnected    = "InstanceConnected"
	EventInstanceDisconnected = "InstanceDisconnected"
	EventInstanceHealthy      = "InstanceHealthy"
	EventInstanceUnhealthy    = "InstanceUnhealthy"

	// Database events
	EventDatabaseCreated = "DatabaseCreated"
	EventDatabaseDeleted = "DatabaseDeleted"
	EventDatabaseUpdated = "DatabaseUpdated"

	// User events
	EventUserCreated       = "UserCreated"
	EventUserDeleted       = "UserDeleted"
	EventUserUpdated       = "UserUpdated"
	EventPasswordRotated   = "PasswordRotated"
	EventCredentialsSynced = "CredentialsSynced"

	// Role events
	EventRoleCreated = "RoleCreated"
	EventRoleDeleted = "RoleDeleted"
	EventRoleUpdated = "RoleUpdated"

	// Cluster Role events (cluster-scoped)
	EventClusterRoleCreated = "ClusterRoleCreated"
	EventClusterRoleDeleted = "ClusterRoleDeleted"
	EventClusterRoleUpdated = "ClusterRoleUpdated"

	// Grant events
	EventGrantApplied = "GrantApplied"
	EventGrantRevoked = "GrantRevoked"

	// Cluster Grant events (cluster-scoped)
	EventClusterGrantApplied = "ClusterGrantApplied"
	EventClusterGrantRevoked = "ClusterGrantRevoked"

	// Backup events
	EventBackupStarted    = "BackupStarted"
	EventBackupCompleted  = "BackupCompleted"
	EventBackupFailed     = "BackupFailed"
	EventRestoreStarted   = "RestoreStarted"
	EventRestoreCompleted = "RestoreCompleted"
	EventRestoreFailed    = "RestoreFailed"
)

// BaseEvent provides common event fields.
// Embed this struct in concrete event types.
type BaseEvent struct {
	name          string
	timestamp     time.Time
	aggregateID   string
	aggregateType string
}

// NewBaseEvent creates a new base event with current timestamp.
func NewBaseEvent(name, aggregateID, aggregateType string) BaseEvent {
	return BaseEvent{
		name:          name,
		timestamp:     time.Now(),
		aggregateID:   aggregateID,
		aggregateType: aggregateType,
	}
}

func (e BaseEvent) EventName() string     { return e.name }
func (e BaseEvent) EventTime() time.Time  { return e.timestamp }
func (e BaseEvent) AggregateID() string   { return e.aggregateID }
func (e BaseEvent) AggregateType() string { return e.aggregateType }

// ============================================================================
// Instance Events
// ============================================================================

// InstanceConnected is published when a DatabaseInstance successfully connects.
type InstanceConnected struct {
	BaseEvent
	InstanceName string
	Namespace    string
	Engine       string
	Version      string
	Host         string
	Port         int
}

// NewInstanceConnected creates a new InstanceConnected event.
func NewInstanceConnected(instanceName, namespace, engine, version, host string, port int) *InstanceConnected {
	return &InstanceConnected{
		BaseEvent:    NewBaseEvent(EventInstanceConnected, instanceName, "DatabaseInstance"),
		InstanceName: instanceName,
		Namespace:    namespace,
		Engine:       engine,
		Version:      version,
		Host:         host,
		Port:         port,
	}
}

// InstanceDisconnected is published when a DatabaseInstance loses connection.
type InstanceDisconnected struct {
	BaseEvent
	InstanceName string
	Namespace    string
	Reason       string
}

// NewInstanceDisconnected creates a new InstanceDisconnected event.
func NewInstanceDisconnected(instanceName, namespace, reason string) *InstanceDisconnected {
	return &InstanceDisconnected{
		BaseEvent:    NewBaseEvent(EventInstanceDisconnected, instanceName, "DatabaseInstance"),
		InstanceName: instanceName,
		Namespace:    namespace,
		Reason:       reason,
	}
}

// InstanceHealthy is published when health check passes.
type InstanceHealthy struct {
	BaseEvent
	InstanceName string
	Namespace    string
}

// NewInstanceHealthy creates a new InstanceHealthy event.
func NewInstanceHealthy(instanceName, namespace string) *InstanceHealthy {
	return &InstanceHealthy{
		BaseEvent:    NewBaseEvent(EventInstanceHealthy, instanceName, "DatabaseInstance"),
		InstanceName: instanceName,
		Namespace:    namespace,
	}
}

// InstanceUnhealthy is published when health check fails.
type InstanceUnhealthy struct {
	BaseEvent
	InstanceName string
	Namespace    string
	Reason       string
}

// NewInstanceUnhealthy creates a new InstanceUnhealthy event.
func NewInstanceUnhealthy(instanceName, namespace, reason string) *InstanceUnhealthy {
	return &InstanceUnhealthy{
		BaseEvent:    NewBaseEvent(EventInstanceUnhealthy, instanceName, "DatabaseInstance"),
		InstanceName: instanceName,
		Namespace:    namespace,
		Reason:       reason,
	}
}

// ============================================================================
// Database Events
// ============================================================================

// DatabaseCreated is published when a Database is successfully created.
type DatabaseCreated struct {
	BaseEvent
	DatabaseName string
	InstanceRef  string
	Namespace    string
	Engine       string
}

// NewDatabaseCreated creates a new DatabaseCreated event.
func NewDatabaseCreated(databaseName, instanceRef, namespace, engine string) *DatabaseCreated {
	return &DatabaseCreated{
		BaseEvent:    NewBaseEvent(EventDatabaseCreated, databaseName, "Database"),
		DatabaseName: databaseName,
		InstanceRef:  instanceRef,
		Namespace:    namespace,
		Engine:       engine,
	}
}

// DatabaseDeleted is published when a Database is deleted.
type DatabaseDeleted struct {
	BaseEvent
	DatabaseName string
	InstanceRef  string
	Namespace    string
}

// NewDatabaseDeleted creates a new DatabaseDeleted event.
func NewDatabaseDeleted(databaseName, instanceRef, namespace string) *DatabaseDeleted {
	return &DatabaseDeleted{
		BaseEvent:    NewBaseEvent(EventDatabaseDeleted, databaseName, "Database"),
		DatabaseName: databaseName,
		InstanceRef:  instanceRef,
		Namespace:    namespace,
	}
}

// DatabaseUpdated is published when a Database is modified.
type DatabaseUpdated struct {
	BaseEvent
	DatabaseName string
	InstanceRef  string
	Namespace    string
	Changes      []string // List of what changed
}

// NewDatabaseUpdated creates a new DatabaseUpdated event.
func NewDatabaseUpdated(databaseName, instanceRef, namespace string, changes []string) *DatabaseUpdated {
	return &DatabaseUpdated{
		BaseEvent:    NewBaseEvent(EventDatabaseUpdated, databaseName, "Database"),
		DatabaseName: databaseName,
		InstanceRef:  instanceRef,
		Namespace:    namespace,
		Changes:      changes,
	}
}

// ============================================================================
// User Events
// ============================================================================

// UserCreated is published when a DatabaseUser is successfully created.
type UserCreated struct {
	BaseEvent
	Username    string
	InstanceRef string
	Namespace   string
	SecretName  string
}

// NewUserCreated creates a new UserCreated event.
func NewUserCreated(username, instanceRef, namespace, secretName string) *UserCreated {
	return &UserCreated{
		BaseEvent:   NewBaseEvent(EventUserCreated, username, "DatabaseUser"),
		Username:    username,
		InstanceRef: instanceRef,
		Namespace:   namespace,
		SecretName:  secretName,
	}
}

// UserDeleted is published when a DatabaseUser is deleted.
type UserDeleted struct {
	BaseEvent
	Username    string
	InstanceRef string
	Namespace   string
}

// NewUserDeleted creates a new UserDeleted event.
func NewUserDeleted(username, instanceRef, namespace string) *UserDeleted {
	return &UserDeleted{
		BaseEvent:   NewBaseEvent(EventUserDeleted, username, "DatabaseUser"),
		Username:    username,
		InstanceRef: instanceRef,
		Namespace:   namespace,
	}
}

// UserUpdated is published when a DatabaseUser is modified.
type UserUpdated struct {
	BaseEvent
	Username    string
	InstanceRef string
	Namespace   string
	Changes     []string
}

// NewUserUpdated creates a new UserUpdated event.
func NewUserUpdated(username, instanceRef, namespace string, changes []string) *UserUpdated {
	return &UserUpdated{
		BaseEvent:   NewBaseEvent(EventUserUpdated, username, "DatabaseUser"),
		Username:    username,
		InstanceRef: instanceRef,
		Namespace:   namespace,
		Changes:     changes,
	}
}

// PasswordRotated is published when a user's password is changed.
type PasswordRotated struct {
	BaseEvent
	Username   string
	Namespace  string
	SecretName string
	RotatedAt  time.Time
	Reason     string // "manual", "scheduled", "policy"
}

// NewPasswordRotated creates a new PasswordRotated event.
func NewPasswordRotated(username, namespace, secretName, reason string) *PasswordRotated {
	return &PasswordRotated{
		BaseEvent:  NewBaseEvent(EventPasswordRotated, username, "DatabaseUser"),
		Username:   username,
		Namespace:  namespace,
		SecretName: secretName,
		RotatedAt:  time.Now(),
		Reason:     reason,
	}
}

// CredentialsSynced is published when credentials are synced to a secret.
type CredentialsSynced struct {
	BaseEvent
	Username      string
	Namespace     string
	SecretName    string
	SecretVersion string
}

// NewCredentialsSynced creates a new CredentialsSynced event.
func NewCredentialsSynced(username, namespace, secretName, secretVersion string) *CredentialsSynced {
	return &CredentialsSynced{
		BaseEvent:     NewBaseEvent(EventCredentialsSynced, username, "DatabaseUser"),
		Username:      username,
		Namespace:     namespace,
		SecretName:    secretName,
		SecretVersion: secretVersion,
	}
}

// ============================================================================
// Role Events
// ============================================================================

// RoleCreated is published when a DatabaseRole is successfully created.
type RoleCreated struct {
	BaseEvent
	RoleName    string
	InstanceRef string
	Namespace   string
}

// NewRoleCreated creates a new RoleCreated event.
func NewRoleCreated(roleName, instanceRef, namespace string) *RoleCreated {
	return &RoleCreated{
		BaseEvent:   NewBaseEvent(EventRoleCreated, roleName, "DatabaseRole"),
		RoleName:    roleName,
		InstanceRef: instanceRef,
		Namespace:   namespace,
	}
}

// RoleDeleted is published when a DatabaseRole is deleted.
type RoleDeleted struct {
	BaseEvent
	RoleName    string
	InstanceRef string
	Namespace   string
}

// NewRoleDeleted creates a new RoleDeleted event.
func NewRoleDeleted(roleName, instanceRef, namespace string) *RoleDeleted {
	return &RoleDeleted{
		BaseEvent:   NewBaseEvent(EventRoleDeleted, roleName, "DatabaseRole"),
		RoleName:    roleName,
		InstanceRef: instanceRef,
		Namespace:   namespace,
	}
}

// RoleUpdated is published when a DatabaseRole is modified.
type RoleUpdated struct {
	BaseEvent
	RoleName    string
	InstanceRef string
	Namespace   string
	Changes     []string
}

// NewRoleUpdated creates a new RoleUpdated event.
func NewRoleUpdated(roleName, instanceRef, namespace string, changes []string) *RoleUpdated {
	return &RoleUpdated{
		BaseEvent:   NewBaseEvent(EventRoleUpdated, roleName, "DatabaseRole"),
		RoleName:    roleName,
		InstanceRef: instanceRef,
		Namespace:   namespace,
		Changes:     changes,
	}
}

// ============================================================================
// Cluster Role Events (cluster-scoped)
// ============================================================================

// ClusterRoleCreated is published when a ClusterDatabaseRole is successfully created.
type ClusterRoleCreated struct {
	BaseEvent
	RoleName           string
	ClusterInstanceRef string
}

// NewClusterRoleCreated creates a new ClusterRoleCreated event.
func NewClusterRoleCreated(roleName, clusterInstanceRef string) *ClusterRoleCreated {
	return &ClusterRoleCreated{
		BaseEvent:          NewBaseEvent(EventClusterRoleCreated, roleName, "ClusterDatabaseRole"),
		RoleName:           roleName,
		ClusterInstanceRef: clusterInstanceRef,
	}
}

// ClusterRoleDeleted is published when a ClusterDatabaseRole is deleted.
type ClusterRoleDeleted struct {
	BaseEvent
	RoleName           string
	ClusterInstanceRef string
}

// NewClusterRoleDeleted creates a new ClusterRoleDeleted event.
func NewClusterRoleDeleted(roleName, clusterInstanceRef string) *ClusterRoleDeleted {
	return &ClusterRoleDeleted{
		BaseEvent:          NewBaseEvent(EventClusterRoleDeleted, roleName, "ClusterDatabaseRole"),
		RoleName:           roleName,
		ClusterInstanceRef: clusterInstanceRef,
	}
}

// ClusterRoleUpdated is published when a ClusterDatabaseRole is modified.
type ClusterRoleUpdated struct {
	BaseEvent
	RoleName           string
	ClusterInstanceRef string
	Changes            []string
}

// NewClusterRoleUpdated creates a new ClusterRoleUpdated event.
func NewClusterRoleUpdated(roleName, clusterInstanceRef string, changes []string) *ClusterRoleUpdated {
	return &ClusterRoleUpdated{
		BaseEvent:          NewBaseEvent(EventClusterRoleUpdated, roleName, "ClusterDatabaseRole"),
		RoleName:           roleName,
		ClusterInstanceRef: clusterInstanceRef,
		Changes:            changes,
	}
}

// ============================================================================
// Grant Events
// ============================================================================

// GrantApplied is published when a DatabaseGrant is successfully applied.
type GrantApplied struct {
	BaseEvent
	GrantName   string
	UserRef     string
	DatabaseRef string
	Namespace   string
	Privileges  []string
}

// NewGrantApplied creates a new GrantApplied event.
func NewGrantApplied(grantName, userRef, databaseRef, namespace string, privileges []string) *GrantApplied {
	return &GrantApplied{
		BaseEvent:   NewBaseEvent(EventGrantApplied, grantName, "DatabaseGrant"),
		GrantName:   grantName,
		UserRef:     userRef,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		Privileges:  privileges,
	}
}

// GrantRevoked is published when a DatabaseGrant is revoked.
type GrantRevoked struct {
	BaseEvent
	GrantName   string
	UserRef     string
	DatabaseRef string
	Namespace   string
	Privileges  []string
}

// NewGrantRevoked creates a new GrantRevoked event.
func NewGrantRevoked(grantName, userRef, databaseRef, namespace string, privileges []string) *GrantRevoked {
	return &GrantRevoked{
		BaseEvent:   NewBaseEvent(EventGrantRevoked, grantName, "DatabaseGrant"),
		GrantName:   grantName,
		UserRef:     userRef,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		Privileges:  privileges,
	}
}

// ============================================================================
// Backup Events
// ============================================================================

// BackupStarted is published when a backup operation begins.
type BackupStarted struct {
	BaseEvent
	BackupName  string
	DatabaseRef string
	Namespace   string
	StorageType string
	StartedAt   time.Time
}

// NewBackupStarted creates a new BackupStarted event.
func NewBackupStarted(backupName, databaseRef, namespace, storageType string) *BackupStarted {
	return &BackupStarted{
		BaseEvent:   NewBaseEvent(EventBackupStarted, backupName, "DatabaseBackup"),
		BackupName:  backupName,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		StorageType: storageType,
		StartedAt:   time.Now(),
	}
}

// BackupCompleted is published when a backup completes successfully.
type BackupCompleted struct {
	BaseEvent
	BackupName  string
	DatabaseRef string
	Namespace   string
	SizeBytes   int64
	Duration    time.Duration
	StoragePath string
	Checksum    string
}

// NewBackupCompleted creates a new BackupCompleted event.
func NewBackupCompleted(backupName, databaseRef, namespace, storagePath, checksum string, sizeBytes int64, duration time.Duration) *BackupCompleted {
	return &BackupCompleted{
		BaseEvent:   NewBaseEvent(EventBackupCompleted, backupName, "DatabaseBackup"),
		BackupName:  backupName,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		SizeBytes:   sizeBytes,
		Duration:    duration,
		StoragePath: storagePath,
		Checksum:    checksum,
	}
}

// BackupFailed is published when a backup operation fails.
type BackupFailed struct {
	BaseEvent
	BackupName  string
	DatabaseRef string
	Namespace   string
	Error       string
	FailedAt    time.Time
}

// NewBackupFailed creates a new BackupFailed event.
func NewBackupFailed(backupName, databaseRef, namespace, errMsg string) *BackupFailed {
	return &BackupFailed{
		BaseEvent:   NewBaseEvent(EventBackupFailed, backupName, "DatabaseBackup"),
		BackupName:  backupName,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		Error:       errMsg,
		FailedAt:    time.Now(),
	}
}

// RestoreStarted is published when a restore operation begins.
type RestoreStarted struct {
	BaseEvent
	RestoreName string
	BackupRef   string
	DatabaseRef string
	Namespace   string
	StartedAt   time.Time
}

// NewRestoreStarted creates a new RestoreStarted event.
func NewRestoreStarted(restoreName, backupRef, databaseRef, namespace string) *RestoreStarted {
	return &RestoreStarted{
		BaseEvent:   NewBaseEvent(EventRestoreStarted, restoreName, "DatabaseRestore"),
		RestoreName: restoreName,
		BackupRef:   backupRef,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		StartedAt:   time.Now(),
	}
}

// RestoreCompleted is published when a restore completes successfully.
type RestoreCompleted struct {
	BaseEvent
	RestoreName string
	BackupRef   string
	DatabaseRef string
	Namespace   string
	Duration    time.Duration
}

// NewRestoreCompleted creates a new RestoreCompleted event.
func NewRestoreCompleted(restoreName, backupRef, databaseRef, namespace string, duration time.Duration) *RestoreCompleted {
	return &RestoreCompleted{
		BaseEvent:   NewBaseEvent(EventRestoreCompleted, restoreName, "DatabaseRestore"),
		RestoreName: restoreName,
		BackupRef:   backupRef,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		Duration:    duration,
	}
}

// RestoreFailed is published when a restore operation fails.
type RestoreFailed struct {
	BaseEvent
	RestoreName string
	BackupRef   string
	DatabaseRef string
	Namespace   string
	Error       string
	FailedAt    time.Time
}

// NewRestoreFailed creates a new RestoreFailed event.
func NewRestoreFailed(restoreName, backupRef, databaseRef, namespace, errMsg string) *RestoreFailed {
	return &RestoreFailed{
		BaseEvent:   NewBaseEvent(EventRestoreFailed, restoreName, "DatabaseRestore"),
		RestoreName: restoreName,
		BackupRef:   backupRef,
		DatabaseRef: databaseRef,
		Namespace:   namespace,
		Error:       errMsg,
		FailedAt:    time.Now(),
	}
}

// ============================================================================
// Cluster Grant Events (cluster-scoped)
// ============================================================================

// ClusterGrantApplied is published when a ClusterDatabaseGrant is successfully applied.
type ClusterGrantApplied struct {
	BaseEvent
	GrantName          string
	TargetRef          string // User or role name
	TargetNamespace    string // Namespace of the target (empty for cluster-scoped roles)
	TargetType         string // "user" or "role"
	ClusterInstanceRef string
	Privileges         []string
}

// NewClusterGrantApplied creates a new ClusterGrantApplied event.
func NewClusterGrantApplied(grantName, targetRef, targetNamespace, targetType, clusterInstanceRef string, privileges []string) *ClusterGrantApplied {
	return &ClusterGrantApplied{
		BaseEvent:          NewBaseEvent(EventClusterGrantApplied, grantName, "ClusterDatabaseGrant"),
		GrantName:          grantName,
		TargetRef:          targetRef,
		TargetNamespace:    targetNamespace,
		TargetType:         targetType,
		ClusterInstanceRef: clusterInstanceRef,
		Privileges:         privileges,
	}
}

// ClusterGrantRevoked is published when a ClusterDatabaseGrant is revoked.
type ClusterGrantRevoked struct {
	BaseEvent
	GrantName          string
	TargetRef          string
	TargetNamespace    string
	TargetType         string
	ClusterInstanceRef string
	Privileges         []string
}

// NewClusterGrantRevoked creates a new ClusterGrantRevoked event.
func NewClusterGrantRevoked(grantName, targetRef, targetNamespace, targetType, clusterInstanceRef string, privileges []string) *ClusterGrantRevoked {
	return &ClusterGrantRevoked{
		BaseEvent:          NewBaseEvent(EventClusterGrantRevoked, grantName, "ClusterDatabaseGrant"),
		GrantName:          grantName,
		TargetRef:          targetRef,
		TargetNamespace:    targetNamespace,
		TargetType:         targetType,
		ClusterInstanceRef: clusterInstanceRef,
		Privileges:         privileges,
	}
}
