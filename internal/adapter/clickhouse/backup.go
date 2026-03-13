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

package clickhouse

import (
	"context"
	"fmt"

	"github.com/db-provision-operator/internal/adapter/types"
)

// Backup is not yet supported for ClickHouse.
func (a *Adapter) Backup(_ context.Context, _ types.BackupOptions) (*types.BackupResult, error) {
	return nil, fmt.Errorf("backup is not yet supported for ClickHouse")
}

// GetBackupProgress is not yet supported for ClickHouse.
func (a *Adapter) GetBackupProgress(_ context.Context, _ string) (int, error) {
	return 0, fmt.Errorf("backup is not yet supported for ClickHouse")
}

// CancelBackup is not yet supported for ClickHouse.
func (a *Adapter) CancelBackup(_ context.Context, _ string) error {
	return fmt.Errorf("backup is not yet supported for ClickHouse")
}
