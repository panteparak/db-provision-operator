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
	"strings"
	"testing"
)

// TestGrantBuilder_AllTargets covers target setters that are untested or only
// partially tested in builder_test.go.
func TestGrantBuilder_AllTargets(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		err  bool
	}{
		{
			name: "OnAllSequencesInSchema",
			sql: func() string {
				q, _ := NewPg().Grant("USAGE").OnAllSequencesInSchema("public").To("reader").Build()
				return q
			}(),
		},
		{
			name: "OnMySQLFunction",
			sql: func() string {
				q, _ := NewMySQL().Grant("EXECUTE").OnMySQLFunction("mydb", "myfn").ToLiteral("appuser").Build()
				return q
			}(),
		},
		{
			name: "OnSequence without schema",
			sql: func() string {
				q, _ := NewPg().Grant("USAGE").OnSequence("", "my_seq").To("reader").Build()
				return q
			}(),
		},
		{
			name: "OnFunction without schema",
			sql: func() string {
				q, _ := NewPg().Grant("EXECUTE").OnFunction("", "my_func").To("reader").Build()
				return q
			}(),
		},
		{
			name: "OnTable without schema",
			sql: func() string {
				q, _ := NewPg().Grant("SELECT").OnTable("", "users").To("reader").Build()
				return q
			}(),
		},
	}

	want := []string{
		`GRANT USAGE ON ALL SEQUENCES IN SCHEMA "public" TO "reader"`,
		"GRANT EXECUTE ON FUNCTION `mydb`.`myfn` TO 'appuser'",
		`GRANT USAGE ON SEQUENCE "my_seq" TO "reader"`,
		`GRANT EXECUTE ON FUNCTION "my_func" TO "reader"`,
		`GRANT SELECT ON TABLE "users" TO "reader"`,
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sql != want[i] {
				t.Errorf("got  %q\nwant %q", tt.sql, want[i])
			}
		})
	}
}

