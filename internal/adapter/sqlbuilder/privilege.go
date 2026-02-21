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

package sqlbuilder

import (
	"fmt"
	"strings"
)

// ValidPostgresPrivileges is the allowlist of PostgreSQL privileges.
var ValidPostgresPrivileges = map[string]bool{
	"SELECT":         true,
	"INSERT":         true,
	"UPDATE":         true,
	"DELETE":         true,
	"TRUNCATE":       true,
	"REFERENCES":     true,
	"TRIGGER":        true,
	"CREATE":         true,
	"CONNECT":        true,
	"TEMPORARY":      true,
	"TEMP":           true,
	"EXECUTE":        true,
	"USAGE":          true,
	"ALL":            true,
	"ALL PRIVILEGES": true,
}

// ValidMySQLPrivileges is the allowlist of MySQL privileges.
var ValidMySQLPrivileges = map[string]bool{
	"ALL":                     true,
	"ALL PRIVILEGES":          true,
	"ALTER":                   true,
	"ALTER ROUTINE":           true,
	"CREATE":                  true,
	"CREATE ROUTINE":          true,
	"CREATE TABLESPACE":       true,
	"CREATE TEMPORARY TABLES": true,
	"CREATE USER":             true,
	"CREATE VIEW":             true,
	"DELETE":                  true,
	"DROP":                    true,
	"EVENT":                   true,
	"EXECUTE":                 true,
	"FILE":                    true,
	"GRANT OPTION":            true,
	"INDEX":                   true,
	"INSERT":                  true,
	"LOCK TABLES":             true,
	"PROCESS":                 true,
	"REFERENCES":              true,
	"RELOAD":                  true,
	"REPLICATION CLIENT":      true,
	"REPLICATION SLAVE":       true,
	"SELECT":                  true,
	"SHOW DATABASES":          true,
	"SHOW VIEW":               true,
	"SHUTDOWN":                true,
	"SUPER":                   true,
	"TRIGGER":                 true,
	"UPDATE":                  true,
	"USAGE":                   true,
}

// ValidatePrivileges checks that every privilege is in the allowlist.
// Returns an error listing the first invalid privilege found.
func ValidatePrivileges(privileges []string, allowlist map[string]bool) error {
	if len(privileges) == 0 {
		return fmt.Errorf("sqlbuilder: no privileges specified")
	}
	for _, p := range privileges {
		upper := strings.ToUpper(strings.TrimSpace(p))
		if !allowlist[upper] {
			return fmt.Errorf("sqlbuilder: invalid privilege %q", p)
		}
	}
	return nil
}
