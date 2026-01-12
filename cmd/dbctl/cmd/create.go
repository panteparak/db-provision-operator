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

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/db-provision-operator/cmd/dbctl/internal"
	"github.com/db-provision-operator/internal/service"
)

var (
	createFile     string
	createPassword string // For user creation
	createForUser  string // For grant creation
)

var createCmd = &cobra.Command{
	Use:   "create -f <file>",
	Short: "Create a resource from a YAML file",
	Long: `Create a database resource from a YAML file.

The YAML file should be in the same format as Kubernetes CRDs.

Examples:
  # Create a database
  dbctl create -f database.yaml

  # Create a user (password will be prompted)
  dbctl create -f user.yaml

  # Create a user with password
  dbctl create -f user.yaml --password mysecret

  # Create grants for a user
  dbctl create -f grant.yaml --for-user myuser`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&createFile, "file", "f", "", "YAML file path (required)")
	createCmd.Flags().StringVar(&createPassword, "password", "", "Password for user creation")
	createCmd.Flags().StringVar(&createForUser, "for-user", "", "Username to apply grants to")
	_ = createCmd.MarkFlagRequired("file")
}

func runCreate(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Load resources from file
	resources, err := internal.LoadFile(createFile)
	if err != nil {
		return fmt.Errorf("failed to load file: %w", err)
	}

	printer := internal.NewPrinter(internal.ParseOutputFormat(outputFormat), os.Stdout)
	ctx := context.Background()

	// Process each resource
	for _, res := range resources {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would create %s '%s'\n", res.Kind, res.Metadata.Name)
			continue
		}

		printVerbose("Creating %s '%s'", res.Kind, res.Metadata.Name)

		result, err := createResource(ctx, cfg, res)
		if err != nil {
			return fmt.Errorf("failed to create %s '%s': %w", res.Kind, res.Metadata.Name, err)
		}

		printer.PrintResult(result.Message)
	}

	return nil
}

func createResource(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	switch res.Kind {
	case internal.KindDatabase:
		return createDatabase(ctx, cfg, res)
	case internal.KindDatabaseUser:
		return createUser(ctx, cfg, res)
	case internal.KindDatabaseRole:
		return createRole(ctx, cfg, res)
	case internal.KindDatabaseGrant:
		return createGrant(ctx, cfg, res)
	default:
		return nil, fmt.Errorf("unsupported resource kind for create: %s", res.Kind)
	}
}

func createDatabase(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseSpec()
	if !ok {
		return nil, fmt.Errorf("invalid database spec")
	}

	svc, err := service.NewDatabaseService(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Create(ctx, spec)
}

func createUser(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseUserSpec()
	if !ok {
		return nil, fmt.Errorf("invalid user spec")
	}

	// Get password
	password := createPassword
	if password == "" {
		// For CLI, password is required
		return nil, fmt.Errorf("password is required for user creation (use --password flag)")
	}

	svc, err := service.NewUserService(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Create(ctx, service.CreateUserServiceOptions{
		Spec:     spec,
		Password: password,
	})
}

func createRole(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseRoleSpec()
	if !ok {
		return nil, fmt.Errorf("invalid role spec")
	}

	svc, err := service.NewRoleService(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Create(ctx, spec)
}

func createGrant(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	return executeGrant(ctx, cfg, res, createForUser, "creation")
}
