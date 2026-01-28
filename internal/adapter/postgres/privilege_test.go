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

package postgres

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// PostgreSQL Privilege Matrix
//
// This test documents and validates the minimum privileges required for each
// database operation. The operator uses a least-privilege admin account
// (dbprovision_admin) instead of superuser.
//
// Setup SQL for creating the least-privilege admin account:
//
//	CREATE ROLE dbprovision_admin WITH
//	    LOGIN
//	    CREATEDB
//	    CREATEROLE
//	    PASSWORD 'your-secure-password';
//
//	GRANT pg_signal_backend TO dbprovision_admin;
//	GRANT pg_read_all_data TO dbprovision_admin;  -- PostgreSQL 14+
//	GRANT CONNECT ON DATABASE template1 TO dbprovision_admin;

var _ = Describe("PostgreSQL Privilege Requirements", func() {
	// privilegeMatrix documents the minimum privileges required for each operation.
	// This serves as both documentation and a test specification.
	type PrivilegeRequirement struct {
		Operation   string
		Privileges  []string
		Description string
		SQLToGrant  string
	}

	privilegeMatrix := []PrivilegeRequirement{
		{
			Operation:   "CreateDatabase",
			Privileges:  []string{"CREATEDB"},
			Description: "Create new databases",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEDB",
		},
		{
			Operation:   "DropDatabase",
			Privileges:  []string{"CREATEDB"},
			Description: "Drop existing databases",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEDB",
		},
		{
			Operation:   "ForceDropDatabase",
			Privileges:  []string{"CREATEDB", "pg_signal_backend"},
			Description: "Drop database by terminating active connections first",
			SQLToGrant:  "GRANT pg_signal_backend TO dbprovision_admin",
		},
		{
			Operation:   "CreateUser",
			Privileges:  []string{"CREATEROLE"},
			Description: "Create new database users/roles",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "DropUser",
			Privileges:  []string{"CREATEROLE"},
			Description: "Drop existing users/roles",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "AlterUser",
			Privileges:  []string{"CREATEROLE"},
			Description: "Modify user attributes (password, connection limit, etc.)",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "CreateRole",
			Privileges:  []string{"CREATEROLE"},
			Description: "Create database roles for permission grouping",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "DropRole",
			Privileges:  []string{"CREATEROLE"},
			Description: "Drop existing roles",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "GrantRole",
			Privileges:  []string{"CREATEROLE"},
			Description: "Grant role membership to users",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "RevokeRole",
			Privileges:  []string{"CREATEROLE"},
			Description: "Revoke role membership from users",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "GrantPrivileges",
			Privileges:  []string{"CREATEROLE"},
			Description: "Grant database/schema/table privileges to users",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "RevokePrivileges",
			Privileges:  []string{"CREATEROLE"},
			Description: "Revoke database/schema/table privileges from users",
			SQLToGrant:  "ALTER ROLE dbprovision_admin WITH CREATEROLE",
		},
		{
			Operation:   "TerminateConnections",
			Privileges:  []string{"pg_signal_backend"},
			Description: "Terminate database connections (for force-drop)",
			SQLToGrant:  "GRANT pg_signal_backend TO dbprovision_admin",
		},
		{
			Operation:   "BackupDatabase",
			Privileges:  []string{"pg_read_all_data"},
			Description: "Read all data for backup operations (PostgreSQL 14+)",
			SQLToGrant:  "GRANT pg_read_all_data TO dbprovision_admin",
		},
		{
			Operation:   "QueryMetadata",
			Privileges:  []string{"CONNECT"},
			Description: "Query system catalogs (pg_database, pg_roles, etc.)",
			SQLToGrant:  "GRANT CONNECT ON DATABASE postgres TO dbprovision_admin",
		},
	}

	Context("Privilege Documentation", func() {
		for _, req := range privilegeMatrix {
			// capture for closure
			It("documents privileges for "+req.Operation, func() {
				GinkgoWriter.Printf("\n=== %s ===\n", req.Operation)
				GinkgoWriter.Printf("Description: %s\n", req.Description)
				GinkgoWriter.Printf("Required Privileges: %v\n", req.Privileges)
				GinkgoWriter.Printf("SQL to Grant: %s\n", req.SQLToGrant)

				// Verify we have documented privileges
				Expect(req.Privileges).NotTo(BeEmpty(),
					"Operation %s should have documented privileges", req.Operation)
				Expect(req.SQLToGrant).NotTo(BeEmpty(),
					"Operation %s should have SQL grant statement", req.Operation)
			})
		}
	})

	Context("Privilege Constraints", func() {
		It("should NOT require SUPERUSER for any operation", func() {
			for _, req := range privilegeMatrix {
				for _, priv := range req.Privileges {
					Expect(priv).NotTo(Equal("SUPERUSER"),
						"Operation %s should not require SUPERUSER privilege", req.Operation)
				}
			}
		})

		It("should document all database operations", func() {
			// Ensure we have documented the minimum expected operations
			expectedOps := []string{
				"CreateDatabase",
				"DropDatabase",
				"CreateUser",
				"DropUser",
				"CreateRole",
				"GrantRole",
			}

			documentedOps := make(map[string]bool)
			for _, req := range privilegeMatrix {
				documentedOps[req.Operation] = true
			}

			for _, op := range expectedOps {
				Expect(documentedOps[op]).To(BeTrue(),
					"Operation %s should be documented in privilege matrix", op)
			}
		})
	})

	Context("Privilege Summary", func() {
		It("should print complete privilege setup SQL", func() {
			GinkgoWriter.Printf("\n\n====== PostgreSQL Least-Privilege Admin Setup ======\n\n")
			GinkgoWriter.Printf("-- Create the operator admin role\n")
			GinkgoWriter.Printf("CREATE ROLE dbprovision_admin WITH\n")
			GinkgoWriter.Printf("    LOGIN\n")
			GinkgoWriter.Printf("    CREATEDB\n")
			GinkgoWriter.Printf("    CREATEROLE\n")
			GinkgoWriter.Printf("    PASSWORD 'your-secure-password';\n\n")
			GinkgoWriter.Printf("-- Grant connection termination capability (required for force-drop)\n")
			GinkgoWriter.Printf("GRANT pg_signal_backend TO dbprovision_admin;\n\n")
			GinkgoWriter.Printf("-- Grant read access for backup operations (PostgreSQL 14+)\n")
			GinkgoWriter.Printf("GRANT pg_read_all_data TO dbprovision_admin;\n\n")
			GinkgoWriter.Printf("-- Grant template1 access for database creation\n")
			GinkgoWriter.Printf("GRANT CONNECT ON DATABASE template1 TO dbprovision_admin;\n\n")
			GinkgoWriter.Printf("-- Verify setup (should show: f, t, t, t)\n")
			GinkgoWriter.Printf("SELECT rolsuper, rolcreaterole, rolcreatedb, rolcanlogin\n")
			GinkgoWriter.Printf("FROM pg_roles WHERE rolname = 'dbprovision_admin';\n\n")
			GinkgoWriter.Printf("====================================================\n\n")
		})
	})
})

