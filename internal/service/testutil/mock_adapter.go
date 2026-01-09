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

package testutil

import (
	"context"

	"github.com/db-provision-operator/internal/adapter/types"
)

// MockAdapter is a mock implementation of the DatabaseAdapter interface for testing.
// Each method has a configurable function field that can be set to customize behavior.
type MockAdapter struct {
	// Connection management
	ConnectFunc    func(ctx context.Context) error
	CloseFunc      func() error
	PingFunc       func(ctx context.Context) error
	GetVersionFunc func(ctx context.Context) (string, error)

	// Database operations
	CreateDatabaseFunc       func(ctx context.Context, opts types.CreateDatabaseOptions) error
	DropDatabaseFunc         func(ctx context.Context, name string, opts types.DropDatabaseOptions) error
	DatabaseExistsFunc       func(ctx context.Context, name string) (bool, error)
	GetDatabaseInfoFunc      func(ctx context.Context, name string) (*types.DatabaseInfo, error)
	UpdateDatabaseFunc       func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error
	VerifyDatabaseAccessFunc func(ctx context.Context, name string) error

	// User operations
	CreateUserFunc     func(ctx context.Context, opts types.CreateUserOptions) error
	DropUserFunc       func(ctx context.Context, username string) error
	UserExistsFunc     func(ctx context.Context, username string) (bool, error)
	UpdateUserFunc     func(ctx context.Context, username string, opts types.UpdateUserOptions) error
	UpdatePasswordFunc func(ctx context.Context, username, password string) error
	GetUserInfoFunc    func(ctx context.Context, username string) (*types.UserInfo, error)

	// Role operations
	CreateRoleFunc  func(ctx context.Context, opts types.CreateRoleOptions) error
	DropRoleFunc    func(ctx context.Context, roleName string) error
	RoleExistsFunc  func(ctx context.Context, roleName string) (bool, error)
	UpdateRoleFunc  func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error
	GetRoleInfoFunc func(ctx context.Context, roleName string) (*types.RoleInfo, error)

	// Grant operations
	GrantFunc                func(ctx context.Context, grantee string, opts []types.GrantOptions) error
	RevokeFunc               func(ctx context.Context, grantee string, opts []types.GrantOptions) error
	GrantRoleFunc            func(ctx context.Context, grantee string, roles []string) error
	RevokeRoleFunc           func(ctx context.Context, grantee string, roles []string) error
	SetDefaultPrivilegesFunc func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error
	GetGrantsFunc            func(ctx context.Context, grantee string) ([]types.GrantInfo, error)

	// Backup operations
	BackupFunc            func(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error)
	GetBackupProgressFunc func(ctx context.Context, backupID string) (int, error)
	CancelBackupFunc      func(ctx context.Context, backupID string) error

	// Restore operations
	RestoreFunc            func(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error)
	GetRestoreProgressFunc func(ctx context.Context, restoreID string) (int, error)
	CancelRestoreFunc      func(ctx context.Context, restoreID string) error

	// Call tracking
	Calls []MethodCall
}

// MethodCall records a method call for verification in tests.
type MethodCall struct {
	Method string
	Args   []interface{}
}

