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

package service

import (
	"context"
	"fmt"
)

// ProvisionStep represents a single step in a resource provisioning workflow.
// Steps are executed sequentially and each step must succeed before the next runs.
type ProvisionStep struct {
	// Name identifies the step for logging and error reporting.
	Name string

	// Execute runs the step logic. Returning an error stops the pipeline.
	Execute func(ctx context.Context) error
}

// ProvisionPipeline executes provisioning steps sequentially.
// It provides structured logging and reports which step failed.
type ProvisionPipeline struct {
	// Steps to execute in order.
	Steps []ProvisionStep
}

// Run executes all steps in order. It returns the number of steps that
// completed successfully and any error from the failed step.
// If all steps succeed, stepsCompleted equals len(Steps) and err is nil.
func (p *ProvisionPipeline) Run(ctx context.Context, rs *ResourceService) (stepsCompleted int, err error) {
	for i, step := range p.Steps {
		op := rs.startOp("Pipeline", step.Name)

		if err := step.Execute(ctx); err != nil {
			op.Error(err, "step failed")
			return i, fmt.Errorf("step %q failed: %w", step.Name, err)
		}

		op.Success("step completed")
	}
	return len(p.Steps), nil
}
