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

package backupschedule

import (
	"time"

	"github.com/db-provision-operator/internal/shared/eventbus"
)

// Event names for the backupschedule module.
const (
	EventScheduledBackupTriggered = "ScheduledBackupTriggered"
)

// ScheduledBackupTriggered is published when a scheduled backup is triggered.
type ScheduledBackupTriggered struct {
	eventbus.BaseEvent
	ScheduleName string
	BackupName   string
	DatabaseRef  string
	Namespace    string
	TriggeredAt  time.Time
}

// NewScheduledBackupTriggered creates a new ScheduledBackupTriggered event.
func NewScheduledBackupTriggered(scheduleName, backupName, databaseRef, namespace string) *ScheduledBackupTriggered {
	return &ScheduledBackupTriggered{
		BaseEvent:    eventbus.NewBaseEvent(EventScheduledBackupTriggered, scheduleName, "DatabaseBackupSchedule"),
		ScheduleName: scheduleName,
		BackupName:   backupName,
		DatabaseRef:  databaseRef,
		Namespace:    namespace,
		TriggeredAt:  time.Now(),
	}
}