// NewMockAdapter creates a new MockAdapter with default no-op implementations.
func NewMockAdapter() *MockAdapter {
	m := &MockAdapter{
		Calls: make([]MethodCall, 0),
	}

	// Set default implementations
	m.ConnectFunc = func(ctx context.Context) error { return nil }
	m.CloseFunc = func() error { return nil }
	m.PingFunc = func(ctx context.Context) error { return nil }
	m.GetVersionFunc = func(ctx context.Context) (string, error) { return "15.0", nil }

	m.CreateDatabaseFunc = func(ctx context.Context, opts types.CreateDatabaseOptions) error { return nil }
	m.DropDatabaseFunc = func(ctx context.Context, name string, opts types.DropDatabaseOptions) error { return nil }
	m.DatabaseExistsFunc = func(ctx context.Context, name string) (bool, error) { return false, nil }
	m.GetDatabaseInfoFunc = func(ctx context.Context, name string) (*types.DatabaseInfo, error) {
		return &types.DatabaseInfo{Name: name}, nil
	}
	m.UpdateDatabaseFunc = func(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error { return nil }
	m.VerifyDatabaseAccessFunc = func(ctx context.Context, name string) error { return nil }

	m.CreateUserFunc = func(ctx context.Context, opts types.CreateUserOptions) error { return nil }
	m.DropUserFunc = func(ctx context.Context, username string) error { return nil }
	m.UserExistsFunc = func(ctx context.Context, username string) (bool, error) { return false, nil }
	m.UpdateUserFunc = func(ctx context.Context, username string, opts types.UpdateUserOptions) error { return nil }
	m.UpdatePasswordFunc = func(ctx context.Context, username, password string) error { return nil }
	m.GetUserInfoFunc = func(ctx context.Context, username string) (*types.UserInfo, error) {
		return &types.UserInfo{Username: username}, nil
	}

	m.CreateRoleFunc = func(ctx context.Context, opts types.CreateRoleOptions) error { return nil }
	m.DropRoleFunc = func(ctx context.Context, roleName string) error { return nil }
	m.RoleExistsFunc = func(ctx context.Context, roleName string) (bool, error) { return false, nil }
	m.UpdateRoleFunc = func(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error { return nil }
	m.GetRoleInfoFunc = func(ctx context.Context, roleName string) (*types.RoleInfo, error) {
		return &types.RoleInfo{Name: roleName}, nil
	}

	m.GrantFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error { return nil }
	m.RevokeFunc = func(ctx context.Context, grantee string, opts []types.GrantOptions) error { return nil }
	m.GrantRoleFunc = func(ctx context.Context, grantee string, roles []string) error { return nil }
	m.RevokeRoleFunc = func(ctx context.Context, grantee string, roles []string) error { return nil }
	m.SetDefaultPrivilegesFunc = func(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
		return nil
	}
	m.GetGrantsFunc = func(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
		return []types.GrantInfo{}, nil
	}

	m.BackupFunc = func(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
		return &types.BackupResult{BackupID: opts.BackupID}, nil
	}
	m.GetBackupProgressFunc = func(ctx context.Context, backupID string) (int, error) { return 100, nil }
	m.CancelBackupFunc = func(ctx context.Context, backupID string) error { return nil }

	m.RestoreFunc = func(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
		return &types.RestoreResult{RestoreID: opts.RestoreID}, nil
	}
	m.GetRestoreProgressFunc = func(ctx context.Context, restoreID string) (int, error) { return 100, nil }
	m.CancelRestoreFunc = func(ctx context.Context, restoreID string) error { return nil }

	return m
}

// record adds a method call to the call tracking list.
func (m *MockAdapter) record(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MethodCall{Method: method, Args: args})
}

// ResetCalls clears the call tracking list.
func (m *MockAdapter) ResetCalls() {
	m.Calls = make([]MethodCall, 0)
}

// GetCallCount returns the number of times a method was called.
func (m *MockAdapter) GetCallCount(method string) int {
	count := 0
	for _, call := range m.Calls {
		if call.Method == method {
			count++
		}
	}
	return count
}

// WasCalledWith checks if a method was called with specific arguments.
func (m *MockAdapter) WasCalledWith(method string, args ...interface{}) bool {
	for _, call := range m.Calls {
		if call.Method != method {
			continue
		}
		if len(call.Args) != len(args) {
			continue
		}
		match := true
		for i, arg := range args {
			if call.Args[i] != arg {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Connection management implementations
func (m *MockAdapter) Connect(ctx context.Context) error {
	m.record("Connect")
	return m.ConnectFunc(ctx)
}

func (m *MockAdapter) Close() error {
	m.record("Close")
	return m.CloseFunc()
}

func (m *MockAdapter) Ping(ctx context.Context) error {
	m.record("Ping")
	return m.PingFunc(ctx)
}

func (m *MockAdapter) GetVersion(ctx context.Context) (string, error) {
	m.record("GetVersion")
	return m.GetVersionFunc(ctx)
}

// Database operations implementations
func (m *MockAdapter) CreateDatabase(ctx context.Context, opts types.CreateDatabaseOptions) error {
	m.record("CreateDatabase", opts.Name)
	return m.CreateDatabaseFunc(ctx, opts)
}

func (m *MockAdapter) DropDatabase(ctx context.Context, name string, opts types.DropDatabaseOptions) error {
	m.record("DropDatabase", name)
	return m.DropDatabaseFunc(ctx, name, opts)
}

func (m *MockAdapter) DatabaseExists(ctx context.Context, name string) (bool, error) {
	m.record("DatabaseExists", name)
	return m.DatabaseExistsFunc(ctx, name)
}

func (m *MockAdapter) GetDatabaseInfo(ctx context.Context, name string) (*types.DatabaseInfo, error) {
	m.record("GetDatabaseInfo", name)
	return m.GetDatabaseInfoFunc(ctx, name)
}

func (m *MockAdapter) UpdateDatabase(ctx context.Context, name string, opts types.UpdateDatabaseOptions) error {
	m.record("UpdateDatabase", name)
	return m.UpdateDatabaseFunc(ctx, name, opts)
}

func (m *MockAdapter) VerifyDatabaseAccess(ctx context.Context, name string) error {
	m.record("VerifyDatabaseAccess", name)
	return m.VerifyDatabaseAccessFunc(ctx, name)
}

// User operations implementations
func (m *MockAdapter) CreateUser(ctx context.Context, opts types.CreateUserOptions) error {
	m.record("CreateUser", opts.Username)
	return m.CreateUserFunc(ctx, opts)
}

func (m *MockAdapter) DropUser(ctx context.Context, username string) error {
	m.record("DropUser", username)
	return m.DropUserFunc(ctx, username)
}

func (m *MockAdapter) UserExists(ctx context.Context, username string) (bool, error) {
	m.record("UserExists", username)
	return m.UserExistsFunc(ctx, username)
}

func (m *MockAdapter) UpdateUser(ctx context.Context, username string, opts types.UpdateUserOptions) error {
	m.record("UpdateUser", username)
	return m.UpdateUserFunc(ctx, username, opts)
}

func (m *MockAdapter) UpdatePassword(ctx context.Context, username, password string) error {
	m.record("UpdatePassword", username)
	return m.UpdatePasswordFunc(ctx, username, password)
}

func (m *MockAdapter) GetUserInfo(ctx context.Context, username string) (*types.UserInfo, error) {
	m.record("GetUserInfo", username)
	return m.GetUserInfoFunc(ctx, username)
}

// Role operations implementations
func (m *MockAdapter) CreateRole(ctx context.Context, opts types.CreateRoleOptions) error {
	m.record("CreateRole", opts.RoleName)
	return m.CreateRoleFunc(ctx, opts)
}

func (m *MockAdapter) DropRole(ctx context.Context, roleName string) error {
	m.record("DropRole", roleName)
	return m.DropRoleFunc(ctx, roleName)
}

func (m *MockAdapter) RoleExists(ctx context.Context, roleName string) (bool, error) {
	m.record("RoleExists", roleName)
	return m.RoleExistsFunc(ctx, roleName)
}

func (m *MockAdapter) UpdateRole(ctx context.Context, roleName string, opts types.UpdateRoleOptions) error {
	m.record("UpdateRole", roleName)
	return m.UpdateRoleFunc(ctx, roleName, opts)
}

func (m *MockAdapter) GetRoleInfo(ctx context.Context, roleName string) (*types.RoleInfo, error) {
	m.record("GetRoleInfo", roleName)
	return m.GetRoleInfoFunc(ctx, roleName)
}

// Grant operations implementations
func (m *MockAdapter) Grant(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	m.record("Grant", grantee)
	return m.GrantFunc(ctx, grantee, opts)
}

func (m *MockAdapter) Revoke(ctx context.Context, grantee string, opts []types.GrantOptions) error {
	m.record("Revoke", grantee)
	return m.RevokeFunc(ctx, grantee, opts)
}

func (m *MockAdapter) GrantRole(ctx context.Context, grantee string, roles []string) error {
	m.record("GrantRole", grantee, roles)
	return m.GrantRoleFunc(ctx, grantee, roles)
}

func (m *MockAdapter) RevokeRole(ctx context.Context, grantee string, roles []string) error {
	m.record("RevokeRole", grantee, roles)
	return m.RevokeRoleFunc(ctx, grantee, roles)
}

func (m *MockAdapter) SetDefaultPrivileges(ctx context.Context, grantee string, opts []types.DefaultPrivilegeGrantOptions) error {
	m.record("SetDefaultPrivileges", grantee)
	return m.SetDefaultPrivilegesFunc(ctx, grantee, opts)
}

func (m *MockAdapter) GetGrants(ctx context.Context, grantee string) ([]types.GrantInfo, error) {
	m.record("GetGrants", grantee)
	return m.GetGrantsFunc(ctx, grantee)
}

// Backup operations implementations
func (m *MockAdapter) Backup(ctx context.Context, opts types.BackupOptions) (*types.BackupResult, error) {
	m.record("Backup", opts.BackupID)
	return m.BackupFunc(ctx, opts)
}

func (m *MockAdapter) GetBackupProgress(ctx context.Context, backupID string) (int, error) {
	m.record("GetBackupProgress", backupID)
	return m.GetBackupProgressFunc(ctx, backupID)
}

func (m *MockAdapter) CancelBackup(ctx context.Context, backupID string) error {
	m.record("CancelBackup", backupID)
	return m.CancelBackupFunc(ctx, backupID)
}

// Restore operations implementations
func (m *MockAdapter) Restore(ctx context.Context, opts types.RestoreOptions) (*types.RestoreResult, error) {
	m.record("Restore", opts.RestoreID)
	return m.RestoreFunc(ctx, opts)
}

func (m *MockAdapter) GetRestoreProgress(ctx context.Context, restoreID string) (int, error) {
	m.record("GetRestoreProgress", restoreID)
	return m.GetRestoreProgressFunc(ctx, restoreID)
}

func (m *MockAdapter) CancelRestore(ctx context.Context, restoreID string) error {
	m.record("CancelRestore", restoreID)
	return m.CancelRestoreFunc(ctx, restoreID)
}

// Verify MockAdapter implements DatabaseAdapter interface
var _ types.DatabaseAdapter = (*MockAdapter)(nil)
