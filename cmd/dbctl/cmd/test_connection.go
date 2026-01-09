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

var testConnectionCmd = &cobra.Command{
	Use:   "test-connection",
	Short: "Test database connection",
	Long: `Test the database connection using the configured environment variables.

This is useful for verifying your environment configuration before
running other commands.

Example:
  dbctl test-connection`,
	Aliases: []string{"test", "ping"},
	RunE:    runTestConnection,
}

func runTestConnection(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := getConfig()
	if err != nil {
		return err
	}

	printer := internal.NewPrinter(internal.ParseOutputFormat(outputFormat), os.Stdout)
	ctx := context.Background()

	svc, err := service.NewInstanceService(cfg)
	if err != nil {
		return err
	}
	defer svc.Close()

	if err := svc.Connect(ctx); err != nil {
		return err
	}

	healthResult, err := svc.HealthCheck(ctx)
	if err != nil {
		return err
	}

	output := internal.HealthOutput{
		Healthy: healthResult.Healthy,
		Engine:  cfg.Engine,
		Version: healthResult.Version,
		Latency: healthResult.Latency.String(),
		Message: healthResult.Message,
	}

	if !healthResult.Healthy {
		output.Message = healthResult.ErrorMessage
		if err := printer.PrintData(output); err != nil {
			return err
		}
		return fmt.Errorf("connection test failed")
	}

	return printer.PrintData(output)
}
