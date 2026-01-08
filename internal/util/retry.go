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

package util

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// RetryConfig defines retry behavior with exponential backoff
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries, -1 = unlimited until MaxInterval reached)
	MaxRetries int
	// InitialInterval is the backoff duration after first failure (first attempt is immediate)
	InitialInterval time.Duration
	// MaxInterval is the maximum backoff duration - interval never exceeds this (default: 30 minutes)
	MaxInterval time.Duration
	// Multiplier is the factor by which the interval increases each retry
	Multiplier float64
	// RandomizationFactor adds jitter to avoid thundering herd (0-1)
	RandomizationFactor float64
}

// DefaultRetryConfig returns sensible defaults for database operations
// Sequence: immediate -> 5s -> 10s -> 20s -> 40s -> 80s (capped at 30min)
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:          5,
		InitialInterval:     5 * time.Second,
		MaxInterval:         30 * time.Minute,
		Multiplier:          2.0,
		RandomizationFactor: 0.1,
	}
}

// ConnectionRetryConfig returns config optimized for connection retries
// Sequence: immediate -> 5s -> 10s -> 20s (capped at 30min)
func ConnectionRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:          3,
		InitialInterval:     5 * time.Second,
		MaxInterval:         30 * time.Minute,
		Multiplier:          2.0,
		RandomizationFactor: 0.2,
	}
}

// BackupRetryConfig returns config for backup operations
// Sequence: immediate -> 10s -> 30s (capped at 30min)
func BackupRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:          2,
		InitialInterval:     10 * time.Second,
		MaxInterval:         30 * time.Minute,
		Multiplier:          3.0,
		RandomizationFactor: 0.1,
	}
}

// RetryResult contains the outcome of a retry operation
type RetryResult struct {
	// Attempts is the number of attempts made
	Attempts int
	// LastError is the last error encountered (nil if successful)
	LastError error
	// TotalTime is the total duration of all attempts
	TotalTime time.Duration
	// LastInterval is the last backoff interval used
	LastInterval time.Duration
}

// RetryWithBackoff executes fn with exponential backoff on failure
// First attempt is immediate, subsequent retries use exponential backoff
// Intervals are capped at MaxInterval (default 30 minutes)
func RetryWithBackoff(ctx context.Context, config RetryConfig, fn func() error) RetryResult {
	startTime := time.Now()
	var lastErr error
	interval := time.Duration(0) // First attempt is immediate

	maxAttempts := config.MaxRetries + 1
	if config.MaxRetries < 0 {
		maxAttempts = 1000 // Effectively unlimited but with a safety cap
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Wait before attempt (0 for first attempt)
		if interval > 0 {
			select {
			case <-ctx.Done():
				return RetryResult{
					Attempts:     attempt,
					LastError:    ctx.Err(),
					TotalTime:    time.Since(startTime),
					LastInterval: interval,
				}
			case <-time.After(interval):
			}
		}

		// Execute the function
		lastErr = fn()
		if lastErr == nil {
			return RetryResult{
				Attempts:     attempt + 1,
				LastError:    nil,
				TotalTime:    time.Since(startTime),
				LastInterval: interval,
			}
		}

		// Check if error is retryable
		if !IsRetryableError(lastErr) {
			return RetryResult{
				Attempts:     attempt + 1,
				LastError:    lastErr,
				TotalTime:    time.Since(startTime),
				LastInterval: interval,
			}
		}

		// Calculate next interval with exponential backoff
		if attempt == 0 {
			interval = config.InitialInterval
		} else {
			interval = time.Duration(float64(interval) * config.Multiplier)
		}

		// Cap at MaxInterval
		if interval > config.MaxInterval {
			interval = config.MaxInterval
		}

		// Apply jitter
		if config.RandomizationFactor > 0 {
			delta := config.RandomizationFactor * float64(interval)
			minInterval := float64(interval) - delta
			maxInterval := float64(interval) + delta
			interval = time.Duration(minInterval + (rand.Float64() * (maxInterval - minInterval)))
		}
	}

	return RetryResult{
		Attempts:     maxAttempts,
		LastError:    lastErr,
		TotalTime:    time.Since(startTime),
		LastInterval: interval,
	}
}

// IsRetryableError determines if an error should trigger a retry
// Returns true for transient errors (network, timeout, connection refused)
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for known transient error types
	errStr := strings.ToLower(err.Error())
	transientPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"i/o timeout",
		"deadline exceeded",
		"temporary failure",
		"too many connections",
		"server is starting up",
		"the database system is starting up",
		"not currently accepting connections", // PostgreSQL SQLSTATE 55000
		"connection timed out",
		"network is unreachable",
		"no route to host",
		"broken pipe",
		"connection closed",
		"eof",
		"unavailable",
		"try again",
	}

	for _, pattern := range transientPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// CalculateRequeueAfter returns the requeue interval based on retry result
func CalculateRequeueAfter(config RetryConfig, result RetryResult) time.Duration {
	if result.LastError == nil {
		// Success - use normal requeue interval
		return 5 * time.Minute
	}

	// Calculate next interval based on last interval
	var nextInterval time.Duration
	if result.LastInterval == 0 {
		nextInterval = config.InitialInterval
	} else {
		nextInterval = time.Duration(float64(result.LastInterval) * config.Multiplier)
	}

	// Cap at MaxInterval
	if nextInterval > config.MaxInterval {
		nextInterval = config.MaxInterval
	}

	// Ensure minimum interval
	if nextInterval < config.InitialInterval {
		nextInterval = config.InitialInterval
	}

	return nextInterval
}
