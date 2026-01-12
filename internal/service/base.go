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

	"github.com/go-logr/logr"
)

// baseService provides common functionality for all services.
// Embed this struct in service implementations to get logging utilities.
//
// Usage:
//
//	type DatabaseService struct {
//	    baseService
//	    adapter adapter.DatabaseAdapter
//	    config  *Config
//	}
//
//	func (s *DatabaseService) Create(ctx context.Context, spec *Spec) (*Result, error) {
//	    op := s.startOp("Create", spec.Name)
//	    // ... business logic ...
//	    op.Success("database created")
//	}
type baseService struct {
	log logr.Logger
}

// newBaseService creates a base service with the given logger and service name.
// The service name is added to the logger for easy filtering.
func newBaseService(cfg *Config, serviceName string) baseService {
	return baseService{
		log: cfg.GetLogger().WithName(serviceName),
	}
}

// startOp creates an operation logger for tracking the operation lifecycle.
// This is the primary method services should use for structured logging.
func (b *baseService) startOp(operation, resource string) *operationLogger {
	return startOperation(b.log, operation, resource)
}

// wrapError wraps errors with appropriate types based on context state.
// Returns TimeoutError if context deadline exceeded, otherwise DatabaseError.
func (b *baseService) wrapError(ctx context.Context, config *Config, operation, resource string, err error) error {
	if ctx.Err() == context.DeadlineExceeded {
		return NewTimeoutError(operation, resource, config.Timeouts.OperationTimeout.String(), err)
	}
	return NewDatabaseError(operation, resource, err)
}
