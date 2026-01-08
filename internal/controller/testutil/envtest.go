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

// Package testutil provides shared testing utilities for controller tests.
package testutil

import (
	"context"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

// TestEnv wraps the envtest.Environment and provides shared test infrastructure.
type TestEnv struct {
	// Env is the underlying envtest.Environment
	Env *envtest.Environment

	// Config is the REST config for the test environment
	Config *rest.Config

	// Client is the Kubernetes client for the test environment
	Client client.Client

	// Ctx is the context with cancellation for the test environment
	Ctx context.Context

	// Cancel is the cancel function for the context
	Cancel context.CancelFunc
}

// TestEnvConfig holds configuration options for the test environment.
type TestEnvConfig struct {
	// CRDDirectoryPaths specifies paths to CRD YAML files
	CRDDirectoryPaths []string

	// ErrorIfCRDPathMissing causes an error if CRD paths are not found
	ErrorIfCRDPathMissing bool

	// UseExistingCluster connects to an existing cluster instead of starting envtest
	UseExistingCluster *bool

	// AttachControlPlaneOutput enables control plane output logging
	AttachControlPlaneOutput bool

	// BinaryAssetsDirectory specifies a custom directory for envtest binaries
	BinaryAssetsDirectory string
}

// DefaultTestEnvConfig returns a default configuration for the test environment.
// It looks for CRDs in the default kubebuilder project structure.
func DefaultTestEnvConfig() *TestEnvConfig {
	return &TestEnvConfig{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
}

// NewTestEnv creates a new test environment with the given configuration.
// If config is nil, default configuration is used.
func NewTestEnv(config *TestEnvConfig) *TestEnv {
	if config == nil {
		config = DefaultTestEnvConfig()
	}

	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	env := &envtest.Environment{
		CRDDirectoryPaths:        config.CRDDirectoryPaths,
		ErrorIfCRDPathMissing:    config.ErrorIfCRDPathMissing,
		UseExistingCluster:       config.UseExistingCluster,
		AttachControlPlaneOutput: config.AttachControlPlaneOutput,
	}

	// Set binary assets directory if provided or auto-detect
	if config.BinaryAssetsDirectory != "" {
		env.BinaryAssetsDirectory = config.BinaryAssetsDirectory
	} else if dir := getFirstFoundEnvTestBinaryDir(); dir != "" {
		env.BinaryAssetsDirectory = dir
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TestEnv{
		Env:    env,
		Ctx:    ctx,
		Cancel: cancel,
	}
}

// Start starts the test environment and initializes the client.
// It registers the dbopsv1alpha1 scheme and creates a client.
func (t *TestEnv) Start() error {
	// Register the scheme
	if err := dbopsv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return err
	}

	// Start the test environment
	cfg, err := t.Env.Start()
	if err != nil {
		return err
	}
	t.Config = cfg

	// Create the client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return err
	}
	t.Client = k8sClient

	return nil
}

// Stop stops the test environment and cancels the context.
func (t *TestEnv) Stop() error {
	t.Cancel()
	return t.Env.Stop()
}

// GetClient returns the Kubernetes client for the test environment.
func (t *TestEnv) GetClient() client.Client {
	return t.Client
}

// GetConfig returns the REST config for the test environment.
func (t *TestEnv) GetConfig() *rest.Config {
	return t.Config
}

// GetContext returns the context for the test environment.
func (t *TestEnv) GetContext() context.Context {
	return t.Ctx
}

// GetScheme returns the scheme used by the test environment.
func (t *TestEnv) GetScheme() *rest.Config {
	return t.Config
}

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// MustStartTestEnv creates and starts a test environment, panicking on error.
// This is useful in test setup functions where error handling is not desired.
func MustStartTestEnv(config *TestEnvConfig) *TestEnv {
	env := NewTestEnv(config)
	if err := env.Start(); err != nil {
		panic(err)
	}
	return env
}

// MustStopTestEnv stops a test environment, panicking on error.
// This is useful in test teardown functions where error handling is not desired.
func MustStopTestEnv(env *TestEnv) {
	if err := env.Stop(); err != nil {
		panic(err)
	}
}
