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

package clickhouse

import (
	"context"
	"strings"
	"testing"

	"github.com/db-provision-operator/internal/adapter/types"
)

// ---------------------------------------------------------------------------
// escapeIdentifier
// ---------------------------------------------------------------------------

func TestEscapeIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple identifier",
			input: "mydb",
			want:  "`mydb`",
		},
		{
			name:  "identifier with underscore",
			input: "my_table",
			want:  "`my_table`",
		},
		{
			name:  "identifier with spaces",
			input: "my table",
			want:  "`my table`",
		},
		{
			name:  "identifier with hyphen",
			input: "my-table",
			want:  "`my-table`",
		},
		{
			name:  "identifier with dollar sign",
			input: "table$name",
			want:  "`table$name`",
		},
		{
			name:  "empty string",
			input: "",
			want:  "``",
		},
		{
			name:  "backtick injection: single backtick",
			input: "a`b",
			want:  "`a``b`",
		},
		{
			name:  "backtick injection: multiple backticks",
			input: "a`b`c",
			want:  "`a``b``c`",
		},
		{
			name:  "backtick injection: leading backtick",
			input: "`malicious",
			want:  "```malicious`",
		},
		{
			name:  "backtick injection: trailing backtick",
			input: "malicious`",
			want:  "`malicious```",
		},
		{
			name:  "backtick injection: attempt to break out",
			input: "db` UNION SELECT 1--",
			want:  "`db`` UNION SELECT 1--`",
		},
		{
			name:  "identifier with newline",
			input: "line1\nline2",
			want:  "`line1\nline2`",
		},
		{
			name:  "identifier with dot",
			input: "schema.table",
			want:  "`schema.table`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeIdentifier(tc.input)
			if got != tc.want {
				t.Errorf("escapeIdentifier(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEscapeIdentifier_AlwaysWrapsInBackticks(t *testing.T) {
	inputs := []string{"normal", "", "with`backtick", "   spaces   "}
	for _, in := range inputs {
		out := escapeIdentifier(in)
		if !strings.HasPrefix(out, "`") || !strings.HasSuffix(out, "`") {
			t.Errorf("escapeIdentifier(%q) = %q: does not start and end with backtick", in, out)
		}
	}
}

// ---------------------------------------------------------------------------
// escapeLiteral
// ---------------------------------------------------------------------------

func TestEscapeLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple value",
			input: "mypassword",
			want:  "'mypassword'",
		},
		{
			name:  "empty string",
			input: "",
			want:  "''",
		},
		{
			name:  "value with spaces",
			input: "hello world",
			want:  "'hello world'",
		},
		{
			name:  "SQL injection: single quote",
			input: "it's",
			want:  "'it''s'",
		},
		{
			name:  "SQL injection: multiple single quotes",
			input: "a'b'c",
			want:  "'a''b''c'",
		},
		{
			name:  "SQL injection: break-out attempt",
			input: "' OR '1'='1",
			want:  "''' OR ''1''=''1'",
		},
		{
			name:  "SQL injection: comment attempt",
			input: "'; DROP TABLE users; --",
			want:  "'''; DROP TABLE users; --'",
		},
		{
			name:  "backslash escaping",
			input: `back\slash`,
			want:  `'back\\slash'`,
		},
		{
			name:  "backslash followed by single quote",
			input: `\'`,
			want:  `'\\'''`,
		},
		{
			name:  "double backslash",
			input: `\\`,
			want:  `'\\\\'`,
		},
		{
			name:  "double quotes should not be escaped",
			input: `my"value`,
			want:  `'my"value'`,
		},
		{
			name:  "newline in value",
			input: "line1\nline2",
			want:  "'line1\nline2'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeLiteral(tc.input)
			if got != tc.want {
				t.Errorf("escapeLiteral(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestEscapeLiteral_AlwaysWrapsInSingleQuotes(t *testing.T) {
	inputs := []string{"normal", "", "with'quote", `with\backslash`}
	for _, in := range inputs {
		out := escapeLiteral(in)
		if !strings.HasPrefix(out, "'") || !strings.HasSuffix(out, "'") {
			t.Errorf("escapeLiteral(%q) = %q: does not start and end with single quote", in, out)
		}
	}
}

// ---------------------------------------------------------------------------
// SystemDatabases / SystemUsers
// ---------------------------------------------------------------------------

func TestSystemDatabases_ContainsExpectedEntries(t *testing.T) {
	required := []string{
		"system",
		"information_schema",
		"INFORMATION_SCHEMA",
		"default",
	}

	index := make(map[string]bool, len(SystemDatabases))
	for _, db := range SystemDatabases {
		index[db] = true
	}

	for _, name := range required {
		if !index[name] {
			t.Errorf("SystemDatabases missing expected entry %q", name)
		}
	}
}

func TestSystemDatabases_NoDuplicates(t *testing.T) {
	seen := make(map[string]int)
	for i, db := range SystemDatabases {
		if prev, ok := seen[db]; ok {
			t.Errorf("SystemDatabases[%d] = %q is a duplicate of index %d", i, db, prev)
		}
		seen[db] = i
	}
}

func TestSystemUsers_ContainsExpectedEntries(t *testing.T) {
	required := []string{"default"}

	index := make(map[string]bool, len(SystemUsers))
	for _, u := range SystemUsers {
		index[u] = true
	}

	for _, name := range required {
		if !index[name] {
			t.Errorf("SystemUsers missing expected entry %q", name)
		}
	}
}

func TestSystemUsers_NoDuplicates(t *testing.T) {
	seen := make(map[string]int)
	for i, u := range SystemUsers {
		if prev, ok := seen[u]; ok {
			t.Errorf("SystemUsers[%d] = %q is a duplicate of index %d", i, u, prev)
		}
		seen[u] = i
	}
}

func TestSystemDatabases_NotEmpty(t *testing.T) {
	if len(SystemDatabases) == 0 {
		t.Error("SystemDatabases must not be empty")
	}
}

func TestSystemUsers_NotEmpty(t *testing.T) {
	if len(SystemUsers) == 0 {
		t.Error("SystemUsers must not be empty")
	}
}

// ---------------------------------------------------------------------------
// buildClickHouseTarget
// ---------------------------------------------------------------------------

func TestBuildClickHouseTarget(t *testing.T) {
	tests := []struct {
		name string
		opt  types.GrantOptions
		want string
	}{
		{
			name: "global level",
			opt:  types.GrantOptions{Level: "global"},
			want: "*.*",
		},
		{
			name: "database level with database name",
			opt:  types.GrantOptions{Level: "database", Database: "mydb"},
			want: "`mydb`.*",
		},
		{
			name: "table level with database and table",
			opt:  types.GrantOptions{Level: "table", Database: "mydb", Table: "mytable"},
			want: "`mydb`.`mytable`",
		},
		{
			name: "default fallback: both database and table set",
			opt:  types.GrantOptions{Level: "", Database: "mydb", Table: "mytable"},
			want: "`mydb`.`mytable`",
		},
		{
			name: "default fallback: only database set",
			opt:  types.GrantOptions{Level: "", Database: "mydb"},
			want: "`mydb`.*",
		},
		{
			name: "default fallback: neither database nor table set",
			opt:  types.GrantOptions{Level: ""},
			want: "*.*",
		},
		{
			name: "unknown level falls back to global when no db/table",
			opt:  types.GrantOptions{Level: "unknown"},
			want: "*.*",
		},
		{
			name: "unknown level with database falls back to db.*",
			opt:  types.GrantOptions{Level: "unknown", Database: "mydb"},
			want: "`mydb`.*",
		},
		{
			name: "database name with backtick is escaped",
			opt:  types.GrantOptions{Level: "database", Database: "db`evil"},
			want: "`db``evil`.*",
		},
		{
			name: "table name with backtick is escaped",
			opt:  types.GrantOptions{Level: "table", Database: "db", Table: "tbl`evil"},
			want: "`db`.`tbl``evil`",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildClickHouseTarget(tc.opt)
			if got != tc.want {
				t.Errorf("buildClickHouseTarget(%+v) = %q, want %q", tc.opt, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseClickHouseGrantString
// ---------------------------------------------------------------------------

func TestParseClickHouseGrantString(t *testing.T) {
	tests := []struct {
		name         string
		grantStr     string
		grantee      string
		wantNil      bool
		wantPrivs    []string
		wantDatabase string
		wantObjType  string
		wantObjName  string
	}{
		{
			name:         "global grant",
			grantStr:     "GRANT SELECT ON *.* TO `myuser`",
			grantee:      "myuser",
			wantPrivs:    []string{"SELECT"},
			wantDatabase: "*",
			wantObjType:  "database",
			wantObjName:  "*",
		},
		{
			name:         "database-level grant",
			grantStr:     "GRANT SELECT, INSERT ON `mydb`.* TO `myuser`",
			grantee:      "myuser",
			wantPrivs:    []string{"SELECT", " INSERT"},
			wantDatabase: "mydb",
			wantObjType:  "database",
			wantObjName:  "mydb",
		},
		{
			name:         "table-level grant",
			grantStr:     "GRANT SELECT ON `mydb`.`mytable` TO `myuser`",
			grantee:      "myuser",
			wantPrivs:    []string{"SELECT"},
			wantDatabase: "mydb",
			wantObjType:  "table",
			wantObjName:  "mytable",
		},
		{
			name:     "malformed: missing ON clause",
			grantStr: "GRANT SELECT mytable TO myuser",
			grantee:  "myuser",
			wantNil:  true,
		},
		{
			name:         "grantee is preserved",
			grantStr:     "GRANT DROP ON `db`.`tbl` TO `alice`",
			grantee:      "alice",
			wantPrivs:    []string{"DROP"},
			wantDatabase: "db",
			wantObjType:  "table",
			wantObjName:  "tbl",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseClickHouseGrantString(tc.grantStr, tc.grantee)

			if tc.wantNil {
				if got != nil {
					t.Errorf("parseClickHouseGrantString(%q, %q) = %+v, want nil", tc.grantStr, tc.grantee, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("parseClickHouseGrantString(%q, %q) = nil, want non-nil", tc.grantStr, tc.grantee)
			}

			if got.Grantee != tc.grantee {
				t.Errorf("Grantee = %q, want %q", got.Grantee, tc.grantee)
			}

			if tc.wantDatabase != "" && got.Database != tc.wantDatabase {
				t.Errorf("Database = %q, want %q", got.Database, tc.wantDatabase)
			}

			if tc.wantObjType != "" && got.ObjectType != tc.wantObjType {
				t.Errorf("ObjectType = %q, want %q", got.ObjectType, tc.wantObjType)
			}

			if tc.wantObjName != "" && got.ObjectName != tc.wantObjName {
				t.Errorf("ObjectName = %q, want %q", got.ObjectName, tc.wantObjName)
			}

			for _, wantPriv := range tc.wantPrivs {
				found := false
				for _, p := range got.Privileges {
					if strings.TrimSpace(p) == strings.TrimSpace(wantPriv) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Privileges = %v, missing expected privilege %q", got.Privileges, wantPriv)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewAdapter
// ---------------------------------------------------------------------------

func TestNewAdapter_StoresConfig(t *testing.T) {
	cfg := types.ConnectionConfig{
		Host:     "clickhouse.example.com",
		Port:     9000,
		Database: "analytics",
		Username: "admin",
		Password: "secret",
	}

	a := NewAdapter(cfg)

	if a == nil {
		t.Fatal("NewAdapter returned nil")
	}
	if a.config.Host != cfg.Host {
		t.Errorf("config.Host = %q, want %q", a.config.Host, cfg.Host)
	}
	if a.config.Port != cfg.Port {
		t.Errorf("config.Port = %d, want %d", a.config.Port, cfg.Port)
	}
	if a.config.Database != cfg.Database {
		t.Errorf("config.Database = %q, want %q", a.config.Database, cfg.Database)
	}
	if a.config.Username != cfg.Username {
		t.Errorf("config.Username = %q, want %q", a.config.Username, cfg.Username)
	}
	if a.config.Password != cfg.Password {
		t.Errorf("config.Password = %q, want %q", a.config.Password, cfg.Password)
	}
	if a.db != nil {
		t.Error("db should be nil before Connect is called")
	}
}

// ---------------------------------------------------------------------------
// getDB / Ping / GetVersion / Close (no-connection path)
// ---------------------------------------------------------------------------

func TestGetDB_WhenNotConnected_ReturnsError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	db, err := a.getDB()

	if err == nil {
		t.Error("expected error from getDB when not connected, got nil")
	}
	if db != nil {
		t.Error("expected nil db from getDB when not connected")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error %q should contain 'not connected'", err.Error())
	}
}

func TestPing_WhenNotConnected_ReturnsError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.Ping(context.Background())

	if err == nil {
		t.Error("expected error from Ping when not connected, got nil")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error %q should contain 'not connected'", err.Error())
	}
}

func TestGetVersion_WhenNotConnected_ReturnsError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	version, err := a.GetVersion(context.Background())

	if err == nil {
		t.Error("expected error from GetVersion when not connected, got nil")
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error %q should contain 'not connected'", err.Error())
	}
}

func TestClose_WhenNotConnected_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.Close()

	if err != nil {
		t.Errorf("Close on unconnected adapter returned unexpected error: %v", err)
	}
}

func TestClose_IsIdempotent(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	for i := range 3 {
		if err := a.Close(); err != nil {
			t.Errorf("Close call #%d returned unexpected error: %v", i+1, err)
		}
	}
}

// ---------------------------------------------------------------------------
// buildTLSConfig (no-connection, pure config logic)
// ---------------------------------------------------------------------------

func TestBuildTLSConfig_DisableMode_ReturnsNil(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "disable",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg != nil {
		t.Error("expected nil TLS config for disable mode")
	}
}

func TestBuildTLSConfig_RequireMode_SetsInsecureSkipVerify(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "require",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true for 'require' mode")
	}
}

func TestBuildTLSConfig_PreferredMode_SetsInsecureSkipVerify(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "preferred",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true for 'preferred' mode")
	}
}

func TestBuildTLSConfig_VerifyFullMode_DoesNotSkipVerify(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "clickhouse.example.com",
		TLSEnabled: true,
		TLSMode:    "verify-full",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=false for 'verify-full' mode")
	}
	if tlsCfg.ServerName != "clickhouse.example.com" {
		t.Errorf("ServerName = %q, want %q", tlsCfg.ServerName, "clickhouse.example.com")
	}
}

func TestBuildTLSConfig_VerifyCAMode_DoesNotSkipVerify(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "verify-ca",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=false for 'verify-ca' mode")
	}
}

func TestBuildTLSConfig_UnknownMode_DefaultsToInsecureSkipVerify(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "totally-unknown",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify=true for unknown mode")
	}
}

func TestBuildTLSConfig_MinTLSVersion(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "require",
	})

	tlsCfg, err := a.buildTLSConfig()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	const tls12 = uint16(0x0303)
	if tlsCfg.MinVersion != tls12 {
		t.Errorf("MinVersion = 0x%04X, want 0x%04X (TLS 1.2)", tlsCfg.MinVersion, tls12)
	}
}

func TestBuildTLSConfig_InvalidCA_ReturnsError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{
		Host:       "localhost",
		TLSEnabled: true,
		TLSMode:    "verify-full",
		TLSCA:      []byte("not-a-valid-pem-certificate"),
	})

	tlsCfg, err := a.buildTLSConfig()

	if err == nil {
		t.Error("expected error for invalid CA cert, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse CA certificate") {
		t.Errorf("error %q should mention 'failed to parse CA certificate'", err.Error())
	}
	if tlsCfg != nil {
		t.Error("expected nil TLS config on error")
	}
}

// ---------------------------------------------------------------------------
// no-op methods
// ---------------------------------------------------------------------------

func TestSetUserAttribute_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.SetUserAttribute(context.Background(), "user", "key", "value")

	if err != nil {
		t.Errorf("SetUserAttribute should be a no-op but returned error: %v", err)
	}
}

func TestGetUserAttribute_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	val, err := a.GetUserAttribute(context.Background(), "user", "key")

	if err != nil {
		t.Errorf("GetUserAttribute should be a no-op but returned error: %v", err)
	}
	if val != "" {
		t.Errorf("GetUserAttribute should return empty string, got %q", val)
	}
}

func TestTransferDatabaseOwnership_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.TransferDatabaseOwnership(context.Background(), "mydb", "newowner")

	if err != nil {
		t.Errorf("TransferDatabaseOwnership should be a no-op but returned error: %v", err)
	}
}

