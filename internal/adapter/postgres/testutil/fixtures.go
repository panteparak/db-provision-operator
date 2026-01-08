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

package testutil

import (
	"github.com/db-provision-operator/internal/adapter/types"
)

const (
	// TestHost is the default test database host
	TestHost = "localhost"

	// TestPort is the default test database port
	TestPort = 5432

	// TestDatabase is the default test database name
	TestDatabase = "testdb"

	// TestUsername is the default test username
	TestUsername = "testuser"

	// TestPassword is the default test password
	TestPassword = "testpassword123"

	// TestApplicationName is the default application name
	TestApplicationName = "db-provision-operator-test"
)

// NewBasicConnectionConfig creates a basic ConnectionConfig for testing
func NewBasicConnectionConfig() types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:     TestHost,
		Port:     TestPort,
		Database: TestDatabase,
		Username: TestUsername,
		Password: TestPassword,
	}
}

// NewConnectionConfigWithTLS creates a ConnectionConfig with TLS enabled
func NewConnectionConfigWithTLS(mode string) types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:       TestHost,
		Port:       TestPort,
		Database:   TestDatabase,
		Username:   TestUsername,
		Password:   TestPassword,
		TLSEnabled: true,
		TLSMode:    mode,
	}
}

// NewConnectionConfigWithSSLMode creates a ConnectionConfig with SSLMode set
func NewConnectionConfigWithSSLMode(sslMode string) types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:     TestHost,
		Port:     TestPort,
		Database: TestDatabase,
		Username: TestUsername,
		Password: TestPassword,
		SSLMode:  sslMode,
	}
}

// NewConnectionConfigWithApplicationName creates a ConnectionConfig with application name
func NewConnectionConfigWithApplicationName(appName string) types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:            TestHost,
		Port:            TestPort,
		Database:        TestDatabase,
		Username:        TestUsername,
		Password:        TestPassword,
		ApplicationName: appName,
	}
}

// NewConnectionConfigWithTimeout creates a ConnectionConfig with connection timeout
func NewConnectionConfigWithTimeout(timeout int32) types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:           TestHost,
		Port:           TestPort,
		Database:       TestDatabase,
		Username:       TestUsername,
		Password:       TestPassword,
		ConnectTimeout: timeout,
	}
}

// NewConnectionConfigWithTLSCerts creates a ConnectionConfig with TLS certificates
func NewConnectionConfigWithTLSCerts(mode string, ca, cert, key []byte) types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:       TestHost,
		Port:       TestPort,
		Database:   TestDatabase,
		Username:   TestUsername,
		Password:   TestPassword,
		TLSEnabled: true,
		TLSMode:    mode,
		TLSCA:      ca,
		TLSCert:    cert,
		TLSKey:     key,
	}
}

