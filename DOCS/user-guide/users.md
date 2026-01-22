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
| `data` | map | Additional data fields with templates |

**Template variables:**

- `{{ .Username }}` - The username
- `{{ .Password }}` - The password
- `{{ .Host }}` - Database host
- `{{ .Port }}` - Database port
- `{{ .Database }}` - Database name
- `{{ .SSLMode }}` - SSL mode (PostgreSQL)

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
        DATABASE_URL: "postgresql://{{ .Username }}:{{ .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode={{ .SSLMode }}"
        JDBC_URL: "jdbc:postgresql://{{ .Host }}:{{ .Port }}/myapp?user={{ .Username }}&password={{ .Password }}"
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

When `passwordSecret.generate: true`, the operator creates a Secret:

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
  # Plus any custom fields from secretTemplate
```

## Password Rotation

To rotate a password:

1. Delete the existing credentials Secret
2. The operator will detect the missing Secret and regenerate

Or use the annotation:

```yaml
metadata:
  annotations:
    dbops.dbprovision.io/rotate-password: "true"
```

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
