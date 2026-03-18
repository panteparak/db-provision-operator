# Database

A `Database` represents a logical database within a `DatabaseInstance`.

## Overview

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
```

## Spec Fields

### instanceRef (required)

Reference to the parent `DatabaseInstance`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the DatabaseInstance |
| `namespace` | string | No | Namespace (default: same as resource) |

### name (optional)

The actual database name in the database server. Immutable after creation.

- Default: Kubernetes resource name (`metadata.name`)
- Must follow database naming rules for the target engine

### owner (optional)

The database owner (role name). If not specified, the database is owned by the connection user from the DatabaseInstance.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `owner` | string | No | Role name to own the database (max 63 chars, must match `^[a-zA-Z_][a-zA-Z0-9_]*$`) |

!!! tip "Role-based ownership for credential rotation"
    When using [password rotation](users.md#password-rotation) with the `role-inheritance` strategy, set `owner` to the **service role** (not a specific user). The service role persists across rotations while individual login users are created and retired. This prevents ownership from pointing to a deleted user.

#### Bidirectional Default Privileges

When `postgres.ownership.setDefaultPrivileges` is enabled (the default), the operator sets **bidirectional** `ALTER DEFAULT PRIVILEGES`:

- **Forward**: Objects created by the owner role → accessible to the app user
- **Reverse**: Objects created by the app user → accessible to the owner role (and all its members)

The reverse direction is critical for applications that create their own tables (e.g., Vault creates `vault_kv_store`). Without it, other role members — including rotated users — cannot access app-created objects. Both directions are applied to the `public` schema and all schemas listed in `postgres.schemas`.

### deletionPolicy (optional)

What happens to the database when the CR is deleted.

| Value | Description |
|-------|-------------|
| `Retain` | Keep the database |
| `Delete` | Delete the database (default) |
| `Snapshot` | Create a backup before deletion |

### deletionProtection (optional)

Prevent accidental deletion. Default: `false`

### initSQL (optional)

SQL statements to execute once after the database is created. Useful for bootstrapping schemas, seed data, or initial configuration.

Exactly one source must be specified:

=== "inline"

    An array of SQL statements executed in order.

    | Field | Type | Required | Description |
    |-------|------|----------|-------------|
    | `inline` | []string | Yes | SQL statements (max 50 items) |

=== "configMapRef"

    References a ConfigMap key containing SQL statements separated by `---`.

    | Field | Type | Required | Description |
    |-------|------|----------|-------------|
    | `configMapRef.name` | string | Yes | ConfigMap name |
    | `configMapRef.key` | string | Yes | Key within the ConfigMap |

=== "secretRef"

    References a Secret key containing SQL statements separated by `---`. Use this for sensitive seed data.

    | Field | Type | Required | Description |
    |-------|------|----------|-------------|
    | `secretRef.name` | string | Yes | Secret name |
    | `secretRef.key` | string | Yes | Key within the Secret |

**Failure policy:**

| Value | Default | Description |
|-------|---------|-------------|
| `Continue` | Yes | Database reaches Ready; `Synced` condition set to `False` with error details |
| `Block` | No | Database stays in `Failed` phase and requeues until SQL succeeds |

!!! info "Init SQL execution context"
    When `spec.owner` is set, init SQL executes as the database owner role via `SET ROLE`. This scopes init SQL to the owner's privileges — it cannot access other databases or perform superuser operations (e.g., `CREATE EXTENSION` should use `spec.postgres.extensions` instead). When no owner is set, init SQL executes with the operator's admin credentials. The operator user must be a superuser or have membership on the owner role for `SET ROLE` to succeed.

!!! tip "Idempotency and re-execution"
    The operator computes a SHA-256 hash of the resolved SQL content and stores it in `status.initSQL.hash`. Init SQL only re-executes when the content hash changes. Write idempotent SQL (e.g., `CREATE TABLE IF NOT EXISTS`, `INSERT ... ON CONFLICT DO NOTHING`) to handle re-execution safely.

### postgres (optional)

PostgreSQL-specific configuration.

| Field | Type | Description |
|-------|------|-------------|
| `encoding` | string | Character encoding (default: UTF8) |
| `lcCollate` | string | Collation order |
| `lcCtype` | string | Character classification |
| `tablespace` | string | Default tablespace |
| `template` | string | Template database (default: template0) |
| `connectionLimit` | int | Max connections (-1 = unlimited) |
| `extensions` | array | Extensions to install |
| `schemas` | array | Schemas to create |

#### extensions

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Extension name |
| `schema` | string | Schema to install into |

#### schemas

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Schema name |
| `owner` | string | Schema owner |

### mysql (optional)

MySQL-specific configuration.

| Field | Type | Description |
|-------|------|-------------|
| `charset` | string | Default charset (default: utf8mb4) |
| `collation` | string | Default collation (default: utf8mb4_unicode_ci) |
| `defaultStorageEngine` | string | Default storage engine |

## Status

| Field | Description |
|-------|-------------|
| `phase` | Current phase |
| `conditions` | Detailed conditions |
| `observedGeneration` | Last observed generation |

### initSQL

Present when `spec.initSQL` is configured.

| Field | Type | Description |
|-------|------|-------------|
| `applied` | bool | Whether init SQL executed successfully |
| `appliedAt` | time | When init SQL was last successfully executed |
| `hash` | string | SHA-256 hash of the resolved SQL content |
| `error` | string | Last error message (empty on success) |
| `statementsExecuted` | int32 | Number of statements successfully executed |

## Examples

### Basic Database

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp
spec:
  instanceRef:
    name: postgres-primary
```