// Sample TLS certificates for testing (self-signed, not for production)
// Generated with: openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 3650 -nodes
const (
	// TestCACert is a sample CA certificate for testing (valid for 10 years from 2024)
	TestCACert = `-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUXxLl4VBzTLm5Qs/8+Iu2qbLdPQ0wDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCVVMxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yNDAxMDEwMDAwMDBaFw0zNDAx
MDEwMDAwMDBaMEUxCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQC7o5e7CkvgMOlHi4vHPIez6lyMhdGRXl8xKMVL4k4Y
fLNaNv+0tNf/sToJBCHsKIPmstIQQH5vXyqCKM5STZJU8c3ThKaBuEfNiGPCAqME
pYrDNCEk8ROzsMkhLCmUWgz5rplU+WF5SpC+FBpLD8W9P7gNRJWTtDH+0I7VTZoq
jqBkVpT6ICEluFYtPCksYjLbmnCPCRT3a8VhHMMXq9oJuTcpVYt/VC1Fzc8LN4Fa
xL7SgMc0G8ZZDX/BdxEp1qSJMXSJbDYmdoYN0E9oStkE7FYQFXQU7ONzP5R3XCTF
QaUT3DDaM9dRZ1h1YDByMl/cxLRB3adv34UXp2V4dS2xAgMBAAGjUzBRMB0GA1Ud
DgQWBBTnJP7DSD8+lRIF4seYkj07r9kBQzAfBgNVHSMEGDAWgBTnJP7DSD8+lRIF
4seYkj07r9kBQzAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBP
qL8CEwpcXhT2h2svQvbHp4M9gjYH7xOIPmSEMoil3vGVnH7d8t+MLnb7grPYBAlv
xNNRx4r9J4PavffRdBfFq4QD9TQHQ2gD1o+iudRPoUn+HieJfEvOk+JTA4s/z0B4
MgPoVq5lZUDKdJGX7P7b8FVkv3rk/IrN7MqND9etPe3g9j9s77nJdLLnBReJJc/E
Q3x2VKvynCN8M2L8HyLBPGVba2A5hknLc7bTC2UTk8ljBBLt8NA57XHOKj0VUyWq
jDBoj3EyL4JKGGUVUdM8V1GsZIYG9lWvbB3dz/7JzJfaT2zAVbQ3bLdz0DPSVzuv
8hEoQ0zE+eHCKl2hcg2q
-----END CERTIFICATE-----`

	// TestClientCert is a sample client certificate for testing (valid for 10 years)
	TestClientCert = `-----BEGIN CERTIFICATE-----
MIIDdzCCAl+gAwIBAgIUbCjEDXk7UM+p+jL3DmdBupiU3LUwDQYJKoZIhvcNAQEL
BQAwSzELMAkGA1UEBhMCVVMxDTALBgNVBAgMBFRlc3QxDTALBgNVBAcMBFRlc3Qx
DTALBgNVBAoMBFRlc3QxDzANBgNVBAMMBmNsaWVudDAeFw0yNjAxMDgxMTE4MzNa
Fw0zNjAxMDYxMTE4MzNaMEsxCzAJBgNVBAYTAlVTMQ0wCwYDVQQIDARUZXN0MQ0w
CwYDVQQHDARUZXN0MQ0wCwYDVQQKDARUZXN0MQ8wDQYDVQQDDAZjbGllbnQwggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDSyRssO2xdHZyqq+KrExtPGoru
CrE3LDWn+CmnvPHccF+Z4ryRyOCxrhQggLjXjKAs83klfPm7u5olatHQC35zsu9I
UK6HuiIFWKqLz72l6WrIFwoWDGqlA2bPUq6Xffd/yMMuaiRHLtQr4TAUV8OAq1/V
tjJ7QQw+eC/5ch9wl3ek5bk8ABMW32PHAapUjQSA8XSMTKOwdm7vj3R7GYRJFPMJ
JAQhGp32RXpB3xcMMQPUs26RYMawsmW40lU7MZ+9CPm7qAQAs2xI0n9IKATLO3Oi
EtpriguDnM3oF6gtkRbQJ2xcBTCVK9UUXqpqyVM3R54lUpcezd6u81RmO3MBAgMB
AAGjUzBRMB0GA1UdDgQWBBTOREl/RTIaBkAMtPSouw22ldJ20TAfBgNVHSMEGDAW
gBTOREl/RTIaBkAMtPSouw22ldJ20TAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3
DQEBCwUAA4IBAQDDk7pCaqgOgTKEHDIr2QxDaN/5Kbi3EqlR3SHnl46MeoeqUQRo
Py/NV1EgBL1wjhD5hfTcrmCdhTCpq6hTbk/WuBzt5MHE724R8HAQI79Msz7qoL8j
V9m7ms/gykeU5uNEFUcPNzzZiWQ4pbE1qyhq9QYPaZgNdnxD0F8ePJ5UnyhippPO
CW+gVMwsjNORa0L0KfDCmMtic+BeHqsYtSf/f54imoCFxNIb3pLOHw+E2MeTH2f2
fVVDr3wBv+nd0njDSuKINzFKHvbIc6YrPOYDPeHBzOzAeUNmCexAo3e6pUAXIWvx
fBGlAmK5k3yohMEisedt5a+9gSx4RuWOq3/4
-----END CERTIFICATE-----`

	// TestClientKey is a sample client private key for testing (matching TestClientCert)
	TestClientKey = `-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQDSyRssO2xdHZyq
q+KrExtPGoruCrE3LDWn+CmnvPHccF+Z4ryRyOCxrhQggLjXjKAs83klfPm7u5ol
atHQC35zsu9IUK6HuiIFWKqLz72l6WrIFwoWDGqlA2bPUq6Xffd/yMMuaiRHLtQr
4TAUV8OAq1/VtjJ7QQw+eC/5ch9wl3ek5bk8ABMW32PHAapUjQSA8XSMTKOwdm7v
j3R7GYRJFPMJJAQhGp32RXpB3xcMMQPUs26RYMawsmW40lU7MZ+9CPm7qAQAs2xI
0n9IKATLO3OiEtpriguDnM3oF6gtkRbQJ2xcBTCVK9UUXqpqyVM3R54lUpcezd6u
81RmO3MBAgMBAAECggEACIKrsPaK4RvBvLRXpns7sHHj26Z6aVUtk8gS2QZphU9e
7AvLZKUXJnbTPdiHjJBku2G0o+MRvO4wpZUaMujEnu+HmXLnZWx+moNLFmZ5g7RS
yRC7QRxeaQNG61T+sA9soPuSnu9DKIev/2500UVyxfiT62QTI29I3JbUf/KqDDXG
nTGD+mgfSt+gsIdmBljewvjdAB16/lDHwrEEQaUUybcMED7hpHkMoftnTcwSety6
z8wm5Y73zATSXQP8M9iBnyu/j6wwTX1PXQ82sFLQsVRInlYkPpl0yZTrLcV/PcJ7
zyycZRktUV4Ypn3S3cCaDWebWq/lnuRtwH9Krg9ctwKBgQDroXNxy9CHtQ9lOkkT
rz8MwPHwA0xLWF359wTV1VPtet7HiSHixVrGXE1cSN3iJIoN3nvizpHT5TJTodvi
2CoO1h2S5xHdatj2yQE0pIid2S9YKnB4MMSNhi27rIXvQxJtDcVkx7GGCo7iOsGn
raMVCNJ38kaPoYb98jR0jqA3OwKBgQDlAdQnbA9P/OhQ5MMbWONZfnuu9PhAcNCI
7zsy7MLXuKO4E9qG9CKzJXadL5UpGWMH+e6eXni/IffsF5swFRQmnIdEUir3B5Ql
ZmnnxIsEeiJ4T9q9jvnHIv8GXNaFsHxWwZBbQm5s1pT19aD6Wp+P3fhAyjayOewU
OAeAfUCy8wKBgGX/WGrNEDJ+ZPCrv1BfDsrlhpUfyFnhIaT/kb9CffcRtffn25w/
U+EDuZUWEb4/lOcWBMiUJLn1v8hGC1nxupr7gofBsJEJHGwPbI8uHdk+V2kxzcep
TJv6ljdkIgIFJafBS04pxyW/0kQJrSR5XFvRmtHDNVodUMMCokRGLQJbAoGAable
aJTKvPLVjgMO0CFJVJfAIhWWRqnOnGlVuzzy9wSXEPSZfpRXML2q9QZypnbB8XzB
XPvgYt0byDNdweT5WJoLGM+WZlVpX5rYadejFn4MS98R7VGEnxrZAeb6Yt4HiUXz
jy4sLLMkMikkGHCeGZ0wbzjr53w2MV9slzU8GWECgYArDdQVLW/ZOzTh1sl7ETAf
f3dMITzhmUjf1pPVABpp6mwqPW1RlY/myUdqpuR8bD5s1X2autpa3WtdvWX6kHiL
pagshXvz5t6jyTFcA6dzVEDDLf+Epaq37HBJJANp5VHf/avwDWVU4cZHBcMn7r4u
zkWxky24AgK4X7ZxaFGdWg==
-----END PRIVATE KEY-----`

	// InvalidCACert is an invalid CA certificate for testing error cases
	InvalidCACert = `-----BEGIN CERTIFICATE-----
invalid certificate data
-----END CERTIFICATE-----`

	// InvalidClientCert is an invalid client certificate for testing error cases
	InvalidClientCert = `-----BEGIN CERTIFICATE-----
invalid client certificate data
-----END CERTIFICATE-----`

	// InvalidClientKey is an invalid client key for testing error cases
	InvalidClientKey = `-----BEGIN RSA PRIVATE KEY-----
invalid client key data
-----END RSA PRIVATE KEY-----`
)

