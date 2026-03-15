# DatabaseUser

A `DatabaseUser` creates and manages database users with automatic credential management.

## Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
```

!!! tip "Database Ownership"
    When creating a Database with `owner: <username>`, create the DatabaseUser first so the role exists. The user's auto-generated Secret provides credentials for applications to access that database. See [Database owner field](databases.md#owner-optional) for details.

## Spec Fields

### instanceRef (required)

Reference to the parent `DatabaseInstance`.

### username (required)

The username in the database. Immutable after creation.

### passwordSecret (optional)

Configuration for password management.

| Field | Type | Description |
|-------|------|-------------|
| `generate` | bool | Auto-generate a secure password |
| `length` | int | Password length (default: 24) |
| `includeSpecialChars` | bool | Include special characters (default: true) |
| `excludeChars` | string | Characters to exclude |
| `secretName` | string | Name for the generated Secret |
| `secretTemplate` | object | Custom Secret template |

#### secretTemplate

Customize the generated Secret:

| Field | Type | Description |
|-------|------|-------------|
| `labels` | map | Additional labels |
| `annotations` | map | Additional annotations |
| `type` | string | Secret type (default: `Opaque`) |
| `data` | map | Templated data keys — **replaces** default keys when provided |

!!! important "Full Replacement Behavior"
    When `secretTemplate.data` is provided, it **fully replaces** the default secret keys (`username`, `password`, `host`, `port`). No merging occurs. If you need those values, include them explicitly in your template.

**Template variables:**

| Variable | Type | Description |
|----------|------|-------------|
| `.Username` | string | The database username |
| `.Password` | string | The password (generated or existing) |
| `.Host` | string | Database host from the parent instance |
| `.Port` | int32 | Database port from the parent instance |
| `.Database` | string | Database name from the parent instance |
| `.SSLMode` | string | TLS mode from the parent instance (e.g., `verify-full`) |
| `.Namespace` | string | Namespace of the DatabaseUser resource |
| `.Name` | string | Name of the DatabaseUser resource |
| `.CA` | string | PEM-encoded CA certificate from instance TLS secret |
| `.TLSCert` | string | PEM-encoded client certificate from instance TLS secret |
| `.TLSKey` | string | PEM-encoded client key from instance TLS secret |

!!! note "TLS Variables"
    `.CA`, `.TLSCert`, and `.TLSKey` are populated from the parent `DatabaseInstance`'s `tls.secretRef`. If TLS is not enabled or the secret is missing, these fields are empty strings.

**Template functions:**

| Function | Description | Example |
|----------|-------------|---------|
| `urlEncode` | URL query-escape | `{{ urlEncode .Password }}` → `p%40ss%21` |
| `urlPathEncode` | URL path-escape | `{{ urlPathEncode .Database }}` → `my%2Fdb` |
| `base64Encode` | Base64 encode | `{{ base64Encode .CA }}` |
| `base64Decode` | Base64 decode | `{{ base64Decode .EncodedValue }}` |
| `upper` / `lower` | Case conversion | `{{ upper .Username }}` |
| `title` | Capitalize first letter | `{{ title .Name }}` |
| `trim` | Trim whitespace | `{{ trim .Value }}` |
| `trimPrefix` / `trimSuffix` | Trim prefix/suffix | `{{ trimPrefix .Host "db-" }}` |
| `replace` | Replace all occurrences | `{{ replace .Host "." "-" }}` |
| `quote` / `squote` | Wrap in double/single quotes | `{{ quote .Password }}` |
| `default` | Fallback for empty values | `{{ default "5432" .Port }}` |
| `toJson` | Marshal to JSON | `{{ toJson .Data }}` |
| `join` | Join string slice | `{{ join .Items "," }}` |

### existingPasswordSecret (optional)

Use an existing password instead of generating one.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Secret name |
| `key` | string | Key containing the password |

### postgres (optional)

PostgreSQL-specific user configuration.

| Field | Type | Description |
|-------|------|-------------|
| `connectionLimit` | int | Max concurrent connections |
| `inherit` | bool | Inherit role privileges (default: true) |
| `login` | bool | Can login (default: true) |
| `createDB` | bool | Can create databases |
| `createRole` | bool | Can create roles |
| `superuser` | bool | Is superuser |
| `replication` | bool | Can replicate |
| `bypassRLS` | bool | Bypass row-level security |
| `inRoles` | array | Roles to inherit from |
| `configParameters` | map | Session parameters |

### mysql (optional)

MySQL-specific user configuration.

| Field | Type | Description |
|-------|------|-------------|
| `maxQueriesPerHour` | int | Query limit (0 = unlimited) |
| `maxUpdatesPerHour` | int | Update limit |
| `maxConnectionsPerHour` | int | Connection limit per hour |
| `maxUserConnections` | int | Max concurrent connections |
| `authPlugin` | string | Authentication plugin |
| `requireSSL` | bool | Require SSL connections |
| `allowedHosts` | array | Allowed connection hosts |
| `accountLocked` | bool | Lock the account |

## Status

| Field | Description |
|-------|-------------|
| `phase` | Current phase |
| `conditions` | Detailed conditions |
| `credentialsSecretRef` | Reference to the credentials Secret |

## Examples

### Basic User with Generated Password

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
```

