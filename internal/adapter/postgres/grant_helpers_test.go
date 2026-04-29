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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
)

var _ = Describe("hasTablePrivilege", func() {
	DescribeTable("classifies privileges",
		func(privs []string, expected bool) {
			Expect(hasTablePrivilege(privs)).To(Equal(expected))
		},
		Entry("empty list", []string{}, false),
		Entry("SELECT alone", []string{"SELECT"}, true),
		Entry("ALL alone", []string{"ALL"}, true),
		Entry("lowercase select", []string{"select"}, true),
		Entry("INSERT and UPDATE", []string{"INSERT", "UPDATE"}, true),
		Entry("only USAGE (schema-level)", []string{"USAGE"}, false),
		Entry("only EXECUTE (function-level)", []string{"EXECUTE"}, false),
		Entry("USAGE then SELECT", []string{"USAGE", "SELECT"}, true),
		Entry("REFERENCES", []string{"REFERENCES"}, true),
		Entry("TRIGGER", []string{"TRIGGER"}, true),
		Entry("TRUNCATE", []string{"TRUNCATE"}, true),
		Entry("CONNECT (database-level)", []string{"CONNECT"}, false),
	)
})

var _ = Describe("buildGrantQuery", func() {
	It("appends WITH GRANT OPTION when withGrantOption is true", func() {
		b := sqlbuilder.NewPg().Grant("SELECT").OnTable("public", "users").To("alice")
		q, err := buildGrantQuery(b, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(q).To(ContainSubstring("WITH GRANT OPTION"))
	})

	It("omits WITH GRANT OPTION when withGrantOption is false", func() {
		b := sqlbuilder.NewPg().Grant("SELECT").OnTable("public", "users").To("alice")
		q, err := buildGrantQuery(b, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(q).NotTo(ContainSubstring("WITH GRANT OPTION"))
	})

	It("propagates builder errors", func() {
		// Empty privilege list triggers a builder error.
		b := sqlbuilder.NewPg().Grant().OnTable("public", "users").To("alice")
		_, err := buildGrantQuery(b, false)
		Expect(err).To(HaveOccurred())
	})
})