func TestUpdateDatabase_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.UpdateDatabase(context.Background(), "mydb", types.UpdateDatabaseOptions{})

	if err != nil {
		t.Errorf("UpdateDatabase should be a no-op but returned error: %v", err)
	}
}

func TestUpdateUser_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.UpdateUser(context.Background(), "testuser", types.UpdateUserOptions{})

	if err != nil {
		t.Errorf("UpdateUser should be a no-op but returned error: %v", err)
	}
}

func TestSetDefaultPrivileges_IsNoop(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	err := a.SetDefaultPrivileges(context.Background(), "testuser", []types.DefaultPrivilegeGrantOptions{
		{Database: "mydb", ObjectType: "tables", Privileges: []string{"SELECT"}},
	})

	if err != nil {
		t.Errorf("SetDefaultPrivileges should be a no-op but returned error: %v", err)
	}
}

func TestGetOwnedObjects_ReturnsEmptySlice(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})

	objs, err := a.GetOwnedObjects(context.Background(), "testuser")

	if err != nil {
		t.Errorf("GetOwnedObjects returned unexpected error: %v", err)
	}
	if objs == nil {
		t.Error("GetOwnedObjects should return empty slice, not nil")
	}
	if len(objs) != 0 {
		t.Errorf("GetOwnedObjects should return empty slice, got %v", objs)
	}
}