### User with Custom Secret Template

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    length: 32
    secretName: myapp-user-credentials
    secretTemplate:
      labels:
        app: myapp
      data:
        DATABASE_URL: "postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode={{ default \"prefer\" .SSLMode }}"
        JDBC_URL: "jdbc:postgresql://{{ .Host }}:{{ .Port }}/myapp?user={{ .Username }}&password={{ .Password }}"
```

### MySQL DSN for Go Applications

```yaml
secretTemplate:
  data:
    DSN: "{{ urlEncode .Username }}:{{ urlEncode .Password }}@tcp({{ .Host }}:{{ .Port }})/{{ .Database }}?tls=required&parseTime=true"
```

### .env File Format

```yaml
secretTemplate:
  data:
    .env: |
      DB_HOST={{ .Host }}
      DB_PORT={{ .Port }}
      DB_USER={{ .Username }}
      DB_PASS={{ .Password }}
      DB_NAME={{ .Database }}
      DB_SSLMODE={{ default "disable" .SSLMode }}
```

### mTLS-Enabled Application

Distribute TLS certificates alongside connection credentials:

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: secure-app-user
spec:
  instanceRef:
    name: postgres-mtls  # Instance with TLS enabled
  username: secure_app
  passwordSecret:
    generate: true
    secretName: secure-app-credentials
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/{{ .Database }}?sslmode=verify-full&sslrootcert=/certs/ca.crt&sslcert=/certs/tls.crt&sslkey=/certs/tls.key"
        ca.crt: "{{ .CA }}"
        tls.crt: "{{ .TLSCert }}"
        tls.key: "{{ .TLSKey }}"
```

### User with Existing Password

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  existingPasswordSecret:
    name: my-existing-password
    key: password
```

### PostgreSQL User with Role Membership

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  instanceRef:
    name: postgres-primary
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-user-credentials
  postgres:
    connectionLimit: 20
    inRoles:
      - readonly_role
      - analytics_role
    configParameters:
      search_path: "app,public"
      statement_timeout: "30000"
```

### MySQL User with Host Restrictions

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-mysql-user
spec:
  instanceRef:
    name: mysql-primary
  username: myapp_user
  passwordSecret:
    generate: true
    secretName: myapp-mysql-credentials
  mysql:
    maxUserConnections: 50
    authPlugin: caching_sha2_password
    allowedHosts:
      - "10.0.0.%"
      - "192.168.1.%"
```

## Generated Secret Structure

When `passwordSecret.generate: true`, the operator creates a Secret.

**Default keys** (no `secretTemplate.data`):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: myapp-user-credentials
  ownerReferences:
    - apiVersion: dbops.dbprovision.io/v1alpha1
      kind: DatabaseUser
      name: myapp-user
type: Opaque
data:
  username: <base64>
  password: <base64>
  host: <base64>
  port: <base64>
```

**Custom keys** (with `secretTemplate.data`):

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: myapp-user-credentials
type: Opaque
data:
  # Only the keys you defined in secretTemplate.data
  DATABASE_URL: <base64-encoded rendered template>
  ca.crt: <base64-encoded PEM certificate>
```

## Password Rotation

The operator automatically recovers when a DatabaseUser's credentials Secret is deleted.
The flow works as follows:

1. The credentials Secret is deleted (manually or by an external process).
2. The `Owns(&corev1.Secret{})` Watch detects the deletion and triggers an immediate reconciliation.
3. The controller generates a new password, calls `SetPassword()` to sync it to the database, and recreates the Secret via `ensureCredentialsSecret()`.
4. A `SecretRegenerated` event is emitted on the DatabaseUser resource.

To manually rotate a password, delete the existing Secret:

```bash
kubectl delete secret <user>-credentials -n <namespace>
```

The operator will regenerate the password and recreate the Secret within seconds.

## Troubleshooting

### User stuck in Pending

- Verify the DatabaseInstance is Ready
- Check the credentials Secret exists (if using existing)

### Password not generated

- Check if `passwordSecret.generate: true` is set
- Verify `secretName` is specified
- Check for errors in operator logs

### Cannot connect with generated password

- Verify the Secret contains the correct password
- Check the user was created in the database
- For PostgreSQL, verify pg_hba.conf allows the connection
