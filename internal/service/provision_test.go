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
	"errors"
	"testing"

	"github.com/db-provision-operator/internal/service/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvisionPipeline_AllStepsSucceed(t *testing.T) {
	mock := testutil.NewMockAdapter()
	cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
	rs := NewResourceServiceWithAdapter(mock, cfg, "TestService")

	var executed []string
	pipeline := &ProvisionPipeline{
		Steps: []ProvisionStep{
			{Name: "Step1", Execute: func(ctx context.Context) error {
				executed = append(executed, "Step1")
				return nil
			}},
			{Name: "Step2", Execute: func(ctx context.Context) error {
				executed = append(executed, "Step2")
				return nil
			}},
			{Name: "Step3", Execute: func(ctx context.Context) error {
				executed = append(executed, "Step3")
				return nil
			}},
		},
	}

	completed, err := pipeline.Run(context.Background(), rs)
	require.NoError(t, err)
	assert.Equal(t, 3, completed)
	assert.Equal(t, []string{"Step1", "Step2", "Step3"}, executed)
}

func TestProvisionPipeline_StopOnError(t *testing.T) {
	mock := testutil.NewMockAdapter()
	cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
	rs := NewResourceServiceWithAdapter(mock, cfg, "TestService")

	stepErr := errors.New("step2 failed")
	var executed []string
	pipeline := &ProvisionPipeline{
		Steps: []ProvisionStep{
			{Name: "Step1", Execute: func(ctx context.Context) error {
				executed = append(executed, "Step1")
				return nil
			}},
			{Name: "Step2", Execute: func(ctx context.Context) error {
				executed = append(executed, "Step2")
				return stepErr
			}},
			{Name: "Step3", Execute: func(ctx context.Context) error {
				executed = append(executed, "Step3")
				return nil
			}},
		},
	}

	completed, err := pipeline.Run(context.Background(), rs)
	require.Error(t, err)
	assert.Equal(t, 1, completed)
	assert.ErrorIs(t, err, stepErr)
	assert.Contains(t, err.Error(), "Step2")
	assert.Equal(t, []string{"Step1", "Step2"}, executed)
}

func TestProvisionPipeline_EmptyPipeline(t *testing.T) {
	mock := testutil.NewMockAdapter()
	cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
	rs := NewResourceServiceWithAdapter(mock, cfg, "TestService")

	pipeline := &ProvisionPipeline{}

	completed, err := pipeline.Run(context.Background(), rs)
	require.NoError(t, err)
	assert.Equal(t, 0, completed)
}

func TestProvisionPipeline_FirstStepFails(t *testing.T) {
	mock := testutil.NewMockAdapter()
	cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
	rs := NewResourceServiceWithAdapter(mock, cfg, "TestService")

	pipeline := &ProvisionPipeline{
		Steps: []ProvisionStep{
			{Name: "FailFirst", Execute: func(ctx context.Context) error {
				return errors.New("immediate failure")
			}},
			{Name: "NeverReached", Execute: func(ctx context.Context) error {
				t.Fatal("should not be reached")
				return nil
			}},
		},
	}

	completed, err := pipeline.Run(context.Background(), rs)
	require.Error(t, err)
	assert.Equal(t, 0, completed)
}

func TestProvisionPipeline_ContextCancellation(t *testing.T) {
	mock := testutil.NewMockAdapter()
	cfg := &Config{Engine: "postgres", Host: "localhost", Port: 5432}
	rs := NewResourceServiceWithAdapter(mock, cfg, "TestService")

	ctx, cancel := context.WithCancel(context.Background())
	pipeline := &ProvisionPipeline{
		Steps: []ProvisionStep{
			{Name: "CancelCtx", Execute: func(ctx context.Context) error {
				cancel()
				return nil
			}},
			{Name: "CheckCtx", Execute: func(ctx context.Context) error {
				return ctx.Err()
			}},
		},
	}

	completed, err := pipeline.Run(ctx, rs)
	require.Error(t, err)
	assert.Equal(t, 1, completed)
}