// DefaultConnectionConfig returns a standard test connection config
// suitable for most PostgreSQL adapter tests.
func DefaultConnectionConfig() types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:            TestHost,
		Port:            TestPort,
		Database:        TestDatabase,
		Username:        TestUsername,
		Password:        TestPassword,
		SSLMode:         "disable",
		TLSEnabled:      false,
		ConnectTimeout:  30,
		ApplicationName: TestApplicationName,
	}
}

// TLSConnectionConfig returns a connection config with TLS enabled
// for testing secure connections.
func TLSConnectionConfig() types.ConnectionConfig {
	return types.ConnectionConfig{
		Host:            TestHost,
		Port:            TestPort,
		Database:        TestDatabase,
		Username:        TestUsername,
		Password:        TestPassword,
		TLSEnabled:      true,
		TLSMode:         "verify-full",
		SSLMode:         "verify-full",
		TLSCA:           []byte(TestCACert),
		TLSCert:         []byte(TestClientCert),
		TLSKey:          []byte(TestClientKey),
		ConnectTimeout:  30,
		ApplicationName: TestApplicationName,
	}
}

// ConnectionConfigWithDatabase returns a connection config for a specific database.
func ConnectionConfigWithDatabase(database string) types.ConnectionConfig {
	config := DefaultConnectionConfig()
	config.Database = database
	return config
}