// ---------------------------------------------------------------------------
// DB-requiring operations return "not connected"
// ---------------------------------------------------------------------------

func TestDBRequiringOps_WhenNotConnected_ReturnError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	t.Run("CreateDatabase", func(t *testing.T) {
		err := a.CreateDatabase(ctx, types.CreateDatabaseOptions{Name: "test"})
		assertNotConnectedError(t, err)
	})

	t.Run("DropDatabase", func(t *testing.T) {
		err := a.DropDatabase(ctx, "test", types.DropDatabaseOptions{})
		assertNotConnectedError(t, err)
	})

	t.Run("DatabaseExists", func(t *testing.T) {
		_, err := a.DatabaseExists(ctx, "test")
		assertNotConnectedError(t, err)
	})

	t.Run("GetDatabaseInfo", func(t *testing.T) {
		_, err := a.GetDatabaseInfo(ctx, "test")
		assertNotConnectedError(t, err)
	})

	t.Run("VerifyDatabaseAccess", func(t *testing.T) {
		err := a.VerifyDatabaseAccess(ctx, "test")
		assertNotConnectedError(t, err)
	})

	t.Run("CreateUser", func(t *testing.T) {
		err := a.CreateUser(ctx, types.CreateUserOptions{Username: "u", Password: "p"})
		assertNotConnectedError(t, err)
	})

	t.Run("DropUser", func(t *testing.T) {
		err := a.DropUser(ctx, "testuser")
		assertNotConnectedError(t, err)
	})

	t.Run("UserExists", func(t *testing.T) {
		_, err := a.UserExists(ctx, "testuser")
		assertNotConnectedError(t, err)
	})

	t.Run("UpdatePassword", func(t *testing.T) {
		err := a.UpdatePassword(ctx, "testuser", "newpass")
		assertNotConnectedError(t, err)
	})

	t.Run("GetUserInfo", func(t *testing.T) {
		_, err := a.GetUserInfo(ctx, "testuser")
		assertNotConnectedError(t, err)
	})

	t.Run("CreateRole", func(t *testing.T) {
		err := a.CreateRole(ctx, types.CreateRoleOptions{RoleName: "testrole"})
		assertNotConnectedError(t, err)
	})

	t.Run("DropRole", func(t *testing.T) {
		err := a.DropRole(ctx, "testrole")
		assertNotConnectedError(t, err)
	})

	t.Run("RoleExists", func(t *testing.T) {
		_, err := a.RoleExists(ctx, "testrole")
		assertNotConnectedError(t, err)
	})

	t.Run("GetRoleInfo", func(t *testing.T) {
		_, err := a.GetRoleInfo(ctx, "testrole")
		assertNotConnectedError(t, err)
	})

	t.Run("Grant", func(t *testing.T) {
		err := a.Grant(ctx, "testuser", []types.GrantOptions{{Level: "global", Privileges: []string{"SELECT"}}})
		assertNotConnectedError(t, err)
	})

	t.Run("Revoke", func(t *testing.T) {
		err := a.Revoke(ctx, "testuser", []types.GrantOptions{{Level: "global", Privileges: []string{"SELECT"}}})
		assertNotConnectedError(t, err)
	})

	t.Run("GrantRole", func(t *testing.T) {
		err := a.GrantRole(ctx, "testuser", []string{"somerole"})
		assertNotConnectedError(t, err)
	})

	t.Run("RevokeRole", func(t *testing.T) {
		err := a.RevokeRole(ctx, "testuser", []string{"somerole"})
		assertNotConnectedError(t, err)
	})

	t.Run("GetGrants", func(t *testing.T) {
		_, err := a.GetGrants(ctx, "testuser")
		assertNotConnectedError(t, err)
	})

	t.Run("ListDatabases", func(t *testing.T) {
		_, err := a.ListDatabases(ctx)
		assertNotConnectedError(t, err)
	})

	t.Run("ListUsers", func(t *testing.T) {
		_, err := a.ListUsers(ctx)
		assertNotConnectedError(t, err)
	})

	t.Run("ListRoles", func(t *testing.T) {
		_, err := a.ListRoles(ctx)
		assertNotConnectedError(t, err)
	})

	t.Run("SetResourceComment", func(t *testing.T) {
		err := a.SetResourceComment(ctx, "database", "mydb", "comment")
		assertNotConnectedError(t, err)
	})

	t.Run("GetResourceComment", func(t *testing.T) {
		_, err := a.GetResourceComment(ctx, "database", "mydb")
		assertNotConnectedError(t, err)
	})
}

