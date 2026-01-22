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

### deletionPolicy (optional)

What happens to the database when the CR is deleted.

| Value | Description |
|-------|-------------|
| `Retain` | Keep the database (default) |
| `Delete` | Delete the database |
| `Snapshot` | Create a backup before deletion |

### deletionProtection (optional)

Prevent accidental deletion. Default: `false`

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
  deletionPolicy: Retain
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

## Lifecycle

### Creation

1. Operator validates the spec
2. Checks if instance is Ready
3. Creates the database in the server
4. Installs extensions (PostgreSQL)
5. Creates schemas (PostgreSQL)
6. Updates status to Ready

### Deletion

Based on `deletionPolicy`:

- **Retain**: CR is deleted, database remains
- **Delete**: Database is dropped, then CR is deleted
- **Snapshot**: Backup is created, database is dropped, CR is deleted

### Updates

Most fields are immutable after creation. Supported updates:

- `deletionPolicy`
- `deletionProtection`
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
- Verify no active connections to the database
- For PostgreSQL, ensure no other databases depend on it
