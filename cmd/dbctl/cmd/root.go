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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/db-provision-operator/internal/service"
)

var (
	// Global flags
	verbose      bool
	outputFormat string
	dryRun       bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "dbctl",
	Short: "Database provisioning CLI tool",
	Long: `dbctl is a command-line tool for managing database resources.

It works with YAML files in the same format as Kubernetes CRDs, allowing
you to create, read, update, and delete database resources manually.

Environment Variables:
  DBCTL_ENGINE     Database engine (postgres|mysql) [required]
  DBCTL_HOST       Database host [required]
  DBCTL_PORT       Database port [required]
  DBCTL_DATABASE   Admin database for connection [required]
  DBCTL_USERNAME   Database username [required]
  DBCTL_PASSWORD   Database password [required]
  DBCTL_SSL_MODE   SSL mode (disable|require|verify-ca|verify-full)

Example:
  export DBCTL_ENGINE=postgres
  export DBCTL_HOST=localhost
  export DBCTL_PORT=5432
  export DBCTL_DATABASE=postgres
  export DBCTL_USERNAME=postgres
  export DBCTL_PASSWORD=secret

  dbctl create -f database.yaml
  dbctl get database mydb
  dbctl delete -f database.yaml`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "table", "Output format (table|yaml|json)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Print what would be done without executing")

	// Add subcommands
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(testConnectionCmd)
	rootCmd.AddCommand(versionCmd)
}

// getConfig loads configuration from environment variables
func getConfig() (*service.Config, error) {
	cfg, err := service.ConfigFromEnv(os.Getenv)
	if err != nil {
		return nil, fmt.Errorf(
			"configuration error: %w\n\n"+
				"Please ensure all required environment variables are set:\n"+
				"  DBCTL_ENGINE, DBCTL_HOST, DBCTL_PORT, DBCTL_DATABASE, "+
				"DBCTL_USERNAME, DBCTL_PASSWORD",
			err,
		)
	}
	return cfg, nil
}

// printVerbose prints verbose output if verbose mode is enabled
func printVerbose(format string, args ...interface{}) {
	if verbose {
		fmt.Printf("[VERBOSE] "+format+"\n", args...)
	}
}
