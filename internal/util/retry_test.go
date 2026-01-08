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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Retry", func() {
	Describe("RetryWithBackoff", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
			config RetryConfig
		)

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
			config = RetryConfig{
				MaxRetries:          3,
				InitialInterval:     10 * time.Millisecond,
				MaxInterval:         100 * time.Millisecond,
				Multiplier:          2.0,
				RandomizationFactor: 0,
			}
		})

		AfterEach(func() {
			cancel()
		})

		Context("when the function succeeds on first attempt", func() {
			It("should return success with 1 attempt", func() {
				callCount := 0
				fn := func() error {
					callCount++
					return nil
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).To(BeNil())
				Expect(result.Attempts).To(Equal(1))
				Expect(callCount).To(Equal(1))
			})
		})

		Context("when the function fails then succeeds", func() {
			It("should retry and succeed", func() {
				callCount := 0
				fn := func() error {
					callCount++
					if callCount < 3 {
						return errors.New("connection refused")
					}
					return nil
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).To(BeNil())
				Expect(result.Attempts).To(Equal(3))
				Expect(callCount).To(Equal(3))
			})

			It("should calculate backoff intervals correctly", func() {
				callCount := 0
				startTime := time.Now()
				fn := func() error {
					callCount++
					if callCount < 2 {
						return errors.New("connection refused")
					}
					return nil
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).To(BeNil())
				Expect(result.Attempts).To(Equal(2))
				// First attempt is immediate, second attempt after InitialInterval
				Expect(time.Since(startTime)).To(BeNumerically(">=", config.InitialInterval))
			})
		})

		Context("when the function fails after max retries", func() {
			It("should return the last error", func() {
				expectedErr := errors.New("connection refused")
				callCount := 0
				fn := func() error {
					callCount++
					return expectedErr
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).To(MatchError("connection refused"))
				Expect(result.Attempts).To(Equal(config.MaxRetries + 1))
				Expect(callCount).To(Equal(config.MaxRetries + 1))
			})

			It("should cap interval at MaxInterval", func() {
				config.MaxRetries = 10
				config.MaxInterval = 20 * time.Millisecond
				callCount := 0
				fn := func() error {
					callCount++
					return errors.New("connection refused")
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).NotTo(BeNil())
				// Last interval should be capped at MaxInterval
				Expect(result.LastInterval).To(BeNumerically("<=", config.MaxInterval))
			})
		})

		Context("when context is cancelled", func() {
			It("should return context error", func() {
				shortCtx, shortCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer shortCancel()

				config.InitialInterval = 100 * time.Millisecond
				callCount := 0
				fn := func() error {
					callCount++
					return errors.New("connection refused")
				}

				result := RetryWithBackoff(shortCtx, config, fn)

				Expect(result.LastError).To(MatchError(context.DeadlineExceeded))
			})

			It("should stop retrying when context is cancelled during wait", func() {
				cancelCtx, cancelFn := context.WithCancel(context.Background())

				config.InitialInterval = 1 * time.Second
				callCount := 0
				fn := func() error {
					callCount++
					if callCount == 1 {
						// Cancel context after first attempt
						go func() {
							time.Sleep(10 * time.Millisecond)
							cancelFn()
						}()
					}
					return errors.New("connection refused")
				}

				result := RetryWithBackoff(cancelCtx, config, fn)

				Expect(result.LastError).To(MatchError(context.Canceled))
				Expect(callCount).To(Equal(1))
			})
		})

		Context("when error is not retryable", func() {
			It("should stop immediately", func() {
				callCount := 0
				fn := func() error {
					callCount++
					return errors.New("syntax error in SQL")
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).To(MatchError("syntax error in SQL"))
				Expect(result.Attempts).To(Equal(1))
				Expect(callCount).To(Equal(1))
			})
		})

		Context("with randomization factor", func() {
			It("should apply jitter to intervals", func() {
				config.RandomizationFactor = 0.5
				config.MaxRetries = 5
				callCount := 0
				fn := func() error {
					callCount++
					return errors.New("connection refused")
				}

				result := RetryWithBackoff(ctx, config, fn)

				Expect(result.LastError).NotTo(BeNil())
				// With jitter, interval can vary by +/- 50%
				Expect(result.LastInterval).To(BeNumerically(">", 0))
			})
		})
	})

	Describe("IsRetryableError", func() {
		Context("when error is nil", func() {
			It("should return false", func() {
				Expect(IsRetryableError(nil)).To(BeFalse())
			})
		})

		Context("when error is connection refused", func() {
			It("should return true", func() {
				err := errors.New("dial tcp 127.0.0.1:5432: connection refused")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is timeout", func() {
			It("should return true for i/o timeout", func() {
				err := errors.New("read tcp 127.0.0.1:5432: i/o timeout")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for connection timed out", func() {
				err := errors.New("dial tcp: connection timed out")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for deadline exceeded", func() {
				err := errors.New("context deadline exceeded")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is network related", func() {
			It("should return true for network unreachable", func() {
				err := errors.New("dial tcp: network is unreachable")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for no such host", func() {
				err := errors.New("dial tcp: lookup db.example.com: no such host")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for no route to host", func() {
				err := errors.New("dial tcp 10.0.0.1:5432: no route to host")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for connection reset", func() {
				err := errors.New("read tcp: connection reset by peer")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for broken pipe", func() {
				err := errors.New("write tcp: broken pipe")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for EOF", func() {
				err := errors.New("unexpected EOF")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is database starting up", func() {
			It("should return true for PostgreSQL starting up", func() {
				err := errors.New("the database system is starting up")
				Expect(IsRetryableError(err)).To(BeTrue())
			})

			It("should return true for server is starting up", func() {
				err := errors.New("FATAL: the server is starting up")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is too many connections", func() {
			It("should return true", func() {
				err := errors.New("too many connections")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is temporary failure", func() {
			It("should return true", func() {
				err := errors.New("temporary failure in name resolution")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is unavailable", func() {
			It("should return true", func() {
				err := errors.New("service unavailable")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error suggests retry", func() {
			It("should return true for try again", func() {
				err := errors.New("resource busy, try again later")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is connection closed", func() {
			It("should return true", func() {
				err := errors.New("connection closed")
				Expect(IsRetryableError(err)).To(BeTrue())
			})
		})

		Context("when error is not retryable", func() {
			It("should return false for syntax error", func() {
				err := errors.New("ERROR: syntax error at or near")
				Expect(IsRetryableError(err)).To(BeFalse())
			})

			It("should return false for permission denied", func() {
				err := errors.New("ERROR: permission denied for table")
				Expect(IsRetryableError(err)).To(BeFalse())
			})

			It("should return false for authentication error", func() {
				err := errors.New("password authentication failed")
				Expect(IsRetryableError(err)).To(BeFalse())
			})

			It("should return false for database does not exist", func() {
				err := errors.New("FATAL: database \"testdb\" does not exist")
				Expect(IsRetryableError(err)).To(BeFalse())
			})
		})
	})

	Describe("CalculateRequeueAfter", func() {
		var config RetryConfig

		BeforeEach(func() {
			config = RetryConfig{
				MaxRetries:          5,
				InitialInterval:     5 * time.Second,
				MaxInterval:         30 * time.Minute,
				Multiplier:          2.0,
				RandomizationFactor: 0,
			}
		})

		Context("when result has no error (success)", func() {
			It("should return 5 minutes", func() {
				result := RetryResult{
					Attempts:     1,
					LastError:    nil,
					TotalTime:    100 * time.Millisecond,
					LastInterval: 0,
				}

				requeueAfter := CalculateRequeueAfter(config, result)

				Expect(requeueAfter).To(Equal(5 * time.Minute))
			})
		})

		Context("when result has error with zero last interval", func() {
			It("should return initial interval", func() {
				result := RetryResult{
					Attempts:     1,
					LastError:    errors.New("connection refused"),
					TotalTime:    100 * time.Millisecond,
					LastInterval: 0,
				}

				requeueAfter := CalculateRequeueAfter(config, result)

				Expect(requeueAfter).To(Equal(config.InitialInterval))
			})
		})

		Context("when result has error with previous interval", func() {
			It("should calculate next interval with multiplier", func() {
				result := RetryResult{
					Attempts:     2,
					LastError:    errors.New("connection refused"),
					TotalTime:    5 * time.Second,
					LastInterval: 5 * time.Second,
				}

				requeueAfter := CalculateRequeueAfter(config, result)

				Expect(requeueAfter).To(Equal(10 * time.Second))
			})

			It("should cap at max interval", func() {
				result := RetryResult{
					Attempts:     10,
					LastError:    errors.New("connection refused"),
					TotalTime:    60 * time.Minute,
					LastInterval: 20 * time.Minute,
				}

				requeueAfter := CalculateRequeueAfter(config, result)

				Expect(requeueAfter).To(Equal(config.MaxInterval))
			})
		})

		Context("when calculated interval is less than initial", func() {
			It("should return initial interval as minimum", func() {
				result := RetryResult{
					Attempts:     1,
					LastError:    errors.New("connection refused"),
					TotalTime:    1 * time.Second,
					LastInterval: 1 * time.Second,
				}

				requeueAfter := CalculateRequeueAfter(config, result)

				Expect(requeueAfter).To(BeNumerically(">=", config.InitialInterval))
			})
		})
	})

	Describe("DefaultRetryConfig", func() {
		It("should return sensible defaults", func() {
			config := DefaultRetryConfig()

			Expect(config.MaxRetries).To(Equal(5))
			Expect(config.InitialInterval).To(Equal(5 * time.Second))
			Expect(config.MaxInterval).To(Equal(30 * time.Minute))
			Expect(config.Multiplier).To(Equal(2.0))
			Expect(config.RandomizationFactor).To(Equal(0.1))
		})
	})

	Describe("ConnectionRetryConfig", func() {
		It("should return config optimized for connections", func() {
			config := ConnectionRetryConfig()

			Expect(config.MaxRetries).To(Equal(3))
			Expect(config.InitialInterval).To(Equal(5 * time.Second))
			Expect(config.MaxInterval).To(Equal(30 * time.Minute))
			Expect(config.Multiplier).To(Equal(2.0))
			Expect(config.RandomizationFactor).To(Equal(0.2))
		})
	})

	Describe("BackupRetryConfig", func() {
		It("should return config for backup operations", func() {
			config := BackupRetryConfig()

			Expect(config.MaxRetries).To(Equal(2))
			Expect(config.InitialInterval).To(Equal(10 * time.Second))
			Expect(config.MaxInterval).To(Equal(30 * time.Minute))
			Expect(config.Multiplier).To(Equal(3.0))
			Expect(config.RandomizationFactor).To(Equal(0.1))
		})
	})
})
