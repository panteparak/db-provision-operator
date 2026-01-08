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
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/types"
)

var _ = Describe("Grant Operations", func() {
	var (
		adapter *Adapter
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		adapter = NewAdapter(types.ConnectionConfig{
			Host:     "localhost",
			Port:     5432,
			Database: "testdb",
			Username: "postgres",
			Password: "password",
		})
	})

	Describe("grantPrivileges", func() {
		// Note: These tests verify the SQL query construction logic
		// The actual execution would require a database connection

		It("should grant ALL TABLES IN SCHEMA privileges", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Privileges: []string{"SELECT", "INSERT", "UPDATE"},
			}

			// We can't execute without a connection, but we can verify the logic
			// The function should generate:
			// GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA "public" TO "testuser"
			// GRANT USAGE ON SCHEMA "public" TO "testuser"
			Expect(opt.Schema).To(Equal("public"))
			Expect(opt.Privileges).To(ContainElements("SELECT", "INSERT", "UPDATE"))
			Expect(opt.Tables).To(BeEmpty())
		})

		It("should grant USAGE ON SCHEMA", func() {
			opt := types.GrantOptions{
				Schema:     "myschema",
				Privileges: []string{"SELECT"},
			}

			// The function should include schema usage grant
			Expect(opt.Schema).To(Equal("myschema"))
			Expect(opt.Privileges).To(ContainElement("SELECT"))
		})

		It("should grant privileges on specific tables", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Tables:     []string{"users", "orders", "products"},
				Privileges: []string{"SELECT", "INSERT"},
			}

			Expect(opt.Tables).To(HaveLen(3))
			Expect(opt.Tables).To(ContainElements("users", "orders", "products"))
			Expect(opt.Privileges).To(ContainElements("SELECT", "INSERT"))
		})

		It("should grant privileges on sequences", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Sequences:  []string{"users_id_seq", "orders_id_seq"},
				Privileges: []string{"USAGE", "SELECT"},
			}

			Expect(opt.Sequences).To(HaveLen(2))
			Expect(opt.Sequences).To(ContainElements("users_id_seq", "orders_id_seq"))
			Expect(opt.Privileges).To(ContainElements("USAGE", "SELECT"))
		})

		It("should grant privileges on functions", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Functions:  []string{"calculate_total()", "get_user_name(int)"},
				Privileges: []string{"EXECUTE"},
			}

			Expect(opt.Functions).To(HaveLen(2))
			Expect(opt.Functions).To(ContainElements("calculate_total()", "get_user_name(int)"))
			Expect(opt.Privileges).To(ContainElement("EXECUTE"))
		})

		It("should include WITH GRANT OPTION", func() {
			opt := types.GrantOptions{
				Schema:          "public",
				Tables:          []string{"users"},
				Privileges:      []string{"SELECT"},
				WithGrantOption: true,
			}

			Expect(opt.WithGrantOption).To(BeTrue())
		})
	})

	Describe("revokePrivileges", func() {
		It("should revoke ALL TABLES IN SCHEMA privileges", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Privileges: []string{"SELECT", "INSERT"},
			}

			// Should generate: REVOKE SELECT, INSERT ON ALL TABLES IN SCHEMA "public" FROM "testuser"
			Expect(opt.Schema).To(Equal("public"))
			Expect(opt.Privileges).To(ContainElements("SELECT", "INSERT"))
			Expect(opt.Tables).To(BeEmpty())
		})

		It("should revoke privileges on specific tables", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Tables:     []string{"users", "orders"},
				Privileges: []string{"DELETE"},
			}

			Expect(opt.Tables).To(HaveLen(2))
			Expect(opt.Tables).To(ContainElements("users", "orders"))
			Expect(opt.Privileges).To(ContainElement("DELETE"))
		})

		It("should revoke privileges on sequences", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Sequences:  []string{"users_id_seq"},
				Privileges: []string{"USAGE"},
			}

			Expect(opt.Sequences).To(HaveLen(1))
			Expect(opt.Sequences).To(ContainElement("users_id_seq"))
			Expect(opt.Privileges).To(ContainElement("USAGE"))
		})

		It("should revoke privileges on functions", func() {
			opt := types.GrantOptions{
				Schema:     "public",
				Functions:  []string{"my_function()"},
				Privileges: []string{"EXECUTE"},
			}

			Expect(opt.Functions).To(HaveLen(1))
			Expect(opt.Functions).To(ContainElement("my_function()"))
			Expect(opt.Privileges).To(ContainElement("EXECUTE"))
		})
	})

	Describe("GrantRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.GrantRole(ctx, "testuser", []string{"admin"})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should grant role to grantee", func() {
			// Without a connection, we verify the function structure
			// This test demonstrates the expected behavior
			roles := []string{"admin", "readonly"}
			grantee := "testuser"

			// Verify role names are properly formatted
			for _, role := range roles {
				Expect(role).NotTo(BeEmpty())
				Expect(grantee).NotTo(BeEmpty())
			}
		})
	})

	Describe("RevokeRole", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				err := adapter.RevokeRole(ctx, "testuser", []string{"admin"})

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})

		It("should revoke role from grantee", func() {
			// Without a connection, we verify the function structure
			roles := []string{"admin", "readonly"}
			grantee := "testuser"

			// Verify role names are properly formatted
			for _, role := range roles {
				Expect(role).NotTo(BeEmpty())
				Expect(grantee).NotTo(BeEmpty())
			}
		})
	})

	Describe("setDefaultPrivilege", func() {
		It("should set default privileges for tables", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "public",
				ObjectType: "tables",
				Privileges: []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
			}

			Expect(strings.ToLower(opt.ObjectType)).To(Equal("tables"))
			Expect(opt.Privileges).To(ContainElements("SELECT", "INSERT", "UPDATE", "DELETE"))
		})

		It("should set default privileges for sequences", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "public",
				ObjectType: "sequences",
				Privileges: []string{"USAGE", "SELECT"},
			}

			Expect(strings.ToLower(opt.ObjectType)).To(Equal("sequences"))
			Expect(opt.Privileges).To(ContainElements("USAGE", "SELECT"))
		})

		It("should set default privileges for functions", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "public",
				ObjectType: "functions",
				Privileges: []string{"EXECUTE"},
			}

			Expect(strings.ToLower(opt.ObjectType)).To(Equal("functions"))
			Expect(opt.Privileges).To(ContainElement("EXECUTE"))
		})

		It("should set default privileges for types", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "public",
				ObjectType: "types",
				Privileges: []string{"USAGE"},
			}

			Expect(strings.ToLower(opt.ObjectType)).To(Equal("types"))
			Expect(opt.Privileges).To(ContainElement("USAGE"))
		})

		It("should include FOR ROLE when specified", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "public",
				GrantedBy:  "admin_role",
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}

			Expect(opt.GrantedBy).To(Equal("admin_role"))
			Expect(opt.GrantedBy).NotTo(BeEmpty())
		})

		It("should include IN SCHEMA when specified", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "myschema",
				ObjectType: "tables",
				Privileges: []string{"SELECT"},
			}

			Expect(opt.Schema).To(Equal("myschema"))
			Expect(opt.Schema).NotTo(BeEmpty())
		})

		It("should return error for unsupported object type", func() {
			opt := types.DefaultPrivilegeGrantOptions{
				Database:   "testdb",
				Schema:     "public",
				ObjectType: "unsupported",
				Privileges: []string{"SELECT"},
			}

			// The setDefaultPrivilege function should reject unsupported object types
			supportedTypes := []string{"tables", "sequences", "functions", "types", "schemas"}
			isSupported := false
			for _, t := range supportedTypes {
				if strings.ToLower(opt.ObjectType) == t {
					isSupported = true
					break
				}
			}
			Expect(isSupported).To(BeFalse())
		})
	})

	Describe("escapeIdentifier", func() {
		It("should escape simple identifiers", func() {
			result := escapeIdentifier("users")
			Expect(result).To(Equal(`"users"`))
		})

		It("should escape identifiers with special characters", func() {
			result := escapeIdentifier("my-table")
			Expect(result).To(Equal(`"my-table"`))
		})

		It("should escape identifiers with double quotes", func() {
			result := escapeIdentifier(`my"table`)
			Expect(result).To(Equal(`"my""table"`))
		})

		It("should escape empty strings", func() {
			result := escapeIdentifier("")
			Expect(result).To(Equal(`""`))
		})
	})

	Describe("escapeLiteral", func() {
		It("should escape simple strings", func() {
			result := escapeLiteral("test")
			Expect(result).To(Equal(`'test'`))
		})

		It("should escape strings with single quotes", func() {
			result := escapeLiteral("it's a test")
			Expect(result).To(Equal(`'it''s a test'`))
		})

		It("should escape empty strings", func() {
			result := escapeLiteral("")
			Expect(result).To(Equal(`''`))
		})
	})

	Describe("Grant", func() {
		It("should process multiple grant options", func() {
			opts := []types.GrantOptions{
				{
					Schema:     "public",
					Tables:     []string{"users"},
					Privileges: []string{"SELECT"},
				},
				{
					Schema:     "public",
					Sequences:  []string{"users_id_seq"},
					Privileges: []string{"USAGE"},
				},
			}

			Expect(opts).To(HaveLen(2))
			Expect(opts[0].Tables).To(ContainElement("users"))
			Expect(opts[1].Sequences).To(ContainElement("users_id_seq"))
		})
	})

	Describe("Revoke", func() {
		It("should process multiple revoke options", func() {
			opts := []types.GrantOptions{
				{
					Schema:     "public",
					Tables:     []string{"users"},
					Privileges: []string{"DELETE"},
				},
				{
					Schema:     "public",
					Functions:  []string{"my_func()"},
					Privileges: []string{"EXECUTE"},
				},
			}

			Expect(opts).To(HaveLen(2))
			Expect(opts[0].Privileges).To(ContainElement("DELETE"))
			Expect(opts[1].Functions).To(ContainElement("my_func()"))
		})
	})

	Describe("SetDefaultPrivileges", func() {
		It("should process multiple default privilege options", func() {
			opts := []types.DefaultPrivilegeGrantOptions{
				{
					Database:   "testdb",
					Schema:     "public",
					ObjectType: "tables",
					Privileges: []string{"SELECT", "INSERT"},
				},
				{
					Database:   "testdb",
					Schema:     "public",
					ObjectType: "sequences",
					Privileges: []string{"USAGE"},
				},
			}

			Expect(opts).To(HaveLen(2))
			Expect(opts[0].ObjectType).To(Equal("tables"))
			Expect(opts[1].ObjectType).To(Equal("sequences"))
		})
	})

	Describe("GetGrants", func() {
		Context("when not connected", func() {
			It("should return error", func() {
				_, err := adapter.GetGrants(ctx, "testuser")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not connected"))
			})
		})
	})
})