// CreateDatabaseOpts returns standard create database options for testing.
func CreateDatabaseOpts(name string) types.CreateDatabaseOptions {
	return types.CreateDatabaseOptions{
		Name:             name,
		Encoding:         "UTF8",
		AllowConnections: true,
	}
}

// CreateDatabaseOptsWithOwner returns create database options with an owner specified.
func CreateDatabaseOptsWithOwner(name, owner string) types.CreateDatabaseOptions {
	opts := CreateDatabaseOpts(name)
	opts.Owner = owner
	return opts
}

// CreateDatabaseOptsWithTemplate returns create database options with a template specified.
func CreateDatabaseOptsWithTemplate(name, template string) types.CreateDatabaseOptions {
	opts := CreateDatabaseOpts(name)
	opts.Template = template
	return opts
}

// CreateDatabaseOptsFull returns create database options with all fields populated.
func CreateDatabaseOptsFull(name, owner, template, encoding, collate, ctype, tablespace string, connLimit int32, isTemplate bool) types.CreateDatabaseOptions {
	return types.CreateDatabaseOptions{
		Name:             name,
		Owner:            owner,
		Template:         template,
		Encoding:         encoding,
		LCCollate:        collate,
		LCCtype:          ctype,
		Tablespace:       tablespace,
		ConnectionLimit:  connLimit,
		IsTemplate:       isTemplate,
		AllowConnections: true,
	}
}

// CreateUserOpts returns standard create user options for testing.
func CreateUserOpts(username, password string) types.CreateUserOptions {
	return types.CreateUserOptions{
		Username: username,
		Password: password,
		Login:    true,
		Inherit:  true,
	}
}

// CreateUserOptsWithAttributes returns create user options with additional attributes.
func CreateUserOptsWithAttributes(username, password string, superuser, createdb bool) types.CreateUserOptions {
	opts := CreateUserOpts(username, password)
	opts.Superuser = superuser
	opts.CreateDB = createdb
	return opts
}

// CreateUserOptsWithRoles returns create user options with role membership.
func CreateUserOptsWithRoles(username, password string, roles []string) types.CreateUserOptions {
	opts := CreateUserOpts(username, password)
	opts.InRoles = roles
	return opts
}

// CreateUserOptsFull returns create user options with all fields populated.
func CreateUserOptsFull(username, password string, connLimit int32, validUntil string, superuser, createDB, createRole, inherit, login, replication, bypassRLS bool, inRoles []string) types.CreateUserOptions {
	return types.CreateUserOptions{
		Username:        username,
		Password:        password,
		ConnectionLimit: connLimit,
		ValidUntil:      validUntil,
		Superuser:       superuser,
		CreateDB:        createDB,
		CreateRole:      createRole,
		Inherit:         inherit,
		Login:           login,
		Replication:     replication,
		BypassRLS:       bypassRLS,
		InRoles:         inRoles,
	}
}

// CreateRoleOpts returns standard create role options for testing.
func CreateRoleOpts(roleName string) types.CreateRoleOptions {
	return types.CreateRoleOptions{
		RoleName: roleName,
		Inherit:  true,
	}
}

// CreateRoleOptsWithAttributes returns create role options with additional attributes.
func CreateRoleOptsWithAttributes(roleName string, login, createDB, superuser bool) types.CreateRoleOptions {
	opts := CreateRoleOpts(roleName)
	opts.Login = login
	opts.CreateDB = createDB
	opts.Superuser = superuser
	return opts
}

