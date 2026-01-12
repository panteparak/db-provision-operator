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
	"strings"

	"github.com/spf13/cobra"

	"github.com/db-provision-operator/cmd/dbctl/internal"
	"github.com/db-provision-operator/internal/service"
)

var getCmd = &cobra.Command{
	Use:   "get <type> [name]",
	Short: "Get information about a resource",
	Long: `Get information about a database resource.

Resource Types:
  database, db       Database
  user, dbu          DatabaseUser
  role, dbr          DatabaseRole
  grant, dbg         DatabaseGrant (requires --for-user)

Examples:
  # Get database info
  dbctl get database mydb

  # Get user info
  dbctl get user myuser

  # Get role info
  dbctl get role myrole

  # Get grants for a user
  dbctl get grant --for-user myuser`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runGet,
}

var getForUser string

func init() {
	getCmd.Flags().StringVar(&getForUser, "for-user", "", "Username to get grants for")
}

func runGet(cmd *cobra.Command, args []string) error {
	resourceType := normalizeResourceType(args[0])
	var name string
	if len(args) > 1 {
		name = args[1]
	}

	// Load configuration
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	printer := internal.NewPrinter(internal.ParseOutputFormat(outputFormat), os.Stdout)
	ctx := context.Background()

	switch resourceType {
	case "database":
		return getDatabase(ctx, cfg, name, printer)
	case "user":
		return getUser(ctx, cfg, name, printer)
	case "role":
		return getRole(ctx, cfg, name, printer)
	case "grant":
		return getGrant(ctx, cfg, printer)
	default:
		return fmt.Errorf("unknown resource type: %s", resourceType)
	}
}

func normalizeResourceType(s string) string {
	switch strings.ToLower(s) {
	case "database", "db", "databases":
		return "database"
	case "user", "dbu", "databaseuser", "users":
		return "user"
	case "role", "dbr", "databaserole", "roles":
		return "role"
	case "grant", "dbg", "databasegrant", "grants":
		return "grant"
	default:
		return strings.ToLower(s)
	}
}

func getDatabase(ctx context.Context, cfg *service.Config, name string, printer *internal.Printer) error {
	if name == "" {
		return fmt.Errorf("database name is required")
	}

	svc, err := service.NewDatabaseService(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return err
	}

	info, err := svc.Get(ctx, name)
	if err != nil {
		return err
	}

	// Convert extension info to string slice
	var extNames []string
	for _, ext := range info.Extensions {
		extNames = append(extNames, ext.Name)
	}

	output := internal.DatabaseOutput{
		Name:       info.Name,
		Engine:     cfg.Engine,
		Size:       internal.FormatSize(info.SizeBytes),
		Owner:      info.Owner,
		Encoding:   info.Encoding,
		Collation:  info.Collation,
		Extensions: strings.Join(extNames, ", "),
	}

	return printer.PrintData(output)
}

func getUser(ctx context.Context, cfg *service.Config, name string, printer *internal.Printer) error {
	if name == "" {
		return fmt.Errorf("username is required")
	}

	svc, err := service.NewUserService(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return err
	}

	info, err := svc.Get(ctx, name)
	if err != nil {
		return err
	}

	// Build attributes from boolean fields
	var attributes []string
	if info.Superuser {
		attributes = append(attributes, "SUPERUSER")
	}
	if info.CreateDB {
		attributes = append(attributes, "CREATEDB")
	}
	if info.CreateRole {
		attributes = append(attributes, "CREATEROLE")
	}
	if info.Login {
		attributes = append(attributes, "LOGIN")
	}
	if info.Replication {
		attributes = append(attributes, "REPLICATION")
	}
	if info.BypassRLS {
		attributes = append(attributes, "BYPASSRLS")
	}

	output := internal.UserOutput{
		Username:   info.Username,
		Roles:      strings.Join(info.InRoles, ", "),
		Attributes: strings.Join(attributes, ", "),
		ValidUntil: info.ValidUntil,
	}

	return printer.PrintData(output)
}

func getRole(ctx context.Context, cfg *service.Config, name string, printer *internal.Printer) error {
	if name == "" {
		return fmt.Errorf("role name is required")
	}

	svc, err := service.NewRoleService(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return err
	}

	info, err := svc.Get(ctx, name)
	if err != nil {
		return err
	}

	// Build attributes from boolean fields
	var attributes []string
	if info.Superuser {
		attributes = append(attributes, "SUPERUSER")
	}
	if info.CreateDB {
		attributes = append(attributes, "CREATEDB")
	}
	if info.CreateRole {
		attributes = append(attributes, "CREATEROLE")
	}
	if info.Login {
		attributes = append(attributes, "LOGIN")
	}
	if info.Replication {
		attributes = append(attributes, "REPLICATION")
	}
	if info.BypassRLS {
		attributes = append(attributes, "BYPASSRLS")
	}

	output := internal.RoleOutput{
		RoleName:   info.Name,
		Attributes: strings.Join(attributes, ", "),
		Members:    strings.Join(info.InRoles, ", "),
	}

	return printer.PrintData(output)
}

func getGrant(ctx context.Context, cfg *service.Config, printer *internal.Printer) error {
	if getForUser == "" {
		return fmt.Errorf("--for-user flag is required for getting grants")
	}

	svc, err := service.NewGrantService(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = svc.Close() }()

	if err := svc.Connect(ctx); err != nil {
		return err
	}

	grants, err := svc.GetGrants(ctx, getForUser)
	if err != nil {
		return err
	}

	var outputs []internal.GrantOutput
	for _, g := range grants {
		outputs = append(outputs, internal.GrantOutput{
			Grantee:    g.Grantee,
			ObjectType: g.ObjectType,
			ObjectName: g.ObjectName,
			Privileges: strings.Join(g.Privileges, ", "),
		})
	}

	if len(outputs) == 0 {
		printer.PrintResult(fmt.Sprintf("No grants found for user '%s'", getForUser))
		return nil
	}

	return printer.PrintData(outputs)
}
