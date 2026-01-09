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
	applyFile     string
	applyPassword string // For user creation
	applyForUser  string // For grant creation
)

var applyCmd = &cobra.Command{
	Use:   "apply -f <file>",
	Short: "Apply a resource from a YAML file (create or update)",
	Long: `Apply a database resource from a YAML file.

If the resource exists, it will be updated. If it doesn't exist, it will be created.

Examples:
  # Apply a database (create if not exists, update if exists)
  dbctl apply -f database.yaml

  # Apply a user
  dbctl apply -f user.yaml --password mysecret

  # Apply grants for a user
  dbctl apply -f grant.yaml --for-user myuser`,
	RunE: runApply,
}

func init() {
	applyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "YAML file path (required)")
	applyCmd.Flags().StringVar(&applyPassword, "password", "", "Password for user creation")
	applyCmd.Flags().StringVar(&applyForUser, "for-user", "", "Username to apply grants to")
	_ = applyCmd.MarkFlagRequired("file")
}

func runApply(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	// Load resources from file
	resources, err := internal.LoadFile(applyFile)
	if err != nil {
		return fmt.Errorf("failed to load file: %w", err)
	}

	printer := internal.NewPrinter(internal.ParseOutputFormat(outputFormat), os.Stdout)
	ctx := context.Background()

	// Process each resource
	for _, res := range resources {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would apply %s '%s'\n", res.Kind, res.Metadata.Name)
			continue
		}

		printVerbose("Applying %s '%s'", res.Kind, res.Metadata.Name)

		result, err := applyResource(ctx, cfg, res)
		if err != nil {
			return fmt.Errorf("failed to apply %s '%s': %w", res.Kind, res.Metadata.Name, err)
		}

		printer.PrintResult(result.Message)
	}

	return nil
}

func applyResource(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	switch res.Kind {
	case internal.KindDatabase:
		return applyDatabase(ctx, cfg, res)
	case internal.KindDatabaseUser:
		return applyUser(ctx, cfg, res)
	case internal.KindDatabaseRole:
		return applyRole(ctx, cfg, res)
	case internal.KindDatabaseGrant:
		return applyGrant(ctx, cfg, res)
	default:
		return nil, fmt.Errorf("unsupported resource kind for apply: %s", res.Kind)
	}
}

func applyDatabase(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseSpec()
	if !ok {
		return nil, fmt.Errorf("invalid database spec")
	}

	svc, err := service.NewDatabaseService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	// Check if exists
	exists, err := svc.Exists(ctx, spec.Name)
	if err != nil {
		return nil, err
	}

	if exists {
		// Update
		return svc.Update(ctx, spec.Name, spec)
	}

	// Create
	return svc.Create(ctx, spec)
}

func applyUser(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseUserSpec()
	if !ok {
		return nil, fmt.Errorf("invalid user spec")
	}

	svc, err := service.NewUserService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	// Check if exists
	exists, err := svc.Exists(ctx, spec.Username)
	if err != nil {
		return nil, err
	}

	if exists {
		// Update
		return svc.Update(ctx, spec.Username, spec)
	}

	// Create - password required
	password := applyPassword
	if password == "" {
		return nil, fmt.Errorf("password is required for user creation (use --password flag)")
	}

	return svc.Create(ctx, service.CreateUserServiceOptions{
		Spec:     spec,
		Password: password,
	})
}

func applyRole(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseRoleSpec()
	if !ok {
		return nil, fmt.Errorf("invalid role spec")
	}

	svc, err := service.NewRoleService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	// Create handles both create and update
	return svc.Create(ctx, spec)
}

func applyGrant(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseGrantSpec()
	if !ok {
		return nil, fmt.Errorf("invalid grant spec")
	}

	// Username is required for grants
	username := applyForUser
	if username == "" {
		return nil, fmt.Errorf("--for-user flag is required for grant application")
	}

	svc, err := service.NewGrantService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	grantResult, err := svc.Apply(ctx, service.ApplyGrantServiceOptions{
		Username: username,
		Spec:     spec,
	})
	if err != nil {
		return nil, err
	}

	// Convert GrantResult to Result
	totalGrants := len(grantResult.AppliedRoles) + grantResult.AppliedDirectGrants + grantResult.AppliedDefaultPrivileges
	return service.NewSuccessResult(fmt.Sprintf("Applied %d grants to user '%s'", totalGrants, username)), nil
}