// ---------------------------------------------------------------------------
// ValidatePrivileges — extracted from Grant/Revoke so the service layer
// can validate before calling the adapter.
// ---------------------------------------------------------------------------

func TestValidatePrivileges_RejectsInvalidPrivileges(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	err := a.ValidatePrivileges(ctx, []string{"SELECT", "INVALID_PRIV"})
	if err == nil {
		t.Error("expected error for invalid privilege, got nil")
	}
	if !strings.Contains(err.Error(), "INVALID_PRIV") {
		t.Errorf("error %q should mention the invalid privilege", err.Error())
	}
}

func TestValidatePrivileges_AcceptsValidPrivileges(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	err := a.ValidatePrivileges(ctx, []string{"SELECT", "INSERT", "CREATE TABLE"})
	if err != nil {
		t.Errorf("expected no error for valid privileges, got: %v", err)
	}
}

func TestValidatePrivileges_EmptyList(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	err := a.ValidatePrivileges(ctx, []string{})
	if err == nil {
		t.Error("expected error for empty privilege list, got nil")
	}
}

func TestFlushPrivileges_NoOp(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	err := a.FlushPrivileges(ctx)
	if err != nil {
		t.Errorf("expected FlushPrivileges to be a no-op, got: %v", err)
	}
}

func TestGrant_NotConnected_ReturnsError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	err := a.Grant(ctx, "testuser", []types.GrantOptions{
		{Level: "global", Privileges: []string{"SELECT"}},
	})
	assertNotConnectedError(t, err)
}

func TestRevoke_NotConnected_ReturnsError(t *testing.T) {
	a := NewAdapter(types.ConnectionConfig{})
	ctx := context.Background()

	err := a.Revoke(ctx, "testuser", []types.GrantOptions{
		{Level: "global", Privileges: []string{"SELECT"}},
	})
	assertNotConnectedError(t, err)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertNotConnectedError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected 'not connected' error, got nil")
		return
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error %q should contain 'not connected'", err.Error())
	}
}
