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
	updateFile string
)

var updateCmd = &cobra.Command{
	Use:   "update -f <file>",
	Short: "Update a resource from a YAML file",
	Long: `Update an existing database resource from a YAML file.

The resource must already exist. To create or update, use 'apply' instead.

Examples:
  # Update a database
  dbctl update -f database.yaml

  # Update a user
  dbctl update -f user.yaml

  # Update a role
  dbctl update -f role.yaml`,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().StringVarP(&updateFile, "file", "f", "", "YAML file path (required)")
	_ = updateCmd.MarkFlagRequired("file")
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Load resources from file
	resources, err := internal.LoadFile(updateFile)
	if err != nil {
		return fmt.Errorf("failed to load file: %w", err)
	}

	printer := internal.NewPrinter(internal.ParseOutputFormat(outputFormat), os.Stdout)
	ctx := context.Background()

	// Process each resource
	for _, res := range resources {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would update %s '%s'\n", res.Kind, res.Metadata.Name)
			continue
		}

		printVerbose("Updating %s '%s'", res.Kind, res.Metadata.Name)

		result, err := updateResource(ctx, cfg, res)
		if err != nil {
			return fmt.Errorf("failed to update %s '%s': %w", res.Kind, res.Metadata.Name, err)
		}

		printer.PrintResult(result.Message)
	}

	return nil
}

func updateResource(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	switch res.Kind {
	case internal.KindDatabase:
		return updateDatabase(ctx, cfg, res)
	case internal.KindDatabaseUser:
		return updateUser(ctx, cfg, res)
	case internal.KindDatabaseRole:
		return updateRole(ctx, cfg, res)
	default:
		return nil, fmt.Errorf("unsupported resource kind for update: %s", res.Kind)
	}
}

func updateDatabase(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
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

	return svc.Update(ctx, spec.Name, spec)
}

func updateUser(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseUserSpec()
	if !ok {
		return nil, fmt.Errorf("invalid user spec")
	}

	svc, err := service.NewUserService(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Update(ctx, spec.Username, spec)
}

func updateRole(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
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

	return svc.Update(ctx, spec.RoleName, spec)
}