// CreateRoleOptsWithMembership returns create role options with role membership.
func CreateRoleOptsWithMembership(roleName string, inRoles []string) types.CreateRoleOptions {
	opts := CreateRoleOpts(roleName)
	opts.InRoles = inRoles
	return opts
}

// CreateRoleOptsFull returns create role options with all fields populated.
func CreateRoleOptsFull(roleName string, login, inherit, createDB, createRole, superuser, replication, bypassRLS bool, inRoles []string) types.CreateRoleOptions {
	return types.CreateRoleOptions{
		RoleName:    roleName,
		Login:       login,
		Inherit:     inherit,
		CreateDB:    createDB,
		CreateRole:  createRole,
		Superuser:   superuser,
		Replication: replication,
		BypassRLS:   bypassRLS,
		InRoles:     inRoles,
	}
}

// GrantOpts returns grant options for testing.
func GrantOpts(database, schema string, privileges []string) types.GrantOptions {
	return types.GrantOptions{
		Database:   database,
		Schema:     schema,
		Privileges: privileges,
	}
}

// GrantOptsForTables returns grant options for specific tables.
func GrantOptsForTables(database, schema string, tables, privileges []string) types.GrantOptions {
	return types.GrantOptions{
		Database:   database,
		Schema:     schema,
		Tables:     tables,
		Privileges: privileges,
	}
}

// GrantOptsForSequences returns grant options for specific sequences.
func GrantOptsForSequences(database, schema string, sequences, privileges []string) types.GrantOptions {
	return types.GrantOptions{
		Database:   database,
		Schema:     schema,
		Sequences:  sequences,
		Privileges: privileges,
	}
}

// GrantOptsForFunctions returns grant options for specific functions.
func GrantOptsForFunctions(database, schema string, functions, privileges []string) types.GrantOptions {
	return types.GrantOptions{
		Database:   database,
		Schema:     schema,
		Functions:  functions,
		Privileges: privileges,
	}
}

// GrantOptsWithGrantOption returns grant options with WITH GRANT OPTION enabled.
func GrantOptsWithGrantOption(database, schema string, privileges []string) types.GrantOptions {
	opts := GrantOpts(database, schema, privileges)
	opts.WithGrantOption = true
	return opts
}

// BackupOpts returns backup options for testing.
func BackupOpts(database string, format string) types.BackupOptions {
	return types.BackupOptions{
		Database: database,
		BackupID: "test-backup-001",
		Format:   format,
	}
}

// BackupOptsCustom returns backup options for custom (binary) format.
func BackupOptsCustom(database string) types.BackupOptions {
	return BackupOpts(database, "custom")
}

// BackupOptsPlain returns backup options for plain text format.
func BackupOptsPlain(database string) types.BackupOptions {
	return BackupOpts(database, "plain")
}

// BackupOptsTar returns backup options for tar format.
func BackupOptsTar(database string) types.BackupOptions {
	return BackupOpts(database, "tar")
}

// BackupOptsDirectory returns backup options for directory format.
func BackupOptsDirectory(database string) types.BackupOptions {
	return BackupOpts(database, "directory")
}

// BackupOptsFull returns backup options with all fields populated.
func BackupOptsFull(database, backupID, format string, jobs int32, dataOnly, schemaOnly, noOwner, noPrivileges bool, schemas, tables []string) types.BackupOptions {
	return types.BackupOptions{
		Database:     database,
		BackupID:     backupID,
		Format:       format,
		Jobs:         jobs,
		DataOnly:     dataOnly,
		SchemaOnly:   schemaOnly,
		NoOwner:      noOwner,
		NoPrivileges: noPrivileges,
		Schemas:      schemas,
		Tables:       tables,
	}
}

// RestoreOpts returns restore options for testing.
func RestoreOpts(database string) types.RestoreOptions {
	return types.RestoreOptions{
		Database:  database,
		RestoreID: "test-restore-001",
	}
}

// RestoreOptsWithDropExisting returns restore options that drop existing objects.
func RestoreOptsWithDropExisting(database string) types.RestoreOptions {
	opts := RestoreOpts(database)
	opts.DropExisting = true
	return opts
}

// RestoreOptsWithCreateDatabase returns restore options that create the database.
func RestoreOptsWithCreateDatabase(database string) types.RestoreOptions {
	opts := RestoreOpts(database)
	opts.CreateDatabase = true
	return opts
}

