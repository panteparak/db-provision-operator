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

package mysql

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// MySQL Privilege Matrix
//
// This test documents and validates the minimum privileges required for each
// database operation. The operator uses a least-privilege admin account
// (dbprovision_admin) instead of root/superuser.
//
// IMPORTANT: MySQL and MariaDB have different privilege syntax:
// - MySQL uses CONNECTION_ADMIN (underscore)
// - MariaDB uses CONNECTION ADMIN (space)
// - MySQL has ROLE_ADMIN privilege
// - MariaDB uses CREATE USER + WITH GRANT OPTION for role management

var _ = Describe("MySQL Privilege Requirements", func() {
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
			Privileges:  []string{"CREATE ON *.*"},
			Description: "Create new databases",
			SQLToGrant:  "GRANT CREATE ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "DropDatabase",
			Privileges:  []string{"DROP ON *.*"},
			Description: "Drop existing databases",
			SQLToGrant:  "GRANT DROP ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "AlterDatabase",
			Privileges:  []string{"ALTER ON *.*"},
			Description: "Alter database properties (character set, collation)",
			SQLToGrant:  "GRANT ALTER ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "ForceDropDatabase",
			Privileges:  []string{"DROP ON *.*", "CONNECTION_ADMIN ON *.*"},
			Description: "Drop database by terminating active connections first",
			SQLToGrant:  "GRANT DROP, CONNECTION_ADMIN ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "CreateUser",
			Privileges:  []string{"CREATE USER ON *.*"},
			Description: "Create new database users",
			SQLToGrant:  "GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "DropUser",
			Privileges:  []string{"CREATE USER ON *.*"},
			Description: "Drop existing users",
			SQLToGrant:  "GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "AlterUser",
			Privileges:  []string{"CREATE USER ON *.*"},
			Description: "Modify user attributes (password, account lock, etc.)",
			SQLToGrant:  "GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "GrantPrivileges",
			Privileges:  []string{"GRANT OPTION ON *.*"},
			Description: "Grant privileges to users (requires having the privilege + GRANT OPTION)",
			SQLToGrant:  "GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "RevokePrivileges",
			Privileges:  []string{"GRANT OPTION ON *.*"},
			Description: "Revoke privileges from users",
			SQLToGrant:  "GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "CreateRole",
			Privileges:  []string{"ROLE_ADMIN ON *.*"},
			Description: "Create database roles (MySQL 8.0+)",
			SQLToGrant:  "GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "DropRole",
			Privileges:  []string{"ROLE_ADMIN ON *.*"},
			Description: "Drop database roles (MySQL 8.0+)",
			SQLToGrant:  "GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "GrantRole",
			Privileges:  []string{"ROLE_ADMIN ON *.*"},
			Description: "Grant role membership to users",
			SQLToGrant:  "GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "RevokeRole",
			Privileges:  []string{"ROLE_ADMIN ON *.*"},
			Description: "Revoke role membership from users",
			SQLToGrant:  "GRANT ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "TerminateConnections",
			Privileges:  []string{"CONNECTION_ADMIN ON *.*"},
			Description: "Kill database connections (for force-drop)",
			SQLToGrant:  "GRANT CONNECTION_ADMIN ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "QueryUserMetadata",
			Privileges:  []string{"SELECT ON mysql.user"},
			Description: "Query user information from mysql.user table",
			SQLToGrant:  "GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "QueryGrantMetadata",
			Privileges:  []string{"SELECT ON mysql.db", "SELECT ON mysql.tables_priv"},
			Description: "Query grant information from mysql system tables",
			SQLToGrant:  "GRANT SELECT ON mysql.db, mysql.tables_priv TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "FlushPrivileges",
			Privileges:  []string{"RELOAD ON *.*"},
			Description: "Reload privilege tables",
			SQLToGrant:  "GRANT RELOAD ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "QueryProcessList",
			Privileges:  []string{"PROCESS ON *.*"},
			Description: "View process list for connection management",
			SQLToGrant:  "GRANT PROCESS ON *.* TO 'dbprovision_admin'@'%'",
		},
		{
			Operation:   "BackupDatabase",
			Privileges:  []string{"SELECT ON *.*", "SHOW VIEW ON *.*", "TRIGGER ON *.*", "LOCK TABLES ON *.*"},
			Description: "Read data and structure for backup operations",
			SQLToGrant:  "GRANT SELECT, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%'",
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
		It("should NOT require SUPER privilege for any operation", func() {
			for _, req := range privilegeMatrix {
				for _, priv := range req.Privileges {
					Expect(priv).NotTo(ContainSubstring("SUPER"),
						"Operation %s should not require SUPER privilege", req.Operation)
				}
			}
		})

		It("should NOT require ALL PRIVILEGES for any operation", func() {
			for _, req := range privilegeMatrix {
				for _, priv := range req.Privileges {
					Expect(priv).NotTo(ContainSubstring("ALL PRIVILEGES"),
						"Operation %s should not require ALL PRIVILEGES", req.Operation)
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
		It("should print complete MySQL privilege setup SQL", func() {
			GinkgoWriter.Printf("\n\n====== MySQL Least-Privilege Admin Setup ======\n\n")
			GinkgoWriter.Printf("-- Create the operator admin user\n")
			GinkgoWriter.Printf("CREATE USER 'dbprovision_admin'@'%%' IDENTIFIED BY 'your-secure-password';\n\n")
			GinkgoWriter.Printf("-- Database operations\n")
			GinkgoWriter.Printf("GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- User management\n")
			GinkgoWriter.Printf("GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT GRANT OPTION ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- System table access for metadata queries\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.global_grants TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- Administrative operations\n")
			GinkgoWriter.Printf("GRANT RELOAD, PROCESS ON *.* TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT CONNECTION_ADMIN, ROLE_ADMIN ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- Data operations (for granting to app users)\n")
			GinkgoWriter.Printf("GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("FLUSH PRIVILEGES;\n\n")
			GinkgoWriter.Printf("-- Verify (should NOT show ALL PRIVILEGES or SUPER)\n")
			GinkgoWriter.Printf("SHOW GRANTS FOR 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("================================================\n\n")
		})

		It("should print complete MariaDB privilege setup SQL", func() {
			GinkgoWriter.Printf("\n\n====== MariaDB Least-Privilege Admin Setup ======\n\n")
			GinkgoWriter.Printf("-- NOTE: MariaDB uses 'CONNECTION ADMIN' (with space)\n")
			GinkgoWriter.Printf("-- NOTE: MariaDB does not have ROLE_ADMIN privilege\n")
			GinkgoWriter.Printf("-- NOTE: MariaDB does not have mysql.global_grants table\n\n")
			GinkgoWriter.Printf("CREATE USER 'dbprovision_admin'@'%%' IDENTIFIED BY 'your-secure-password';\n\n")
			GinkgoWriter.Printf("-- Database operations\n")
			GinkgoWriter.Printf("GRANT CREATE, DROP, ALTER ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- User management (WITH GRANT OPTION is critical for role delegation)\n")
			GinkgoWriter.Printf("GRANT CREATE USER ON *.* TO 'dbprovision_admin'@'%%' WITH GRANT OPTION;\n\n")
			GinkgoWriter.Printf("-- System table access\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.user TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.db TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.tables_priv TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.columns_priv TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT SELECT ON mysql.procs_priv TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- Administrative (note: CONNECTION ADMIN with space)\n")
			GinkgoWriter.Printf("GRANT RELOAD, PROCESS ON *.* TO 'dbprovision_admin'@'%%';\n")
			GinkgoWriter.Printf("GRANT CONNECTION ADMIN ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("-- Data operations\n")
			GinkgoWriter.Printf("GRANT SELECT, INSERT, UPDATE, DELETE, SHOW VIEW, TRIGGER, LOCK TABLES ON *.* TO 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("FLUSH PRIVILEGES;\n\n")
			GinkgoWriter.Printf("-- Verify\n")
			GinkgoWriter.Printf("SHOW GRANTS FOR 'dbprovision_admin'@'%%';\n\n")
			GinkgoWriter.Printf("=================================================\n\n")
		})
	})
})

// TestMySQLLeastPrivilegeOperations is a standard Go test that documents
// the privilege requirements for MySQL operations.
func TestMySQLLeastPrivilegeOperations(t *testing.T) {
	t.Log("MySQL Least-Privilege Requirements")
	t.Log("===================================")

	privilegeMatrix := map[string][]string{
		"CreateDatabase":       {"CREATE ON *.*"},
		"DropDatabase":         {"DROP ON *.*"},
		"AlterDatabase":        {"ALTER ON *.*"},
		"ForceDropDatabase":    {"DROP ON *.*", "CONNECTION_ADMIN ON *.*"},
		"CreateUser":           {"CREATE USER ON *.*"},
		"DropUser":             {"CREATE USER ON *.*"},
		"AlterUser":            {"CREATE USER ON *.*"},
		"GrantPrivileges":      {"GRANT OPTION ON *.*"},
		"RevokePrivileges":     {"GRANT OPTION ON *.*"},
		"CreateRole":           {"ROLE_ADMIN ON *.*"},
		"DropRole":             {"ROLE_ADMIN ON *.*"},
		"GrantRole":            {"ROLE_ADMIN ON *.*"},
		"RevokeRole":           {"ROLE_ADMIN ON *.*"},
		"TerminateConnections": {"CONNECTION_ADMIN ON *.*"},
		"FlushPrivileges":      {"RELOAD ON *.*"},
	}

	for operation, requiredPrivs := range privilegeMatrix {
		t.Run(operation, func(t *testing.T) {
			t.Logf("Operation: %s", operation)
			t.Logf("Required privileges: %v", requiredPrivs)

			// Verify no operation requires SUPER or ALL PRIVILEGES
			for _, priv := range requiredPrivs {
				if priv == "SUPER" || priv == "SUPER ON *.*" {
					t.Errorf("Operation %s should not require SUPER", operation)
				}
				if priv == "ALL PRIVILEGES" || priv == "ALL PRIVILEGES ON *.*" {
					t.Errorf("Operation %s should not require ALL PRIVILEGES", operation)
				}
			}
		})
	}
}

// TestMariaDBLeastPrivilegeOperations documents MariaDB-specific differences.
func TestMariaDBLeastPrivilegeOperations(t *testing.T) {
	t.Log("MariaDB Least-Privilege Requirements")
	t.Log("=====================================")
	t.Log("Key differences from MySQL:")
	t.Log("- Uses 'CONNECTION ADMIN' (with space) instead of 'CONNECTION_ADMIN'")
	t.Log("- Does not have ROLE_ADMIN privilege (roles are users)")
	t.Log("- Does not have mysql.global_grants table")
	t.Log("")

	mariaDBPrivilegeMatrix := map[string][]string{
		"CreateDatabase":       {"CREATE ON *.*"},
		"DropDatabase":         {"DROP ON *.*"},
		"AlterDatabase":        {"ALTER ON *.*"},
		"ForceDropDatabase":    {"DROP ON *.*", "CONNECTION ADMIN ON *.*"},
		"CreateUser":           {"CREATE USER ON *.*"},
		"DropUser":             {"CREATE USER ON *.*"},
		"AlterUser":            {"CREATE USER ON *.*"},
		"GrantPrivileges":      {"WITH GRANT OPTION"},
		"CreateRole":           {"CREATE USER ON *.*"}, // Roles are users in MariaDB
		"DropRole":             {"CREATE USER ON *.*"},
		"GrantRole":            {"CREATE USER ON *.*", "WITH GRANT OPTION"},
		"RevokeRole":           {"CREATE USER ON *.*", "WITH GRANT OPTION"},
		"TerminateConnections": {"CONNECTION ADMIN ON *.*"},
		"FlushPrivileges":      {"RELOAD ON *.*"},
	}

	for operation, requiredPrivs := range mariaDBPrivilegeMatrix {
		t.Run("MariaDB_"+operation, func(t *testing.T) {
			t.Logf("Operation: %s", operation)
			t.Logf("Required privileges: %v", requiredPrivs)

			// Verify no operation requires SUPER
			for _, priv := range requiredPrivs {
				if priv == "SUPER" || priv == "SUPER ON *.*" {
					t.Errorf("Operation %s should not require SUPER", operation)
				}
			}
		})
	}
}
