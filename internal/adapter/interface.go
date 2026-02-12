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

package adapter

import (
	"github.com/db-provision-operator/internal/adapter/types"
)

// Re-export all types from the types package for backward compatibility
// This allows existing code to continue importing from the adapter package
type (
	DatabaseAdapter              = types.DatabaseAdapter
	DatabaseManager              = types.DatabaseManager
	UserManager                  = types.UserManager
	RoleManager                  = types.RoleManager
	GrantManager                 = types.GrantManager
	BackupManager                = types.BackupManager
	RestoreManager               = types.RestoreManager
	ResourceDiscovery            = types.ResourceDiscovery
	ResourceTracker              = types.ResourceTracker
	TrackingMetadata             = types.TrackingMetadata
	ConnectionConfig             = types.ConnectionConfig
	CreateDatabaseOptions        = types.CreateDatabaseOptions
	DropDatabaseOptions          = types.DropDatabaseOptions
	UpdateDatabaseOptions        = types.UpdateDatabaseOptions
	ExtensionOptions             = types.ExtensionOptions
	SchemaOptions                = types.SchemaOptions
	DefaultPrivilegeOptions      = types.DefaultPrivilegeOptions
	DatabaseInfo                 = types.DatabaseInfo
	ExtensionInfo                = types.ExtensionInfo
	CreateUserOptions            = types.CreateUserOptions
	UpdateUserOptions            = types.UpdateUserOptions
	UserInfo                     = types.UserInfo
	CreateRoleOptions            = types.CreateRoleOptions
	UpdateRoleOptions            = types.UpdateRoleOptions
	RoleInfo                     = types.RoleInfo
	GrantOptions                 = types.GrantOptions
	DefaultPrivilegeGrantOptions = types.DefaultPrivilegeGrantOptions
	GrantInfo                    = types.GrantInfo
	BackupOptions                = types.BackupOptions
	BackupResult                 = types.BackupResult
	RestoreOptions               = types.RestoreOptions
	RestoreResult                = types.RestoreResult
)
