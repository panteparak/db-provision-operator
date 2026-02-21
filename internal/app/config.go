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

package app

import "time"

// OperatorConfig holds operator-wide configuration set via CLI flags.
type OperatorConfig struct {
	// DefaultDriftInterval is the default drift detection interval for resources
	// that don't specify their own DriftPolicy.Interval (default: 8h).
	DefaultDriftInterval time.Duration

	// InstanceID partitions resources across multiple operators on the same cluster.
	// Resources are matched by the "dbops.dbprovision.io/operator-instance-id" label.
	// The default value "default" also manages unlabeled resources for backward compatibility.
	InstanceID string
}

// DefaultOperatorConfig returns an OperatorConfig with production defaults.
func DefaultOperatorConfig() OperatorConfig {
	return OperatorConfig{
		DefaultDriftInterval: 8 * time.Hour,
		InstanceID:           "default",
	}
}