// TestLeastPrivilegeOperations is a standard Go test that documents
// the privilege requirements for PostgreSQL operations.
func TestLeastPrivilegeOperations(t *testing.T) {
	t.Log("PostgreSQL Least-Privilege Requirements")
	t.Log("========================================")

	privilegeMatrix := map[string][]string{
		"CreateDatabase":       {"CREATEDB"},
		"DropDatabase":         {"CREATEDB"},
		"ForceDropDatabase":    {"CREATEDB", "pg_signal_backend"},
		"CreateUser":           {"CREATEROLE"},
		"DropUser":             {"CREATEROLE"},
		"AlterUser":            {"CREATEROLE"},
		"CreateRole":           {"CREATEROLE"},
		"DropRole":             {"CREATEROLE"},
		"GrantRole":            {"CREATEROLE"},
		"RevokeRole":           {"CREATEROLE"},
		"GrantPrivileges":      {"CREATEROLE"},
		"RevokePrivileges":     {"CREATEROLE"},
		"TerminateConnections": {"pg_signal_backend"},
		"BackupDatabase":       {"pg_read_all_data"},
	}

	for operation, requiredPrivs := range privilegeMatrix {
		t.Run(operation, func(t *testing.T) {
			t.Logf("Operation: %s", operation)
			t.Logf("Required privileges: %v", requiredPrivs)

			// Verify no operation requires SUPERUSER
			for _, priv := range requiredPrivs {
				if priv == "SUPERUSER" {
					t.Errorf("Operation %s should not require SUPERUSER", operation)
				}
			}
		})
	}
}
