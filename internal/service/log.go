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
	"time"

	"github.com/go-logr/logr"
)

// operationLogger provides consistent structured logging for service operations.
// It tracks operation lifecycle with automatic duration calculation.
//
// Usage:
//
//	op := s.startOp("Create", "mydb")
//	op.Debug("checking existence")
//	if err != nil {
//	    op.Error(err, "failed to create database")
//	    return err
//	}
//	op.Success("database created")
type operationLogger struct {
	log       logr.Logger
	operation string
	resource  string
	start     time.Time
}

// startOperation creates a logger for tracking an operation lifecycle.
// It logs the operation start at debug level (V(1)).
func startOperation(log logr.Logger, operation, resource string) *operationLogger {
	ol := &operationLogger{
		log:       log.WithValues("operation", operation, "resource", resource),
		operation: operation,
		resource:  resource,
		start:     time.Now(),
	}
	ol.log.V(1).Info("starting operation")
	return ol
}

// Success logs successful completion with duration.
// Use this when the operation completes successfully.
func (ol *operationLogger) Success(msg string) {
	ol.log.Info(msg, "duration", time.Since(ol.start).String())
}

// Error logs an error with operation context and duration.
// Use this when the operation fails.
func (ol *operationLogger) Error(err error, msg string) {
	ol.log.Error(err, msg, "duration", time.Since(ol.start).String())
}

// Debug logs debug-level info (V(1)).
// Use this for intermediate steps, parameter details, etc.
func (ol *operationLogger) Debug(msg string, keysAndValues ...interface{}) {
	ol.log.V(1).Info(msg, keysAndValues...)
}

// Info logs info-level messages.
// Use this for notable events that aren't errors or success.
func (ol *operationLogger) Info(msg string, keysAndValues ...interface{}) {
	ol.log.Info(msg, keysAndValues...)
}

// WithValues returns a new operationLogger with additional key-value pairs.
func (ol *operationLogger) WithValues(keysAndValues ...interface{}) *operationLogger {
	return &operationLogger{
		log:       ol.log.WithValues(keysAndValues...),
		operation: ol.operation,
		resource:  ol.resource,
		start:     ol.start,
	}
}
