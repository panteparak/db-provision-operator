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
	deleteFile    string
	deleteForUser string // For grant revocation
	deleteForce   bool   // Force delete
)

var deleteCmd = &cobra.Command{
	Use:   "delete {-f <file> | <type> <name>}",
	Short: "Delete a resource",
	Long: `Delete a database resource from a YAML file or by type and name.

Examples:
  # Delete from file
  dbctl delete -f database.yaml

  # Delete by type and name
  dbctl delete database mydb

  # Delete user
  dbctl delete user myuser

  # Delete role
  dbctl delete role myrole

  # Revoke grants for a user
  dbctl delete -f grant.yaml --for-user myuser

  # Force delete (for databases with connections)
  dbctl delete database mydb --force`,
	RunE: runDelete,
}

func init() {
	deleteCmd.Flags().StringVarP(&deleteFile, "file", "f", "", "YAML file path")
	deleteCmd.Flags().StringVar(&deleteForUser, "for-user", "", "Username to revoke grants from")
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Force delete")
}

func runDelete(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	printer := internal.NewPrinter(internal.ParseOutputFormat(outputFormat), os.Stdout)
	ctx := context.Background()

	// If file is provided, delete from file
	if deleteFile != "" {
		return deleteFromFile(ctx, cfg, printer)
	}

	// Otherwise, delete by type and name
	if len(args) < 2 {
		return fmt.Errorf("either -f <file> or <type> <name> is required")
	}

	resourceType := normalizeResourceType(args[0])
	name := args[1]

	return deleteByName(ctx, cfg, resourceType, name, printer)
}

func deleteFromFile(ctx context.Context, cfg *service.Config, printer *internal.Printer) error {
	resources, err := internal.LoadFile(deleteFile)
	if err != nil {
		return fmt.Errorf("failed to load file: %w", err)
	}

	for _, res := range resources {
		if dryRun {
			fmt.Printf("[DRY-RUN] Would delete %s '%s'\n", res.Kind, res.Metadata.Name)
			continue
		}

		printVerbose("Deleting %s '%s'", res.Kind, res.Metadata.Name)

		result, err := deleteResource(ctx, cfg, res)
		if err != nil {
			return fmt.Errorf("failed to delete %s '%s': %w", res.Kind, res.Metadata.Name, err)
		}

		printer.PrintResult(result.Message)
	}

	return nil
}

func deleteByName(ctx context.Context, cfg *service.Config, resourceType, name string, printer *internal.Printer) error {
	if dryRun {
		fmt.Printf("[DRY-RUN] Would delete %s '%s'\n", resourceType, name)
		return nil
	}

	var result *service.Result
	var err error

	switch resourceType {
	case "database":
		result, err = deleteDatabaseByName(ctx, cfg, name)
	case "user":
		result, err = deleteUserByName(ctx, cfg, name)
	case "role":
		result, err = deleteRoleByName(ctx, cfg, name)
	default:
		return fmt.Errorf("delete by name not supported for type: %s (use -f <file>)", resourceType)
	}

	if err != nil {
		return err
	}

	printer.PrintResult(result.Message)
	return nil
}

func deleteResource(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	switch res.Kind {
	case internal.KindDatabase:
		return deleteDatabase(ctx, cfg, res)
	case internal.KindDatabaseUser:
		return deleteUser(ctx, cfg, res)
	case internal.KindDatabaseRole:
		return deleteRole(ctx, cfg, res)
	case internal.KindDatabaseGrant:
		return deleteGrant(ctx, cfg, res)
	default:
		return nil, fmt.Errorf("unsupported resource kind for delete: %s", res.Kind)
	}
}

func deleteDatabase(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseSpec()
	if !ok {
		return nil, fmt.Errorf("invalid database spec")
	}

	return deleteDatabaseByName(ctx, cfg, spec.Name)
}

func deleteDatabaseByName(ctx context.Context, cfg *service.Config, name string) (*service.Result, error) {
	svc, err := service.NewDatabaseService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Delete(ctx, name, deleteForce)
}

func deleteUser(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseUserSpec()
	if !ok {
		return nil, fmt.Errorf("invalid user spec")
	}

	return deleteUserByName(ctx, cfg, spec.Username)
}

func deleteUserByName(ctx context.Context, cfg *service.Config, name string) (*service.Result, error) {
	svc, err := service.NewUserService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Delete(ctx, name)
}

func deleteRole(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseRoleSpec()
	if !ok {
		return nil, fmt.Errorf("invalid role spec")
	}

	return deleteRoleByName(ctx, cfg, spec.RoleName)
}

func deleteRoleByName(ctx context.Context, cfg *service.Config, name string) (*service.Result, error) {
	svc, err := service.NewRoleService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Delete(ctx, name)
}

func deleteGrant(ctx context.Context, cfg *service.Config, res internal.Resource) (*service.Result, error) {
	spec, ok := res.GetDatabaseGrantSpec()
	if !ok {
		return nil, fmt.Errorf("invalid grant spec")
	}

	// Username is required for grants
	username := deleteForUser
	if username == "" {
		return nil, fmt.Errorf("--for-user flag is required for grant deletion")
	}

	svc, err := service.NewGrantService(cfg)
	if err != nil {
		return nil, err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return nil, err
	}

	return svc.Revoke(ctx, service.ApplyGrantServiceOptions{
		Username: username,
		Spec:     spec,
	})
}
