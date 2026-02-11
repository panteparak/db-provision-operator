# API Reference

Complete API reference for all DB Provision Operator Custom Resource Definitions.

!!! tip "Auto-Generated Reference"
    For the complete auto-generated API reference with all fields and validation rules, see [API Reference (Full)](reference.md).

## API Group

All resources belong to the `dbops.dbprovision.io` API group.

```yaml
apiVersion: dbops.dbprovision.io/v1alpha1
```

## Resources Overview

| Kind | Description | Scope |
|------|-------------|-------|
| [`DatabaseInstance`](#databaseinstance) | Database server connection | Namespaced |
| [`Database`](#database) | Logical database | Namespaced |
| [`DatabaseUser`](#databaseuser) | Database user | Namespaced |
| [`DatabaseRole`](#databaserole) | Permission group | Namespaced |
| [`DatabaseGrant`](#databasegrant) | Fine-grained permissions | Namespaced |
| [`DatabaseBackup`](#databasebackup) | One-time backup | Namespaced |
| [`DatabaseBackupSchedule`](#databasebackupschedule) | Scheduled backups | Namespaced |
| [`DatabaseRestore`](#databaserestore) | Restore operation | Namespaced |

---

## DatabaseInstance

Represents a connection to a database server.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `engine` | string | Yes | Database engine: `postgres`, `mysql`, `mariadb`, `cockroachdb` |
| `connection` | object | Yes | Connection configuration |
| `healthCheck` | object | No | Health check configuration |

#### connection

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `host` | string | Yes | Database host |
| `port` | int | No | Database port (default: engine-specific) |
| `database` | string | No | Database name for connection |
| `sslMode` | string | No | PostgreSQL SSL mode |
| `secretRef` | object | Yes | Reference to credentials Secret |
| `tls` | object | No | TLS configuration |

#### healthCheck

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | true | Enable health checks |
| `intervalSeconds` | int | 30 | Check interval |
| `timeoutSeconds` | int | 5 | Check timeout |
| `failureThreshold` | int | 3 | Failures before unhealthy |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase: `Pending`, `Ready`, `Failed` |
| `conditions` | array | Detailed conditions |
| `observedGeneration` | int | Last observed generation |
| `version` | string | Detected database version |

---

## Database

Represents a logical database within a DatabaseInstance.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instanceRef` | object | Yes | Reference to DatabaseInstance |
| `name` | string | No | Database name (default: metadata.name) |
| `deletionPolicy` | string | No | `Retain`, `Delete`, `Snapshot` |
| `deletionProtection` | bool | No | Prevent deletion |
| `postgres` | object | No | PostgreSQL-specific options |
| `mysql` | object | No | MySQL/MariaDB-specific options |

#### postgres

| Field | Type | Description |
|-------|------|-------------|
| `encoding` | string | Character encoding (default: UTF8) |
| `lcCollate` | string | Collation order |
| `lcCtype` | string | Character classification |
| `tablespace` | string | Default tablespace |
| `template` | string | Template database |
| `connectionLimit` | int | Max connections |
| `extensions` | array | Extensions to install |
| `schemas` | array | Schemas to create |

#### mysql

| Field | Type | Description |
|-------|------|-------------|
| `charset` | string | Default charset |
| `collation` | string | Default collation |
| `defaultStorageEngine` | string | Default storage engine |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase |
| `conditions` | array | Detailed conditions |
| `observedGeneration` | int | Last observed generation |

---

## DatabaseUser

Represents a database user with credential management.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instanceRef` | object | Yes | Reference to DatabaseInstance |
| `username` | string | Yes | Username in database |
| `passwordSecret` | object | No | Password generation config |
| `existingPasswordSecret` | object | No | Use existing password |
| `postgres` | object | No | PostgreSQL-specific options |
| `mysql` | object | No | MySQL/MariaDB-specific options |

#### passwordSecret

| Field | Type | Description |
|-------|------|-------------|
| `generate` | bool | Auto-generate password |
| `length` | int | Password length (default: 24) |
| `includeSpecialChars` | bool | Include special chars |
| `excludeChars` | string | Characters to exclude |
| `secretName` | string | Generated Secret name |
| `secretTemplate` | object | Custom Secret template |

#### postgres (user)

| Field | Type | Description |
|-------|------|-------------|
| `login` | bool | Can login (default: true) |
| `inherit` | bool | Inherit privileges |
| `createDB` | bool | Can create databases |
| `createRole` | bool | Can create roles |
| `superuser` | bool | Is superuser |
| `replication` | bool | Can replicate |
| `bypassRLS` | bool | Bypass row-level security |
| `connectionLimit` | int | Max connections |
| `inRoles` | array | Roles to inherit |
| `configParameters` | map | Session parameters |

#### mysql (user)

| Field | Type | Description |
|-------|------|-------------|
| `maxQueriesPerHour` | int | Query limit |
| `maxUpdatesPerHour` | int | Update limit |
| `maxConnectionsPerHour` | int | Connection limit per hour |
| `maxUserConnections` | int | Max concurrent connections |
| `authPlugin` | string | Authentication plugin |
| `requireSSL` | bool | Require SSL |
| `allowedHosts` | array | Allowed connection hosts |
| `accountLocked` | bool | Lock account |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase |
| `conditions` | array | Detailed conditions |
| `credentialsSecretRef` | object | Reference to credentials Secret |

---

## DatabaseRole

Represents a permission group (primarily PostgreSQL).

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `instanceRef` | object | Yes | Reference to DatabaseInstance |
| `roleName` | string | Yes | Role name in database |
| `postgres` | object | No | PostgreSQL-specific options |
| `mysql` | object | No | MySQL-specific options (8.0+) |

#### postgres (role)

| Field | Type | Description |
|-------|------|-------------|
| `login` | bool | Can login (default: false for roles) |
| `inherit` | bool | Inherit privileges |
| `createDB` | bool | Can create databases |
| `createRole` | bool | Can create roles |
| `superuser` | bool | Is superuser |
| `replication` | bool | Can replicate |
| `bypassRLS` | bool | Bypass row-level security |
| `connectionLimit` | int | Max connections |
| `inRoles` | array | Roles to inherit |
| `grants` | array | Permission grants |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase |
| `conditions` | array | Detailed conditions |

---

## DatabaseGrant

Represents fine-grained permissions for users or roles.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `userRef` | object | Conditional | Reference to DatabaseUser |
| `roleRef` | object | Conditional | Reference to DatabaseRole |
| `databaseRef` | object | No | Reference to Database |
| `postgres` | object | No | PostgreSQL grants |
| `mysql` | object | No | MySQL grants |

!!! note "userRef or roleRef"
    Either `userRef` or `roleRef` is required, but not both.

#### postgres (grants)

| Field | Type | Description |
|-------|------|-------------|
| `roles` | array | Roles to assign |
| `grants` | array | Direct permission grants |
| `defaultPrivileges` | array | Default privileges for future objects |

#### mysql (grants)

| Field | Type | Description |
|-------|------|-------------|
| `roles` | array | Roles to assign (MySQL 8.0+) |
| `grants` | array | Permission grants |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase |
| `conditions` | array | Detailed conditions |

---

## DatabaseBackup

Represents a one-time backup operation.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `databaseRef` | object | Yes | Reference to Database |
| `storage` | object | Yes | Storage configuration |
| `compression` | object | No | Compression settings |
| `encryption` | object | No | Encryption settings |
| `ttl` | string | No | Time-to-live (e.g., "168h") |
| `postgres` | object | No | PostgreSQL backup options |
| `mysql` | object | No | MySQL backup options |

#### storage

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `s3`, `gcs`, `azure`, `pvc` |
| `s3` | object | S3/MinIO configuration |
| `gcs` | object | Google Cloud Storage config |
| `azure` | object | Azure Blob Storage config |
| `pvc` | object | PersistentVolumeClaim config |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | `Pending`, `Running`, `Completed`, `Failed` |
| `startTime` | string | Backup start time |
| `completionTime` | string | Backup completion time |
| `backupSize` | string | Size of backup |
| `backupPath` | string | Path to backup file |

---

## DatabaseBackupSchedule

Represents scheduled backup operations with retention.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `databaseRef` | object | Yes | Reference to Database |
| `schedule` | string | Yes | Cron expression |
| `timezone` | string | No | Timezone (default: UTC) |
| `retention` | object | No | Retention policy |
| `concurrencyPolicy` | string | No | `Allow`, `Forbid`, `Replace` |
| `backupTemplate` | object | Yes | Backup configuration |

#### retention

| Field | Type | Description |
|-------|------|-------------|
| `keepLast` | int | Keep last N backups |
| `keepDaily` | int | Keep daily backups for N days |
| `keepWeekly` | int | Keep weekly backups for N weeks |
| `keepMonthly` | int | Keep monthly backups for N months |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `lastBackupTime` | string | Last backup time |
| `lastBackupName` | string | Last backup resource name |
| `nextBackupTime` | string | Next scheduled backup |
| `activeBackups` | int | Number of retained backups |

---

## DatabaseRestore

Represents a restore operation from a backup.

### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `backupRef` | object | Yes | Reference to DatabaseBackup |
| `targetDatabaseRef` | object | Yes | Target Database reference |
| `createTarget` | bool | No | Create database if not exists |
| `postgres` | object | No | PostgreSQL restore options |
| `mysql` | object | No | MySQL restore options |

### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | `Pending`, `Running`, `Completed`, `Failed` |
| `startTime` | string | Restore start time |
| `completionTime` | string | Restore completion time |

---

## Common Types

### ObjectReference

Used for referencing other resources.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Resource name |
| `namespace` | string | No | Namespace (default: same as referencing resource) |

### SecretKeySelector

Used for referencing Secret keys.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Secret name |
| `key` | string | Yes | Key within Secret |

### Condition

Standard Kubernetes condition.

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Condition type |
| `status` | string | `True`, `False`, `Unknown` |
| `reason` | string | Machine-readable reason |
| `message` | string | Human-readable message |
| `lastTransitionTime` | string | Last transition time |

## Phases

All resources use consistent phase values:

| Phase | Description |
|-------|-------------|
| `Pending` | Resource is being processed |
| `Ready` | Resource is ready for use |
| `Failed` | Resource encountered an error |
| `Deleting` | Resource is being deleted |

For backup/restore operations:

| Phase | Description |
|-------|-------------|
| `Pending` | Operation not started |
| `Running` | Operation in progress |
| `Completed` | Operation successful |
| `Failed` | Operation failed |