// RestoreOptsFull returns restore options with all fields populated.
func RestoreOptsFull(database, restoreID string, dropExisting, createDB, dataOnly, schemaOnly, noOwner, noPrivileges, disableTriggers, analyze bool, schemas, tables []string, jobs int32) types.RestoreOptions {
	return types.RestoreOptions{
		Database:        database,
		RestoreID:       restoreID,
		DropExisting:    dropExisting,
		CreateDatabase:  createDB,
		DataOnly:        dataOnly,
		SchemaOnly:      schemaOnly,
		NoOwner:         noOwner,
		NoPrivileges:    noPrivileges,
		Schemas:         schemas,
		Tables:          tables,
		Jobs:            jobs,
		DisableTriggers: disableTriggers,
		Analyze:         analyze,
	}
}

// DropDatabaseOpts returns drop database options for testing.
func DropDatabaseOpts(force bool) types.DropDatabaseOptions {
	return types.DropDatabaseOptions{
		Force: force,
	}
}

// UpdateDatabaseOpts returns update database options for testing.
func UpdateDatabaseOpts(extensions []types.ExtensionOptions, schemas []types.SchemaOptions) types.UpdateDatabaseOptions {
	return types.UpdateDatabaseOptions{
		Extensions: extensions,
		Schemas:    schemas,
	}
}

// ExtensionOpts returns extension options for testing.
func ExtensionOpts(name string) types.ExtensionOptions {
	return types.ExtensionOptions{
		Name: name,
	}
}

// ExtensionOptsWithSchema returns extension options with a schema specified.
func ExtensionOptsWithSchema(name, schema string) types.ExtensionOptions {
	return types.ExtensionOptions{
		Name:   name,
		Schema: schema,
	}
}

// ExtensionOptsWithVersion returns extension options with a version specified.
func ExtensionOptsWithVersion(name, version string) types.ExtensionOptions {
	return types.ExtensionOptions{
		Name:    name,
		Version: version,
	}
}

// SchemaOpts returns schema options for testing.
func SchemaOpts(name string) types.SchemaOptions {
	return types.SchemaOptions{
		Name: name,
	}
}

// SchemaOptsWithOwner returns schema options with an owner specified.
func SchemaOptsWithOwner(name, owner string) types.SchemaOptions {
	return types.SchemaOptions{
		Name:  name,
		Owner: owner,
	}
}

// DefaultPrivilegeOpts returns default privilege options for testing.
func DefaultPrivilegeOpts(role, schema, objectType string, privileges []string) types.DefaultPrivilegeOptions {
	return types.DefaultPrivilegeOptions{
		Role:       role,
		Schema:     schema,
		ObjectType: objectType,
		Privileges: privileges,
	}
}

// DefaultPrivilegeGrantOpts returns default privilege grant options for testing.
func DefaultPrivilegeGrantOpts(database, schema, grantedBy, objectType string, privileges []string) types.DefaultPrivilegeGrantOptions {
	return types.DefaultPrivilegeGrantOptions{
		Database:   database,
		Schema:     schema,
		GrantedBy:  grantedBy,
		ObjectType: objectType,
		Privileges: privileges,
	}
}

// Sample data for testing
var (
	// SampleDatabaseNames provides common database names for testing.
	SampleDatabaseNames = []string{"testdb", "myapp", "production", "staging", "development"}

	// SampleUsernames provides common usernames for testing.
	SampleUsernames = []string{"appuser", "readonly", "admin", "service_account", "migration_user"}

	// SampleRoleNames provides common role names for testing.
	SampleRoleNames = []string{"app_read", "app_write", "app_admin", "app_readonly", "app_readwrite"}

	// SamplePrivileges provides common privilege combinations for testing.
	SamplePrivileges = map[string][]string{
		"readonly":  {"SELECT"},
		"readwrite": {"SELECT", "INSERT", "UPDATE", "DELETE"},
		"all":       {"ALL"},
		"execute":   {"EXECUTE"},
		"usage":     {"USAGE"},
	}

	// SampleExtensions provides common PostgreSQL extensions for testing.
	SampleExtensions = []string{"uuid-ossp", "pgcrypto", "hstore", "pg_trgm", "postgis"}

	// SampleSchemas provides common schema names for testing.
	SampleSchemas = []string{"public", "app", "audit", "archive", "reporting"}
)
