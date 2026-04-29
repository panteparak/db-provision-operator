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

package cockroachdb

import (
	"strings"

	"github.com/db-provision-operator/internal/adapter/sqlbuilder"
)

// tablePrivileges enumerates the privilege keywords that imply a TABLE-level
// grant. ALL is included because it covers the table-level subset.
var tablePrivileges = map[string]struct{}{
	"SELECT":     {},
	"INSERT":     {},
	"UPDATE":     {},
	"DELETE":     {},
	"TRUNCATE":   {},
	"REFERENCES": {},
	"TRIGGER":    {},
	"ALL":        {},
}

// hasTablePrivilege reports whether any privilege in privs is a table-level
// keyword. Used to decide whether ALL TABLES IN SCHEMA needs a GRANT.
func hasTablePrivilege(privs []string) bool {
	for _, p := range privs {
		if _, ok := tablePrivileges[strings.ToUpper(p)]; ok {
			return true
		}
	}
	return false
}

// buildGrantQuery applies the WITH GRANT OPTION flag if requested, then builds
// the final SQL. Centralises the WithGrantOption + Build pair.
func buildGrantQuery(b *sqlbuilder.GrantBuilder, withGrantOption bool) (string, error) {
	if withGrantOption {
		b.WithGrantOption()
	}
	return b.Build()
}