### PostgreSQL with Extensions

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  deletionPolicy: Delete
  deletionProtection: true
  postgres:
    encoding: UTF8
    lcCollate: en_US.UTF-8
    lcCtype: en_US.UTF-8
    connectionLimit: 100
    extensions:
      - name: uuid-ossp
        schema: public
      - name: pgcrypto
        schema: public
      - name: hstore
        schema: extensions
    schemas:
      - name: app
        owner: myapp_admin
      - name: audit
        owner: myapp_admin
```

### MySQL Database

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-mysql
spec:
  instanceRef:
    name: mysql-primary
  name: myapp
  mysql:
    charset: utf8mb4
    collation: utf8mb4_unicode_ci
```

### Database with Owner

Create a DatabaseUser first, then reference its username as the database owner:

```yaml
# Step 1: Create the user (role must exist before the database)
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
    secretTemplate:
      data:
        DATABASE_URL: "postgresql://{{ urlEncode .Username }}:{{ urlEncode .Password }}@{{ .Host }}:{{ .Port }}/myapp?sslmode=prefer"
---
# Step 2: Create the database owned by that user
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  owner: myapp_user
```

The DatabaseUser automatically creates a Secret (`myapp-user-credentials`) containing the `DATABASE_URL` connection string. See [DatabaseUser secret templates](users.md#user-with-custom-secret-template) for more formats.

### Database with Delete Policy

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: temp-database
spec:
  instanceRef:
    name: postgres-primary
  name: tempdb
  deletionPolicy: Delete  # Database will be dropped when CR is deleted
```

### Database with Inline Init SQL

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  initSQL:
    inline:
      - |
        CREATE TABLE IF NOT EXISTS users (
          id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
          email TEXT UNIQUE NOT NULL,
          created_at TIMESTAMPTZ DEFAULT now()
        );
      - CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);
      - |
        INSERT INTO users (email) VALUES ('admin@example.com')
        ON CONFLICT (email) DO NOTHING;
    failurePolicy: Continue
```

### Database with ConfigMap Init SQL

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: myapp-init-sql
data:
  bootstrap.sql: |
    CREATE TABLE IF NOT EXISTS tenants (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      name TEXT UNIQUE NOT NULL
    );
    ---
    CREATE TABLE IF NOT EXISTS tenant_configs (
      tenant_id UUID REFERENCES tenants(id),
      key TEXT NOT NULL,
      value JSONB,
      PRIMARY KEY (tenant_id, key)
    );
    ---
    INSERT INTO tenants (name) VALUES ('default')
    ON CONFLICT (name) DO NOTHING;
---
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  initSQL:
    configMapRef:
      name: myapp-init-sql
      key: bootstrap.sql
    failurePolicy: Block
```

### Database with Secret Init SQL

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
kind: Database
metadata:
  name: myapp-database
spec:
  instanceRef:
    name: postgres-primary
  name: myapp
  initSQL:
    secretRef:
      name: myapp-seed-data
      key: seed.sql
    failurePolicy: Block
```

Use `secretRef` when your init SQL contains sensitive data such as API keys, default passwords, or license keys that should not be stored in a ConfigMap.

## Lifecycle

### Creation

1. Operator validates the spec
2. Checks if instance is Ready
3. Creates the database in the server
4. Installs extensions (PostgreSQL)
5. Creates schemas (PostgreSQL)
6. Executes init SQL if configured (see [initSQL](#initsql-optional))
7. Updates status to Ready

### Deletion

The full deletion flow for a Database:

1. **Deletion protection check**: If `spec.deletionProtection: true`, deletion is blocked unless the `dbops.dbprovision.io/force-delete: "true"` annotation is present.
2. **Child dependency check**: If DatabaseGrant children reference this Database, deletion is blocked (Phase=Failed, condition=DependenciesExist) unless force-delete is set.
3. **Cascade confirmation**: When force-delete is set and children exist, the operator enters `PhasePendingDeletion` and requires the `confirm-force-delete` annotation with the hash from `status.deletionConfirmation.hash`. Each child grant is deleted according to its own deletion policy. See [Force Delete with Children](deletion-protection.md#force-delete-with-children-cascade-confirmation).
4. **Deletion policy**: Controls what happens to the external database:
    - **Retain**: CR is deleted, database remains
    - **Delete** (default): Database is dropped, then CR is deleted
    - **Snapshot**: Backup is created, database is dropped, CR is deleted
5. **Force-delete and external failures**: If the database drop fails and force-delete is set, the operator continues with finalizer removal anyway (the external database is left as-is).

### Active Connections

The operator automatically terminates active database connections before dropping a database.
This prevents the common `"database is being accessed by other users"` error that occurs
when running `DROP DATABASE` directly.

| Engine | Method |
|--------|--------|
| PostgreSQL | `pg_terminate_backend()` for all sessions |
| CockroachDB | `DROP DATABASE ... CASCADE` handles connections |
| MySQL | `KILL` for all sessions via `information_schema.processlist` |
| ClickHouse | `DROP DATABASE` succeeds regardless of connections |

Applications connected to the database will lose their connections immediately.
If graceful draining is important, scale down application pods before deleting the Database CR.

### Updates

Most fields are immutable after creation. Supported updates:

- `deletionPolicy`
- `deletionProtection`
- `initSQL` (changing content triggers re-execution based on hash comparison)
- `postgres.connectionLimit`
- Adding new extensions/schemas

## Troubleshooting

### Database stuck in Pending

- Verify the DatabaseInstance is in Ready state
- Check operator logs for errors

### Extension installation failed

- Verify the extension is available in the PostgreSQL installation
- Check the user has CREATE EXTENSION permission
- Review operator logs for specific errors

### Cannot delete database

- Check if `deletionProtection` is enabled
- The operator automatically terminates active connections before dropping — see [Active Connections](#active-connections)
- For PostgreSQL, ensure no other databases depend on it

### Init SQL failed with Continue policy

The database is Ready but the `Synced` condition is `False`:

```bash
kubectl get database myapp-database -o jsonpath='{.status.initSQL}'
```

- Check `status.initSQL.error` for the error message
- Check `status.initSQL.statementsExecuted` to see how many statements succeeded
- Fix the SQL and update the resource — the changed hash triggers re-execution

### Init SQL failed with Block policy

The database is stuck in `Failed` phase:

- Check operator logs and `status.initSQL.error` for the specific error
- Fix the SQL in the inline array, ConfigMap, or Secret
- The operator will automatically retry on the next reconciliation

### Init SQL not re-executing after update

The operator uses SHA-256 hash comparison to detect changes:

- Verify the content actually changed (whitespace changes do count)
- Check `status.initSQL.hash` — if it matches the new content hash, the SQL was already applied
- For `configMapRef`/`secretRef`, ensure the referenced resource was updated