// TestGrantBuilder_AlterDefaultPrivileges_AllObjectTypes covers ADP object
// type setters not tested in builder_test.go.
func TestGrantBuilder_AlterDefaultPrivileges_AllObjectTypes(t *testing.T) {
	tests := []struct {
		name    string
		builder func() (string, error)
		want    string
	}{
		{
			name: "OnFunctions",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("owner", "public").
					Grant("EXECUTE").OnFunctions().To("reader").Build()
			},
			want: `ALTER DEFAULT PRIVILEGES FOR ROLE "owner" IN SCHEMA "public" GRANT EXECUTE ON FUNCTIONS TO "reader"`,
		},
		{
			name: "OnTypes",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("owner", "public").
					Grant("USAGE").OnTypes().To("reader").Build()
			},
			want: `ALTER DEFAULT PRIVILEGES FOR ROLE "owner" IN SCHEMA "public" GRANT USAGE ON TYPES TO "reader"`,
		},
		{
			name: "OnSchemas",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("owner", "").
					Grant("USAGE").OnSchemas().To("reader").Build()
			},
			want: `ALTER DEFAULT PRIVILEGES FOR ROLE "owner" GRANT USAGE ON SCHEMAS TO "reader"`,
		},
		{
			name: "without forRole",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("", "public").
					Grant("SELECT").OnTables().To("reader").Build()
			},
			want: `ALTER DEFAULT PRIVILEGES IN SCHEMA "public" GRANT SELECT ON TABLES TO "reader"`,
		},
		{
			name: "without forRole and without schema",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("", "").
					Grant("SELECT").OnTables().To("reader").Build()
			},
			want: `ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO "reader"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.builder()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestGrantBuilder_ErrorPaths covers all error cases in GrantBuilder.Build().
func TestGrantBuilder_ErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		builder func() (string, error)
		wantErr string
	}{
		{
			// Zero-value grantAction is actionGrant (iota=0), so it enters
			// buildGrantRevoke which validates privileges first.
			name:    "zero-value builder hits privilege validation",
			builder: func() (string, error) { return (&GrantBuilder{dialect: PgDialect{}}).Build() },
			wantErr: "no privileges specified",
		},
		{
			// An action value beyond the defined constants hits the default case.
			name:    "unknown action",
			builder: func() (string, error) { return (&GrantBuilder{dialect: PgDialect{}, action: 99}).Build() },
			wantErr: "no action specified",
		},
		{
			name: "grant no target",
			builder: func() (string, error) {
				return NewPg().Grant("SELECT").To("user").Build()
			},
			wantErr: "no target object specified",
		},
		{
			name: "grant no grantee",
			builder: func() (string, error) {
				return NewPg().Grant("SELECT").OnTable("", "t").Build()
			},
			wantErr: "no grantee specified",
		},
		{
			name: "grant invalid privilege",
			builder: func() (string, error) {
				return NewPg().Grant("INVALID_PRIV").OnTable("", "t").To("u").Build()
			},
			wantErr: "invalid privilege",
		},
		{
			name: "grant empty privileges",
			builder: func() (string, error) {
				return NewPg().Grant().OnTable("", "t").To("u").Build()
			},
			wantErr: "no privileges specified",
		},
		{
			name: "revoke no target",
			builder: func() (string, error) {
				return NewPg().Revoke("SELECT").From("user").Build()
			},
			wantErr: "no target object specified",
		},
		{
			name: "revoke no grantee",
			builder: func() (string, error) {
				return NewPg().Revoke("SELECT").OnTable("", "t").Build()
			},
			wantErr: "no grantee specified",
		},
		{
			name: "grant role empty role",
			builder: func() (string, error) {
				return NewPg().GrantRole("").To("user").Build()
			},
			wantErr: "no role specified",
		},
		{
			name: "grant role no grantee",
			builder: func() (string, error) {
				return NewPg().GrantRole("admin").Build()
			},
			wantErr: "no grantee specified",
		},
		{
			name: "revoke role empty role",
			builder: func() (string, error) {
				return NewPg().RevokeRole("").From("user").Build()
			},
			wantErr: "no role specified",
		},
		{
			name: "revoke role no grantee",
			builder: func() (string, error) {
				return NewPg().RevokeRole("admin").Build()
			},
			wantErr: "no grantee specified",
		},
		{
			name: "ADP no object type",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("owner", "public").
					Grant("SELECT").To("reader").Build()
			},
			wantErr: "no object type specified",
		},
		{
			name: "ADP no grantee",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("owner", "public").
					Grant("SELECT").OnTables().Build()
			},
			wantErr: "no grantee specified",
		},
		{
			name: "ADP invalid privilege",
			builder: func() (string, error) {
				return NewPg().AlterDefaultPrivileges("owner", "public").
					Grant("FAKE").OnTables().To("reader").Build()
			},
			wantErr: "invalid privilege",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.builder()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil (sql=%q)", tt.wantErr, got)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
			if got != "" {
				t.Errorf("expected empty string on error, got %q", got)
			}
		})
	}
}

// TestGrantBuilder_ChainingBehavior documents how repeated/conflicting
// setter calls behave.
func TestGrantBuilder_ChainingBehavior(t *testing.T) {
	tests := []struct {
		name    string
		builder func() (string, error)
		want    string
	}{
		{
			name: "To called twice - last wins",
			builder: func() (string, error) {
				return NewPg().Grant("SELECT").OnTable("", "t").To("first").To("second").Build()
			},
			want: `GRANT SELECT ON TABLE "t" TO "second"`,
		},
		{
			name: "WithGrantOption on REVOKE is silently ignored",
			builder: func() (string, error) {
				return NewPg().Revoke("SELECT").OnTable("", "t").From("user").WithGrantOption().Build()
			},
			want: `REVOKE SELECT ON TABLE "t" FROM "user"`,
		},
		{
			name: "target overwritten - last target setter wins",
			builder: func() (string, error) {
				return NewPg().Grant("SELECT").OnTable("", "first_table").
					OnSchema("my_schema").To("user").Build()
			},
			want: `GRANT SELECT ON SCHEMA "my_schema" TO "user"`,
		},
		{
			name: "From called after To - overwrites grantee",
			builder: func() (string, error) {
				return NewPg().Revoke("SELECT").OnTable("", "t").To("first").From("second").Build()
			},
			want: `REVOKE SELECT ON TABLE "t" FROM "second"`,
		},
		{
			name: "privilege normalization - whitespace and case",
			builder: func() (string, error) {
				return NewPg().Grant(" select ", " INSERT ").OnTable("", "t").To("u").Build()
			},
			want: `GRANT SELECT, INSERT ON TABLE "t" TO "u"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.builder()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}
