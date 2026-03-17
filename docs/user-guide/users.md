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

## Deletion

### Deletion Protection

DatabaseUser uses **annotations** (not spec fields) for deletion protection:

```yaml
metadata:
  annotations:
    dbops.dbprovision.io/deletion-protection: "true"
```

To remove protection:

```bash
kubectl annotate databaseuser myapp-user dbops.dbprovision.io/deletion-protection-
```

### Deletion Policy

DatabaseUser uses an **annotation** for deletion policy:

```yaml
metadata:
  annotations:
    dbops.dbprovision.io/deletion-policy: "Delete"  # or Retain (default)
```

- **Retain** (default): CR is deleted, but the database user is kept
- **Delete**: Database user is dropped, then CR is deleted

### Deletion Flow

1. **Deletion protection check**: Blocked if annotation `deletion-protection: "true"` is present, unless `force-delete` annotation is set.
2. **Child dependency check**: If DatabaseGrant children reference this user, deletion is blocked (Phase=Failed, condition=DependenciesExist) unless force-delete is set.
3. **Cascade confirmation**: When force-delete is set and grants exist, the operator enters `PhasePendingDeletion` and requires the `confirm-force-delete` annotation with the hash from `status.deletionConfirmation.hash`. See [Force Delete with Children](deletion-protection.md#force-delete-with-children-cascade-confirmation).
4. **Deletion policy**: The annotation `dbops.dbprovision.io/deletion-policy` controls whether the external user is dropped.
5. **Force-delete and external failures**: If the user drop fails and force-delete is set, the operator continues with finalizer removal.

## Password Rotation

### Automatic Rotation

The operator supports scheduled password rotation via `spec.passwordRotation`. When enabled, the controller evaluates the cron schedule during each reconciliation and performs rotation when due.

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: DatabaseUser
metadata:
  name: myapp-user
spec:
  username: myapp
  instanceRef:
    name: prod-pg
  passwordRotation:
    enabled: true
    schedule: "0 0 1 * *"   # Monthly at midnight
    strategy: role-inheritance
    serviceRole:
      name: svc_myapp       # Optional, defaults to svc_<username>
      autoCreate: true
    userNaming: "{{.Username}}_{{.Date}}"
    oldUserPolicy:
      action: delete         # delete | disable | retain
      gracePeriodDays: 7
      ownershipCheck: true
```

### Role-Inheritance Strategy (PostgreSQL / CockroachDB)

The `role-inheritance` strategy creates a NOLOGIN **service role** that holds all privileges, and LOGIN users that inherit from it:

1. **Service role created** — `svc_myapp` (NOLOGIN, INHERIT). Holds privilege grants.
2. **New login user created** — e.g., `myapp_20260315` with membership in `svc_myapp`.
3. **Password generated** — stored in the credentials Secret (same Secret key layout).
4. **Old user deprecated** — added to `status.rotation.pendingDeletion` with a grace period.
5. **Old user cleaned up** — after `gracePeriodDays`, the action (`delete`/`disable`/`retain`) is applied.

Existing connections using the old user continue working during the grace period. New connections use the updated Secret.

### Configuration Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable automatic rotation |
| `schedule` | string | — | Cron expression (5-field: min hour dom month dow) |
| `strategy` | string | `role-inheritance` | Rotation strategy (`role-inheritance` or `dual-password`) |
| `serviceRole.name` | string | `svc_<username>` | Service role name for PostgreSQL |
| `serviceRole.autoCreate` | bool | `true` | Create service role if it doesn't exist |
| `userNaming` | string | `{{.Username}}_{{.Date}}` | Template for rotated usernames. Variables: `{{.Username}}`, `{{.Date}}`, `{{.Timestamp}}` |
| `oldUserPolicy.action` | string | `delete` | What to do with old users: `delete`, `disable`, or `retain` |
| `oldUserPolicy.gracePeriodDays` | int | `7` | Days before applying the action |
| `oldUserPolicy.ownershipCheck` | bool | `true` | Block deletion if user owns database objects |

### Status Tracking

Rotation state is tracked in `status.rotation`:

| Field | Description |
|-------|-------------|
| `lastRotatedAt` | Timestamp of last rotation |
| `nextRotationAt` | Scheduled next rotation |
| `activeUser` | Currently active database username |
| `serviceRole` | Service role name |
| `pendingDeletion` | List of users awaiting cleanup, with status (`pending`/`blocked`/`deleted`) |

### Old User Cleanup

After the grace period expires, the configured action is applied:

- **delete**: Drops the user from the database. Blocked if `ownershipCheck` is enabled and the user owns objects — status shows `blocked` with the REASSIGN command.
- **disable**: Sets NOLOGIN on the user, preventing new connections but preserving the role for audit.
- **retain**: No action taken; the user remains as-is.

### Manual Rotation (Fallback)

The operator also automatically recovers when a DatabaseUser's credentials Secret is deleted:

1. The credentials Secret is deleted (manually or by an external process).
2. The `Owns(&corev1.Secret{})` Watch detects the deletion and triggers an immediate reconciliation.
3. The controller generates a new password, calls `SetPassword()` to sync it to the database, and recreates the Secret via `ensureCredentialsSecret()`.
4. A `SecretRegenerated` event is emitted on the DatabaseUser resource.

```bash
kubectl delete secret <user>-credentials -n <namespace>
```

The operator will regenerate the password and recreate the Secret within seconds.

### Troubleshooting Rotation

**Rotation not triggering:**

- Verify `spec.passwordRotation.enabled: true`
- Check the cron expression is valid (5-field format)
- Look for `InvalidRotationSchedule` events on the resource

**Old user stuck in pending:**

- Check `status.rotation.pendingDeletion[].status` — if `blocked`, the user owns objects
- Run the REASSIGN command from `status.rotation.pendingDeletion[].resolution`
- Or set `ownershipCheck: false` to skip the check

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
